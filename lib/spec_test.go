package terraspec

import (
	"testing"

	"github.com/hashicorp/terraform/configs/configschema"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

func TestParsing(t *testing.T) {
	schemas := &terraform.Schemas{
		Providers: map[string]*terraform.ProviderSchema{
			"ressource": {
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
			"data": {
				DataSources: map[string]*configschema.Block{
					"data_type": {
						Attributes: map[string]*configschema.Attribute{
							"query": {Type: cty.Number},
							"id":    {Type: cty.Number},
							"name":  {Type: cty.String},
						},
					},
				},
			},
		},
	}
	spec, diags := ReadSpec("testdata/scenario.tfspec", schemas)
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

	if nb := len(spec.Mocks); nb != 1 {
		t.Errorf("Spec should have 1 mock. Got %d", nb)
	} else {
		mock := spec.Mocks[0]
		if mock.Type != "data_type" {
			t.Errorf("refute type is wrong. Got %s", mock.Type)
		}
		if mock.Name != "name" {
			t.Errorf("refute name is wrong. Got %s", mock.Name)
		}
		expectedData := cty.ObjectVal(
			map[string]cty.Value{
				"id":    cty.NumberIntVal(12345),
				"name":  cty.StringVal("fetched_data"),
				"query": cty.NumberIntVal(12345),
			},
		)
		if !mock.Data.RawEquals(expectedData) {
			t.Errorf("mock.Data not as expected. \nGot %s\nWant %s", mock.Data.GoString(), expectedData.GoString())
		}
		expectedQuery := cty.ObjectVal(
			map[string]cty.Value{
				"query": cty.NumberIntVal(12345),
				"id":    cty.NullVal(cty.Number),
				"name":  cty.NullVal(cty.String),
			},
		)
		if !mock.Query.RawEquals(expectedQuery) {
			t.Errorf("mock.Query not as expected. \nGot %s\nWant %s", mock.Query.GoString(), expectedQuery.GoString())
		}
	}
}

func TestResourceType(t *testing.T) {
	tests := map[string]struct {
		given    string
		expected string
	}{
		"typeOnly":        {given: "resource", expected: "resource"},
		"module.resource": {given: "module.resource", expected: "resource"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := resourceType(tt.given); got != tt.expected {
				t.Errorf("Error : Got %s - Expected %s", got, tt.expected)
			}
		})
	}
}

func TestFindAttribute(t *testing.T) {
	object := cty.ObjectVal(map[string]cty.Value{
		"id":    cty.StringVal("valueId"),
		"name":  cty.StringVal("valueName"),
		"count": cty.NumberIntVal(2),
	})
	stringObject := cty.StringVal("no-attribute")
	deepObject := cty.ObjectVal(map[string]cty.Value{
		"id":   cty.StringVal("rootId"),
		"name": cty.StringVal("rootName"),
		"value": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("innerName"),
		}),
	})

	if got := findAttribute(cty.StringVal("unknown"), object); got != cty.NilVal {
		t.Errorf("findAttribute(unknown) should return NilVal. Got %v", got)
	}
	if got := findAttribute(cty.StringVal("unknown"), stringObject); got != cty.NilVal {
		t.Errorf("findAttribute(unknown) should return NilVal. Got %v", got)
	}

	if got := findAttribute(cty.StringVal("id"), object); got.Equals(cty.StringVal("valueId")).False() {
		t.Errorf("findAttribute(name) should return valueId. Got %v", got)
	}
	if got := findAttribute(cty.StringVal("name"), object); got.Equals(cty.StringVal("valueName")).False() {
		t.Errorf("findAttribute(name) should return valueName. Got %v", got)
	}
	if got := findAttribute(cty.StringVal("count"), object); got.Equals(cty.NumberIntVal(2)).False() {
		t.Errorf("findAttribute(count) should return 2. Got %v", got.GoString())
	}

	if got := findAttribute(cty.StringVal("name"), deepObject); got.Equals(cty.StringVal("rootName")).False() {
		t.Errorf("findAttribute(name) should return rootName. Got %v", got)
	}
}

func TestCheckAssert(t *testing.T) {
	exampleVal := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("a"),
	})

	expectedSet := cty.NewValueSet(exampleVal.Type())
	expectedSet.Add(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("a"),
	}))
	expectedSet.Add(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("x"),
	}))
	expectedSet.Add(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("y"),
	}))
	expected := cty.ObjectVal(map[string]cty.Value{
		"name":   cty.StringVal("test-name"),
		"region": cty.StringVal("wrong"),
		"block": cty.ObjectVal(map[string]cty.Value{
			"count": cty.NumberIntVal(2),
			"sub-block": cty.ObjectVal(map[string]cty.Value{
				"enable": cty.BoolVal(true),
				"delete": cty.BoolVal(false),
			}),
		}),
		"tags": cty.MapVal(map[string]cty.Value{
			"Name":          cty.StringVal("test-name"),
			"Wrong-Value":   cty.StringVal("wrong-tag"),
			"Missing-Value": cty.StringVal("missing-tag"),
		}),
		"multi-value": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"ipaddress": cty.StringVal("10.0.0.1"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"ipaddress": cty.StringVal("10.0.0.2"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"ipaddress": cty.StringVal("10.0.0.3"),
			}),
		}),
		"set": cty.SetValFromValueSet(expectedSet),
	})

	gotSet := cty.NewValueSet(exampleVal.Type())
	gotSet.Add(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("a"),
	}))
	gotSet.Add(cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("z"),
	}))
	got := cty.ObjectVal(map[string]cty.Value{
		"name":   cty.StringVal("test-name"),
		"region": cty.StringVal("right"),
		"block": cty.ObjectVal(map[string]cty.Value{
			"count":          cty.NumberIntVal(3),
			"additionnalKey": cty.StringVal("ignore-me"),
			"sub-block": cty.ObjectVal(map[string]cty.Value{
				"enable": cty.BoolVal(true),
				"delete": cty.BoolVal(true),
			}),
		}),
		"tags": cty.MapVal(map[string]cty.Value{
			"Name":              cty.StringVal("test-name"),
			"Wrong-Value":       cty.StringVal("right-tag"),
			"Additionnal-Value": cty.StringVal("ignore-me"),
		}),
		"multi-value": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"ipaddress": cty.StringVal("10.0.0.1"),
			}),
			cty.ObjectVal(map[string]cty.Value{
				"ipaddress": cty.StringVal("10.10.0.1"),
			}),
		}),
		"set": cty.SetValFromValueSet(gotSet),
	})

	rootPath := cty.Path{}.GetAttr("test")
	var expectedResult tfdiags.Diagnostics
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("block").GetAttr("count"), 2, 3))
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("block").GetAttr("sub-block").GetAttr("delete"), false, true))
	expectedResult = expectedResult.Append(SuccessDiags(rootPath.GetAttr("block").GetAttr("sub-block").GetAttr("enable"), true))
	expectedResult = expectedResult.Append(SuccessDiags(rootPath.GetAttr("multi-value").Index(cty.NumberIntVal(0)).GetAttr("ipaddress"), "10.0.0.1"))
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("multi-value").Index(cty.NumberIntVal(1)).GetAttr("ipaddress"), "10.0.0.2", "10.10.0.1"))
	expectedResult = expectedResult.Append(ErrorDiags(rootPath.GetAttr("multi-value").Index(cty.NumberIntVal(2)), "Could not find child at index 2"))
	expectedResult = expectedResult.Append(SuccessDiags(rootPath.GetAttr("name"), "test-name"))
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("region"), "wrong", "right"))

	expectedResult = expectedResult.Append(SuccessDiags(rootPath.GetAttr("set").Index(cty.NumberIntVal(0)).GetAttr("name"), "a"))
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("set").Index(cty.NumberIntVal(1)).GetAttr("name"), "x", "z"))
	expectedResult = expectedResult.Append(ErrorDiags(rootPath.GetAttr("set").Index(cty.NumberIntVal(2)), "Could not find child at index 2"))

	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("tags").GetAttr("Missing-Value"), "missing-tag", nil))
	expectedResult = expectedResult.Append(SuccessDiags(rootPath.GetAttr("tags").GetAttr("Name"), "test-name"))
	expectedResult = expectedResult.Append(AssertErrorDiags(rootPath.GetAttr("tags").GetAttr("Wrong-Value"), "wrong-tag", "right-tag"))

	result := checkAssert(rootPath, expected, got)

	if !result.HasErrors() {
		t.Fatalf("checkAssert didn't return any errors. Got %+v", result)
	}

	for i, diag := range expectedResult {
		if i >= len(result) {
			t.Errorf("Missing diag#%d : %s", i, diag.Description().Detail)
			continue
		}
		if result[i].Description().Detail != diag.Description().Detail {
			t.Errorf("Wrong Details for diag#%d Got %+v want %+v", i, result[i].Description().Detail, diag.Description().Detail)
		}
		if result[i].Severity() != diag.Severity() {
			t.Errorf("Diagnostic#%d has wrong severity. Got [%c] - Want [%c]", i, result[i].Severity(), diag.Severity())
		}
		if d, ok := diag.(*TerraspecDiagnostic); ok {
			if r, ok := result[i].(*TerraspecDiagnostic); ok {
				e := tfdiags.GetAttribute(d.Diagnostic)
				g := tfdiags.GetAttribute(r.Diagnostic)
				if !e.Equals(g) {
					t.Errorf("Diagnostic#%d has wrong attribute. Got %v - Want %v", i, g, e)
				}
			} else {
				t.Errorf("diag #%d is not a TerraspecDiagnostic. Got %T", i, result[i])
			}
		}
	}
	for i, diag := range result {
		if i >= len(expectedResult) {
			t.Errorf("Unexpected diag#%d : [%c] %s", i, diag.Severity(), diag.Description().Detail)
		}
	}

}

func TestCheckOutput(t *testing.T) {

	var path = cty.Path{}.GetAttr("test").GetAttr("output_value")
	var tests = map[string]struct {
		given    cty.Value
		expected *TerraspecDiagnostic
	}{
		"goodOutput": {
			given: cty.ObjectVal(map[string]cty.Value{
				"value": cty.StringVal("good-result"),
			}),
			expected: SuccessDiags(path, "good-result"),
		},
		"wrongOutput": {
			given: cty.ObjectVal(map[string]cty.Value{
				"value": cty.StringVal("wrong-result"),
			}),
			expected: AssertErrorDiags(path, "wrong-result", "good-result"),
		},
		"badOutput": {
			given: cty.ObjectVal(map[string]cty.Value{
				"novalue": cty.StringVal("no value !"),
			}),
			expected: ErrorDiags(path, "Bad Assertion : Assertion on outputs should have a value parameter"),
		},
	}

	var output = cty.TupleVal([]cty.Value{cty.StringVal("good-result")})

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			result := checkOutput(path, tt.given, output)
			if nb := len(result); nb != 1 {
				t.Errorf("checkOutput should return only 1 diagsnostic, got %d", nb)
				if nb == 0 {
					return
				}
			}
			if result[0].Description().Detail != tt.expected.Description().Detail {
				t.Errorf("Wrong Details. Got %+v want %+v", result[0].Description().Detail, tt.expected.Description().Detail)
			}
			if result[0].Severity() != tt.expected.Severity() {
				t.Errorf("Wrong severity. Got [%c] - Want [%c]", result[0].Severity(), tt.expected.Severity())
			}
			if r, ok := result[0].(*TerraspecDiagnostic); ok {
				e := tfdiags.GetAttribute(tt.expected.Diagnostic)
				g := tfdiags.GetAttribute(r.Diagnostic)
				if !e.Equals(g) {
					t.Errorf("Wrong attribute. Got %v - Want %v", g, e)
				}
			} else {
				t.Errorf("checkOutput diagnostic is not a TerraspecDiagnostic. Got %T", result[0])
			}

		})
	}

}
