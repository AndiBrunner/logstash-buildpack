package supply

import (
	"os"
	"strings"
	"github.com/cloudfoundry/libbuildpack"
	"path/filepath"
	"io/ioutil"
	"fmt"
)


func (gs *Supplier) BPDir() string {
	return gs.BuildpackDir
}

func (gs *Supplier) ReadCachedDependencies() error {

	gs.CachedDepsByLocation = make(map[string]string)
	gs.CachedDepsByName = make(map[string]string)

	cacheDir, err := ioutil.ReadDir(gs.Stager.CacheDir())
	if err != nil {
		gs.Log.Error("  --> failed reading cache directory: %s", err)
		return err
	}

	for _, dirEntry := range cacheDir{
		if dirEntry.IsDir(){
			dirParts := strings.Split(dirEntry.Name(),"-")
			if len(dirParts) == 2 {
				gs.CachedDepsByLocation[dirEntry.Name()] = ""
				gs.CachedDepsByName[dirParts[0]] = dirParts[1]
			}
		}
	}

	return nil
}

func (gs *Supplier) NewDependency(name string, versionParts int, configVersion string) (Dependency, error){
	var dependency = Dependency{Name: name, VersionParts: versionParts, ConfigVersion: configVersion}

	if parsedVersion, err := gs.SelectDependencyVersion(dependency); err != nil {
		gs.Log.Error("Unable to determine the version of %s: %s", dependency, err.Error())
		return dependency, err
	} else {
		dependency.Version = parsedVersion
		dependency.DirName = dependency.Name+"-"+dependency.Version
		dependency.RuntimeLocation = gs.EvalRuntimeLocation(dependency)
		dependency.StagingLocation = gs.EvalStagingLocation(dependency)
		dependency.CacheLocation = gs.EvalCacheLocation(dependency)
	}

	return dependency, nil
}


func (gs *Supplier) WriteDependencyProfileD(dependency Dependency, content string) error {

	if err := gs.Stager.WriteProfileD(dependency.Name+".sh", content); err != nil {
		gs.Log.Error("Error writing profile.d script for %s: %s", dependency.Name,err.Error())
		return err
	}
	return nil
}

func (gs *Supplier) RemoveUnusedDependencies () error{

	for cacheLocation, value := range gs.CachedDepsByLocation{
		if value == "" {
			gs.Log.Info(fmt.Sprintf("--> deleting unused dependency '%s' from appliction cache", cacheLocation))
			os.RemoveAll(filepath.Join(gs.Stager.CacheDir(), cacheLocation))
		}
	}
	return nil
}

func (gs *Supplier) InstallDependency(dependency Dependency) error {

	dep := libbuildpack.Dependency{Name: dependency.Name, Version: dependency.Version}

	//Check Cache

	_, isDependencyInCache := gs.CachedDepsByLocation[dependency.DirName]

	for key, value := range gs.CachedDepsByLocation{
		gs.Log.Info("x> %s : %s", key, value)
	}
	gs.Log.Info("O> %s", dependency.DirName, isDependencyInCache)


	if !isDependencyInCache { //check for different version in cache and if so, delete from cache
		orphandVersion, orphandVersionFound := gs.CachedDepsByName[dependency.Name]

		if orphandVersionFound {
			orphandLocation := dependency.Name + "-" +orphandVersion
			gs.Log.Info(fmt.Sprintf("--> deleting unused dependency version '%s' from application cache", orphandLocation))
			os.RemoveAll(filepath.Join(gs.Stager.CacheDir(), orphandLocation))
			gs.CachedDepsByLocation[orphandLocation] = "deleted" //mark as deleted
		}
	}else{
		// mark as "in use"
		gs.CachedDepsByLocation[dependency.DirName] = "in use"
	}

	if !gs.Manifest.IsCached() && isDependencyInCache { //if online or non-cached system buildpack and dependency exists in cache dir
		//copy from cache to deps dir
		source := filepath.Join(gs.Stager.CacheDir(), dependency.DirName)
		dest := filepath.Join(gs.Stager.DepDir())
		gs.Log.Info(fmt.Sprintf("--> installing dependency '%s' from application cache", dependency.DirName))
		err := libbuildpack.CopyDirectory(source, dest)
		if err != nil {
		}else{
			return nil //when successfull we are done, otherwise we will install with the help of the "Manifest"
		}
	}

	//install with the help of the "Manifest"
	if gs.Manifest.IsCached(){
		gs.Log.Info(fmt.Sprintf("--> installing dependency '%s' from buildpack cache", dependency.DirName))
	}else{
		gs.Log.Info(fmt.Sprintf("--> installing dependency '%s' from remote gallery", dependency.DirName))
	}
	if err := gs.Manifest.InstallDependency(dep, dependency.StagingLocation); err != nil {
		gs.Log.Error("Error installing '%s': %s", dependency.Name, err.Error())
		return err
	}

	if !gs.Manifest.IsCached(){ //if online or non-cached system buildpack
		// copy deps dir to cache
		source := filepath.Join(gs.Stager.DepDir(), dependency.DirName)
		dest := filepath.Join(gs.Stager.CacheDir())
		libbuildpack.CopyDirectory(source, dest)
		gs.Log.Info("source:%s dest:s%", source, dest)
		gs.Log.Info(fmt.Sprintf("--> dependency '%s' saved to application cache", dependency.DirName))

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
	return filepath.Join(gs.Stager.DepsIdx(), dependency.DirName)
}

func (gs *Supplier) EvalStagingLocation(dependency Dependency) string {
	return filepath.Join(gs.Stager.DepDir(), dependency.DirName)
}

func (gs *Supplier) EvalCacheLocation(dependency Dependency) string {
	return filepath.Join(gs.Stager.CacheDir(), dependency.DirName)
}
