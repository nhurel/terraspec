package terraspec_test

import (
	"strings"
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

func TestMarshalValue(t *testing.T) {

	var tests = map[string]struct {
		given    cty.Value
		expected string
	}{
		"terraform_remote_state": {
			given: cty.ObjectVal(map[string]cty.Value{
				"backend": cty.StringVal("s3"),
				"config": cty.MapVal(map[string]cty.Value{
					"bucket": cty.StringVal("mybucket"),
					"key":    cty.StringVal("path/to/my/key"),
					"region": cty.StringVal("us-east-1"),
				}),
			}),
			expected: ` {
        backend = "s3"
        config {
            bucket = "mybucket"
            key    = "path/to/my/key"
            region = "us-east-1"
        }
    }
`,
		},
		"aws_ami": {
			given: cty.ObjectVal(map[string]cty.Value{
				"most_recent": cty.True,
				"owners":      cty.ListVal([]cty.Value{cty.StringVal("amazon")}),
				"filter": cty.SetVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"name":   cty.StringVal("name"),
						"values": cty.ListVal([]cty.Value{cty.StringVal("amzn-ami-hvm-*-x86_64-gp2")}),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"name":   cty.StringVal("owner-alias"),
						"values": cty.ListVal([]cty.Value{cty.StringVal("amazon")}),
					}),
				}),
			},
			),
			expected: ` {
              filter = [
				{ 
                  name = "name"
                  values = [
					  "amzn-ami-hvm-*-x86_64-gp2"
					] 
				}
				,  {
                  name = "owner-alias"
                  values = [
					  "amazon"
					  ] 
			   }

			   ]
              most_recent = true
              owners      = [
				  "amazon"
				  ]
			}
`,
		},
		"partially_known_block": {
			given: cty.ObjectVal(map[string]cty.Value{
				"name":              cty.StringVal("known"),
				"unknown_primitive": cty.UnknownVal(cty.Number),
				"partial_list": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"name":   cty.StringVal("known"),
						"unkown": cty.UnknownVal(cty.String),
					}),
				}),
			}),
			expected: `{
				name = "known"
				partial_list = [
					{
						name = "known"
					}
					
				]
				}
				`,
		},
	}
	for k, tt := range tests {
		t.Run(k, func(t *testing.T) {
			e := strings.ReplaceAll(strings.ReplaceAll(tt.expected, " ", ""), "\t", "")
			got := string(terraspec.MarshalValue(tt.given))
			if g := strings.ReplaceAll(strings.ReplaceAll(string(got), " ", ""), "\t", ""); g != e {
				t.Errorf("Value not marshalled as expected.\nGot : [%s]\nWant : [%s]", g, e)
			}
		})
	}
}
