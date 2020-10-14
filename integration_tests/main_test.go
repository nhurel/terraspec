package integrationtests

import (
	"testing"
)

type terraformTest struct {
	terraformVersion string
	testProjectPath string
}

func TestExecTerraspecWithTestProjectSucceeds(t *testing.T) {
	cwd := Getwd(t)
	rootDir := cwd + "/.."

	testCases := []terraformTest {
		{
			terraformVersion: "0.13.4",
			testProjectPath: "test_project",
		},
		{
			terraformVersion: "0.12.29",
			testProjectPath: "test_project_tf12",
		},
	} 

	for _, testCase := range testCases {
		func() {
			t.Logf("Testing integration with terraform %s", testCase.terraformVersion)

			// backup the plugin folder and create an empty one
			_, restorePluginFolder := EnsureEmptyPluginFolder(t)
			defer restorePluginFolder()

			_, _, _ = InstallLegacyProvider(t, testCase.terraformVersion)

			// after this we are in the test_project folder
			terraformPath := GetTerraform(t, testCase.terraformVersion, rootDir)
			cleanupTerraform := TerraformInit(t, terraformPath, testCase.testProjectPath)
			defer cleanupTerraform()
			
			terraspecPath := GetTerraspec(t, rootDir)
			RunTerraspec(t, terraspecPath, ".")
		}()
	}
}