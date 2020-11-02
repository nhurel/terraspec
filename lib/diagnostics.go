package terraspec

import (
	"fmt"

	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Info is an additional Severity level to display Info messages
const Info tfdiags.Severity = 'I'

// TerraspecDiagnostic is an assertion diagnostic, either a success or error
type TerraspecDiagnostic struct {
	tfdiags.Diagnostic
}

var _ tfdiags.Diagnostic = &TerraspecDiagnostic{}

// SuccessDiags creates a diagnostic at Info level to indicate the user a given assertion matches
func SuccessDiags(path cty.Path, value interface{}) *TerraspecDiagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(Info, "", fmt.Sprintf("%v", value), path)}
}

// AssertErrorDiags returns a diagnostic at Error level to indicate the user a given assertion failed
func AssertErrorDiags(path cty.Path, expected, got interface{}) *TerraspecDiagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(tfdiags.Error, "", fmt.Sprintf("%v != %v", got, expected), path)}
}

// ErrorDiags returns a diagnostic at Error level with given error message
func ErrorDiags(path cty.Path, detail string) *TerraspecDiagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(tfdiags.Error, "", detail, path)}
}

// RejectErrorDiags returns a diagnostic at Error level to indicate the user a given reject assertion failed
func RejectErrorDiags(path cty.Path, rejected, got interface{}) *TerraspecDiagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(tfdiags.Error, "", fmt.Sprintf("%v matches %v", got, rejected), path)}
}

//RejectValueErrorDiags returns a diagnostic at Error level toi indicate the user a reject assertion failed
func RejectValueErrorDiags(path cty.Path, key, rejected, got cty.Value) *TerraspecDiagnostic {
	errorElement := cty.ObjectVal(map[string]cty.Value{key.AsString(): got})
	errorReject := cty.ObjectVal(map[string]cty.Value{key.AsString(): rejected})
	return RejectErrorDiags(path.GetAttr(key.AsString()), string(MarshalValue(errorReject)), string(MarshalValue(errorElement)))
}

// RejectSuccessDiags returns a diagnostic at Info level to indicate the user a given reject assertion succeeded
func RejectSuccessDiags(path cty.Path, message string, rejected interface{}) *TerraspecDiagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(Info, "", message, path)}
}

// Compare returns the difference in error numbers between one and other
// if result == 0, then the 2 diagnostics have same number of errors
// if result < 0, one has less error than other
// if result > 0, one has more error than other
func Compare(one, other tfdiags.Diagnostics) int {
	oneErrors, otherErrors := 0, 0
	for _, d := range one {
		if d.Severity() == tfdiags.Error {
			oneErrors++
		}
	}
	for _, d := range other {
		if d.Severity() == tfdiags.Error {
			otherErrors++
		}
	}
	return oneErrors - otherErrors
}
