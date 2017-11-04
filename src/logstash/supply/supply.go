package supply

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"github.com/cloudfoundry/libbuildpack"
	"os/exec"
	"fmt"
	"regexp"
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
	Dockerize Dependency
	Jq Dependency
	Ofelia Dependency
	Curator Dependency
	Logstash Dependency
	OpenJdk  Dependency
}

type Dependency struct{
	Name string
	Version string
	VersionParts int
	EnvVersion string
	RuntimeLocation string
	StagingLocation string
}

func Run(gs *Supplier) error {

	if err := gs.SourceLogstashFile(); err != nil {
		gs.Log.Error("Unable to source Logstash file: %s", err.Error())
		return err
	}

	//Install Dockerize
	gs.Dockerize = Dependency{Name: "dockerize", VersionParts: 3, EnvVersion: "DOCKERIZE_VERSION"}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Dockerize); err != nil {
		gs.Log.Error("Unable to determine the Dockerize version to install: %s", err.Error())
		return err
	}else{
		gs.Dockerize.Version = parsedVersion
		gs.Dockerize.RuntimeLocation = gs.EvalRuntimeLocation(gs.Dockerize)
		gs.Dockerize.StagingLocation = gs.EvalStagingLocation(gs.Dockerize)
	}

	if err := gs.InstallDependency(gs.Dockerize); err != nil {
		gs.Log.Error("Error installing Dockerize: %s", err.Error())
		return err
	}

	content := TrimLines(fmt.Sprintf(`
				export DOCKERIZE_HOME=$DEPS_DIR/%s
				PATH=$PATH:$DOCKERIZE_HOME
				`, gs.Dockerize.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Dockerize, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Dockerize: %s", err.Error())
		return err
	}

	//Install JQ
	gs.Jq = Dependency{Name: "jq", VersionParts: 3, EnvVersion: "JQ_VERSION"}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Jq); err != nil {
		gs.Log.Error("Unable to determine the Jq version to install: %s", err.Error())
		return err
	}else{
		gs.Jq.Version = parsedVersion
		gs.Jq.RuntimeLocation = gs.EvalRuntimeLocation(gs.Jq)
		gs.Jq.StagingLocation = gs.EvalStagingLocation(gs.Jq)
	}

	if err := gs.InstallDependency(gs.Jq); err != nil {
		gs.Log.Error("Error installing Jq: %s", err.Error())
		return err
	}

	content = TrimLines(fmt.Sprintf(`
				export JQ_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JQ_HOME
				`, gs.Jq.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Jq, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Jq: %s", err.Error())
		return err
	}

	//Install Ofelia
	gs.Ofelia = Dependency{Name: "ofelia", VersionParts: 3, EnvVersion: "OFELIA_VERSION"}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Ofelia); err != nil {
		gs.Log.Error("Unable to determine the Ofelia version to install: %s", err.Error())
		return err
	}else{
		gs.Ofelia.Version = parsedVersion
		gs.Ofelia.RuntimeLocation = gs.EvalRuntimeLocation(gs.Ofelia)
		gs.Ofelia.StagingLocation = gs.EvalStagingLocation(gs.Ofelia)
	}

	if err := gs.InstallDependency(gs.Ofelia); err != nil {
		gs.Log.Error("Error installing Ofelia: %s", err.Error())
		return err
	}

	content = TrimLines(fmt.Sprintf(`
				export OFELIA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$OFELIA_HOME
				`, gs.Ofelia.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Ofelia, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Ofelia: %s", err.Error())
		return err
	}

	//Install Curator
	gs.Curator = Dependency{Name: "curator", VersionParts: 3, EnvVersion: "CURATOR_VERSION"}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Curator); err != nil {
		gs.Log.Error("Unable to determine the Curator version to install: %s", err.Error())
		return err
	}else{
		gs.Curator.Version = parsedVersion
		gs.Curator.RuntimeLocation = gs.EvalRuntimeLocation(gs.Curator)
		gs.Curator.StagingLocation = gs.EvalStagingLocation(gs.Curator)
	}

	if err := gs.InstallDependency(gs.Curator); err != nil {
		gs.Log.Error("Error installing Curator: %s", err.Error())
		return err
	}

	content = TrimLines(fmt.Sprintf(`
				export CURATOR_HOME=$DEPS_DIR/%s
				PATH=$PATH:$CURATOR_HOME
				`, gs.Curator.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Curator, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Curator: %s", err.Error())
		return err
	}

	//Install OpenJDK
	gs.OpenJdk = Dependency{Name: "openjdk", VersionParts: 3, EnvVersion: "OPENJDK_VERSION"}

	if parsedVersion, err := gs.SelectDependencyVersion(gs.OpenJdk); err != nil {
		gs.Log.Error("Unable to determine the Java version to install: %s", err.Error())
		return err
	}else{
		gs.OpenJdk.Version = parsedVersion
		gs.OpenJdk.RuntimeLocation = gs.EvalRuntimeLocation(gs.OpenJdk)
		gs.OpenJdk.StagingLocation = gs.EvalStagingLocation(gs.OpenJdk)
	}

	if err := gs.InstallDependency(gs.OpenJdk); err != nil {
		gs.Log.Error("Error installing Java: %s", err.Error())
		return err
	}

	content = TrimLines(fmt.Sprintf(`
				export JAVA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JAVA_HOME/bin
				`, gs.OpenJdk.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.OpenJdk, content); err != nil {
		gs.Log.Error("Error writing profile.d script for JDK: %s", err.Error())
		return err
	}


	//Install Logstash
	gs.Logstash = Dependency{Name: "logstash", VersionParts: 3, EnvVersion: "LOGSTASH_VERSION"}

	if parsedVersion, err := gs.SelectDependencyVersion(gs.Logstash); err != nil {
		gs.Log.Error("Unable to determine the Logstash version to install: %s", err.Error())
		return err
	}else{
		gs.Logstash.Version = parsedVersion
		gs.Logstash.RuntimeLocation = gs.EvalRuntimeLocation(gs.Logstash)
		gs.Logstash.StagingLocation = gs.EvalStagingLocation(gs.Logstash)
	}

	if err := gs.InstallDependency(gs.Logstash); err != nil {
		gs.Log.Error("Error installing Logstash: %s", err.Error())
		return err
	}

	content = TrimLines(fmt.Sprintf(`
				export LOGSTASH_HOME=$DEPS_DIR/%s
				PATH=$PATH:$LOGSTASH_HOME/bin
				`, gs.Logstash.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Logstash, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Logstash: %s", err.Error())
		return err
	}


	//WriteConfigYml
	config := map[string]string{
		"LogstashVersion":  gs.Logstash.Version,
	}

	if err:= gs.Stager.WriteConfigYml(config); err != nil {
		gs.Log.Error("Error writing config.yml: %s", err.Error())
		return err
	}

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




// GENERAL

func TrimLines(text string) string{
	re := regexp.MustCompile("(?m)^(\\s)*")
	return re.ReplaceAllString(text, "")
}

func (gs *Supplier) WriteDependencyProfileD(dependency Dependency, content string) error {

	if err := gs.Stager.WriteProfileD(dependency.Name + ".sh", content ); err != nil {
		return err
	}
	return nil
}

func (gs *Supplier) SelectDependencyVersion(dependency Dependency) (string, error) {

	dependencyVersion := os.Getenv(dependency.EnvVersion)

	if dependencyVersion == "" {
		defaultDependencyVersion, err := gs.Manifest.DefaultVersion(dependency.Name)
		if err != nil {
			return "", err
		}
		dependencyVersion = defaultDependencyVersion.Version
	}

	return gs.parseDependencyVersion(dependency,dependencyVersion )
}

func (gs *Supplier) parseDependencyVersion(dependency Dependency, partialDependencyVersion string) (string, error) {
	existingVersions := gs.Manifest.AllDependencyVersions(dependency.Name)

	if len(strings.Split(partialDependencyVersion, ".")) < dependency.VersionParts {
		partialDependencyVersion += ".x"
	}

	expandedVer, err := libbuildpack.FindMatchingVersion(partialDependencyVersion, existingVersions)
	if err != nil {
		return "", err
	}

	return expandedVer, nil
}

func (gs *Supplier) EvalRuntimeLocation (dependency Dependency) (string){
	return filepath.Join(gs.Stager.DepsIdx(), dependency.Name + "-" + dependency.Version)
}

func (gs *Supplier) EvalStagingLocation (dependency Dependency) (string){
	return filepath.Join(gs.Stager.DepDir(), dependency.Name + "-" + dependency.Version)
}

func (gs *Supplier) InstallDependency(dependency Dependency) error {

	dep := libbuildpack.Dependency{Name: dependency.Name, Version: dependency.Version }
	if err := gs.Manifest.InstallDependency(dep, dependency.StagingLocation); err != nil {
		return err
	}

	return nil
}
