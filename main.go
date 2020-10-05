package main

import (
	"fmt"
	"os"

	tfversion "github.com/hashicorp/terraform/version"
	terraspec "github.com/nhurel/terraspec/lib"
	"gopkg.in/alecthomas/kingpin.v2"
)

var (
	//Version is the version of the app. This is set at build time
	Version string
	app     = kingpin.New("terraspec", "Unit test terraform config")
	// dir = app.Flag("dir", "path to terraform config dir to test").Default(".").String()
	specDir     = app.Flag("spec", "path to folder containing test cases").Default("spec").String()
	displayPlan = app.Flag("display-plan", "Print the full plan before the results").Default("false").Bool()
	tfVersion   = app.Flag("claim-version", "Simulate terraform version : This flag is a workaround to help upgrading terraspec and terraform independently. This flag won't change terraspec behavior but will make it pass version check").String()
)

func init() {
	var versionString = `Terraspec Version : %s
Terraform Version : %s`
	app.Version(fmt.Sprintf(versionString, Version, tfversion.SemVer))
}

func main() {

	kingpin.MustParse(app.Parse(os.Args[1:]))

	exitCode := terraspec.ExecTerraspec(*specDir, *displayPlan, *tfVersion)
	
	os.Exit(exitCode)
}
