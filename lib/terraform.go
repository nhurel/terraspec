package terraspec

import (
	"path"
	"path/filepath"

	"github.com/hashicorp/terraform/configs"
	"github.com/hashicorp/terraform/configs/configload"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/hashicorp/terraform/version"
	"github.com/zclconf/go-cty/cty"

	goversion "github.com/hashicorp/go-version"
)

// NewContext creates a new terraform.Context able to compute configs in the context of terraspec
// It returns the built Context or a Diagnostics if error occured
func NewContext(dir, varFile string, resolver *ProviderResolver, tsCtx *Context, workspace string) (*terraform.Context, tfdiags.Diagnostics) {
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
	tsCtx.WorkaroundOnce.Do(func() { workaroundVersionCheck(cfg, tsCtx.UserVersion) })

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

	providers := resolver.ResolveProviders()

	opts := &terraform.ContextOpts{
		Config:       cfg,
		Parallelism:  10,
		Providers:    providers,
		Provisioners: ProvisionersFactory(),
		Variables:    variables,
		Meta: &terraform.ContextMeta{
			Env: workspace,
		},
	}

	return terraform.NewContext(opts)
}

func workaroundVersionCheck(cfg *configs.Config, userVersion *goversion.Version) {
	if userVersion == nil {
		return
	}
	diags := terraform.CheckCoreVersionRequirements(cfg)
	if !diags.HasErrors() {
		return
	}
	version.SemVer = userVersion
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

// LookupProviderSchema searches for the schema matching the given type in the collection of known schemas
func LookupProviderSchema(schemas *terraform.Schemas, providerType string) *terraform.ProviderSchema {
	for k, v := range schemas.Providers {
		if k.Type == providerType {
			return v
		}
	}
	return nil
}
