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
	"strconv"
	"strings"
	"testing"

	"github.com/facebookgo/symwalk"
	svchost "github.com/hashicorp/terraform-svchost"
	"github.com/hashicorp/terraform/addrs"
	terraspec "github.com/nhurel/terraspec/lib"
)

// GetTerraspec computes the path of the terraspec executable.
// If it cannot be found it is build.
func GetTerraspec(t *testing.T, rootDir string) string {
	terraspecFileName := "terraspec"
	if runtime.GOOS == "windows" {
		terraspecFileName = terraspecFileName + ".exe"
	}
	terraspecPath := rootDir + "/" + terraspecFileName

	_, err := os.Stat(terraspecPath)
	if os.IsNotExist(err) {
		changeBack := Chdir(t, rootDir)
		defer changeBack()

		buildTerraspec(t, terraspecFileName)
	} else if err != nil {
		t.Fatalf("Could not stat file %s: %v", terraspecPath, err)
	}

	return terraspecPath
}

func buildTerraspec(t *testing.T, terraspecFileName string) {
	t.Logf("Building terraspec executable: go build -o %s .", terraspecFileName)
	cmd := exec.Command("go", "build", "-o", terraspecFileName, ".")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Could not build terraspec, output is\r\n%s\r\nerror: %v", string(output), err)
	}
}

// RunTerraspec runs the given terraspec executable in the specified project path.
func RunTerraspec(t *testing.T, terraspecPath string, projectPath string) {
	changeBack := Chdir(t, projectPath)
	defer changeBack()

	cwd := Getwd(t)

	t.Logf("Running %s in %s", terraspecPath, cwd)
	output, err := exec.Command(terraspecPath).CombinedOutput()
	t.Logf("Terraspec output is:\r\n%s", string(output))
	if err != nil {
		t.Fatalf("Error while executing terraspec: %v", err)
	}
}



// GetTerraform downloads terraform with the stated version into the root project directory.
func GetTerraform(t *testing.T, version string, rootDir string) (string) {
	terraformZIPFileName := "terraform"
	terraformFileName := "terraform_v" + version
	if runtime.GOOS == "windows" {
		terraformZIPFileName += ".exe"
		terraformFileName += ".exe"
	}

	terraformPath := rootDir + "/" + terraformFileName
	_, err := os.Stat(terraformPath)
	if err == nil {
		return terraformPath
	}

	zipFilePath := "terraform.zip"
	terraformURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%s/terraform_%s_%s_%s.zip", version, version, runtime.GOOS, runtime.GOARCH)
	downloadFile(t, zipFilePath, terraformURL)
	defer os.Remove(zipFilePath)

	terraformPath, err = unzip(zipFilePath, terraformZIPFileName, terraformFileName, rootDir)
	if err != nil {
		t.Fatalf("Could not unzip terraform download: %v", err)
	}

	return terraformPath
}

// TerraformInit switches to and initializes the terraform project in the given path.
// Returns a function to cleanup the terraform folder and switch back to current path.
func TerraformInit(t *testing.T, terraformPath string, projectPath string) func() {
	changeBack := Chdir(t, projectPath)

	cwd := Getwd(t)

	pluginFolder, err := terraspec.GetPluginFolder()
	if err != nil {
		changeBack()
		t.Fatalf("%v", err)
	}
	t.Logf("Contents of global plugin folder before tf init (%s)", pluginFolder)
	symwalk.Walk(pluginFolder, func(path string, info os.FileInfo, err error) error {
		t.Logf("- %s", path)

		return nil
	})

	t.Logf("Execute %s init in %s", terraformPath, cwd)

	cmd := exec.Command(terraformPath, "init")
	output, err := cmd.CombinedOutput()
	t.Log(string(output))
	if err != nil {
		os.RemoveAll(".terraform")
		changeBack()
		t.Fatalf("%v", err)
	}

	t.Logf("Contents of .terraform folder after tf init")
	symwalk.Walk(".terraform", func(path string, info os.FileInfo, err error) error {
		t.Logf("- %s", path)

		return nil
	})

	return func() {
		err := os.RemoveAll(".terraform")
		if err != nil {
			t.Fatalf("%v", err)
		}
		changeBack()
	}
}

// EnsureEmptyPluginFolder ensures that there is an empty plugin folder in the home dir.
// It backups the original folder. You can restore it with the returned func.
// It also returns the path to the plugin folder.
func EnsureEmptyPluginFolder(t *testing.T) (string, func()) {
	// backup the plugin folder and create an empty one
	pluginFolder, err := terraspec.GetPluginFolder()
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
func InstallLegacyProvider(t *testing.T, terraformVersion string) (addrs.Provider, string, string) {
	t.Log("Installing legacy cloudfoundry provider")
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

	providerTargetFolder := getLegacyProviderTargetFolder(t, terraformVersion, providerHostName, providerNamespace, providerName, providerVersion)
	err := os.MkdirAll(providerTargetFolder, 0777)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = downloadFile(t, zipFileName, providerLink)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer os.Remove(zipFileName)

	providerPath, err := unzip(zipFileName, providerFileName, providerFileName, providerTargetFolder)
	if err != nil {
		t.Fatalf("%v", err)
	}
	// make the provider executable
	err = os.Chmod(providerPath, 0777) 
	if err != nil {
		t.Fatalf("Could not change permissions to 0777 on legacy provider %s", providerPath)
	}

	t.Logf("Legacy cloudfoundry provider installed to %s", providerPath)

	return addrs.Provider{
			Namespace: providerNamespace,
			Hostname:  svchost.Hostname(providerHostName),
			Type:      providerName,
		},
		providerVersion,
		filepath.FromSlash(providerPath)
}

// ParseTerraformVersion parses the given version string and returns major, minor and patch version.
func ParseTerraformVersion(t *testing.T, version string) (int64, int64, int64) {
	versionParts := strings.Split(version, ".")

	majorVersion, err := strconv.ParseInt(versionParts[0], 10, 64)
	if err != nil {
		t.Fatalf("Could not parse terraform version %s: %v", version, err)
	}

	minorVersion, err := strconv.ParseInt(versionParts[1], 10, 64)
	if err != nil {
		t.Fatalf("Could not parse terraform version %s: %v", version, err)
	}

	patchVersion, err := strconv.ParseInt(versionParts[2], 10, 64)
	if err != nil {
		t.Fatalf("Could not parse terraform version %s: %v", version, err)
	}

	return majorVersion, minorVersion, patchVersion
}

func getLegacyProviderTargetFolder(t *testing.T, terraformVersion string, hostName string, namespace string, name string, version string) string {
	pluginDir, err := terraspec.GetPluginFolder()
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, minorVersion, _ := ParseTerraformVersion(t, terraformVersion)

	osArch := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	if minorVersion >= 13 {
		return path.Join(pluginDir, hostName, namespace, name, version, osArch)
	}

	// before terraform 13 the plugins were placed directly in the plugins folder
	// if runtime.GOOS == "windows" {
	// 	return pluginDir
	// }

	// on linux they live in the os_arch folder
	return path.Join(pluginDir, osArch)
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

func unzip(zipFile string, extractedFileName string, destinationFileName string, destinationFolder string) (string, error) {
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

			filePath := fmt.Sprintf("%s/%s", destinationFolder, destinationFileName)
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

func Getwd(t *testing.T) string {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Could get current working directory: %v", err)
	}

	return cwd
}

func Chdir(t *testing.T, targetDir string) func() {
	cwd := Getwd(t)
	err := os.Chdir(targetDir)
	if err != nil {
		t.Fatalf("Could not switch to directory %s: %v", targetDir, err)
	}

	return func() {
		err := os.Chdir(cwd)
		if err != nil {
			t.Fatalf("Could not switch back to directory %s: %v", cwd, err)
		}
	}
}
