package terraspec

import (
	"github.com/hashicorp/terraform/command"
	"github.com/hashicorp/terraform/provisioners"
)

// ProvisionersFactory return a map of mock provisioners
// as provisioners are not meant to be called in the context of terraspec
func ProvisionersFactory() map[string]provisioners.Factory {
	factories := make(map[string]provisioners.Factory)
	for name := range command.InternalProvisioners {
		factories[name] = func() (provisioners.Interface, error) {
			return &MockProvisionerInterface{}, nil
		}
	}
	return factories
}

// MockProvisionerInterface is a NoOp implementation of terraform's provisioners.Interface
type MockProvisionerInterface struct {
}

// GetSchema returns the schema for the provisioner configuration.
func (p *MockProvisionerInterface) GetSchema() provisioners.GetSchemaResponse {
	return provisioners.GetSchemaResponse{}
}

// ValidateProvisionerConfig allows the provisioner to validate the
// configuration values.
func (p *MockProvisionerInterface) ValidateProvisionerConfig(req provisioners.ValidateProvisionerConfigRequest) provisioners.ValidateProvisionerConfigResponse {
	return provisioners.ValidateProvisionerConfigResponse{}
}

// ProvisionResource runs the provisioner with provided configuration.
// ProvisionResource blocks until the execution is complete.
// If the returned diagnostics contain any errors, the resource will be
// left in a tainted state.
func (p *MockProvisionerInterface) ProvisionResource(req provisioners.ProvisionResourceRequest) provisioners.ProvisionResourceResponse {
	return provisioners.ProvisionResourceResponse{}
}

// Stop is called to interrupt the provisioner.
//
// Stop should not block waiting for in-flight actions to complete. It
// should take any action it wants and return immediately acknowledging it
// has received the stop request. Terraform will not make any further API
// calls to the provisioner after Stop is called.
//
// The error returned, if non-nil, is assumed to mean that signaling the
// stop somehow failed and that the user should expect potentially waiting
// a longer period of time.
func (p *MockProvisionerInterface) Stop() error { return nil }

// Close shuts down the plugin process if applicable.
func (p *MockProvisionerInterface) Close() error { return nil }
