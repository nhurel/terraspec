// +build integrationtests

package integrationtests

import (
	"os"
	"strings"
	"testing"
)

type terraformTest struct {
	terraformVersion string
	testProjectPath  string
}

func TestExecTerraspecWithTestProjectSucceeds(t *testing.T) {
	cwd := Getwd(t)
	rootDir := cwd + "/.."

	testCases := []terraformTest{
		{
			terraformVersion: "0.13.4",
			testProjectPath:  "test_project",
		},
		{
			terraformVersion: "0.14.7",
			testProjectPath:  "test_project",
		},
		{
			terraformVersion: "0.12.29",
			testProjectPath:  "test_project_tf12",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testProjectPath, func(t *testing.T) {
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
			exitCode, _, err := RunTerraspec(t, terraspecPath, testCase.testProjectPath)
			if err != nil {
				t.Fatalf("Error while executing terraspec: %v", err)
			}
			if exitCode != 0 {
				t.Errorf("Terraspec return with exit code %d\n", exitCode)
			}
		})
	}
}

func TestExecTerraspecFailsProperlyWhenTerraformInitNotRun(t *testing.T) {
	cwd := Getwd(t)
	rootDir := cwd + "/.."

	testCases := []terraformTest{
		{
			terraformVersion: "0.13.4",
			testProjectPath:  "test_project",
		},
		{
			terraformVersion: "0.14.7",
			testProjectPath:  "test_project",
		},
		{
			terraformVersion: "0.12.29",
			testProjectPath:  "test_project_tf12",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.testProjectPath, func(t *testing.T) {
			t.Logf("Testing integration with terraform %s", testCase.terraformVersion)

			terraspecPath := GetTerraspec(t, rootDir)
			err := os.RemoveAll(testCase.testProjectPath + "/.terraform")
			if err != nil {
				t.Fatalf("%v", err)
			}
			exitCode, output, err := RunTerraspec(t, terraspecPath, testCase.testProjectPath)
			if err == nil {
				t.Error("No error returned")
			}
			if exitCode != 1 {
				t.Errorf("Terraspec return with exit code %d\n", exitCode)
			}
			if strings.Contains(output, "panic") {
				t.Errorf("program paniced !")
			}
		})
	}
}
