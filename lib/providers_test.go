package terraspec

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/plugin/discovery"
)

func TestBuildProviderResolver(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Could not get cwd: %v", err)
	}

	provResolver, err := BuildProviderResolver("testdata")
	if err != nil {
		t.Fatalf("Could not build provider resolver: %v", err)
	}

	pluginMeta := provResolver.KnownPlugins[addrs.Provider{Hostname: "no.registry.com", Namespace: "nocorp", Type: "testprovider"}]

	pluginMeta.Path = filepath.ToSlash(pluginMeta.Path)

	providerExe := "terraform-provider-testprovider_v0.1.2"
	if runtime.GOOS == "windows" {
		providerExe += ".exe"
	}
	expectedMeta := discovery.PluginMeta{
		Name: "testprovider",
		Version: "0.1.2",
		Path: filepath.ToSlash(
			fmt.Sprintf("%s/testdata/.terraform/plugins/no.registry.com/nocorp/testprovider/0.1.2/%s_%s/%s", 
				cwd, runtime.GOOS, runtime.GOARCH, providerExe)),
	}

	if pluginMeta != expectedMeta  {
		t.Errorf("PluginMeta not correct. Got %v. Expected %v.", pluginMeta, expectedMeta)
	}
}