package terraspec

import (
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform/addrs"
	"github.com/stretchr/testify/assert"
)

func TestBuildProviderResolverFindsLegacyProviderInHome(t *testing.T) {
	// backup the plugin folder and create an empty one
	_, restorePluginFolder := EnsureEmptyPluginFolder(t)
	defer restorePluginFolder()

	provider, providerVersion, providerPath := InstallLegacyProvider(t)

	cleanupTerraform := TerraformInit(t, "testdata/test_project")
	defer cleanupTerraform()

	cwd, err := os.Getwd()
	assert.Nil(t, err)
	projectPluginFolder := path.Join(cwd, ".terraform/plugins")

	osArch := runtime.GOOS + "_" + runtime.GOARCH
	providerFileName := filepath.Base(providerPath)
	cloudfoundryPath := filepath.FromSlash(path.Join(projectPluginFolder, string(provider.Hostname), provider.Namespace, provider.Type, providerVersion, osArch, providerFileName))

	awsFileName := "terraform-provider-aws_v3.8.0_x5"
	if runtime.GOOS == "windows" {
		awsFileName = awsFileName + ".exe"
	}
	awsPath := filepath.FromSlash(path.Join(projectPluginFolder, "registry.terraform.io", "hashicorp", "aws", "3.8.0", osArch, awsFileName))
	

	providerResolver, err := BuildProviderResolver(".")
	assert.Nil(t, err)

	providerMetaCloudfoundry := providerResolver.KnownPlugins[provider]
	providerMetaAws := providerResolver.KnownPlugins[addrs.Provider{Namespace: "hashicorp", Hostname: "registry.terraform.io", Type: "aws"}]

	assert.NotEqual(t, 0, len(providerResolver.KnownPlugins), "Could not find any provider plugins")
	assert.Equal(t, cloudfoundryPath, providerMetaCloudfoundry.Path)
	assert.Equal(t, provider.Type, providerMetaCloudfoundry.Name)
	assert.Equal(t, providerVersion, string(providerMetaCloudfoundry.Version))

	assert.Equal(t, awsPath, providerMetaAws.Path)
	assert.Equal(t, "aws", providerMetaAws.Name)
	assert.Equal(t, "3.8.0", string(providerMetaAws.Version))
}
