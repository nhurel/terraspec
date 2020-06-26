package terraspec_test

import (
	"testing"

	terraspec "github.com/nhurel/terraspec/lib"
	"github.com/zclconf/go-cty/cty"
)

func TestPrimitiveValue(t *testing.T) {

	var tests = map[string]struct {
		given    cty.Value
		expected interface{}
	}{
		"int": {
			given:    cty.NumberIntVal(12),
			expected: 12,
		},
		"string": {
			given:    cty.StringVal("somestring"),
			expected: "somestring",
		},
		"bool": {
			given:    cty.True,
			expected: true,
		},
		"list": {
			given:    cty.ListVal([]cty.Value{cty.StringVal("")}),
			expected: nil,
		},
		"dynamic": {
			given:    cty.DynamicVal,
			expected: nil,
		},
		"unknown": {
			given:    cty.UnknownVal(cty.String),
			expected: nil,
		},
		"null": {
			given:    cty.NilVal,
			expected: nil,
		},
	}

	for k, tt := range tests {
		t.Run(k, func(t *testing.T) {
			if got := terraspec.PrimitiveValue(tt.given); got != tt.expected {
				t.Errorf("Error : Got %v - Want %v", got, tt.expected)
			}
		})
	}
}

func TestIsNull(t *testing.T) {
	var tests = map[string]struct {
		given    cty.Value
		expected bool
	}{
		"emptystring": {
			given:    cty.StringVal(""),
			expected: false,
		},
		"null": {
			given:    cty.NilVal,
			expected: true,
		},
		"dyamic": {
			given:    cty.DynamicVal,
			expected: false,
		},
		"bool": {
			given:    cty.False,
			expected: false,
		},
		"nilList": {
			given:    cty.ListVal([]cty.Value{cty.NilVal}),
			expected: true,
		},
		"nonnilList": {
			given:    cty.ListVal([]cty.Value{cty.UnknownVal(cty.String), cty.StringVal("")}),
			expected: false,
		},
		"unknown": {
			given:    cty.UnknownVal(cty.String),
			expected: false,
		},
		"emptylist": {
			given:    cty.ListValEmpty(cty.String),
			expected: true,
		},
	}

	for k, tt := range tests {
		t.Run(k, func(t *testing.T) {
			if got := terraspec.IsNull(tt.given); got != tt.expected {
				t.Errorf("Error : Got %t - Want %t", got, tt.expected)
			}
		})
	}
}
