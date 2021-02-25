// +build integrationtests

package integrationtests

import (
	"path"
	"path/filepath"
	"runtime"
	"testing"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform/addrs"
	terraspec "github.com/nhurel/terraspec/lib"
)

func TestBuildProviderResolverFindsCustomProvider(t *testing.T) {
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

	pluginFolder, err := terraspec.GetPluginFolder()
	if err != nil {
		t.Fatalf("Could not find global terraform plugin folder: %v", err)
	}

	for _, testCase := range testCases {
		t.Logf("Testing that custom provider is found with terraform %s", testCase.terraformVersion)

		func() {
			_, minorVersion, _ := ParseTerraformVersion(t, testCase.terraformVersion)
			isTf13 := minorVersion >= 13

			// backup the plugin folder and create an empty one
			_, restorePluginFolder := EnsureEmptyPluginFolder(t)
			defer restorePluginFolder()

			provider, providerVersion, providerPath := InstallLegacyProvider(t, testCase.terraformVersion)

			terraformPath := GetTerraform(t, testCase.terraformVersion, rootDir)
			cleanupTerraform := TerraformInit(t, terraformPath, testCase.testProjectPath)
			defer cleanupTerraform()

			testFolder := Getwd(t)
			projectPluginFolder := path.Join(testFolder, testCase.testProjectPath, ".terraform/plugins")

			osArch := runtime.GOOS + "_" + runtime.GOARCH
			providerFileName := filepath.Base(providerPath)
			cloudfoundryPath := filepath.FromSlash(path.Join(projectPluginFolder, string(provider.Hostname), provider.Namespace, provider.Type, providerVersion, osArch, providerFileName))

			awsFileName := "terraform-provider-aws_v3.8.0_x5"
			if runtime.GOOS == "windows" {
				awsFileName = awsFileName + ".exe"
			}
			awsHostname := svchost.Hostname("registry.terraform.io")
			awsNamespace := "hashicorp"
			awsPath := filepath.FromSlash(path.Join(projectPluginFolder, "registry.terraform.io", "hashicorp", "aws", "3.8.0", osArch, awsFileName))
			if !isTf13 {
				provider.Hostname = addrs.DefaultRegistryHost
				provider.Namespace = "hashicorp" // addrs.LegacyProviderNamespace
				cloudfoundryPath = filepath.FromSlash(path.Join(pluginFolder, osArch, providerFileName))

				awsHostname = addrs.DefaultRegistryHost
				awsNamespace = "hashicorp" // addrs.LegacyProviderNamespace
				awsPath = filepath.FromSlash(path.Join(projectPluginFolder, osArch, awsFileName))
			}

			providerResolver, err := terraspec.BuildProviderResolver(testCase.testProjectPath)
			if err != nil {
				t.Fatalf("%v", err)
			}

			providerMetaCloudfoundry := providerResolver.KnownPlugins[provider]
			providerMetaAws := providerResolver.KnownPlugins[addrs.Provider{Namespace: awsNamespace, Hostname: awsHostname, Type: "aws"}]

			if len(providerResolver.KnownPlugins) == 0 {
				t.Fatal("Could not find any provider plugins")
			}

			if providerMetaCloudfoundry.Path != cloudfoundryPath {
				t.Errorf("Unexpected path for cloudfoundry provider. Expected %s. Got %s.", cloudfoundryPath, providerMetaCloudfoundry.Path)
			}
			if providerMetaCloudfoundry.Name != provider.Type {
				t.Errorf("Unexpected name for cloudfoundry provider. Expected %s. Got %s.", provider.Type, providerMetaCloudfoundry.Name)
			}
			if string(providerMetaCloudfoundry.Version) != providerVersion {
				t.Errorf("Unexpected version for cloudfoundry provider. Expected %s. Got %s.", providerVersion, string(providerMetaCloudfoundry.Version))
			}

			if providerMetaAws.Path != awsPath {
				t.Errorf("Unexpected path for aws provider. Expected %s. Got %s.", awsPath, providerMetaAws.Path)
			}
			if providerMetaAws.Name != "aws" {
				t.Errorf("Unexpected name for aws provider. Expected %s. Got %s.", "aws", providerMetaAws.Name)
			}
			if string(providerMetaAws.Version) != "3.8.0" {
				t.Errorf("Unexpected version for aws provider. Expected %s. Got %s.", "3.8.0", string(providerMetaAws.Version))
			}
		}()
	}

}
