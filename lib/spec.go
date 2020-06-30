package terraspec

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform/addrs"
	"github.com/hashicorp/terraform/configs/configschema"
	"github.com/hashicorp/terraform/plans"
	"github.com/hashicorp/terraform/terraform"
	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Spec struct contains the assertions described in .tfspec file
type Spec struct {
	Asserts []*Assert
	Refutes []*Assert
}

// Assert struct contains the definition of an assertion
type Assert struct {
	Type  string
	Name  string
	Value cty.Value
}

// Key return fully qualified name of an Assert
func (a *Assert) Key() string {
	return fmt.Sprintf("%s.%s", a.Type, a.Name)
}

// Validate checks all the assertions of this Spec against the given terraform Plan.
// It return all failed assertion in a Diagnostics and an error
// if a technical error happened while testing the plan
func (s *Spec) Validate(plan *plans.Plan) (tfdiags.Diagnostics, error) {
	var diags tfdiags.Diagnostics

	if plan.Changes == nil {
		diags = diags.Append(errors.New("plan has no changes"))
		return diags, nil
	}

	for _, assert := range s.Asserts {
		if assert.Type == "output" {
			output := findOuput(assert.Key(), plan.Changes.Outputs)
			path := cty.Path{}.GetAttr("output").GetAttr(assert.Key())
			if output == nil {
				diags = diags.Append(ErrorDiags(path, "Missing value"))
				continue
			}
			// checkValue(assert.Key(), assert.Value, output.Addr.OutputValue)
			change, err := output.Decode()
			if err != nil {
				return nil, fmt.Errorf("Error happened while decoding planned output %s : %v", assert.Name, err)
			}

			assertDiags := checkOutput(path, assert.Value, change.Change.After)
			diags = diags.Append(assertDiags)
		} else {
			resource := findResource(assert.Key(), plan.Changes.Resources)
			if resource == nil {
				diags = diags.Append(fmt.Errorf("Could not find resource %s in changes", assert.Key()))
				continue
			}

			iType, err := resource.After.ImpliedType()
			if err != nil {
				return nil, fmt.Errorf("Error happened while decoding planned resource %s : %v", assert.Name, err)
			}
			change, err := resource.After.Decode(iType)
			if err != nil {
				return nil, fmt.Errorf("Error happened while decoding planned resource %s : %v", assert.Name, err)
			}
			assertDiags := checkAssert(cty.Path{}.GetAttr(assert.Key()), assert.Value, change)
			diags = diags.Append(assertDiags)
		}
	}

	return diags, nil
}

func findOuput(name string, outputs []*plans.OutputChangeSrc) *plans.OutputChangeSrc {
	for _, output := range outputs {
		if name == output.Addr.String() {
			return output
		}
	}
	return nil
}
func findResource(name string, resources []*plans.ResourceInstanceChangeSrc) *plans.ResourceInstanceChangeSrc {
	for _, resource := range resources {
		if name == resource.Addr.String() {
			return resource
		}
	}
	return nil
}

func findAttribute(key, value cty.Value) cty.Value {
	if value.CanIterateElements() {
		it := value.ElementIterator()
		for it.Next() {
			k, val := it.Element()
			if k.AsString() == key.AsString() {
				return val
			}
		}
	}
	return cty.NilVal
}

func checkAssert(path cty.Path, expected, got cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if expected.Type().IsPrimitiveType() {
		if !got.IsKnown() || !expected.Equals(got).True() {
			diags = diags.Append(AssertErrorDiags(path, PrimitiveValue(expected), PrimitiveValue(got)))
		} else {
			diags = diags.Append(SuccessDiags(path, PrimitiveValue(got)))
		}
		return diags
	}
	if expected.CanIterateElements() {
		if !got.CanIterateElements() {
			diags = diags.Append(ErrorDiags(path, "Element don't have multiple properties"))
			return diags
		}

		it := expected.ElementIterator()
		gt := got.ElementIterator()
		childNb := 0
		for it.Next() {
			key, value := it.Element()
			if IsNull(value) {
				continue //skip attributes with no spec
			}

			if key.Type() == cty.Number {
				// looping over an array rather than a map of properties
				if gt.Next() {
					_, g := gt.Element()
					diags = diags.Append(checkAssert(path.Index(key), value, g)) // path or path.Index ?
				} else {
					diags = diags.Append(ErrorDiags(path.Index(key), fmt.Sprintf("Could not find child at index %d", PrimitiveValue(key))))
				}
			} else if key.Type() == cty.String {
				g := findAttribute(key, got)
				diags = diags.Append(checkAssert(path.GetAttr(key.AsString()), value, g))
			} else if key.CanIterateElements() {
				// looping over a set:
				if gt.Next() {
					_, g := gt.Element()
					diags = diags.Append(checkAssert(path.Index(cty.NumberIntVal(int64(childNb))), value, g))
				} else {
					diags = diags.Append(ErrorDiags(path.Index(cty.NumberIntVal(int64(childNb))), fmt.Sprintf("Could not find child at index %d", childNb)))
				}
			}
			childNb++
		}
		return diags
	}

	return diags
}

func checkOutput(path cty.Path, expected, got cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if !got.CanIterateElements() {
		diags = diags.Append(ErrorDiags(path, "Cannot parse planned output"))
		return diags
	}
	it := got.ElementIterator()
	if !it.Next() {
		diags = diags.Append(ErrorDiags(path, "Planned output is empty"))
		return diags
	}
	_, value := it.Element()

	exp := findAttribute(cty.StringVal("value"), expected)
	if exp.IsNull() {
		//should never happen
		diags = diags.Append(ErrorDiags(path, "Bad Assertion : Assertion on outputs should have a value parameter"))
		return diags
	}
	return checkAssert(path, exp, value)

}

// ReadSpec reads the .tfspec file and returns the resulting Spec or a Diagnostics if error occured in the process
func ReadSpec(filename string, schemas *terraform.Schemas) (*Spec, tfdiags.Diagnostics) {
	spec, err := ioutil.ReadFile(filename)
	var tfdiags tfdiags.Diagnostics
	if err != nil {
		return nil, tfdiags.Append(&hcl.Diagnostic{Severity: hcl.DiagError, Detail: err.Error(), Summary: "Failed to read file"})
	}

	s, diags := ParseSpec(spec, filename, schemas)
	return s, tfdiags.Append(diags)

}

// ParseSpec parses the spec contained in the []byte parameter and returns the resulting Spec or a Diagnostics if error occured in the process
func ParseSpec(spec []byte, filename string, schemas *terraform.Schemas) (*Spec, hcl.Diagnostics) {
	type assert struct {
		Type      string         `hcl:"type,label"`
		Name      string         `hcl:"name,label"`
		Config    hcl.Body       `hcl:",remain"`
		DependsOn hcl.Expression `hcl:"depends_on,attr"`
	}
	type root struct {
		Asserts []*assert `hcl:"assert,block"`
		Refutes []*assert `hcl:"refute,block"`
		// Modules   []*Module   `hcl:"module,block"`
	}

	var r root
	parsed := &Spec{}
	file, diags := hclsyntax.ParseConfig(
		spec,
		filename, hcl.Pos{Line: 1, Column: 1},
	)
	if diags.HasErrors() {
		return nil, diags
	}
	diags = gohcl.DecodeBody(file.Body, nil, &r)
	if diags.HasErrors() {
		return nil, diags
	}

	for _, assert := range r.Asserts {
		val, diags := decodeBody(&assert.Config, assert.Type, schemas)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Asserts = append(parsed.Asserts, &Assert{Name: assert.Name, Type: assert.Type, Value: val})
	}

	for _, assert := range r.Refutes {
		val, diags := decodeBody(&assert.Config, assert.Type, schemas)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Refutes = append(parsed.Refutes, &Assert{Name: assert.Name, Type: assert.Type, Value: val})
	}

	return parsed, diags
}

func decodeBody(body *hcl.Body, bodyType string, schemas *terraform.Schemas) (cty.Value, hcl.Diagnostics) {
	rawType := resourceType(bodyType)
	provName := strings.Split(rawType, "_")[0]
	var val cty.Value
	var partialSchema *configschema.Block
	if provName == "output" {
		partialSchema = &configschema.Block{
			Attributes: map[string]*configschema.Attribute{
				"value": {Type: cty.String, Computed: false},
			},
		}
	} else {
		schema := laxSchema(schemas.ProviderSchema(provName))
		partialSchema, _ = schema.SchemaForResourceType(addrs.ManagedResourceMode, rawType)
	}

	val, diags := hcldec.Decode(*body, partialSchema.DecoderSpec(), nil)
	return val, diags
}

// Extract the resource type from a fully qualified resource name, eg module.name.resourceType
func resourceType(fullName string) string {
	parts := strings.Split(fullName, ".")
	return parts[len(parts)-1]
}

// laxSchema returns a schema with all resource types and their properties defined as optional
func laxSchema(schema *terraform.ProviderSchema) *terraform.ProviderSchema {
	laxed := &terraform.ProviderSchema{ResourceTypes: make(map[string]*configschema.Block, len(schema.ResourceTypes))}
	for k, rt := range schema.ResourceTypes {
		laxed.ResourceTypes[k] = rt.NoneRequired()
	}
	return laxed
}
