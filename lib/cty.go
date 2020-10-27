package terraspec

import (
	"fmt"
	"log"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// PrimitiveValue will return the implied value if it's a primitive type
// Number will be returned as int
// If a non primitive is given, nil will be returned
func PrimitiveValue(val cty.Value) interface{} {
	if !val.IsKnown() || val.IsNull() {
		return nil
	}
	switch val.Type() {
	case cty.Bool:
		return val.True()
	case cty.Number:
		v, _ := val.AsBigFloat().Int64()
		return int(v)
	case cty.String:
		return val.AsString()
	default:
		//TODO should be a debug log only
		log.Printf("Can get primitive value from non primitive %v\n", val)
		return nil
	}
}

// IsNull returns true if val is null or all its properties (recrusively) are null
func IsNull(val cty.Value) bool {
	if val.IsNull() {
		return true
	}
	if val.CanIterateElements() {
		if !val.IsKnown() {
			return true
		}
		it := val.ElementIterator()
		for it.Next() {
			_, v := it.Element()
			if !IsNull((v)) {
				return false
			}
		}
		return true
	}
	return false
}

// IsEmptyCollection returns true if value is a collection type of length 0
func IsEmptyCollection(value cty.Value) bool {
	if value.Type().IsListType() || value.Type().IsSetType() {
		return value.AsValueSet().Length() == 0
	}
	return false
}

//MarshalValue serializes a cty.Value in hcl format
func MarshalValue(value cty.Value) []byte {
	f := hclwrite.NewEmptyFile()
	marshalValue(value, f.Body())
	return f.Bytes()
}

func marshalValue(value cty.Value, writer *hclwrite.Body) {
	var w = writer
	cty.Walk(value, func(path cty.Path, val cty.Value) (bool, error) {
		if IsNull(val) {
			return true, nil
		}

		var key string
		if len(path) > 0 {
			lastPath := path[len(path)-1]

			switch p := lastPath.(type) {
			case cty.GetAttrStep:
				key = p.Name
			case cty.IndexStep:
				key = fmt.Sprintf("%v", PrimitiveValue(p.Key))
			}
		}
		if val.Type().IsListType() || val.Type().IsSetType() {
			w.SetAttributeRaw(key, hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte{'['}}})
			it := val.ElementIterator()
			it.Next()
			for {
				_, v := it.Element()
				marshalValue(v, w)
				if it.Next() {
					w.AppendUnstructuredTokens(hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte{','}}})
				} else {
					break
				}
			}
			w.AppendNewline()
			w.AppendUnstructuredTokens(hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte{']'}}})
			w.AppendNewline()
			return false, nil
		}

		if val.Type().IsPrimitiveType() {
			if val.IsKnown() {
				if key != "" {
					w.SetAttributeValue(key, val)
				} else {
					w.AppendUnstructuredTokens(hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}}})
					w.AppendUnstructuredTokens(hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenStringLit, Bytes: []byte(fmt.Sprintf("%v", PrimitiveValue(val)))}})
					w.AppendUnstructuredTokens(hclwrite.Tokens{&hclwrite.Token{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}}})
				}
			}
		} else {
			b := w.AppendNewBlock(key, nil)
			w = b.Body()
		}

		return true, nil
	})
}

//Merge returns a new cty.Value whose attributes are set with values from override if present,
// or from original
func Merge(original, override cty.Value) cty.Value {
	if !original.Type().IsObjectType() {
		if original.Type().IsPrimitiveType() {
			if override.IsNull() || !override.IsKnown() {
				return original
			}
			return override
		}
		if original.Type().IsCollectionType() {
			if !IsEmptyCollection(override) {
				return override
			}
			return original
		}
		return original
	}
	attributes := original.Type().AttributeTypes()
	merged := make(map[string]cty.Value, len(attributes))
	for att := range attributes {
		if !override.Type().HasAttribute(att) || override.GetAttr(att).IsNull() || !override.GetAttr(att).IsKnown() {
			merged[att] = original.GetAttr(att)
		} else {
			merged[att] = Merge(original.GetAttr(att), override.GetAttr(att))
		}
	}
	return cty.ObjectVal(merged)
}
