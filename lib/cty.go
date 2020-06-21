package terraspec

import (
	"log"

	"github.com/zclconf/go-cty/cty"
)

// PrimitiveValue will return the implied value if it's a primitive type
// Number will be returned as int
// If a non primitive is given, nil will be returned
func PrimitiveValue(val cty.Value) interface{} {
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
