package supply

import (

	"os"
	"path/filepath"
	"strings"
	"github.com/cloudfoundry/libbuildpack"

	"fmt"
	"regexp"
	conf "logstash/config"
	"io/ioutil"

	"os/exec"

	"log"
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
	MemoryCalculator Dependency
	LogstashConfig conf.LogstashConfig
	App conf.App
}

type Dependency struct{
	Name string
	Version string
	VersionParts int
	ConfigVersion string
	RuntimeLocation string
	StagingLocation string
}

func Run(gs *Supplier) error {

	//Eval Logstash file
	if err := gs.EvalLogstashFile(); err != nil {
		gs.Log.Error("Unable to evaluate Logstash file: %s", err.Error())
		return err
	}

	//Eval Environment
	if err := gs.EvalEnvironment(); err != nil {
		gs.Log.Error("Unable to evaluate environment: %s", err.Error())
		return err
	}

	//Install Memory Calculator
	if err := gs.InstallMemoryCalculator(); err != nil {
		return err
	}

	//Install Dockerize
	if err := gs.InstallDockerize(); err != nil {
		return err
	}

	//Install JQ
	if err := gs.InstallJq(); err != nil {
		return err
	}

	if gs.LogstashConfig.Curator.Install{

		//Install Ofelia
		if err := gs.InstallOfelia(); err != nil {
			return err
		}

		//Install Curator
		if err := gs.InstallCurator(); err != nil {
			return err
		}

	}


	//Install OpenJDK
	if err := gs.InstallOpenJdk(); err != nil {
		return err
	}

	//Install Logstash
	if err := gs.InstallLogstash(); err != nil {
		return err
	}


	//Prepare Stating Environment
	if err := gs.PrepareStatingEnvironment(); err != nil {
		return err
	}

	//Install Logstash Plugins
	if err := gs.InstallLogstashPlugins(); err != nil {
		return err
	}

	if gs.LogstashConfig.Logstash.ConfigCheck{
		//Install Logstash Plugins
		if err := gs.CheckLogstash(); err != nil {
			return err
		}

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


func (gs *Supplier) EvalLogstashFile() error {
	gs.LogstashConfig = conf.LogstashConfig{
		Logstash: conf.Logstash{ConfigCheck: true, MemoryCalculation: conf.MemoryCalculation{NumberClasses: 30000, NumberThreads: 30}},
		Curator:	conf.Curator{Install: false}}

	logstashFile := filepath.Join(gs.Stager.BuildDir(), "Logstash")

	data, err := ioutil.ReadFile(logstashFile)
	if err != nil {
		return err
	}
	if err := gs.LogstashConfig.Parse(data); err != nil {
		return err
	}


	//ToDo Eval values
	if gs.LogstashConfig.Curator.Schedule == "" {
		gs.LogstashConfig.Curator.Schedule = "@daily"
	}

	return nil
}

func (gs *Supplier) EvalEnvironment() error {
	gs.App = conf.App{}

	data := os.Getenv("VCAP_APPLICATION")

	if err := gs.App.Parse([]byte(data)); err != nil {
		return err
	}

	return nil
}

func (gs *Supplier) InstallDockerize() error {
	gs.Dockerize = Dependency{Name: "dockerize", VersionParts: 3, ConfigVersion: ""}
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
	return nil
}

func (gs *Supplier) InstallMemoryCalculator() error {
	gs.MemoryCalculator = Dependency{Name: "memory-calculator", VersionParts: 3, ConfigVersion: ""}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.MemoryCalculator); err != nil {
		gs.Log.Error("Unable to determine the Memory-Calculator version to install: %s", err.Error())
		return err
	}else{
		gs.MemoryCalculator.Version = parsedVersion
		gs.MemoryCalculator.RuntimeLocation = gs.EvalRuntimeLocation(gs.MemoryCalculator)
		gs.MemoryCalculator.StagingLocation = gs.EvalStagingLocation(gs.MemoryCalculator)
	}

	if err := gs.InstallDependency(gs.MemoryCalculator); err != nil {
		gs.Log.Error("Error installing Memory-Calculator: %s", err.Error())
		return err
	}

	content := TrimLines(fmt.Sprintf(`
				export MEMORY_CALCULATOR_HOME=$DEPS_DIR/%s
				PATH=$PATH:$MEMORY_CALCULATOR_HOME
				`, gs.MemoryCalculator.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.MemoryCalculator, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Memory-Calculator: %s", err.Error())
		return err
	}
	return nil

	// ps -fe | grep /0/[l]ogstash | awk '{print $2}' --> PID
	// ps -feT | grep /0/[l]ogstash | wc -l --> number of threads
	//  ./jmap -histo <PID> | wc -l --> number of classes
}

func (gs *Supplier) InstallJq() error {
	gs.Jq = Dependency{Name: "jq", VersionParts: 3, ConfigVersion: ""}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Jq); err != nil {
		gs.Log.Error("Unable to determine the Jq version to install: %s", err.Error())
		return err
	} else {
		gs.Jq.Version = parsedVersion
		gs.Jq.RuntimeLocation = gs.EvalRuntimeLocation(gs.Jq)
		gs.Jq.StagingLocation = gs.EvalStagingLocation(gs.Jq)
	}

	if err := gs.InstallDependency(gs.Jq); err != nil {
		gs.Log.Error("Error installing Jq: %s", err.Error())
		return err
	}

	content := TrimLines(fmt.Sprintf(`
				export JQ_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JQ_HOME
				`, gs.Jq.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Jq, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Jq: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallCurator() error {
	gs.Curator = Dependency{Name: "curator", VersionParts: 3, ConfigVersion: gs.LogstashConfig.Curator.Version}
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

	content := TrimLines(fmt.Sprintf(`
				export CURATOR_HOME=$DEPS_DIR/%s
				PATH=$PATH:$CURATOR_HOME
				`, gs.Curator.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Curator, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Curator: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallOfelia() error {
	gs.Ofelia = Dependency{Name: "ofelia", VersionParts: 3, ConfigVersion: ""}
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

	content := TrimLines(fmt.Sprintf(`
				export OFELIA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$OFELIA_HOME
				`, gs.Ofelia.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Ofelia, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Ofelia: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallOpenJdk() error {
	gs.OpenJdk = Dependency{Name: "openjdk", VersionParts: 3, ConfigVersion: ""}

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

	content := TrimLines(fmt.Sprintf(`
				export JAVA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JAVA_HOME/bin
				`, gs.OpenJdk.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.OpenJdk, content); err != nil {
		gs.Log.Error("Error writing profile.d script for JDK: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallLogstash() error {
	gs.Logstash = Dependency{Name: "logstash", VersionParts: 3, ConfigVersion: gs.LogstashConfig.Logstash.Version}

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

	content := TrimLines(fmt.Sprintf(`
				export LOGSTASH_HOME=$DEPS_DIR/%s
				PATH=$PATH:$LOGSTASH_HOME/bin
				`, gs.Logstash.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Logstash, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Logstash: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) PrepareStatingEnvironment() error {
	mem := gs.App.Limits.Mem / 10 * 8
	os.Setenv("JAVA_HOME", gs.OpenJdk.StagingLocation)
	os.Setenv("LS_JAVA_OPTS", fmt.Sprintf("-Xmx%dm -Xms%dm", mem ,mem ))

	gs.Log.Info("JAVA_HOME", gs.OpenJdk.StagingLocation)
	gs.Log.Info("LS_JAVA_OPTS", fmt.Sprintf("-Xmx%dm -Xms%dm", mem ,mem ))
	return nil
}

func (gs *Supplier) InstallLogstashPlugins() error {

	localPlugins, _ := gs.ReadLocalPlugins(gs.Stager.BuildDir() + "/plugins")

    for i := 0; i < len(gs.LogstashConfig.Logstash.Plugins); i++{

    	localPlugin := localPlugins[gs.LogstashConfig.Logstash.Plugins[i]]

    	pluginToInstall := gs.LogstashConfig.Logstash.Plugins[i]

    	if localPlugin != "" {
			pluginToInstall = gs.Stager.BuildDir() + "/plugins/" + localPlugin
		}
		cmd := exec.Command(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation), "install", pluginToInstall)
		gs.Log.Info(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation))

		err := cmd.Run()
		if err != nil{
			gs.Log.Error("Error installing Logstash plugin %s: %s", gs.LogstashConfig.Logstash.Plugins[i], err.Error())
			return  err
		}
		gs.Log.Info("Logstash plugin %s installed", gs.LogstashConfig.Logstash.Plugins[i])
	}
	return nil
}

func (gs *Supplier) CheckLogstash() error {

	localPlugins, _ := gs.ReadLocalPlugins(gs.Stager.BuildDir() + "/plugins")

	for i := 0; i < len(gs.LogstashConfig.Logstash.Plugins); i++{

		localPlugin := localPlugins[gs.LogstashConfig.Logstash.Plugins[i]]

		pluginToInstall := gs.LogstashConfig.Logstash.Plugins[i]

		if localPlugin != "" {
			pluginToInstall = gs.Stager.BuildDir() + "/plugins/" + localPlugin
		}
		cmd := exec.Command(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation), "install", pluginToInstall)
		gs.Log.Info(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation))

		err := cmd.Run()
		if err != nil{
			gs.Log.Error("Error installing Logstash plugin %s: %s", gs.LogstashConfig.Logstash.Plugins[i], err.Error())
			return  err
		}
		gs.Log.Info("Logstash plugin %s installed", gs.LogstashConfig.Logstash.Plugins[i])
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

	dependencyVersion := os.Getenv(dependency.ConfigVersion)

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

func fileExists(filePath string) (exists bool) {
	exists = true

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		exists = false
	}

	return
}

func readAllFiles(filePath string) error {

	file, err := os.Open(filePath)
	if err != nil {
		log.Fatalf("failed opening directory: %s", err)
	}
	defer file.Close()

	list,_ := file.Readdirnames(0) // 0 to read all files and folders
	for _, name := range list {
		fmt.Println(name)
	}

	return nil
}


func (gs *Supplier)ReadLocalPlugins(filePath string) (map[string]string, error) {

	var localPlugins map[string]string
	localPlugins = make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		gs.Log.Error("failed opening directory: %s", err)
		return localPlugins, err
	}
	defer file.Close()

	list,_ := file.Readdirnames(0) // 0 to read all files and folders
	for _, name := range list {
		pluginParts := strings.Split(name, "-")

		if len(pluginParts) == 4 {
			pluginName := pluginParts[0] + "-" + pluginParts[1] + "-" + pluginParts[2]
			localPlugins[pluginName] = name
		}
	}

	return localPlugins, nil
}