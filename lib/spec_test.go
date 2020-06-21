package terraspec_test

import (
	"testing"

	"github.com/hashicorp/terraform/configs/configschema"
	"github.com/hashicorp/terraform/terraform"
	terraspec "github.com/nhurel/terraspec/lib"
	"github.com/zclconf/go-cty/cty"
)

func TestParsing(t *testing.T) {
	schemas := &terraform.Schemas{
		Providers: map[string]*terraform.ProviderSchema{
			"ressource": &terraform.ProviderSchema{
				ResourceTypes: map[string]*configschema.Block{
					"ressource_type": {
						Attributes: map[string]*configschema.Attribute{
							"property": {Type: cty.String},
						},

						BlockTypes: map[string]*configschema.NestedBlock{
							"inner": {
								Block: configschema.Block{
									Attributes: map[string]*configschema.Attribute{
										"inner_prop": {Type: cty.String},
									},
								},
								Nesting: configschema.NestingSingle,
							},
						},
					},
				},
				ResourceTypeSchemaVersions: map[string]uint64{
					"ressource_type": 0,
				},
			},
		},
	}
	spec, diags := terraspec.ReadSpec("testdata/scenario.tfspec", schemas)
	if diags.HasErrors() {
		t.Fatal(diags.Err())
	}

	if spec == nil {
		t.Fatal("spec is nil")
	}

	if nb := len(spec.Asserts); nb != 1 {
		t.Errorf("spec should have 1 assert, got %d", nb)
	} else {
		assert := spec.Asserts[0]
		if assert.Type != "ressource_type" {
			t.Errorf("assert type is wrong. Got %s", assert.Type)
		}
		if assert.Name != "name" {
			t.Errorf("assert name is wrong. Got %s", assert.Name)
		}
		expected := cty.ObjectVal(
			map[string]cty.Value{
				"property": cty.StringVal("value"),
				"inner": cty.ObjectVal(
					map[string]cty.Value{
						"inner_prop": cty.StringVal("value2"),
					}),
			},
		)
		if !assert.Value.RawEquals(expected) {
			t.Errorf("assert.Value not as expected. \nGot %s\nWant %s", assert.Value.GoString(), expected.GoString())
		}
	}

	if nb := len(spec.Refutes); nb != 1 {
		t.Errorf("Spec should have 1 refutes. Got %d", nb)
	} else {
		refute := spec.Refutes[0]
		if refute.Type != "output" {
			t.Errorf("refute type is wrong. Got %s", refute.Type)
		}
		if refute.Name != "name" {
			t.Errorf("refute name is wrong. Got %s", refute.Name)
		}
		expected := cty.ObjectVal(map[string]cty.Value{
			"value": cty.StringVal("value"),
		})
		if !refute.Value.RawEquals(expected) {
			t.Errorf("refute.Value not as expected. \nGot %s\nWant %s", refute.Value.GoString(), expected.GoString())
		}
	}
}
