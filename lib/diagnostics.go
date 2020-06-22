package terraspec

import (
	"fmt"

	"github.com/hashicorp/terraform/tfdiags"
	"github.com/zclconf/go-cty/cty"
)

// Info is an additional Severity level to display Info messages
const Info tfdiags.Severity = 'I'

type TerraspecDiagnostic tfdiags.Diagnostic

func SuccessDiags(path cty.Path, value interface{}) tfdiags.Diagnostic {
	return TerraspecDiagnostic(tfdiags.AttributeValue(Info, "", fmt.Sprintf("%v", value), path))
}

func AssertErrorDiags(path cty.Path, expected, got interface{}) tfdiags.Diagnostic {
	return TerraspecDiagnostic(tfdiags.AttributeValue(tfdiags.Error, "", fmt.Sprintf("%v != %v", got, expected), path))
}

func ErrorDiags(path cty.Path, detail string) tfdiags.Diagnostic {
	return TerraspecDiagnostic(tfdiags.AttributeValue(tfdiags.Error, "", detail, path))
}
