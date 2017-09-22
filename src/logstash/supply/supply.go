package supply

import (
	"errors"
	"fmt"
	"golang"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudfoundry/libbuildpack"
	"os/exec"
)

type Manifest interface {
	AllDependencyVersions(string) []string
	DefaultVersion(string) (libbuildpack.Dependency, error)
	InstallDependency(libbuildpack.Dependency, string) error
	InstallOnlyVersion(string, string) error
}

type Stager interface {
	AddBinDependencyLink(string, string) error
	BuildDir() string
	DepDir() string
	DepsIdx() string
	WriteConfigYml(interface{}) error
	WriteEnvFile(string, string) error
	WriteProfileD(string, string) error
}

type Supplier struct {
	Stager     Stager
	Manifest   Manifest
	Log        *libbuildpack.Logger
	VendorTool string
	LogstashVersion  string
	OpenJDKVersion  string
	Godep      golang.Godep
}

func Run(gs *Supplier) error {

	if err := gs.SourceLogstashFile(); err != nil {
		gs.Log.Error("Unable to source Logstash file: %s", err.Error())
		return err
	}

	if err := gs.SelectOpenJDKVersion(); err != nil {
		gs.Log.Error("Unable to determine Java version to install: %s", err.Error())
		return err
	}

	if err := gs.InstallOpenJDK(); err != nil {
		gs.Log.Error("Error installing Java: %s", err.Error())
		return err
	}

	if err := gs.WriteJDKToProfileD(); err != nil {
		gs.Log.Error("Error writing profile.d script for JDK: %s", err.Error())
		return err
	}


	if err := gs.SelectLogstashVersion(); err != nil {
		gs.Log.Error("Unable to determine Logstash version to install: %s", err.Error())
		return err
	}

	if err := gs.InstallLogstash(); err != nil {
		gs.Log.Error("Error installing Logstash: %s", err.Error())
		return err
	}

	if err := gs.WriteLogstashToProfileD(); err != nil {
		gs.Log.Error("Error writing profile.d script for logstash: %s", err.Error())
		return err
	}

	if err := gs.WriteConfigYml(); err != nil {
		gs.Log.Error("Error writing config.yml: %s", err.Error())
		return err
	}
	/*	if err := gs.SelectVendorTool(); err != nil {
			gs.Log.Error("Unable to select Go vendor tool: %s", err.Error())
			return err
		}

		if err := gs.InstallVendorTools(); err != nil {
			gs.Log.Error("Unable to install vendor tools", err.Error())
			return err
		}

		if err := gs.SelectGoVersion(); err != nil {
			gs.Log.Error("Unable to determine Go version to install: %s", err.Error())
			return err
		}

		if err := gs.InstallGo(); err != nil {
			gs.Log.Error("Error installing Go: %s", err.Error())
			return err
		}

		if err := gs.WriteGoRootToProfileD(); err != nil {
			gs.Log.Error("Error writing GOROOT to profile.d: %s", err.Error())
			return err
		}

		if err := gs.WriteConfigYml(); err != nil {
			gs.Log.Error("Error writing config.yml: %s", err.Error())
			return err
		}
	*/
	return nil
}

func (gs *Supplier) SourceLogstashFile() error {
	logstashFile := filepath.Join(gs.Stager.BuildDir(), "Logstash")

	isLogstash, err := libbuildpack.FileExists(logstashFile)
	if err != nil {
		return err
	}

	if !isLogstash{
		return errors.New("Lostash file not found")
	}

	cmd := exec.Command("bash", "-c", "source " + logstashFile)
	err = cmd.Run()

	if err != nil {
		return err
	}
   return nil
}


func (gs *Supplier) SelectOpenJDKVersion() error {
	openJDKVersion := os.Getenv("OPENJDK_VERSION")


	if openJDKVersion == "" {
		defaultOpenJDK, err := gs.Manifest.DefaultVersion("openjdk")
		if err != nil {
			return err
		}
		openJDKVersion = fmt.Sprintf("openjdk%s", defaultOpenJDK.Version)
	}


	parsed, err := gs.parseOpenJDKVersion(openJDKVersion)
	if err != nil {
		return err
	}

	gs.OpenJDKVersion = parsed
	return nil
}

func (gs *Supplier) InstallOpenJDK() error {
	openJDKInstallDir := filepath.Join(gs.Stager.DepDir(), "openjdk-"+gs.OpenJDKVersion)

	dep := libbuildpack.Dependency{Name: "openjdk", Version: gs.OpenJDKVersion}
	if err := gs.Manifest.InstallDependency(dep, openJDKInstallDir); err != nil {
		return err
	}

/*	if err := gs.Stager.AddBinDependencyLink(filepath.Join(openJDKInstallDir), "openjdk"); err != nil {
		return err
	}
*/
	return nil
}


func (gs *Supplier) SelectLogstashVersion() error {
	logstashVersion := os.Getenv("LOGSTASH_VERSION")


	if logstashVersion == "" {
		defaultLogstash, err := gs.Manifest.DefaultVersion("logstash")
		if err != nil {
			return err
		}
		logstashVersion = fmt.Sprintf("logstash%s", defaultLogstash.Version)
	}


	parsed, err := gs.parseLogstashVersion(logstashVersion)
	if err != nil {
		return err
	}

	gs.LogstashVersion = parsed
	return nil
}

func (gs *Supplier) InstallLogstash() error {
	logstashInstallDir := filepath.Join(gs.Stager.DepDir(), "logstash-"+gs.LogstashVersion)

	dep := libbuildpack.Dependency{Name: "logstash", Version: gs.LogstashVersion}
	if err := gs.Manifest.InstallDependency(dep, logstashInstallDir); err != nil {
		return err
	}

/*	if err := gs.Stager.AddBinDependencyLink(filepath.Join(logstashInstallDir, "logstash-"+gs.LogstashVersion, "bin", "logstash"), "logstash"); err != nil {
		return err
	}
*/
	return nil
}

func (gs *Supplier) SelectVendorTool() error {
	godepsJSONFile := filepath.Join(gs.Stager.BuildDir(), "Godeps", "Godeps.json")

	godirFile := filepath.Join(gs.Stager.BuildDir(), ".godir")
	isGodir, err := libbuildpack.FileExists(godirFile)
	if err != nil {
		return err
	}
	if isGodir {
		gs.Log.Error(golang.GodirError())
		return errors.New(".godir deprecated")
	}

	isGoPath, err := gs.isGoPath()
	if err != nil {
		return err
	}
	if isGoPath {
		gs.Log.Error(golang.GBError())
		return errors.New("gb unsupported")
	}

	isGodep, err := libbuildpack.FileExists(godepsJSONFile)
	if err != nil {
		return err
	}
	if isGodep {
		gs.Log.BeginStep("Checking Godeps/Godeps.json file")

		err = libbuildpack.NewJSON().Load(filepath.Join(gs.Stager.BuildDir(), "Godeps", "Godeps.json"), &gs.Godep)
		if err != nil {
			gs.Log.Error("Bad Godeps/Godeps.json file")
			return err
		}

		gs.Godep.WorkspaceExists, err = libbuildpack.FileExists(filepath.Join(gs.Stager.BuildDir(), "Godeps", "_workspace", "src"))
		if err != nil {
			return err
		}

		gs.VendorTool = "godep"
		return nil
	}

	glideFile := filepath.Join(gs.Stager.BuildDir(), "glide.yaml")
	isGlide, err := libbuildpack.FileExists(glideFile)
	if err != nil {
		return err
	}
	if isGlide {
		gs.VendorTool = "glide"
		return nil
	}

	gs.VendorTool = "go_nativevendoring"
	return nil
}



func (gs *Supplier) WriteJDKToProfileD() error {
	javaRuntimeLocation := filepath.Join(gs.Stager.DepsIdx(), "openjdk-"+gs.OpenJDKVersion)
	if err := gs.Stager.WriteProfileD("javahome.sh", golang.JDKProfileD(javaRuntimeLocation)); err != nil {
		return err
	}
	return nil
}

func (gs *Supplier) WriteLogstashToProfileD() error {
	logstashRuntimeLocation := filepath.Join(gs.Stager.DepsIdx(), "logstash-"+gs.LogstashVersion, "logstash-"+gs.LogstashVersion)
	if err := gs.Stager.WriteProfileD("logstash.sh", golang.LogstashProfileD(logstashRuntimeLocation)); err != nil {
		return err
	}
	return nil
}

/*
func (gs *Supplier) WriteGoRootToProfileD() error {
	goRuntimeLocation := filepath.Join("$DEPS_DIR", gs.Stager.DepsIdx(), "go"+gs.GoVersion, "go")
	if err := gs.Stager.WriteProfileD("goroot.sh", golang.GoRootScript(goRuntimeLocation)); err != nil {
		return err
	}
	return nil
}
*/

func (gs *Supplier) InstallVendorTools() error {
	tools := []string{"godep", "glide"}

	for _, tool := range tools {
		installDir := filepath.Join(gs.Stager.DepDir(), tool)
		if err := gs.Manifest.InstallOnlyVersion(tool, installDir); err != nil {
			return err
		}

		if err := gs.Stager.AddBinDependencyLink(filepath.Join(installDir, "bin", tool), tool); err != nil {
			return err
		}
	}

	return nil
}

/*
func (gs *Supplier) SelectGoVersion() error {
	goVersion := os.Getenv("GOVERSION")

	if gs.VendorTool == "godep" {
		if goVersion != "" {
			gs.Log.Warning(golang.GoVersionOverride(goVersion))
		} else {
			goVersion = gs.Godep.GoVersion
		}
	} else {
		if goVersion == "" {
			defaultGo, err := gs.Manifest.DefaultVersion("go")
			if err != nil {
				return err
			}
			goVersion = fmt.Sprintf("go%s", defaultGo.Version)
		}
	}

	parsed, err := gs.parseGoVersion(goVersion)
	if err != nil {
		return err
	}

	gs.GoVersion = parsed
	return nil
}




func (gs *Supplier) InstallGo() error {
	goInstallDir := filepath.Join(gs.Stager.DepDir(), "go"+gs.GoVersion)

	dep := libbuildpack.Dependency{Name: "go", Version: gs.GoVersion}
	if err := gs.Manifest.InstallDependency(dep, goInstallDir); err != nil {
		return err
	}

	if err := gs.Stager.AddBinDependencyLink(filepath.Join(goInstallDir, "go", "bin", "go"), "go"); err != nil {
		return err
	}

	return gs.Stager.WriteEnvFile("GOROOT", filepath.Join(goInstallDir, "go"))
}
*/
func (gs *Supplier) WriteConfigYml() error {
	config := map[string]string{
		"LogstashVersion":  gs.LogstashVersion,
	}

	return gs.Stager.WriteConfigYml(config)
}

func (gs *Supplier) parseLogstashVersion(partialLogstashVersion string) (string, error) {
	existingVersions := gs.Manifest.AllDependencyVersions("logstash")

	if len(strings.Split(partialLogstashVersion, ".")) < 3 {
		partialLogstashVersion += ".x"
	}

	strippedLogstashVersion := strings.TrimLeft(partialLogstashVersion, "logstash")

	expandedVer, err := libbuildpack.FindMatchingVersion(strippedLogstashVersion, existingVersions)
	if err != nil {
		return "", err
	}

	return expandedVer, nil
}

func (gs *Supplier) parseOpenJDKVersion(partialOpenJDKVersion string) (string, error) {
	existingVersions := gs.Manifest.AllDependencyVersions("openjdk")

	if len(strings.Split(partialOpenJDKVersion, ".")) < 3 {
		partialOpenJDKVersion += ".x"
	}

	strippedOpenJDKVersion := strings.TrimLeft(partialOpenJDKVersion, "openjdk")

	expandedVer, err := libbuildpack.FindMatchingVersion(strippedOpenJDKVersion, existingVersions)
	if err != nil {
		return "", err
	}

	return expandedVer, nil
}

func (gs *Supplier) isGoPath() (bool, error) {
	srcDir := filepath.Join(gs.Stager.BuildDir(), "src")
	srcDirAtAppRoot, err := libbuildpack.FileExists(srcDir)
	if err != nil {
		return false, err
	}

	if !srcDirAtAppRoot {
		return false, nil
	}

	files, err := ioutil.ReadDir(filepath.Join(gs.Stager.BuildDir(), "src"))
	if err != nil {
		return false, err
	}

	for _, file := range files {
		if file.Mode().IsDir() {
			err = filepath.Walk(filepath.Join(srcDir, file.Name()), isGoFile)
			if err != nil {
				if err.Error() == "found Go file" {
					return true, nil
				}

				return false, err
			}
		}
	}

	return false, nil
}

func isGoFile(path string, info os.FileInfo, err error) error {
	if err != nil {
		return err
	}

	if strings.HasSuffix(path, ".go") {
		return errors.New("found Go file")
	}

	return nil
}
