package supply

import (
	"github.com/cloudfoundry/libbuildpack"
	"os"
	"path/filepath"
	"strings"

	"fmt"
	"io/ioutil"
	conf "logstash/config"

	"errors"
	"logstash/util"
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
	CacheDir() string
	DepDir() string
	DepsIdx() string
	WriteConfigYml(interface{}) error
	WriteEnvFile(string, string) error
	WriteProfileD(string, string) error
}

type Supplier struct {
	Stager             Stager
	Manifest           Manifest
	Log                *libbuildpack.Logger
	BuildpackDir       string
	GTE                Dependency
	Jq                 Dependency
	Ofelia             Dependency
	Curator            Dependency
	OpenJdk            Dependency
	Logstash           Dependency
	LogstashPlugins    Dependency
	LogstashConfig     conf.LogstashConfig
	TemplatesConfig    conf.TemplatesConfig
	VcapApp            conf.VcapApp
	VcapServices       conf.VcapServices
	ConfigFilesExists  bool
	CuratorFilesExists bool
	TemplatesToInstall []conf.Template
	PluginsToInstall   map[string]string
}

type Dependency struct {
	Name            string
	Version         string
	VersionParts    int
	ConfigVersion   string
	RuntimeLocation string
	StagingLocation string
}

func Run(gs *Supplier) error {

	//Eval Logstash file and prepare dir structure
	if err := gs.EvalLogstashFile(); err != nil {
		gs.Log.Error("Unable to evaluate Logstash file: %s", err.Error())
		return err
	}

	if err := gs.PrepareAppDirStructure(); err != nil {
		gs.Log.Error("Unable to prepare directory structure for the app: %s", err.Error())
		return err
	}

	//Eval Templates file
	if err := gs.EvalTemplatesFile(); err != nil {
		gs.Log.Error("Unable to evaluate Templates file: %s", err.Error())
		return err
	}

	//Eval Environment
	if err := gs.EvalEnvironment(); err != nil {
		gs.Log.Error("Unable to evaluate environment: %s", err.Error())
		return err
	}

	//Install Dependencies
	if err := gs.InstallDependencyGTE(); err != nil {
		return err
	}
	if err := gs.InstallDependencyJq(); err != nil {
		return err
	}
	if gs.LogstashConfig.Curator.Install {
		if err := gs.InstallDependencyOfelia(); err != nil {
			return err
		}
		if err := gs.InstallDependencyCurator(); err != nil {
			return err
		}

	}

	if err := gs.InstallDependencyOpenJdk(); err != nil {
		return err
	}

	if err := gs.InstallDependencyLogstashPlugins(); err != nil {
		return err
	}

	//Prepare Staging Environment
	if err := gs.PrepareStagingEnvironment(); err != nil {
		return err
	}

	//Install templates
	if err := gs.InstallTemplates(); err != nil {
		gs.Log.Error("Unable to install template file: %s", err.Error())
		return err
	}

	//Install User Certificates
	if err := gs.InstallUserCertificates(); err != nil {
		return err
	}

	//Install Curator/Ofelia
	if err := gs.PrepareCurator(); err != nil {
		return err
	}

	//Install Logstash
	if err := gs.InstallLogstash(); err != nil {
		return err
	}

	//Install Logstash Plugins
	if err := gs.InstallLogstashPlugins(); err != nil {
		return err
	}

	if gs.LogstashConfig.ConfigCheck {
		//Install Logstash Plugins
		if err := gs.CheckLogstash(); err != nil {
			return err
		}

	}
	//WriteConfigYml
	config := map[string]string{
		"LogstashVersion": gs.Logstash.Version,
	}

	if err := gs.Stager.WriteConfigYml(config); err != nil {
		gs.Log.Error("Error writing config.yml: %s", err.Error())
		return err
	}

	return nil
}

func (gs *Supplier) BPDir() string {

	return gs.BuildpackDir
}

func (gs *Supplier) EvalLogstashFile() error {
	gs.LogstashConfig = conf.LogstashConfig{
		Set:            true,
		ConfigCheck:    false,
		ReservedMemory: 300,
		HeapPercentage: 90,
		Curator:        conf.Curator{Set: true, Install: false}}

	logstashFile := filepath.Join(gs.Stager.BuildDir(), "Logstash")

	data, err := ioutil.ReadFile(logstashFile)
	if err != nil {
		return err
	}
	if err := gs.LogstashConfig.Parse(data); err != nil {
		return err
	}

	if !gs.LogstashConfig.Set {
		gs.LogstashConfig.HeapPercentage = 90
		gs.LogstashConfig.ReservedMemory = 300
		gs.LogstashConfig.ConfigCheck = true
	}
	if !gs.LogstashConfig.Curator.Set {
		gs.LogstashConfig.Curator.Install = false //not really needed but maybe we will switch to true later
	}

	//ToDo Eval values
	if gs.LogstashConfig.Curator.Schedule == "" {
		gs.LogstashConfig.Curator.Schedule = "@daily"
	}

	return nil
}

func (gs *Supplier) PrepareAppDirStructure() error {

	//create dir conf.d in DepDir
	dir := filepath.Join(gs.Stager.DepDir(), "conf.d")
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir logstash.conf.d in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "logstash.conf.d")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir grok-patterns in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "grok-patterns")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir mappings in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "mappings")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir plugins in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "plugins")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir curator.d in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "curator.d")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir curator in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "curator")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	//create dir ofelia in DepDir
	dir = filepath.Join(gs.Stager.DepDir(), "ofelia")
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		return err
	}

	return nil
}

func (gs *Supplier) EvalTemplatesFile() error {
	gs.TemplatesConfig = conf.TemplatesConfig{}

	templateFile := filepath.Join(gs.BPDir(), "defaults/templates/templates.yml")

	data, err := ioutil.ReadFile(templateFile)
	if err != nil {
		return err
	}
	if err := gs.TemplatesConfig.Parse(data); err != nil {
		return err
	}

	return nil
}

func (gs *Supplier) EvalEnvironment() error {

	//get VCAP_APPLICATIOM
	gs.VcapApp = conf.VcapApp{}
	dataApp := os.Getenv("VCAP_APPLICATION")
	if err := gs.VcapApp.Parse([]byte(dataApp)); err != nil {
		return err
	}

	// get VCAP_SERVICES
	gs.VcapServices = conf.VcapServices{}
	dataServices := os.Getenv("VCAP_SERVICES")
	if err := gs.VcapServices.Parse([]byte(dataServices)); err != nil {
		return err
	}

	//check if files (also directories) exist in the application's "conf.d" directory
	configDir := filepath.Join(gs.Stager.BuildDir(), "conf.d")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		gs.ConfigFilesExists = false
		return nil
	}

	files, err := ioutil.ReadDir(configDir)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		gs.ConfigFilesExists = true
	}

	//check if curator files (also directories) exist in the application's "curator.d" directory
	curatorDir := filepath.Join(gs.Stager.BuildDir(), "curator.d")
	if _, err := os.Stat(curatorDir); os.IsNotExist(err) {
		gs.CuratorFilesExists = false
		return nil
	}

	curatorFiles, err := ioutil.ReadDir(curatorDir)
	if err != nil {
		return err
	}
	if len(curatorFiles) > 0 {
		gs.CuratorFilesExists = true
	}

	return nil
}

func (gs *Supplier) InstallDependencyGTE() error {
	gs.GTE = Dependency{Name: "gte", VersionParts: 3, ConfigVersion: ""}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.GTE); err != nil {
		gs.Log.Error("Unable to determine the GTE version to install: %s", err.Error())
		return err
	} else {
		gs.GTE.Version = parsedVersion
		gs.GTE.RuntimeLocation = gs.EvalRuntimeLocation(gs.GTE)
		gs.GTE.StagingLocation = gs.EvalStagingLocation(gs.GTE)
	}

	if err := gs.InstallDependency(gs.GTE); err != nil {
		gs.Log.Error("Error installing GTE: %s", err.Error())
		return err
	}

	content := util.TrimLines(fmt.Sprintf(`
				export GTE_HOME=$DEPS_DIR/%s
				PATH=$PATH:$GTE_HOME
				`, gs.GTE.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.GTE, content); err != nil {
		gs.Log.Error("Error writing profile.d script for GTE: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallDependencyJq() error {
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

	content := util.TrimLines(fmt.Sprintf(`
				export JQ_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JQ_HOME
				`, gs.Jq.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Jq, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Jq: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallDependencyOfelia() error {
	gs.Ofelia = Dependency{Name: "ofelia", VersionParts: 3, ConfigVersion: ""}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Ofelia); err != nil {
		gs.Log.Error("Unable to determine the Ofelia version to install: %s", err.Error())
		return err
	} else {
		gs.Ofelia.Version = parsedVersion
		gs.Ofelia.RuntimeLocation = gs.EvalRuntimeLocation(gs.Ofelia)
		gs.Ofelia.StagingLocation = gs.EvalStagingLocation(gs.Ofelia)
	}

	if err := gs.InstallDependency(gs.Ofelia); err != nil {
		gs.Log.Error("Error installing Ofelia: %s", err.Error())
		return err
	}

	content := util.TrimLines(fmt.Sprintf(`
				export OFELIA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$OFELIA_HOME
				`, gs.Ofelia.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Ofelia, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Ofelia: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallDependencyCurator() error {
	gs.Curator = Dependency{Name: "curator", VersionParts: 3, ConfigVersion: gs.LogstashConfig.Curator.Version}
	if parsedVersion, err := gs.SelectDependencyVersion(gs.Curator); err != nil {
		gs.Log.Error("Unable to determine the Curator version to install: %s", err.Error())
		return err
	} else {
		gs.Curator.Version = parsedVersion
		gs.Curator.RuntimeLocation = gs.EvalRuntimeLocation(gs.Curator)
		gs.Curator.StagingLocation = gs.EvalStagingLocation(gs.Curator)
	}

	if err := gs.InstallDependency(gs.Curator); err != nil {
		gs.Log.Error("Error installing Curator: %s", err.Error())
		return err
	}

	content := util.TrimLines(fmt.Sprintf(`
				export CURATOR_HOME=$DEPS_DIR/%s
				PATH=${CURATOR_HOME}/python3/bin:${CURATOR_HOME}/curator/bin:${PATH}
				`, gs.Curator.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Curator, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Curator: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) PrepareCurator() error {

	//create Curator start script
	content := util.TrimLines(fmt.Sprintf(`
				#!/bin/bash
				export PYTHONHOME=${CURATOR_HOME}/python3
				export PYTHONPATH=${CURATOR_HOME}/curator/lib/python3.4/site-packages
				export LC_ALL=en_US.UTF-8
				export LANG=en_US.UTF-8
				export PATH=${CURATOR_HOME}/python3/bin:${CURATOR_HOME}/curator/bin:${PATH}
				${CURATOR_HOME}/python3/bin/python3 ${CURATOR_HOME}/curator/bin/curator --config ${HOME}/curator.d/curator.yml ${HOME}/curator.d/actions.yml
				`))

	err := ioutil.WriteFile(filepath.Join(gs.Stager.DepDir(), "curator", "curator.sh"), []byte(content), 0755)
	if err != nil {
		gs.Log.Error("Unable to create Curator start script: %s", err.Error())
		return err
	}

	//create Curator start script
	content = util.TrimLines(fmt.Sprintf(`
				[job-local "curator"]
				schedule = %s
				command = {{- .Env.HOME -}}/bin/curator.sh
				`,
		gs.LogstashConfig.Curator.Schedule))

	err = ioutil.WriteFile(filepath.Join(gs.Stager.DepDir(), "ofelia", "schedule.ini"), []byte(content), 0644)
	if err != nil {
		gs.Log.Error("Unable to create Ofelia schedule.ini: %s", err.Error())
		return err
	}

	// pre-processing of curator config templates if no user files exist
	if !gs.CuratorFilesExists {

		templateFile := filepath.Join(gs.BPDir(), "defaults/curator")
		destFile := filepath.Join(gs.Stager.DepDir(), "curator.d")

		err := exec.Command(fmt.Sprintf("%s/gte", gs.GTE.StagingLocation), "-d", "<<:>>", templateFile, destFile).Run()
		if err != nil {
			gs.Log.Error("Error pre-processing curator config templates: %s", err.Error())
			return err
		}

	}

	return nil
}

func (gs *Supplier) InstallDependencyOpenJdk() error {
	gs.OpenJdk = Dependency{Name: "openjdk", VersionParts: 3, ConfigVersion: ""}

	if parsedVersion, err := gs.SelectDependencyVersion(gs.OpenJdk); err != nil {
		gs.Log.Error("Unable to determine the Java version to install: %s", err.Error())
		return err
	} else {
		gs.OpenJdk.Version = parsedVersion
		gs.OpenJdk.RuntimeLocation = gs.EvalRuntimeLocation(gs.OpenJdk)
		gs.OpenJdk.StagingLocation = gs.EvalStagingLocation(gs.OpenJdk)
	}

	if err := gs.InstallDependency(gs.OpenJdk); err != nil {
		gs.Log.Error("Error installing Java: %s", err.Error())
		return err
	}

	content := util.TrimLines(fmt.Sprintf(`
				export JAVA_HOME=$DEPS_DIR/%s
				PATH=$PATH:$JAVA_HOME/bin
				`, gs.OpenJdk.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.OpenJdk, content); err != nil {
		gs.Log.Error("Error writing profile.d script for JDK: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) InstallDependencyLogstashPlugins() error {

	//Install logstash-plugins from S3
	gs.LogstashPlugins = Dependency{Name: "logstash-plugins", VersionParts: 3, ConfigVersion: gs.LogstashConfig.Version} //same version as Logstash
	if parsedVersion, err := gs.SelectDependencyVersion(gs.LogstashPlugins); err != nil {
		gs.Log.Error("Unable to determine the version of the default Logstash Plugins: %s", err.Error())
		return err
	} else {
		gs.LogstashPlugins.Version = parsedVersion
		gs.LogstashPlugins.RuntimeLocation = gs.EvalRuntimeLocation(gs.LogstashPlugins)
		gs.LogstashPlugins.StagingLocation = gs.EvalStagingLocation(gs.LogstashPlugins)
	}

	if err := gs.InstallDependency(gs.LogstashPlugins); err != nil {
		gs.Log.Error("Error installing the default Logstash Plugins: %s", err.Error())
		return err
	}

	return nil
}

func (gs *Supplier) InstallLogstash() error {
	gs.Logstash = Dependency{Name: "logstash", VersionParts: 3, ConfigVersion: gs.LogstashConfig.Version}

	if parsedVersion, err := gs.SelectDependencyVersion(gs.Logstash); err != nil {
		gs.Log.Error("Unable to determine the Logstash version to install: %s", err.Error())
		return err
	} else {
		gs.Logstash.Version = parsedVersion
		gs.Logstash.RuntimeLocation = gs.EvalRuntimeLocation(gs.Logstash)
		gs.Logstash.StagingLocation = gs.EvalStagingLocation(gs.Logstash)
	}

	if err := gs.InstallDependency(gs.Logstash); err != nil {
		gs.Log.Error("Error installing Logstash: %s", err.Error())
		return err
	}

	curatorEnabled := ""
	if gs.LogstashConfig.Curator.Install {
		curatorEnabled = "enabled"
	}
	content := util.TrimLines(fmt.Sprintf(`
			export LS_BP_RESERVED_MEMORY=%d
			export LS_BP_HEAP_PERCENTAGE=%d
			export LS_BP_JAVA_OPTS=%s
			export LS_CMD_ARGS=%s
			export LS_ROOT=$DEPS_DIR/%s
			export LS_CURATOR_ENABLED=%s
			export LOGSTASH_HOME=$DEPS_DIR/%s
			PATH=$PATH:$LOGSTASH_HOME/bin
			`,
		gs.LogstashConfig.ReservedMemory,
		gs.LogstashConfig.HeapPercentage,
		gs.LogstashConfig.JavaOpts,
		gs.LogstashConfig.CmdArgs,
		gs.Stager.DepsIdx(),
		curatorEnabled,
		gs.Logstash.RuntimeLocation))

	if err := gs.WriteDependencyProfileD(gs.Logstash, content); err != nil {
		gs.Log.Error("Error writing profile.d script for Logstash: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) PrepareStagingEnvironment() error {
	vmOptions := gs.LogstashConfig.JavaOpts

	if vmOptions != "" {
		os.Setenv("LS_JAVA_OPTS", fmt.Sprintf("%s", vmOptions))
	} else {
		mem := (gs.VcapApp.Limits.Mem - gs.LogstashConfig.ReservedMemory) / 100 * gs.LogstashConfig.HeapPercentage
		os.Setenv("LS_JAVA_OPTS", fmt.Sprintf("-Xmx%dm -Xms%dm", mem, mem))
	}

	os.Setenv("JAVA_HOME", gs.OpenJdk.StagingLocation)
	os.Setenv("PATH", os.Getenv("PATH")+":"+gs.OpenJdk.StagingLocation+"/bin")
	os.Setenv("PORT", "8080") //dummy PORT: used by template processing for logstash check

	if strings.ToLower(gs.LogstashConfig.LogLevel) == "debug" {
		gs.Log.Info(" ### JAVA_HOME %s", os.Getenv("JAVA_HOME"))
		gs.Log.Info(" ### PATH %s", os.Getenv("PATH"))
		gs.Log.Info(" ### LS_JAVA_OPTS %s", os.Getenv("LS_JAVA_OPTS"))
	}
	return nil
}

func (gs *Supplier) InstallUserCertificates() error {

	if len(gs.LogstashConfig.Certificates) == 0 { // no certificates to install
		return nil
	}

	localCerts, _ := gs.ReadLocalCertificates(gs.Stager.BuildDir() + "/certificates")

	for i := 0; i < len(gs.LogstashConfig.Certificates); i++ {

		localCert := localCerts[gs.LogstashConfig.Certificates[i]]

		if localCert != "" {
			gs.Log.Info(fmt.Sprintf("----> installing user certificate '%s' to TrustStore ... ", gs.LogstashConfig.Certificates[i]))
			certToInstall := gs.Stager.BuildDir() + "/certificates/" + localCert
			out, err := exec.Command(fmt.Sprintf("%s/bin/keytool", gs.OpenJdk.StagingLocation), "-import", "-trustcacerts", "-keystore", fmt.Sprintf("%s/jre/lib/security/cacerts", gs.OpenJdk.StagingLocation), "-storepass", "changeit", "-noprompt", "-alias", gs.LogstashConfig.Certificates[i], "-file", certToInstall).CombinedOutput()
			gs.Log.Info(string(out))
			if err != nil {
				gs.Log.Warning("Error installing user certificate '%s' to TrustStore: %s", gs.LogstashConfig.Certificates[i], err.Error())
			}
		} else {
			err := errors.New("crt file for certificate not found in directory")
			gs.Log.Error("File %s.crt not found in directory '/certificates'", gs.LogstashConfig.Certificates[i])
			return err
		}
	}

	return nil

}

func (gs *Supplier) InstallTemplates() error {

	gs.TemplatesToInstall = []conf.Template{}

	if !gs.ConfigFilesExists && len(gs.LogstashConfig.ConfigTemplates) == 0 {
		// install all default templates

		//copy default templates to config
		for _, t := range gs.TemplatesConfig.Templates {

			if t.IsDefault {

				if len(t.Tags) > 0 {
					servicesWithTag := gs.VcapServices.WithTags(t.Tags)

					if len(servicesWithTag) == 0 {

						if gs.LogstashConfig.EnableServiceFallback {
							ti := t
							ti.ServiceInstanceName = ""
							gs.TemplatesToInstall = append(gs.TemplatesToInstall, ti)
							gs.Log.Warning("No service found for template %s, will do the fallback. Please bind a service and restage the app", ti.Name)
						} else {
							return errors.New("no service found for template")
						}
					} else if len(servicesWithTag) > 1 {
						return errors.New("more than one service found for template")
					} else {
						ti := t
						ti.ServiceInstanceName = servicesWithTag[0].Name
						gs.TemplatesToInstall = append(gs.TemplatesToInstall, ti)
					}
				} else {
					ti := t
					ti.ServiceInstanceName = ""
					gs.TemplatesToInstall = append(gs.TemplatesToInstall, ti)
				}
			}
		}

	} else {
		//only install explicitly defined templates, if any
		//check them all

		for _, ct := range gs.LogstashConfig.ConfigTemplates {
			found := false
			templateName := strings.Trim(ct.Name, " ")
			if len(templateName) == 0 {
				gs.Log.Warning("Skipping template: no valid name defined for template in Logstash file")
				continue
			}
			for _, t := range gs.TemplatesConfig.Templates {
				if templateName == t.Name {
					serviceInstanceName := strings.Trim(ct.ServiceInstanceName, " ")
					if len(serviceInstanceName) == 0 && len(t.Tags) > 0 {
						gs.Log.Error("No service instance name defined for template %s in Logstash file", templateName)
						return errors.New("no service instance name defined for template in Logstash file")
					}

					ti := t
					if len(serviceInstanceName) > 0 && len(t.Tags) == 0 {
						gs.Log.Warning("Service instance name '%s' is defined for template %s in Logstash file but template can not be bound to a service.", serviceInstanceName, templateName)
					} else {
						ti.ServiceInstanceName = serviceInstanceName
					}
					gs.TemplatesToInstall = append(gs.TemplatesToInstall, ti)

					found = true
					break
				}
			}
			if !found {
				gs.Log.Warning("Template %s defined in Logstash file does not exist", templateName)
			}
		}
	}

	//copy templates --> conf.d
	for _, ti := range gs.TemplatesToInstall {

		os.Setenv("SERVICE_INSTANCE_NAME", ti.ServiceInstanceName)
		templateFile := filepath.Join(gs.BPDir(), "defaults/templates/", ti.Name+".conf")
		destFile := filepath.Join(gs.Stager.DepDir(), "conf.d", ti.Name+".conf")

		err := exec.Command(fmt.Sprintf("%s/gte", gs.GTE.StagingLocation), "-d", "<<:>>", templateFile, destFile).Run()
		if err != nil {
			gs.Log.Error("Error pre-processing template %s: %s", ti.Name, err.Error())
			return err
		}

	}

	// copy grok-patterns, mappings and plugins
	var mappingsToInstall map[string]string
	var groksToInstall map[string]string

	mappingsToInstall = make(map[string]string)
	groksToInstall = make(map[string]string)
	gs.PluginsToInstall = make(map[string]string)

	for i := 0; i < len(gs.TemplatesToInstall); i++ {

		for m := 0; m < len(gs.TemplatesToInstall[i].Mappings); m++ {
			mappingsToInstall[gs.TemplatesToInstall[i].Mappings[m]] = ""
		}
		for g := 0; g < len(gs.TemplatesToInstall[i].Groks); g++ {
			groksToInstall[gs.TemplatesToInstall[i].Groks[g]] = ""
		}
		for p := 0; p < len(gs.TemplatesToInstall[i].Plugins); p++ {
			gs.PluginsToInstall[gs.TemplatesToInstall[i].Plugins[p]] = ""
		}
	}

	for key, _ := range mappingsToInstall {
		mappingFile := filepath.Join(gs.BPDir(), "defaults/mappings", key)
		destFile := filepath.Join(gs.Stager.DepDir(), "mappings", key)

		out, err := exec.Command(fmt.Sprintf("%s/gte", gs.GTE.StagingLocation), "-d", "<<:>>", mappingFile, destFile).CombinedOutput()
		if err != nil {
			gs.Log.Error(string(out))
			gs.Log.Error("Error pre-processing mapping template %s: %s", key, err.Error())
			return err
		}
	}

	for key, _ := range groksToInstall {
		grokFile := filepath.Join(gs.BPDir(), "defaults/grok-patterns", key)
		destFile := filepath.Join(gs.Stager.DepDir(), "grok-patterns", key)

		err := exec.Command(fmt.Sprintf("%s/gte", gs.GTE.StagingLocation), "-d", "<<:>>", grokFile, destFile).Run()
		if err != nil {
			gs.Log.Error("Error pre-processing grok-patterns template %s: %s", key, err.Error())
			return err
		}
	}

	//default Plugins will be installed in method "InstallLogstashPlugin"

	return nil
}

func (gs *Supplier) InstallLogstashPlugins() error {

	//copy the user defined plugins to the PluginsToInstall map which may already have the default plugins in it
	for i := 0; i < len(gs.LogstashConfig.Plugins); i++ {
		gs.PluginsToInstall[gs.LogstashConfig.Plugins[i]] = ""
	}

	if len(gs.PluginsToInstall) == 0 { // no plugins to install at all
		return nil
	}

	defaultPlugins, _ := gs.ReadLocalPlugins(gs.LogstashPlugins.StagingLocation)
	userPlugins, _ := gs.ReadLocalPlugins(gs.Stager.BuildDir() + "/plugins")

	gs.Log.Info("----> Installing Logstash plugins ...")
	for key, _ := range gs.PluginsToInstall {
		//Priorisation
		pluginToInstall := key // Prio 3 (online installation)
		defaultPlugin := gs.GetLocalPlugin(key, defaultPlugins)
		userPlugin := ""
		if defaultPlugin != "" {
			pluginToInstall = filepath.Join(gs.LogstashPlugins.StagingLocation, defaultPlugin) // Prio 1 (offline installation)
		} else {
			userPlugin = gs.GetLocalPlugin(key, userPlugins)
			if userPlugin != "" {
				pluginToInstall = filepath.Join(gs.Stager.BuildDir(), "plugins", userPlugin) // Prio 2 (offline installation)
			}
		}

		//Install Plugin
		out, err := exec.Command(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation), "install", pluginToInstall).CombinedOutput()
		if err != nil {
			gs.Log.Error(string(out))
			gs.Log.Error("Error installing Logstash plugin %s: %s", key, err.Error())
			return err
		}
	}

	gs.Log.Info("----> Listing all installed Logstash plugins ...")

	cmd := exec.Command(fmt.Sprintf("%s/bin/logstash-plugin", gs.Logstash.StagingLocation), "list", "--verbose")
	err := cmd.Run()
	if err != nil {
		gs.Log.Error("Error listing all installed Logstash plugins: %s", err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) CheckLogstash() error {

	gs.Log.Info("----> Starting Logstash config check...")

	// template processing for check
	templateDir := filepath.Join(gs.Stager.DepDir(), "conf.d")
	destDir := filepath.Join(gs.Stager.DepDir(), "logstash.conf.d")
	err := exec.Command(fmt.Sprintf("%s/gte", gs.GTE.StagingLocation), templateDir, destDir).Run()
	if err != nil {
		gs.Log.Error("Error processing templates for Logstash config check: %s", err.Error())
		return err
	}

	// list files in logstash.conf.d
	file, err := os.Open(destDir)
	if err != nil {
		gs.Log.Error("  --> failed opening logstash.conf.d directory: %s", err)
		return err
	}
	defer file.Close()

	gs.Log.Info("  --> Listing files in logstash.conf.d directory ...")
	list, _ := file.Readdirnames(0) // 0 to read all files
	found := false
	for _, name := range list {
		found = true
		gs.Log.Info("      " + name)
	}
	if !found {
		gs.Log.Warning("      " + "no files found")
	}

	gs.Log.Info("  --> Checking Logstash config ...")
	// check logstash config
	out, err := exec.Command(fmt.Sprintf("%s/bin/logstash", gs.Logstash.StagingLocation), "-f", destDir, "-t").CombinedOutput()
	gs.Log.Info(string(out))
	if err != nil {
		gs.Log.Error("Error checking Logstash config: %s", err.Error())
		return err
	}

	gs.Log.Info("  --> Finished Logstash config check...")

	return nil
}

// GENERAL

func (gs *Supplier) WriteDependencyProfileD(dependency Dependency, content string) error {

	if err := gs.Stager.WriteProfileD(dependency.Name+".sh", content); err != nil {
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

	return gs.parseDependencyVersion(dependency, dependencyVersion)
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

func (gs *Supplier) EvalRuntimeLocation(dependency Dependency) string {
	return filepath.Join(gs.Stager.DepsIdx(), dependency.Name+"-"+dependency.Version)
}

func (gs *Supplier) EvalStagingLocation(dependency Dependency) string {
	return filepath.Join(gs.Stager.DepDir(), dependency.Name+"-"+dependency.Version)
}

func (gs *Supplier) InstallDependency(dependency Dependency) error {

	dep := libbuildpack.Dependency{Name: dependency.Name, Version: dependency.Version}

	//Check Cache

	if err := gs.Manifest.InstallDependency(dep, dependency.StagingLocation); err != nil {
		return err
	}

	return nil
}

func (gs *Supplier) ReadLocalCertificates(filePath string) (map[string]string, error) {

	var localCerts map[string]string
	localCerts = make(map[string]string)

	file, err := os.Open(filePath)
	if err != nil {
		gs.Log.Error("failed opening certificates directory: %s", err)
		return localCerts, err
	}
	defer file.Close()

	list, _ := file.Readdirnames(0) // 0 to read all files and folders
	for _, name := range list {

		if strings.HasSuffix(name, ".crt") {
			certParts := strings.Split(name, ".crt")

			if len(certParts) == 2 {
				certName := certParts[0]
				localCerts[certName] = name
			}

		}
	}

	return localCerts, nil
}

func (gs *Supplier) ReadLocalPlugins(filePath string) ([]string, error) {

	file, err := os.Open(filePath)
	if err != nil {
		gs.Log.Error("failed opening directory: %s", err)
		return nil, err
	}
	defer file.Close()

	list, _ := file.Readdirnames(0) // 0 to read all files and folders

	return list, nil
}

func (gs *Supplier) GetLocalPlugin(pluginName string, pluginFileNames []string) string {

	for i := 0; i < len(pluginFileNames); i++ {
		if strings.HasPrefix(pluginFileNames[i], pluginName) {
			return pluginFileNames[i]
		}
	}

	return ""
}
