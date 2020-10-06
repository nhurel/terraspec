package integrationtests

import (
	"testing"

	terraspec "github.com/nhurel/terraspec/lib"
)

func TestExecTerraspecWithTestProjectSucceeds(t *testing.T) {
	// backup the plugin folder and create an empty one
	_, restorePluginFolder := EnsureEmptyPluginFolder(t)
	defer restorePluginFolder()

	_, _, _ = InstallLegacyProvider(t)

	cleanupTerraform := TerraformInit(t, "test_project")
	defer cleanupTerraform()
	
	result := terraspec.ExecTerraspec("spec", false, "")

	if result != 0 {
		t.Fatalf("Exit code from ExecTerraspec does not indicate success")
	}
}