package terraspec

import (
	"fmt"
	"log"

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
			w.SetAttributeValue(key, val)
			return false, nil
		}

		if val.Type().IsPrimitiveType() {
			w.SetAttributeValue(key, val)
		} else {
			b := w.AppendNewBlock(key, nil)
			w = b.Body()
		}

		return true, nil
	})

}
