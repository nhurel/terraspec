package integrationtests

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform/addrs"
	terraspec "github.com/nhurel/terraspec/lib"
)

func TestBuildProviderResolverFindsLegacyProviderInHome(t *testing.T) {
	// backup the plugin folder and create an empty one
	_, restorePluginFolder := EnsureEmptyPluginFolder(t)
	defer restorePluginFolder()

	provider, providerVersion, providerPath := InstallLegacyProvider(t)

	cleanupTerraform := TerraformInit(t, "test_project")
	defer cleanupTerraform()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	projectPluginFolder := path.Join(cwd, ".terraform/plugins")

	osArch := runtime.GOOS + "_" + runtime.GOARCH
	providerFileName := filepath.Base(providerPath)
	cloudfoundryPath := filepath.FromSlash(path.Join(projectPluginFolder, string(provider.Hostname), provider.Namespace, provider.Type, providerVersion, osArch, providerFileName))

	awsFileName := "terraform-provider-aws_v3.8.0_x5"
	if runtime.GOOS == "windows" {
		awsFileName = awsFileName + ".exe"
	}
	awsPath := filepath.FromSlash(path.Join(projectPluginFolder, "registry.terraform.io", "hashicorp", "aws", "3.8.0", osArch, awsFileName))
	

	providerResolver, err := terraspec.BuildProviderResolver(".")
	if err != nil {
		t.Fatalf("%v", err)
	}

	providerMetaCloudfoundry := providerResolver.KnownPlugins[provider]
	providerMetaAws := providerResolver.KnownPlugins[addrs.Provider{Namespace: "hashicorp", Hostname: "registry.terraform.io", Type: "aws"}]

	if  len(providerResolver.KnownPlugins) == 0 {
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

}
