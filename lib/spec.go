package terraspec

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"

	goversion "github.com/hashicorp/go-version"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/hcl/v2/hclparse"
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
	Asserts          []*Assert
	Refutes          []*Assert
	Mocks            []*Mock
	DataSourceReader *MockDataSourceReader
	Terraspec        *TerraspecConfig 
}

// Terraspec contains a global element for a spec with common configuration similar to terraform hcl element.
type TerraspecConfig struct {
	Workspace string
}

// Assert struct contains the definition of an assertion
type Assert struct {
	Type  string
	Name  string
	Value cty.Value
}

// Mock struct contains the definition of mocked data resources
type Mock struct {
	Type  string
	Name  string
	Query cty.Value
	Data  cty.Value
	Body  []byte
	calls int
}

// Context struct holds terraspec options and internal state
type Context struct {
	TerraformVersion *goversion.Version
	UserVersion      *goversion.Version
	WorkaroundOnce   sync.Once
}

// Key return fully qualified name of an Assert
func (a *Assert) Key() string {
	return fmt.Sprintf("%s.%s", a.Type, a.Name)
}

// Call marks the mock as called and returns its data
func (m *Mock) Call() cty.Value {
	m.calls++
	return m.Data
}

// Called indicates if mock was called at least once
func (m *Mock) Called() bool {
	return m.calls > 0
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
			path := cty.GetAttrPath("output").GetAttr(assert.Key())
			if output == nil {
				diags = diags.Append(ErrorDiags(path, "Missing value"))
				continue
			}
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

			change, err := resource.After.Decode(assert.Value.Type())
			if err != nil {
				return nil, fmt.Errorf("Error happened while decoding planned resource %s : %v", assert.Name, err)
			}

			assertDiags := checkAssert(cty.GetAttrPath(assert.Key()), assert.Value, change)
			diags = diags.Append(assertDiags)
		}
	}

	return diags, nil
}

// ValidateMocks checks all mocks were called as expected
func (s *Spec) ValidateMocks() tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	var allMissedCalls string
	for _, mock := range s.Mocks {
		if !mock.Called() {
			if allMissedCalls == "" {
				var sb strings.Builder
				for _, call := range s.DataSourceReader.UnmatchedCalls() {
					sb.Write(MarshalValue(call))
					sb.WriteString("\n")
				}
				allMissedCalls = sb.String()
			}
			diags = diags.Append(ErrorDiags(cty.GetAttrPath(mock.Type).GetAttr(mock.Name), fmt.Sprintf("No data resource matched :\n%s\nUncatched data source calls are :\n%s", string(mock.Body), allMissedCalls)))
		} else {
			diags = diags.Append(SuccessDiags(cty.GetAttrPath(mock.Type).GetAttr(mock.Name), fmt.Sprintf("mock has been called %d time(s)", mock.calls)))
		}
	}
	return diags
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
		childIndex := 0
		for it.Next() {
			key, value := it.Element()
			if IsNull(value) {
				continue //skip attributes with no spec
			}
			if key.Type() == cty.String {
				// Looping over object properties or a map
				g := findAttribute(key, got)
				diags = diags.Append(checkAssert(path.GetAttr(key.AsString()), value, g))
			} else {
				// looping over a set or an array:
				if gt.Next() {
					_, g := gt.Element()
					diags = diags.Append(checkAssert(path.Index(cty.NumberIntVal(int64(childIndex))), value, g))
				} else {
					diags = diags.Append(ErrorDiags(path.Index(cty.NumberIntVal(int64(childIndex))), fmt.Sprintf("Could not find child at index %d", childIndex)))
				}
			}
			childIndex++
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
	type terraspec struct {
		Body hcl.Body       `hcl:",remain"`
	}
	type assert struct {
		Type      string         `hcl:"type,label"`
		Name      string         `hcl:"name,label"`
		Config    hcl.Body       `hcl:",remain"`
		DependsOn hcl.Expression `hcl:"depends_on,attr"`
	}
	type mock struct {
		Type   string   `hcl:"type,label"`
		Name   string   `hcl:"name,label"`
		Config hcl.Body `hcl:",remain"`
	}
	type root struct {
		Asserts []*assert `hcl:"assert,block"`
		Refutes []*assert `hcl:"refute,block"`
		Mocks   []*mock   `hcl:"mock,block"`
		// Modules   []*Module   `hcl:"module,block"`
		Terraspec *terraspec `hcl:"terraspec,block"`
	}

	var r root
	parsed := &Spec{}
	file, diags := hclparse.NewParser().ParseHCL(spec, filename)
	ctx := &hcl.EvalContext{
		Variables: make(map[string]cty.Value),
	}

	if diags.HasErrors() {
		return nil, diags
	}
	diags = gohcl.DecodeBody(file.Body, nil, &r)
	if diags.HasErrors() {
		return nil, diags
	}
	
	if r.Terraspec != nil && r.Terraspec.Body != nil {
		terraspecConfig, diags := decodeTerraspecConfig(&r.Terraspec.Body, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Terraspec = terraspecConfig
	} else {
		parsed.Terraspec = &TerraspecConfig{}
	}	

	for _, assert := range r.Asserts {
		val, diags := decodeBody(&assert.Config, assert.Type, schemas, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Asserts = append(parsed.Asserts, &Assert{Name: assert.Name, Type: assert.Type, Value: val})
	}

	for _, assert := range r.Refutes {
		val, diags := decodeBody(&assert.Config, assert.Type, schemas, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Refutes = append(parsed.Refutes, &Assert{Name: assert.Name, Type: assert.Type, Value: val})
	}
	for _, mock := range r.Mocks {
		query, mocked, diags := decodeMockBody(mock.Config, mock.Type, schemas, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		var body []byte
		if r, ok := mock.Config.(*hclsyntax.Body); ok {
			body = r.Range().SliceBytes(file.Bytes)
		}
		parsed.Mocks = append(parsed.Mocks, &Mock{Name: mock.Name, Type: mock.Type, Query: query, Data: mocked, Body: body})
	}

	return parsed, diags
}

func decodeTerraspecConfig(body *hcl.Body, ctx *hcl.EvalContext) (*TerraspecConfig, hcl.Diagnostics) {
	spec := hcldec.ObjectSpec{
		"workspace": &hcldec.AttrSpec{
			Name:     "workspace",
			Type:     cty.String,
			Required: false,
		},
	}

	val, diags := hcldec.Decode(*body, spec, nil)
	if diags.HasErrors() {
		return nil, diags
	}

	workspaceName := ""
	if !val.IsNull() {
		workspace := val.GetAttr("workspace")
		ctx.Variables["terraspec"] = val
		workspaceName = workspace.AsString()
	}

	return &TerraspecConfig{
		Workspace: workspaceName,
	}, nil
}

func decodeBody(body *hcl.Body, bodyType string, schemas *terraform.Schemas, ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
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
		schema := laxSchema(LookupProviderSchema(schemas, provName))
		partialSchema, _ = schema.SchemaForResourceType(addrs.ManagedResourceMode, rawType)
	}

	val, diags := hcldec.Decode(*body, partialSchema.DecoderSpec(), ctx)
	return val, diags
}

func decodeMockBody(body hcl.Body, bodyType string, schemas *terraform.Schemas, ctx *hcl.EvalContext) (query, mock cty.Value, diags hcl.Diagnostics) {
	var codedMock hcl.Body
	provName := strings.Split(bodyType, "_")[0]
	schema := LookupProviderSchema(schemas, provName)
	partialSchema, _ := schema.SchemaForResourceType(addrs.DataResourceMode, bodyType)

	query, codedMock, diags = hcldec.PartialDecode(body, partialSchema.DecoderSpec(), ctx)
	if diags.HasErrors() {
		return
	}
	mockedSchema := toMockSchema(partialSchema)
	mock, moreDiags := hcldec.Decode(codedMock, mockedSchema.DecoderSpec(), ctx)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return
	}
	mock = mock.GetAttr("return")

	mock, err := cty.Transform(mock, func(path cty.Path, value cty.Value) (cty.Value, error) {
		if value.IsNull() {
			return path.Apply(query)
		}
		return value, nil
	})
	if err != nil {
		diags = diags.Append(&hcl.Diagnostic{Severity: hcl.DiagError, Detail: err.Error()})
	}

	return
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
func toMockSchema(schema *configschema.Block) *configschema.Block {
	laxed := schema.NoneRequired()
	mocked := &configschema.Block{
		BlockTypes: map[string]*configschema.NestedBlock{
			"return": {
				Block:   *laxed,
				Nesting: configschema.NestingSingle,
			},
		},
	}

	return mocked
}
