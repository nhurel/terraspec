package terraspec

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/plugin/discovery"
	"github.com/zclconf/go-cty/cty"
)

func TestBuildProviderResolver(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Could not get cwd: %v", err)
	}

	provResolver, err := BuildProviderResolver("testdata", false)
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
		Name:    "testprovider",
		Version: "0.1.2",
		Path: filepath.ToSlash(
			fmt.Sprintf("%s/testdata/.terraform/plugins/no.registry.com/nocorp/testprovider/0.1.2/%s_%s/%s",
				cwd, runtime.GOOS, runtime.GOARCH, providerExe)),
	}

	if pluginMeta != expectedMeta {
		t.Errorf("PluginMeta not correct. Got %v. Expected %v.", pluginMeta, expectedMeta)
	}
}

// TestBuildProviderResolverLegacy test that pre terraform 0.13 providers are recognized correctly.
func TestBuildProviderResolverLegacy(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Could not get cwd: %v", err)
	}

	provResolver, err := BuildProviderResolver("testdata12", false)
	if err != nil {
		t.Fatalf("Could not build provider resolver: %v", err)
	}

	pluginMeta := provResolver.KnownPlugins[addrs.Provider{
		Hostname:  addrs.DefaultRegistryHost,
		Namespace: "hashicorp", // addrs.LegacyProviderNamespace,
		Type:      "testprovider",
	}]

	pluginMeta.Path = filepath.ToSlash(pluginMeta.Path)

	providerExe := "terraform-provider-testprovider_v0.1.2"
	if runtime.GOOS == "windows" {
		providerExe += ".exe"
	}
	expectedMeta := discovery.PluginMeta{
		Name:    "testprovider",
		Version: "0.1.2",
		Path: filepath.ToSlash(
			fmt.Sprintf("%s/testdata12/.terraform/plugins/%s_%s/%s",
				cwd, runtime.GOOS, runtime.GOARCH, providerExe)),
	}

	if pluginMeta != expectedMeta {
		t.Errorf("PluginMeta not correct. Got %v. Expected %v.", pluginMeta, expectedMeta)
	}
}

func TestGetFake(t *testing.T) {

	commonQuery := cty.ObjectVal(map[string]cty.Value{
		"key":      cty.StringVal("good-key"),
		"property": cty.StringVal("good-value"),
	})

	goodResult := cty.ObjectVal(map[string]cty.Value{
		"id": cty.NumberIntVal(1000),
	})
	badResult := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("bad result"),
	})

	tests := map[string]struct {
		typeName string
		query    cty.Value
		expected *Assert
		others   []*Assert
	}{
		"no fakes": {
			typeName: "type",
			query:    commonQuery,
			expected: &Assert{Return: cty.NilVal},
			others:   []*Assert{},
		},
		"no good typeName": {
			typeName: "type",
			query:    commonQuery,
			expected: &Assert{TypeName: TypeName{Type: "other", Name: "bad"}, Return: cty.NilVal},
			others:   []*Assert{},
		},
		"one good typeName": {
			typeName: "type",
			query:    commonQuery,
			expected: &Assert{
				TypeName: TypeName{Type: "type", Name: "good"},
				Value: cty.ObjectVal(map[string]cty.Value{
					"key":      cty.StringVal("bad-key"),
					"property": cty.StringVal("bad-value"),
				}),
				Return: goodResult},
			others: []*Assert{
				{
					TypeName: TypeName{Type: "other", Name: "wrong"},
					Value:    commonQuery,
					Return:   badResult,
				},
			},
		},
		"best good typeName": {
			typeName: "type",
			query:    commonQuery,
			expected: &Assert{
				TypeName: TypeName{Type: "type", Name: "good"},
				Value: cty.ObjectVal(map[string]cty.Value{
					"key":      cty.StringVal("good-key"),
					"property": cty.StringVal("bad-value"),
				}),
				Return: goodResult},
			others: []*Assert{
				{
					TypeName: TypeName{Type: "other", Name: "wrong"},
					Value:    commonQuery,
					Return:   badResult,
				},
				{
					TypeName: TypeName{Type: "type", Name: "good"},
					Value: cty.ObjectVal(map[string]cty.Value{
						"key":      cty.StringVal("bad-key"),
						"property": cty.StringVal("bad-value"),
					}),
					Return: badResult,
				},
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			frc := &FakeResourceCreator{}
			frc.SetFakes(append(tt.others, tt.expected))
			got := frc.GetFake(tt.typeName, tt.query)

			if got.IsNull() && tt.expected.Return.IsNull() {
				t.Logf("GetFake returned cty.NilVal as expected")
			} else {
				if (got.IsNull() != tt.expected.Return.IsNull()) || !got.RawEquals(tt.expected.Return) {
					t.Errorf("GetFake didn't return expected value. Got: %v\n Expected: %v", got, tt.expected.Return)
				}
			}
		})
	}

}

func TestReadDataSource(t *testing.T) {
	providerConfig := map[string]cty.Value{
		"prov": cty.ObjectVal(map[string]cty.Value{
			"region": cty.StringVal("default"),
		}),
		"prov.region1": cty.ObjectVal(map[string]cty.Value{
			"region": cty.StringVal("region-1"),
		}),
		"prov.region2": cty.ObjectVal(map[string]cty.Value{
			"region": cty.StringVal("region-2"),
		}),
	}

	datasourceQuery := cty.ObjectVal(map[string]cty.Value{"query": cty.StringVal("123")})

	mockDefault := &Mock{TypeName: TypeName{Type: "prov_type", Name: "resource"}, Query: datasourceQuery, Data: cty.MapVal(map[string]cty.Value{"name": cty.StringVal("default")}), ProviderAlias: "prov"}
	mockRegion1 := &Mock{TypeName: TypeName{Type: "prov_type", Name: "resource"}, Query: datasourceQuery, Data: cty.MapVal(map[string]cty.Value{"name": cty.StringVal("region1")}), ProviderAlias: "prov.region1"}
	mockRegion2 := &Mock{TypeName: TypeName{Type: "prov_type", Name: "resource"}, Query: datasourceQuery, Data: cty.MapVal(map[string]cty.Value{"name": cty.StringVal("region2")}), ProviderAlias: "prov.region2"}

	tests := map[string]struct {
		//all providerConfigs known by mockDataSourceReader
		provConfig map[string]cty.Value
		//all mocks known by mockDataSourceReader
		mocks []*Mock
		//datasource query
		configParam cty.Value
		//the config of the provider that runs the query
		providerParam cty.Value
		expected      cty.Value
	}{
		"shouldFindRegion1": {
			provConfig:    providerConfig,
			mocks:         []*Mock{mockDefault, mockRegion1, mockRegion2},
			configParam:   datasourceQuery,
			providerParam: providerConfig[mockRegion1.ProviderAlias],
			expected:      mockRegion1.Data,
		},
		"shouldFindRegion2": {
			provConfig:    providerConfig,
			mocks:         []*Mock{mockDefault, mockRegion1, mockRegion2},
			configParam:   datasourceQuery,
			providerParam: providerConfig[mockRegion2.ProviderAlias],
			expected:      mockRegion2.Data,
		},
		"shouldFindDefault": {
			provConfig:    providerConfig,
			mocks:         []*Mock{mockDefault, mockRegion1, mockRegion2},
			configParam:   datasourceQuery,
			providerParam: providerConfig[mockDefault.ProviderAlias],
			expected:      mockDefault.Data,
		},
		"shouldReturnQueryWhenNoMockFound": {
			provConfig:    providerConfig,
			mocks:         []*Mock{mockDefault, mockRegion2},
			configParam:   datasourceQuery,
			providerParam: providerConfig[mockRegion1.ProviderAlias],
			expected:      datasourceQuery,
		},
		"shouldReturnFirstMatchWhenAliasIsDefault": {
			provConfig:    providerConfig,
			mocks:         []*Mock{mockRegion1, mockRegion2},
			configParam:   datasourceQuery,
			providerParam: providerConfig[mockDefault.ProviderAlias],
			expected:      mockRegion1.Data,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mdsr := &MockDataSourceReader{}
			mdsr.SetProviderConfig(tt.provConfig)
			mdsr.SetMock(tt.mocks)

			got := mdsr.ReadDataSource("prov_type", tt.configParam, tt.providerParam)
			if !got.RawEquals(tt.expected) {
				t.Errorf("ReadDataSource didn't return expected value. Got: %v\n Expected: %v", got, tt.expected)
			}
		})
	}

}
