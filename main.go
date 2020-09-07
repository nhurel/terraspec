package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/terraform/backend/local"
	"github.com/hashicorp/terraform/helper/logging"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
	terraspec "github.com/nhurel/terraspec/lib"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	//Version is the version of the app. This is set at build time
	Version string
	app     = kingpin.New("terraspec", "Unit test terraform config")
	// dir = app.Flag("dir", "path to terraform config dir to test").Default(".").String()
	specDir     = app.Flag("spec", "path to folder containing test cases").Default("spec").String()
	displayPlan = app.Flag("display-plan", "Print the full plan before the results").Default("false").Bool()
	version     = app.Version(Version)
)

type testCase struct {
	dir          string
	variableFile string
	specFile     string
}

func (tc *testCase) name() string {
	return filepath.Base(tc.dir)
}

type testReport struct {
	name   string
	plan   string
	report tfdiags.Diagnostics
}

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	log.SetFlags(0)

	testCases := findCases(*specDir)
	if len(testCases) == 0 {
		log.Fatalf("No test case found in %s directory\n", *specDir)
	}

	reports := make(chan *testReport)

	// Start measuring execution time of test suites
	var startTime = time.Now()
	var wg sync.WaitGroup
	for _, tc := range testCases {
		wg.Add(1)
		go func(tc *testCase) {
			runTestCase(tc, reports)
			wg.Done()
		}(tc)
	}

	var duration time.Duration
	go func() {
		wg.Wait()
		close(reports)
		// End measuring execution time of test suites onces they all finished
		duration = time.Since(startTime)
	}()

	var success, errors = 0, 0

	exitCode := 0
	for r := range reports {
		fmt.Printf("🏷  %s\n", r.name)
		if r.report.HasErrors() {
			errors++
			exitCode = 1
		} else {
			success++
		}
		if *displayPlan {
			fmt.Println(r.plan)
		}
		printDiags(r.report)
	}
	fmt.Printf("\n🏁 %d suites run in %s \terror : %d \tsuccess : %d\n", len(testCases), duration.String(), errors, success)
	os.Exit(exitCode)
}

func runTestCase(tc *testCase, results chan<- *testReport) {
	// Disable terraform verbose logging except if TF_LOG is set
	logging.SetOutput()
	var planOutput string

	tfCtx, spec, ctxDiags := PrepareTestSuite(".", tc)
	if fatalReport(tc.name(), ctxDiags, planOutput, results) {
		return
	}
	//Refresh is required to have datasources read
	_, ctxDiags = tfCtx.Refresh()
	ctxDiags = ctxDiags.Append(spec.ValidateMocks())
	if fatalReport(tc.name(), ctxDiags, planOutput, results) {
		return
	}

	// Finally, compute the terraform plan
	plan, planDiags := tfCtx.Plan()
	ctxDiags = ctxDiags.Append(planDiags)
	if fatalReport(tc.name(), ctxDiags, planOutput, results) {
		return
	}

	log.SetOutput(os.Stderr)
	var stdout = &strings.Builder{}

	if *displayPlan {
		ui := &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      stdout,
			ErrorWriter: stdout,
		}
		local.RenderPlan(plan, nil, nil, tfCtx.Schemas(), ui, &colorstring.Colorize{Colors: colorstring.DefaultColors})
		planOutput = stdout.String()
	}
	logging.SetOutput()

	validateDiags, err := spec.Validate(plan)
	ctxDiags = ctxDiags.Append(validateDiags)
	if err != nil {
		// TODO manage this error by returning a report with an error diagnostic
		log.Fatal(err)
	}
	results <- &testReport{name: tc.name(), report: ctxDiags, plan: planOutput}
}

// PrepareTestSuite builds the terraform.Context that can compute the plan in given dir
// and parses the spec file containing all assertions. Returned diagnostics may contain errors
func PrepareTestSuite(dir string, tc *testCase) (*terraform.Context, *terraspec.Spec, tfdiags.Diagnostics) {
	var ctxDiags tfdiags.Diagnostics

	absDir, err := filepath.Abs(dir)
	if err != nil {
		ctxDiags = ctxDiags.Append(err)
		return nil, nil, ctxDiags

	}
	providerResolver, err := terraspec.BuildProviderResolver(absDir)
	if err != nil {
		ctxDiags = ctxDiags.Append(err)
		return nil, nil, ctxDiags
	}

	// Parse specs may return mocked data source result
	mockMetadata, diags := terraspec.ReadSpecMetadataMock(tc.specFile)
	ctxDiags = ctxDiags.Append(diags)
	if ctxDiags.HasErrors() {
		return nil, nil, ctxDiags
	}

	tfCtx, diags := terraspec.NewContext(dir, tc.variableFile, *providerResolver, mockMetadata) // Setting a different folder works to parse configuration but not the modules :/
	ctxDiags = ctxDiags.Append(diags)
	if ctxDiags.HasErrors() {
		return nil, nil, ctxDiags
	}

	// Parse specs may return mocked data source result
	spec, diags := terraspec.ReadSpec(tc.specFile, tfCtx.Schemas())
	ctxDiags = ctxDiags.Append(diags)
	if ctxDiags.HasErrors() {
		return nil, nil, ctxDiags
	}

	//If spec contains mocked data source results, they must be provided to the DataSourceReader
	if len(spec.Mocks) > 0 {
		providerResolver.DataSourceReader.SetMock(spec.Mocks)
	}
	spec.DataSourceReader = providerResolver.DataSourceReader
	return tfCtx, spec, ctxDiags
}

func findCases(rootDir string) []*testCase {
	testCases := make([]*testCase, 0)

	rootFis, err := ioutil.ReadDir(rootDir)
	if err != nil {
		return nil
	}

	for _, rootFi := range rootFis {
		if !rootFi.IsDir() {
			continue
		}
		if testCase := findCase(filepath.Join(rootDir, rootFi.Name())); testCase != nil {
			testCases = append(testCases, testCase)
		}
	}
	if testCase := findCase(rootDir); testCase != nil {
		testCases = append(testCases, testCase)
	}
	return testCases
}

func findCase(rootDir string) *testCase {
	fis, err := ioutil.ReadDir(rootDir)
	if err != nil {
		return nil
	}
	var varFile, specFile string
	for _, fi := range fis {
		if fi.IsDir() {
			continue
		}
		if filepath.Ext(fi.Name()) == ".tfvars" {
			varFile = filepath.Join(rootDir, fi.Name())
		}
		if filepath.Ext(fi.Name()) == ".tfspec" {
			specFile = filepath.Join(rootDir, fi.Name())
		}
	}
	if specFile != "" {
		return &testCase{dir: rootDir, variableFile: varFile, specFile: specFile}
	}
	return nil
}

func fatalReport(name string, err tfdiags.Diagnostics, plan string, reports chan<- *testReport) bool {
	if err.HasErrors() {
		reports <- &testReport{name: name, report: err, plan: plan}
		return true
	}
	return false
}

func printDiags(ctxDiags tfdiags.Diagnostics) {
	for _, diag := range ctxDiags {
		switch d := diag.(type) {
		case *terraspec.TerraspecDiagnostic:
			if diag.Severity() == terraspec.Info {
				fmt.Print(" ✔  ")
			} else {
				fmt.Print(" ❌  ")
			}
			if path := tfdiags.GetAttribute(d.Diagnostic); path != nil {
				colorstring.Printf("[bold]%s ", formatPath(path))
			}
			if diag.Severity() == terraspec.Info {
				colorstring.Printf("= [green]%s\n", diag.Description().Detail)
			} else {
				colorstring.Printf(": [red]%s\n", diag.Description().Detail)

			}

		default:
			if subj := diag.Source().Subject; subj != nil {
				colorstring.Printf("[bold]%s#%d,%d : ", subj.Filename, subj.Start.Line, subj.Start.Column)
			}

			if diag.Description().Summary != "" {
				colorstring.Printf("[red]%s : ", diag.Description().Summary)
			}
			colorstring.Printf("[red]%s\n", diag.Description().Detail)

		}
	}
}

func formatPath(path cty.Path) string {
	sb := strings.Builder{}
	for i, pa := range path {
		switch p := pa.(type) {
		case cty.GetAttrStep:
			if i > 0 {
				sb.WriteRune('.')
			}
			sb.WriteString(p.Name)
		case cty.IndexStep:
			sb.WriteRune('[')
			val, _ := p.Key.AsBigFloat().Int64()
			sb.WriteString(strconv.Itoa(int(val)))
			sb.WriteRune(']')
		}
	}
	return sb.String()
}
