package terraspec

import (
	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/providers"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
	"log"
	"path"
	"path/filepath"
)

// NewContext creates a new terraform.Context able to compute configs in the context of terraspec
// It returns the built Context or a Diagnostics if error occured
func NewContext(dir, varFile string, resolver ProviderResolver, mockMetadata *MockMetadata) (*terraform.Context, tfdiags.Diagnostics) {
	absDir, err := filepath.Abs(dir)
	diags := make(tfdiags.Diagnostics, 0)
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}

	modulesDir := path.Join(absDir, ".terraform/modules")

	c, err := configload.NewLoader(&configload.Config{
		ModulesDir: modulesDir,
	})
	if err != nil {
		diags = diags.Append(err)
		return nil, diags
	}
	cfg, hclDiag := c.LoadConfig(absDir)
	if hclDiag.HasErrors() {
		diags = diags.Append(hclDiag)
		return nil, diags
	}

	var variables terraform.InputValues
	if varFile != "" {
		absVarFile, err := filepath.Abs(varFile)
		if err != nil {
			diags = diags.Append(err)
			return nil, diags
		}
		values, hclDiags := c.Parser().LoadValuesFile(absVarFile)
		if hclDiags.HasErrors() {
			diags = diags.Append(hclDiags)
			return nil, diags
		}

		variables = InputValuesFromType(values, terraform.ValueFromNamedFile)
	}

	// Bind available provider plugins to the constraints in config
	var providerFactories map[addrs.Provider]providers.Factory
	log.Printf("[TRACE] terraform.NewContext: resolving provider version selections")
	var providerDiags tfdiags.Diagnostics
	providerFactories, providerDiags = resourceProviderFactories(resolver)
	diags = diags.Append(providerDiags)

	if diags.HasErrors() {
		return nil, diags
	}

	opts := &terraform.ContextOpts{
		Meta: &terraform.ContextMeta{
			Env: mockMetadata.Workspace,
		},
		Config:           cfg,
		Parallelism:      10,
		Providers: providerFactories,
		Provisioners:     ProvisionersFactory(),
		Variables:        variables,
	}

	return terraform.NewContext(opts)
}

func resourceProviderFactories(resolver ProviderResolver) (map[addrs.Provider]providers.Factory, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics
	ret, errs := resolver.ResolveProviders()
	if errs != nil {
		diags = diags.Append(
			tfdiags.Sourceless(tfdiags.Error,
				"Could not satisfy plugin requirements",
				"Plugin reinitialization required. Please run \"terraform init -backend=false\".",
			),
		)

		for _, err := range errs {
			diags = diags.Append(err)
		}

		return nil, diags
	}

	return ret, nil
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
