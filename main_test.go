package main

import (
	"testing"

	terraspec "github.com/nhurel/terraspec/lib"
	"github.com/stretchr/testify/assert"
)

func TestExecTerraspecWithTestProjectSucceeds(t *testing.T) {
	// backup the plugin folder and create an empty one
	_, restorePluginFolder := terraspec.EnsureEmptyPluginFolder(t)
	defer restorePluginFolder()

	_, _, _ = terraspec.InstallLegacyProvider(t)

	cleanupTerraform := terraspec.TerraformInit(t, "lib/testdata/test_project")
	defer cleanupTerraform()
	
	result := execTerraspec("spec", false, "")

	assert.Equal(t, 0, result)
}