package main

import (
	"fmt"
	"log"
	"os"

	"github.com/hashicorp/terraform/backend/local"
	"github.com/hashicorp/terraform/helper/logging"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/colorstring"
	terraspec "github.com/nhurel/terraspec/lib"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	app = kingpin.New("terraspec", "Unit test terraform config")
	// dir = app.Flag("dir", "path to terraform config dir to test").Default(".").String()
	specDir     = app.Flag("spec", "path to folder containing test cases").Default("spec").String()
	displayPlan = app.Flag("display-plan", "Print the full plan before the results").Default("false").Bool()
)

func main() {
	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Disable terraform verbose logging except if TF_LOF is set
	logging.SetOutput()

	tfCtx, ctxDiags := terraspec.NewContext(".", "spec/with_public_ip/override.tfvars") // Setting a different folder works to parse configuration but not the modules :/
	FatalDiags(ctxDiags)
	plan, ctxDiags := tfCtx.Plan()
	FatalDiags(ctxDiags)

	log.SetOutput(os.Stderr)

	if *displayPlan {
		ui := &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		}
		local.RenderPlan(plan, nil, tfCtx.Schemas(), ui, &colorstring.Colorize{Colors: colorstring.DefaultColors})
	}
	logging.SetOutput()
	// TODO Browse all specs in spec dir, compute them in parallel and output all their results
	spec, diags := terraspec.ReadSpec("spec/with_public_ip/with_public_ip.tfspec", tfCtx.Schemas())
	FatalDiags(diags)
	diags, err := spec.Validate(plan)
	if err != nil {
		log.Fatal(err)
	}
	FatalDiags(diags)
	os.Exit(0)

}

// FatalDiags prints all errors contained in given Diagnostics and exit
// If given Diagnostics has no error, application is not exited
func FatalDiags(ctxDiags tfdiags.Diagnostics) {
	if ctxDiags.HasErrors() {
		log.SetOutput(os.Stderr)
		for _, diag := range ctxDiags {
			if subj := diag.Source().Subject; subj != nil {
				fmt.Printf("%s#%d,%d : ", subj.Filename, subj.Start.Line, subj.Start.Column)
			}
			if diag.Description().Summary != "" {
				fmt.Println(diag.Description().Summary)
			} else {
				fmt.Println(diag.Description().Detail)
			}

		}
		os.Exit(1)
	}
}
