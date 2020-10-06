package integrationtests

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"testing"

	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform/addrs"
	"github.com/mitchellh/go-homedir"
)

// getPluginFolder tries to compute the user plugin folder for the current user.
// For windows: %APPDATA%/terraform.d/plugins
// For linux: ~/terraform.d/plugins
func getPluginFolder() (string, error) {
	homeDir, err := homedir.Dir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		return filepath.FromSlash(fmt.Sprintf("%s/AppData/Roaming/terraform.d/plugins", homeDir)), nil
	}

	return filepath.FromSlash(fmt.Sprintf("%s/.terraform.d/plugins", homeDir)), nil
}

// TerraformInit switches to and initializes the terraform project in the given path.
// Returns a function to cleanup the terraform folder and switch back to current path.
func TerraformInit(t *testing.T, projectPath string) func() {
	t.Logf("Execute terraform init in %s", projectPath)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("%v", err)
	}
	os.Chdir(projectPath)

	cmd := exec.Command("terraform", "init")
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		t.Fatalf("%v", err)
	}

	return func() {
		err := os.RemoveAll(".terraform")
		if err != nil {
			t.Fatalf("%v", err)
		}
		os.Chdir(cwd)
	}
}

// EnsureEmptyPluginFolder ensures that there is an empty plugin folder in the home dir.
// It backups the original folder. You can restore it with the returned func.
// It also returns the path to the plugin folder.
func EnsureEmptyPluginFolder(t *testing.T) (string, func()) {
	// backup the plugin folder and create an empty one
	pluginFolder, err := getPluginFolder()
	if err != nil {
		t.Fatalf("%v", err)
	}

	t.Logf("Ensure empty plugin folder %s", pluginFolder)

	backupFolder := fmt.Sprintf("%s_bak", pluginFolder)
	
	t.Logf("Backup plugin folder to %s", backupFolder)
	os.Rename(pluginFolder, backupFolder)

	os.MkdirAll(pluginFolder, 0777)

	return pluginFolder, func() {
		// restore plugin folder
		err := os.RemoveAll(pluginFolder)
		if err != nil {
			t.Fatalf("%v", err)
		}
		os.Rename(backupFolder, pluginFolder)
	}
}

// InstallLegacyProvider installs a legacy provider.
// Currently the installed provider is the one for cloudfoundry.
// You should first use ensureEmptyPluginFolder to avoid damaging the local plugin folder.
// Returns the addrs.Provider with hostname, namespace, and name, as well as the version of the provider, and the full path to the file.
func InstallLegacyProvider(t *testing.T) (addrs.Provider, string, string) {
	t.Log("Installing legacy cloudfoundry provider")
	pluginDir, err := getPluginFolder()
	if err != nil {
		t.Fatalf("%v", err)
	}

	providerExt := ""
	if runtime.GOOS == "windows" {
			providerExt = ".exe"
	}

	providerHostName := "no.registry.com"
	providerNamespace := "nocorp"
	providerName := "cloudfoundry"
	providerVersion := "0.12.4"
	providerFileName := fmt.Sprintf("terraform-provider-%s_v%s%s", providerName, providerVersion, providerExt)
	zipFileName := fmt.Sprintf("terraform-provider-cloudfoundry_%s_%s_%s.zip", providerVersion, runtime.GOOS, runtime.GOARCH)
	providerLink := fmt.Sprintf(
		"https://github.com/cloudfoundry-community/terraform-provider-cloudfoundry/releases/download/v%s/%s",
		providerVersion,
		zipFileName,
	)

	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	providerTargetFolder := path.Join(pluginDir, providerHostName, providerNamespace, providerName, providerVersion, osArch)
	err = os.MkdirAll(providerTargetFolder, 0777)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = downloadFile(t, zipFileName, providerLink)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Remove(zipFileName)

	providerPath, err := unzip(zipFileName, providerFileName, providerTargetFolder)
	if err != nil {
		t.Fatalf("%v", err)
	}

	t.Logf("Legacy cloudfoundry provider installed to %s", providerPath)

	return addrs.Provider{ 
		Namespace: providerNamespace,
		Hostname: svchost.Hostname(providerHostName),
		Type: providerName,
	},
	providerVersion,
	filepath.FromSlash(providerPath)
}

func downloadFile(t *testing.T, filepath string, url string) error {
	t.Logf("Downloading %s to %s", url, filepath)
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func unzip(zipFile string, extractedFileName string, destinationFolder string) (string, error) {
    r, err := zip.OpenReader(zipFile)
    if err != nil {
        return "", err
    }
    defer r.Close()

    for _, f := range r.File {
		if f.Name == extractedFileName {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			filePath := fmt.Sprintf("%s/%s", destinationFolder, extractedFileName)
			out, err := os.Create(filePath)
			if err != nil {
				return "", err
			}
			defer out.Close()

			io.Copy(out, rc)

			return filePath, nil
		}
	}
	
	return "", fmt.Errorf("Could not find file %s in zip file", extractedFileName)
}
