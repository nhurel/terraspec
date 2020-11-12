package terraspec

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/facebookgo/symwalk"
	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform/addrs"
	terraformProvider "github.com/hashicorp/terraform/builtin/providers/terraform"
	"github.com/hashicorp/terraform/plugin"
	"github.com/hashicorp/terraform/plugin/discovery"
	"github.com/hashicorp/terraform/providers"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/mitchellh/go-homedir"
	"github.com/zclconf/go-cty/cty"
)

// ProviderResolver is reponsible for finding all provider implementations that can be instanciated
type ProviderResolver struct {
	KnownPlugins     map[addrs.Provider]discovery.PluginMeta
	DataSourceReader *MockDataSourceReader
	ResourceCreator  *FakeResourceCreator
}

// MockDataSourceReader can mock a call to ReadDataSource and return appropriate mocked data
type MockDataSourceReader struct {
	mockDataSources []*Mock
	unmatchedCalls  []cty.Value
	mux             sync.RWMutex
}

// SetMock populates mock data
func (m *MockDataSourceReader) SetMock(mocks []*Mock) {
	m.mockDataSources = mocks
}

// ReadDataSource returns a mock response for the datasource call
func (m *MockDataSourceReader) ReadDataSource(config cty.Value) cty.Value {
	var mockedResult cty.Value = config
	for _, mock := range m.mockDataSources {
		if mock.Query.RawEquals(config) {
			mockedResult = mock.Call()
			return mockedResult
		}
	}

	m.mux.Lock()
	m.unmatchedCalls = append(m.unmatchedCalls, config)
	m.mux.Unlock()

	return mockedResult
}

// UnmatchedCalls returns the list of all data source calls that were not mocked
func (m *MockDataSourceReader) UnmatchedCalls() []cty.Value {
	m.mux.RLock()
	uc := make([]cty.Value, len(m.unmatchedCalls))
	copy(uc, m.unmatchedCalls)
	m.mux.RUnlock()
	return uc
}

// FakeResourceCreator can fake a resource creation by setting all resource attributes as defined in an assertion
type FakeResourceCreator struct {
	fakeResources []*Assert
}

// SetFakes populates fake data
func (f *FakeResourceCreator) SetFakes(fakes []*Assert) {
	f.fakeResources = fakes
}

// GetFake return the attributes to set to the resource terraspec fakes to create
func (f *FakeResourceCreator) GetFake(typeName string, parameters cty.Value) cty.Value {
	var closest *Assert
	var closestDiags, diags tfdiags.Diagnostics

	for _, assert := range f.fakeResources {
		if assert.TypeName.Type == typeName || strings.HasSuffix(assert.TypeName.Type, fmt.Sprintf(".%s", typeName)) {
			diags = checkAssert(cty.Path{}, assert.Value, parameters)
			if closest == nil {
				closest = assert
				closestDiags = diags
			} else {
				if Compare(closestDiags, diags) > 0 {
					closestDiags = diags
					closest = assert
				}
			}
			if !closestDiags.HasErrors() {
				break // early break as soon as there's a successDiags
			}
		}
	}
	if closest != nil {
		return closest.Return
	}
	return cty.NilVal
}

// GetPluginFolder tries to compute the user plugin folder for the current user.
// For windows: %APPDATA%/terraform.d/plugins
// For linux: ~/terraform.d/plugins
func GetPluginFolder() (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		return filepath.FromSlash(fmt.Sprintf("%s/AppData/Roaming/terraform.d/plugins", homeDir)), nil
	}

	return filepath.FromSlash(fmt.Sprintf("%s/.terraform.d/plugins", homeDir)), nil
}

// parseProviderValues retrives the values for hostname, namespace and provider name from the path.
func parseProviderValues(provMeta discovery.PluginMeta) (*addrs.Provider, error) {
	parts := strings.Split(filepath.ToSlash(provMeta.Path), "/")

	partCount := len(parts)

	// overall the path does not have to be that long for tf12
	if partCount < 3 {
		return nil, fmt.Errorf("No valid provider path: %s", provMeta.Path)
	}

	if parts[partCount-3] == "plugins" {
		// plugins folder was initialized by terraform <=0.12

		// HACK: A default provider contains the default registry host and hashicorp namespace
		// because we are using terraform 12 semantics without required spec in the terraform block
		// this is exactly what terraform 13 searches for
		provider := addrs.NewDefaultProvider(provMeta.Name)
		return &provider, nil
	}

	// for tf13 the path has to have a much longer length
	if partCount < 6 {
		return nil, fmt.Errorf("No valid provider path for tf13: %s", provMeta.Path)
	}

	// this is for terraform 0.13 onwards
	return &addrs.Provider{
		Hostname:  svchost.Hostname(parts[partCount-6]),
		Namespace: parts[partCount-5],
		Type:      parts[partCount-4],
	}, nil
}

// BuildProviderResolver returns a ProviderResolver able to find all providers
// provided by plugins
func BuildProviderResolver(dir string) (*ProviderResolver, error) {

	pluginsSchema := make(map[addrs.Provider]discovery.PluginMeta)

	// find plugins in project dir
	projectPluginDir := path.Join(dir, ".terraform/plugins/")
	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	// TODO: this check could probably be improved
	_, err := os.Stat(path.Join(projectPluginDir, osArch))
	isTf13 := os.IsNotExist(err)

	pluginFolders := make([]string, 0)
	// terraform init creates symlinks under linux
	symwalk.Walk(projectPluginDir, func(path string, info os.FileInfo, err error) error {
		if info != nil && info.IsDir() && info.Name() == osArch {
			pluginFolders = append(pluginFolders, path)
		}

		return nil
	})

	if !isTf13 {
		// for terraform 12 add the global plugin folder
		// because tf 12 does not put the locally installed providers into the project folder
		pluginFolder, err := GetPluginFolder()
		if err != nil {
			return nil, err
		}

		pluginFolders = append(pluginFolders, pluginFolder, path.Join(pluginFolder, osArch))
	}

	projectPluginMetaSet := discovery.FindPlugins(plugin.ProviderPluginName, pluginFolders)
	for k := range projectPluginMetaSet {
		provider, err := parseProviderValues(k)
		if err != nil {
			return nil, err
		}
		pluginsSchema[*provider] = k
	}
	return &ProviderResolver{KnownPlugins: pluginsSchema, DataSourceReader: &MockDataSourceReader{}, ResourceCreator: &FakeResourceCreator{}}, nil
}

func newClient(pluginName discovery.PluginMeta) *goplugin.Client {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "plugin",
		Level:  hclog.Error,
		Output: os.Stderr,
	})

	c := goplugin.NewClient(
		&goplugin.ClientConfig{
			Cmd:              exec.Command(pluginName.Path),
			HandshakeConfig:  plugin.Handshake,
			VersionedPlugins: plugin.VersionedPlugins,
			Managed:          true,
			Logger:           logger,
			AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
			AutoMTLS:         true,
		},
	)
	return c
}

// ResolveProviders returns a map of factory capable of instanciating the required plugin to serve the provider
func (r *ProviderResolver) ResolveProviders() map[addrs.Provider]providers.Factory {
	result := make(map[addrs.Provider]providers.Factory)
	for k, p := range r.KnownPlugins {
		result[k] = buildFactory(p, r.DataSourceReader, r.ResourceCreator)
	}

	tfProvider := terraformProvider.NewProvider()
	result[addrs.NewBuiltInProvider("terraform")] = buildWrappedFactory(discovery.PluginMeta{Name: "terraform"}, r.DataSourceReader, tfProvider)
	return result
}

func buildFactory(p discovery.PluginMeta, dsProvider *MockDataSourceReader, resourceCreator *FakeResourceCreator) providers.Factory {
	return func() (providers.Interface, error) {
		return &ProviderInterface{pluginMeta: p, dataSourceProvider: dsProvider, resourceCreator: resourceCreator}, nil
	}
}

func buildWrappedFactory(p discovery.PluginMeta, dsProvider *MockDataSourceReader, wrapped providers.Interface) providers.Factory {
	return func() (providers.Interface, error) {
		return &WrappedProviderInterface{pluginMeta: p, dataSourceProvider: dsProvider, wrapped: wrapped}, nil
	}
}

// ProviderInterface implements providers.Interface for the purpose of
// testing described config
type ProviderInterface struct {
	pluginMeta         discovery.PluginMeta
	dataSourceProvider *MockDataSourceReader
	resourceCreator    *FakeResourceCreator
	_plugin            *plugin.GRPCProvider
	lock               sync.Mutex
}

var _ providers.Interface = (*ProviderInterface)(nil)

func (m *ProviderInterface) plugin() (*plugin.GRPCProvider, error) {
	if m._plugin != nil {
		return m._plugin, nil
	}
	m.lock.Lock()
	defer m.lock.Unlock()

	clientPlugin := newClient(m.pluginMeta)
	c, err := clientPlugin.Client()
	if err != nil {
		return nil, fmt.Errorf("Failed to load plugin %s : %v", m.pluginMeta.Name, err)
	}
	raw, err := c.Dispense(plugin.ProviderPluginName)
	if err != nil {
		return nil, fmt.Errorf("Failed to instantiate the plugin %s : %v", m.pluginMeta.Name, err)
	}
	p, ok := raw.(*plugin.GRPCProvider)
	if !ok {
		return nil, fmt.Errorf("plugin %s is not a provider : %v", m.pluginMeta.Name, err)
	}
	p.PluginClient = clientPlugin
	m._plugin = p
	return m._plugin, nil
}

// GetSchema returns the complete schema for the provider.
func (m *ProviderInterface) GetSchema() providers.GetSchemaResponse {
	var s providers.GetSchemaResponse
	p, err := m.plugin()
	if err != nil {
		s.Diagnostics = s.Diagnostics.Append(err)
	} else {
		s = p.GetSchema()
	}

	return s
}

// PrepareProviderConfig allows the provider to validate the configuration
// values, and set or override any values with defaults.
func (m *ProviderInterface) PrepareProviderConfig(req providers.PrepareProviderConfigRequest) providers.PrepareProviderConfigResponse {
	var s providers.PrepareProviderConfigResponse
	p, err := m.plugin()
	if err != nil {
		s.Diagnostics = s.Diagnostics.Append(err)
	} else {
		s = p.PrepareProviderConfig(req)
	}
	return s
}

// ValidateResourceTypeConfig allows the provider to validate the resource
// configuration values.
func (m *ProviderInterface) ValidateResourceTypeConfig(req providers.ValidateResourceTypeConfigRequest) providers.ValidateResourceTypeConfigResponse {
	// TODO : If useful, implement validation based on schema
	return providers.ValidateResourceTypeConfigResponse{}
}

// ValidateDataSourceConfig allows the provider to validate the data source
// configuration values.
func (m *ProviderInterface) ValidateDataSourceConfig(req providers.ValidateDataSourceConfigRequest) providers.ValidateDataSourceConfigResponse {
	// TODO : If useful, implement validation based on schema
	return providers.ValidateDataSourceConfigResponse{}
}

// UpgradeResourceState is called when the state loader encounters an
// instance state whose schema version is less than the one reported by the
// currently-used version of the corresponding provider, and the upgraded
// result is used for any further processing.
func (m *ProviderInterface) UpgradeResourceState(req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	// FIXME Hopefully this will never be required
	// Make sure terraspec is always run from an empty state (may need to override the backend)
	return providers.UpgradeResourceStateResponse{}
}

// Configure configures and initialized the provider.
func (m *ProviderInterface) Configure(req providers.ConfigureRequest) providers.ConfigureResponse {
	return providers.ConfigureResponse{}
}

// Stop is called when the provider should halt any in-flight actions.
//
// Stop should not block waiting for in-flight actions to complete. It
// should take any action it wants and return immediately acknowledging it
// has received the stop request. Terraform will not make any further API
// calls to the provider after Stop is called.
//
// The error returned, if non-nil, is assumed to mean that signaling the
// stop somehow failed and that the user should expect potentially waiting
// a longer period of time.
func (m *ProviderInterface) Stop() error {
	return nil
}

// ReadResource refreshes a resource and returns its current state.
func (m *ProviderInterface) ReadResource(req providers.ReadResourceRequest) providers.ReadResourceResponse {
	return providers.ReadResourceResponse{}
}

// PlanResourceChange takes the current state and proposed state of a
// resource, and returns the planned final state.
func (m *ProviderInterface) PlanResourceChange(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	var s providers.PlanResourceChangeResponse
	p, err := m.plugin()
	if err != nil {
		s.Diagnostics = s.Diagnostics.Append(err)
	} else {
		s = p.PlanResourceChange(req)
		fake := m.resourceCreator.GetFake(req.TypeName, req.Config)
		if !fake.IsNull() {
			s.PlannedState = Merge(s.PlannedState, fake)
		}
	}
	return s
}

// ApplyResourceChange takes the planned state for a resource, which may
// yet contain unknown computed values, and applies the changes returning
// the final state.
func (m *ProviderInterface) ApplyResourceChange(req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{
		NewState: req.PlannedState,
	}
}

// ImportResourceState requests that the given resource be imported.
func (m *ProviderInterface) ImportResourceState(req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	return providers.ImportResourceStateResponse{}
}

// ReadDataSource returns the data source's current state.
func (m *ProviderInterface) ReadDataSource(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	mockedResult := m.dataSourceProvider.ReadDataSource(req.Config)
	return providers.ReadDataSourceResponse{State: mockedResult}
}

// Close shuts down the plugin process if applicable.
func (m *ProviderInterface) Close() error {
	if m._plugin != nil {
		m.lock.Lock()
		if m._plugin != nil {
			if m._plugin.PluginClient != nil {
				m._plugin.PluginClient.Kill()
			}
			m._plugin = nil
		}
		m.lock.Unlock()
	}
	return nil
}

// WrappedProviderInterface implements providers.Interface by wrapping a true
// provider.Interface and only call allowed method
type WrappedProviderInterface struct {
	pluginMeta         discovery.PluginMeta
	dataSourceProvider *MockDataSourceReader
	wrapped            providers.Interface
}

// GetSchema returns the complete schema for the provider.
func (w *WrappedProviderInterface) GetSchema() providers.GetSchemaResponse {
	return w.wrapped.GetSchema()
}

// PrepareProviderConfig allows the provider to validate the configuration
// values, and set or override any values with defaults.
func (w *WrappedProviderInterface) PrepareProviderConfig(req providers.PrepareProviderConfigRequest) providers.PrepareProviderConfigResponse {
	return w.wrapped.PrepareProviderConfig(req)
}

// ValidateResourceTypeConfig allows the provider to validate the resource
// configuration values.
func (w *WrappedProviderInterface) ValidateResourceTypeConfig(req providers.ValidateResourceTypeConfigRequest) providers.ValidateResourceTypeConfigResponse {
	return w.wrapped.ValidateResourceTypeConfig(req)
}

// ValidateDataSourceConfig allows the provider to validate the data source
// configuration values.
func (w *WrappedProviderInterface) ValidateDataSourceConfig(req providers.ValidateDataSourceConfigRequest) providers.ValidateDataSourceConfigResponse {
	return providers.ValidateDataSourceConfigResponse{}
}

// UpgradeResourceState is called when the state loader encounters an
// instance state whose schema version is less than the one reported by the
// currently-used version of the corresponding provider, and the upgraded
// result is used for any further processing.
func (w *WrappedProviderInterface) UpgradeResourceState(req providers.UpgradeResourceStateRequest) providers.UpgradeResourceStateResponse {
	return w.wrapped.UpgradeResourceState(req)
}

// Configure configures and initialized the provider.
func (w *WrappedProviderInterface) Configure(req providers.ConfigureRequest) providers.ConfigureResponse {
	return providers.ConfigureResponse{}
}

// Stop is called when the provider should halt any in-flight actions.
//
// Stop should not block waiting for in-flight actions to complete. It
// should take any action it wants and return immediately acknowledging it
// has received the stop request. Terraform will not make any further API
// calls to the provider after Stop is called.
//
// The error returned, if non-nil, is assumed to mean that signaling the
// stop somehow failed and that the user should expect potentially waiting
// a longer period of time.
func (w *WrappedProviderInterface) Stop() error {
	return w.wrapped.Stop()
}

// ReadResource refreshes a resource and returns its current state.
func (w *WrappedProviderInterface) ReadResource(req providers.ReadResourceRequest) providers.ReadResourceResponse {
	return providers.ReadResourceResponse{}
}

// PlanResourceChange takes the current state and proposed state of a
// resource, and returns the planned final state.
func (w *WrappedProviderInterface) PlanResourceChange(req providers.PlanResourceChangeRequest) providers.PlanResourceChangeResponse {
	return w.wrapped.PlanResourceChange(req)
}

// ApplyResourceChange takes the planned state for a resource, which may
// yet contain unknown computed values, and applies the changes returning
// the final state.
func (w *WrappedProviderInterface) ApplyResourceChange(req providers.ApplyResourceChangeRequest) providers.ApplyResourceChangeResponse {
	return providers.ApplyResourceChangeResponse{}
}

// ImportResourceState requests that the given resource be imported.
func (w *WrappedProviderInterface) ImportResourceState(req providers.ImportResourceStateRequest) providers.ImportResourceStateResponse {
	return providers.ImportResourceStateResponse{}
}

// ReadDataSource returns the data source's current state.
func (w *WrappedProviderInterface) ReadDataSource(req providers.ReadDataSourceRequest) providers.ReadDataSourceResponse {
	mockedResult := w.dataSourceProvider.ReadDataSource(req.Config)
	return providers.ReadDataSourceResponse{State: mockedResult}
}

// Close shuts down the plugin process if applicable.
func (w *WrappedProviderInterface) Close() error {
	return w.wrapped.Close()
}
