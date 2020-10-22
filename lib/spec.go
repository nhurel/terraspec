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
	Rejects          []*TypeName
	Mocks            []*Mock
	Expects          []*Expect
	DataSourceReader *MockDataSourceReader
	ResourceReader   *ExceptedResourceReader
	Terraspec        *TerraspecConfig
}

// Terraspec contains a global element for a spec with common configuration similar to terraform hcl element.
type TerraspecConfig struct {
	Workspace string
}

// Assert struct contains the definition of an assertion
type Assert struct {
	TypeName
	Value cty.Value
}

// Mock struct contains the definition of mocked data sources
type Mock struct {
	TypeName
	Query cty.Value
	Data  cty.Value
	Body  []byte
	calls int
}

// Expect struct contains the definition of mocked resources
type Expect struct {
	TypeName
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

type TypeName struct {
	Type string
	Name string
}

func NewAssert(aType, aName string, aValue cty.Value) *Assert {
	return &Assert{TypeName: TypeName{Type: aType, Name: aName}, Value: aValue}
}

func NewMock(aType, aName string, aQuery, aData cty.Value, aBody []byte) *Mock {
	return &Mock{TypeName: TypeName{Type: aType, Name: aName}, Query: aQuery, Data: aData, Body: aBody}
}

func NewExpect(aType, aName string, aQuery, aData cty.Value, aBody []byte) *Expect {
	return &Expect{TypeName: TypeName{Type: aType, Name: aName}, Query: aQuery, Data: aData, Body: aBody}
}

// Key return fully qualified name of an Assert
func (a *Assert) Key() string {
	return fmt.Sprintf("%s.%s", a.Type, a.Name)
}

// Key return fully qualified name
func (a *TypeName) Key() string {
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

// Key return fully qualified name of an Expect
func (e *Expect) Key() string {
	return fmt.Sprintf("%s.%s", e.Type, e.Name)
}

// Call marks the mock as called and returns its data
func (e *Expect) Call() cty.Value {
	e.calls++
	return e.Data
}

// Called indicates if mock was called at least once
func (e *Expect) Called() bool {
	return e.calls > 0
}

// Called indicates if mock was called at least once
func (e *Expect) Match(typeName string, config cty.Value) bool {
	return e.Type == typeName && e.Query.RawEquals(config)
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
			change, err := resource.After.Decode(untransformType(assert.Value.Type()))
			if err != nil {
				return nil, fmt.Errorf("Error happened while decoding planned resource %s : %v", assert.Name, err)
			}

			assertDiags := checkAssert(cty.GetAttrPath(assert.Key()), assert.Value, change)
			diags = diags.Append(assertDiags)
		}
	}

	for _, reject := range s.Rejects {
		fmt.Println(reject.Key())
		resource := findResource(reject.Key(), plan.Changes.Resources)
		if resource != nil {
			diags = diags.Append(RejectErrorDiags(cty.GetAttrPath(reject.Key()), reject, resource))
		} else {
			diags = diags.Append(RejectSuccessDiags(cty.GetAttrPath(reject.Key()), "Resource not created", reject))
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
			diags = diags.Append(ErrorDiags(cty.GetAttrPath(mock.Type).GetAttr(mock.Name), fmt.Sprintf("No data source matched :\n%s\nUncatched data source calls are :\n%s", string(mock.Body), allMissedCalls)))
		} else {
			diags = diags.Append(SuccessDiags(cty.GetAttrPath(mock.Type).GetAttr(mock.Name), fmt.Sprintf("mock has been called %d time(s)", mock.calls)))
		}
	}
	return diags
}


// ValidateExcepts checks all expects were called as expected
func (s *Spec) ValidateExcepts() (diags tfdiags.Diagnostics) {
	var allMissedCalls string
	for _, expect := range s.Expects {
		if !expect.Called() {
			if allMissedCalls == "" {
				var sb strings.Builder
				for _, call := range s.ResourceReader.UnmatchedCalls() {
					sb.Write(MarshalValue(call))
					sb.WriteString("\n")
				}
				allMissedCalls = sb.String()
			}
			diags = diags.Append(ErrorDiags(cty.GetAttrPath(expect.Type).GetAttr(expect.Name), fmt.Sprintf("No resource matched :\n%s %s\nUncatched resource calls are :\n%s", expect.Type, string(expect.Body), allMissedCalls)))
		} else {
			diags = diags.Append(SuccessDiags(cty.GetAttrPath(expect.Type).GetAttr(expect.Name), fmt.Sprintf("expect has been called %d time(s)", expect.calls)))
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
			if key.Type() == cty.String && key.AsString() == "reject" {
				diags = diags.Append(checkReject(path.GetAttr(key.AsString()), value, got))
				continue
			}
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

// checkAssertAmong will test assertion among all element in given ElementIterator and only return
// the diagnostict for the closest match
func checkAssertAmong(path cty.Path, expected cty.Value, got cty.ElementIterator) tfdiags.Diagnostics {
	var closestDiags, diags tfdiags.Diagnostics
	// looping over a set or an array:
	for got.Next() {
		_, g := got.Element()
		diags = checkAssert(path, expected, g)
		if closestDiags == nil {
			closestDiags = diags
		} else {
			if Compare(closestDiags, diags) > 0 {
				closestDiags = diags
			}
		}
		if !closestDiags.HasErrors() {
			break // early break as soon as there's a successDiags
		}
	}
	return closestDiags
}

func checkReject(path cty.Path, rejected, got cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if rejected.CanIterateElements() && !rejected.IsNull() {
		it := rejected.ElementIterator()
		for it.Next() {
			key, value := it.Element()
			if value.IsNull() || IsEmptyCollection(value) {
				continue
			}
			found := findAttribute(key, got)
			if IsNull(value) {
				// If value is nil, it means that the rejected property is only defined as an empty block
				if !IsNull(found) {
					errorElement := cty.ObjectVal(map[string]cty.Value{key.AsString(): found})
					diags = diags.Append(RejectErrorDiags(path.GetAttr(key.AsString()), key.AsString(), string(MarshalValue(errorElement))))
				} else {
					diags = diags.Append(RejectSuccessDiags(path.GetAttr(key.AsString()), fmt.Sprintf("No attribute matching %v", key.AsString()), value))
				}
			} else {
				if value.Type().IsListType() || value.Type().IsSetType() {
					diags = diags.Append(checkRejectCollection(path, key, value, found))
				} else {
					assertDiags := checkAssert(path.GetAttr(key.AsString()), value, found)
					if assertDiags.HasErrors() {
						//this means checkAssert is wrong, so found block doesn't match the reject block : it's a success
						diags = diags.Append(RejectSuccessDiags(path, fmt.Sprintf("No attribute matching %v definition", key.AsString()), value))
					} else {
						//this means checkAssert is correct, so found block matches the reject block : it's an error
						diags = diags.Append(RejectValueErrorDiags(path, key, value, found))
					}
				}
			}
		}
	}
	return diags
}

// checkRejectCollection will check all rejections of a colllection type
func checkRejectCollection(path cty.Path, key cty.Value, reject, found cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if reject.CanIterateElements() {
		it := reject.ElementIterator()
		for it.Next() {
			_, r := it.Element()
			var assertDiags tfdiags.Diagnostics
			if found.Type().IsSetType() || found.Type().IsListType() {
				assertDiags = checkAssertAmong(path, r, found.ElementIterator())
			}
			if assertDiags.HasErrors() {
				//this means checkAssert is wrong, so found block doesn't match the reject block : it's a success
				diags = diags.Append(RejectSuccessDiags(path, fmt.Sprintf("No attribute matching %v definition", key.AsString()), r))
			} else {
				//this means checkAssert is correct, so found block matches the reject block : it's an error
				diags = diags.Append(RejectValueErrorDiags(path, key, r, found))
			}
		}
	}
	return diags
}

func checkOutput(path cty.Path, expected, got cty.Value) tfdiags.Diagnostics {
	var diags tfdiags.Diagnostics
	if got.CanIterateElements() {
		//diags = diags.Append(ErrorDiags(path, "Cannot parse planned output"))
		//return diags
		it := got.ElementIterator()
		if !it.Next() {
			diags = diags.Append(ErrorDiags(path, "Planned output is empty"))
			return diags
		}
		_, got = it.Element()
	}

	exp := findAttribute(cty.StringVal("value"), expected)
	if exp.IsNull() {
		//should never happen
		diags = diags.Append(ErrorDiags(path, "Bad Assertion : Assertion on outputs should have a value parameter"))
		return diags
	}
	return checkAssert(path, exp, got)

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
		Body hcl.Body `hcl:",remain"`
	}
	type assert struct {
		Type      string         `hcl:"type,label"`
		Name      string         `hcl:"name,label"`
		Config    hcl.Body       `hcl:",remain"`
		DependsOn hcl.Expression `hcl:"depends_on,attr"`
	}
	type expect struct {
		Type      string         `hcl:"type,label"`
		Name      string         `hcl:"name,label"`
		Config    hcl.Body       `hcl:",remain"`
	}
	type mock struct {
		Type   string   `hcl:"type,label"`
		Name   string   `hcl:"name,label"`
		Config hcl.Body `hcl:",remain"`
	}
	type reject struct {
		Type   string   `hcl:"type,label"`
		Name   string   `hcl:"name,label"`
		Config hcl.Body `hcl:",remain"`
	}
	type root struct {
		Asserts []*assert `hcl:"assert,block"`
		Rejects []*reject `hcl:"reject,block"`
		Mocks   []*mock   `hcl:"mock,block"`
		Terraspec *terraspec `hcl:"terraspec,block"`
		Expects []*expect `hcl:"expect,block"`
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
		terraspecConfig, diags := decodeTerraspecConfig(r.Terraspec.Body, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Terraspec = terraspecConfig
	} else {
		parsed.Terraspec = &TerraspecConfig{}
	}

	for _, assert := range r.Asserts {
		val, diags := decodeBody(assert.Config, assert.Type, schemas, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		parsed.Asserts = append(parsed.Asserts, NewAssert(assert.Type, assert.Name, val))
	}

	for _, assert := range r.Rejects {
		parsed.Rejects = append(parsed.Rejects, &TypeName{Name: assert.Name, Type: assert.Type})
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
		parsed.Mocks = append(parsed.Mocks, NewMock(mock.Type, mock.Name, query, mocked, body))
	}

	for _, expect := range r.Expects {
		query, expected, diags := decodeExpectBody(expect.Config, expect.Type, schemas, ctx)
		if diags.HasErrors() {
			return nil, diags
		}
		var body []byte
		if r, ok := expect.Config.(*hclsyntax.Body); ok {
			body = r.Range().SliceBytes(file.Bytes)
		}
		parsed.Expects = append(parsed.Expects, NewExpect(expect.Type, expect.Name, query, expected, body))
	}

	return parsed, diags
}

func decodeTerraspecConfig(body hcl.Body, ctx *hcl.EvalContext) (*TerraspecConfig, hcl.Diagnostics) {
	spec := hcldec.ObjectSpec{
		"workspace": &hcldec.AttrSpec{
			Name:     "workspace",
			Type:     cty.String,
			Required: false,
		},
	}

	val, diags := hcldec.Decode(body, spec, nil)
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

func decodeBody(body hcl.Body, bodyType string, schemas *terraform.Schemas, ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
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
		schema = transformSchema(schema)
		partialSchema, _ = schema.SchemaForResourceType(addrs.ManagedResourceMode, rawType)
	}

	val, diags := hcldec.Decode(body, partialSchema.DecoderSpec(), ctx)
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

func decodeExpectBody(body hcl.Body, bodyType string, schemas *terraform.Schemas, ctx *hcl.EvalContext) (query, expect cty.Value, diags hcl.Diagnostics) {
	var codedExpect hcl.Body
	provName := strings.Split(bodyType, "_")[0]

	schema := LookupProviderSchema(schemas, provName)
	partialSchema, _ := schema.SchemaForResourceType(addrs.ManagedResourceMode, bodyType)

	query, codedExpect, diags = hcldec.PartialDecode(body, partialSchema.DecoderSpec(), ctx)
	if diags.HasErrors() {
		return
	}
	expectSchema := toMockSchema(partialSchema)
	expect, moreDiags := hcldec.Decode(codedExpect, expectSchema.DecoderSpec(), ctx)
	diags = append(diags, moreDiags...)
	if diags.HasErrors() {
		return
	}
	expect = expect.GetAttr("return")

	expect, err := cty.Transform(expect, func(path cty.Path, value cty.Value) (cty.Value, error) {
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

// transformSchema upgrades the given schema by adding it support for reject blocks
func transformSchema(schema *terraform.ProviderSchema) *terraform.ProviderSchema {
	transformed := &terraform.ProviderSchema{ResourceTypes: make(map[string]*configschema.Block, len(schema.ResourceTypes))}
	for k, rt := range schema.ResourceTypes {
		transformed.ResourceTypes[k] = transformBlock(rt) //.NoneRequired()
	}
	return transformed
}

// transformBlock modifies a block definition by adding it support for reject blocks
func transformBlock(original *configschema.Block) *configschema.Block {
	transformed := &configschema.Block{}
	transformed.Attributes = make(map[string]*configschema.Attribute, len(original.Attributes))
	rejects := &configschema.NestedBlock{MaxItems: 0, MinItems: 0, Nesting: configschema.NestingSingle}
	rejects.BlockTypes = make(map[string]*configschema.NestedBlock, len(original.BlockTypes))
	for k, v := range original.Attributes {
		transformed.Attributes[k] = v
	}

	transformed.BlockTypes = make(map[string]*configschema.NestedBlock, len(original.BlockTypes)+1)
	for k, v := range original.BlockTypes {
		t := transformBlock(&v.Block)
		transformed.BlockTypes[k] = &configschema.NestedBlock{Block: *t, MaxItems: 0, MinItems: 0, Nesting: v.Nesting}
		rejects.BlockTypes[k] = &configschema.NestedBlock{Block: *t.NoneRequired(), MaxItems: 0, MinItems: 0, Nesting: v.Nesting}
	}
	if len(rejects.BlockTypes) > 0 {
		transformed.BlockTypes["reject"] = rejects
	}

	return transformed
}

// untransformType remove "reject" attributes from type definition
func untransformType(t cty.Type) cty.Type {
	if !t.IsObjectType() {
		return t
	}
	if _, ok := t.AttributeTypes()["reject"]; !ok {
		return t
	}
	proto := make(map[string]cty.Type, len(t.AttributeTypes())-1)
	for k, v := range t.AttributeTypes() {
		if k != "reject" {
			proto[k] = untransformType(v)
		}
	}
	return cty.Object(proto)
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
