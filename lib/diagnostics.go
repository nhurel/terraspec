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

// SuccessDiags creates a diagnostic at Info level to indicate the user a given assertion matches
func SuccessDiags(path cty.Path, value interface{}) tfdiags.Diagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(Info, "", fmt.Sprintf("%v", value), path)}
}

// AssertErrorDiags returns a diagnostic at Error level to indicate the user a given assertion failed
func AssertErrorDiags(path cty.Path, expected, got interface{}) tfdiags.Diagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(tfdiags.Error, "", fmt.Sprintf("%v != %v", got, expected), path)}
}

// ErrorDiags returns a diagnostic at Error level with given error message
func ErrorDiags(path cty.Path, detail string) tfdiags.Diagnostic {
	return &TerraspecDiagnostic{tfdiags.AttributeValue(tfdiags.Error, "", detail, path)}
}
