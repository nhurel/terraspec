package terraspec

import (
	"fmt"
	"path"
	"path/filepath"

	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// NewContextOptions creates a new terraform.ContextOptions able to compute configs in the context of terraspec
// It returns the built ContextOption or a Diagnostics if error occured
func NewContextOptions(dir, varFile string) (*terraform.ContextOpts, tfdiags.Diagnostics) {
	absDir, err := filepath.Abs(dir)
	diags := make(tfdiags.Diagnostics, 0)
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

	opts := &terraform.ContextOpts{
		Config:           cfg,
		Parallelism:      10,
		ProviderResolver: resolver,
		Provisioners:     ProvisionersFactory(),
		Variables:        variables,
	}

	return opts, nil
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

// InjectMockedData will update the ProviderResolver set in this tfOption to populate mock data
func InjectMockedData(tfOptions *terraform.ContextOpts, mocks []*Mock) tfdiags.Diagnostics {
	var ctxDiags tfdiags.Diagnostics
	pr, ok := tfOptions.ProviderResolver.(*ProviderResolver)
	if !ok {
		ctxDiags = ctxDiags.Append(fmt.Errorf("ProviderResolver is not terraspec's implementation : %T", tfOptions.ProviderResolver))
		return ctxDiags
	}
	pr.DataSourceReader.SetMock(mocks)
	return ctxDiags
}
