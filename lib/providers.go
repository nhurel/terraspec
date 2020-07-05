package terraspec

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"runtime"
	"sync"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/plugin"
	"github.com/hashicorp/terraform/plugin/discovery"
	"github.com/hashicorp/terraform/providers"
	"github.com/zclconf/go-cty/cty"
)

// ProviderResolver implements terraform's providers.Resolver interface
type ProviderResolver struct {
	KnownPlugins     map[addrs.Provider]discovery.PluginMeta
	DataSourceReader *MockDataSourceReader
}

// MockDataSourceReader can mock a call to ReadDataSource and return appropriate mocked data
type MockDataSourceReader struct {
	mockDataSources []*Mock
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
			mockedResult = mock.Data
			break
		}
	}
	return mockedResult
}

// BuildProviderResolver returns a ProviderResolver able to find all providers
// provided by plugins
func BuildProviderResolver(dir string) (*ProviderResolver, error) {
	var pluginDir = path.Join(dir, fmt.Sprintf(".terraform/plugins/%s_%s", runtime.GOOS, runtime.GOARCH))

	pluginMetaSet := discovery.FindPlugins(plugin.ProviderPluginName, []string{pluginDir})
	pluginsSchema := make(map[addrs.Provider]discovery.PluginMeta)

	for k := range pluginMetaSet {
		pluginsSchema[addrs.NewLegacyProvider(k.Name)] = k
	}
	return &ProviderResolver{KnownPlugins: pluginsSchema, DataSourceReader: &MockDataSourceReader{}}, nil
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

// ResolveProviders defined by providers.Resolver interface.
// It returns a factory capable of instanciating the required plugin to serve the provider
func (r *ProviderResolver) ResolveProviders(reqd discovery.PluginRequirements) (map[addrs.Provider]providers.Factory, []error) {
	result := make(map[addrs.Provider]providers.Factory)
	for k, p := range r.KnownPlugins {
		result[k] = buildFactory(p, r.DataSourceReader)
	}
	return result, nil
}

func buildFactory(p discovery.PluginMeta, dsProvider *MockDataSourceReader) providers.Factory {
	return func() (providers.Interface, error) {

		return &ProviderInterface{pluginMeta: p, dataSourceProvider: dsProvider}, nil
	}
}

// ProviderInterface implements providers.Interface for the purpose of
// testing described config
type ProviderInterface struct {
	pluginMeta         discovery.PluginMeta
	dataSourceProvider *MockDataSourceReader
	_plugin            *plugin.GRPCProvider
	lock               sync.Mutex
}

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
