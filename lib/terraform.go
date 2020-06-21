package terraspec

import (
	"path"
	"path/filepath"

	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// NewContext creates a new terraform.Context able to compute configs in the context of terraspec
// It returns the built Context or a Diagnostics if error occured
func NewContext(dir, varFile string) (*terraform.Context, tfdiags.Diagnostics) {
	absDir, err := filepath.Abs(dir)
	diags := make(tfdiags.Diagnostics, 0)
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}
	absVarFile, err := filepath.Abs(varFile)
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}
	modulesDir := path.Join(absDir, ".terraform/modules")
	resolver, err := BuildProviderResolver(absDir)

	c, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}
	cfg, _ := c.LoadConfig(absDir)

	values, hclDiags := c.Parser().LoadValuesFile(absVarFile)
	if hclDiags.HasErrors() {
		diags = diags.Append(hclDiags)
		return nil, diags
	}

	variables := InputValuesFromType(values, terraform.ValueFromNamedFile)

	opts := &terraform.ContextOpts{
		Config:           cfg,
		Parallelism:      10,
		ProviderResolver: resolver,
		Provisioners:     ProvisionersFactory(),
		Variables:        variables,
	}

	return terraform.NewContext(opts)
}

// InputValuesFromType converts a map of values file into InputValues with the given SourceType
func InputValuesFromType(values map[string]cty.Value, sourceType terraform.ValueSourceType) terraform.InputValues {
	vals := make(terraform.InputValues, len(values))
	for k, v := range values {
		vals[k] = &terraform.InputValue{
			Value:      v,
			SourceType: sourceType,
		}
	}
	return vals
}

// type ContextComponentFactory struct {
// 	provisionersFactory map[string]provisioners.Factory
// 	providersFactory    map[string]providers.Factory
// }

// func (c *ContextComponentFactory) ResourceProvider(typ, uid string) (providers.Interface, error) {
// 	if p, ok := c.providersFactory[typ]; ok {
// 		return p()
// 	}
// 	return nil, fmt.Errorf("No provider factory fond for type %s", typ)
// }
// func (c *ContextComponentFactory) ResourceProviders() []string {
// 	p := make([]string, 0, len(c.providersFactory))
// 	for k := range c.providersFactory {
// 		p = append(p, k)
// 	}
// 	return p
// }

// func (c *ContextComponentFactory) ResourceProvisioner(typ, uid string) (provisioners.Interface, error) {
// 	if p, ok := c.provisionersFactory[typ]; ok {
// 		return p()
// 	}
// 	return nil, fmt.Errorf("No provisioner factory fond for type %s", typ)
// }
// func (c *ContextComponentFactory) ResourceProvisioners() []string {
// 	p := make([]string, 0, len(c.provisionersFactory))
// 	for k := range c.provisionersFactory {
// 		p = append(p, k)
// 	}
// 	return p
// }

// func BuildContextComponentFactory(dir string) (*ContextComponentFactory, tfdiags.Diagnostics) {
// 	absDir, err := filepath.Abs(dir)
// 	diags := make(tfdiags.Diagnostics, 0)
// 	if err != nil {
// 		diags = diags.Append(err)
// 		return nil, diags
// 	}

// 	providersResolver, err := BuildProviderResolver(absDir)
// 	if err != nil {
// 		diags = diags.Append(err)
// 		return nil, diags
// 	}
// 	providerFactories := make(map[string]providers.Factory, 0)
// 	for k, v := range providersResolver.KnownPlugins {
// 		providerFactories[k.Type] = buildFactory(v)
// 	}

// 	return &ContextComponentFactory{
// 		provisionersFactory: ProvisionersFactory(),
// 		providersFactory:    providerFactories,
// 	}, nil
// }
