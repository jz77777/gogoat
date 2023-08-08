package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"github.com/u3mur4/megadl"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type progressWriter struct {
	totalSize   int64
	downloaded  int64
	lastPercent int
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.downloaded += int64(n)
	percent := int((pw.downloaded * 100) / pw.totalSize)

	if percent > pw.lastPercent {
		pw.lastPercent = percent
		fmt.Printf("\rProgress: %d%%", percent)
	}

	return n, nil
}

func download(url string, file string) error {
	var reader io.ReadCloser
	var size int64
	var err error

	if strings.HasPrefix(url, "https://mega.nz/") {
		url = strings.Replace(url, "#", "!", 1)
		url = strings.Replace(url, "/file/", "/#!", 1)

		var info *megadl.Info

		reader, info, err = megadl.Download(url)
		size = int64(info.Size)
	} else {
		var response *http.Response
		response, err = http.Get(url)
		reader = response.Body
		size = response.ContentLength
	}

	if err != nil {
		return err
	}
	defer reader.Close()

	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	progress := &progressWriter{
		totalSize:   size,
		downloaded:  0,
		lastPercent: -1,
	}

	_, err = io.Copy(out, io.TeeReader(reader, progress))
	return err
}

func readVersion(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func fileExists(file string) bool {
	info, err := os.Stat(file)
	return !os.IsNotExist(err) && !info.IsDir()
}

func extractArchive() error {
	var err error
	zip, err := zip.OpenReader("_patch.zip")
	if err != nil {
		return err
	}
	defer zip.Close()

	for _, file := range zip.File {
		if file.FileInfo().IsDir() {
			continue
		}

		skipExtraction, err := archiveFileIsTheSame(file.Name, file)
		if err != nil {
			return err
		}

		if skipExtraction {
			continue
		}

		//if !strings.HasPrefix(file.Name, "Scripts/") {
		//	fmt.Println("Extracting " + file.Name)
		//}

		err = os.MkdirAll(filepath.Dir(file.Name), os.ModePerm)
		if err != nil {
			return err
		}

		out, err := os.Create(file.Name)
		if err != nil {
			return err
		}
		defer out.Close()

		in, err := file.Open()
		if err != nil {
			return err
		}
		defer in.Close()

		_, err = io.Copy(out, in)
		if err != nil {
			return err
		}
	}

	return nil
}

func archiveFileIsTheSame(fileName string, zipFile *zip.File) (bool, error) {
	if !fileExists(fileName) {
		return false, nil
	}

	var err error
	zipContent, err := zipFile.Open()
	if err != nil {
		return false, err
	}

	zipSum, err := calculateFileHash(zipContent)
	if err != nil {
		return false, err
	}

	existingReader, err := os.Open(fileName)
	if err != nil {
		return false, err
	}

	existingSum, err := calculateFileHash(existingReader)
	if err != nil {
		return false, err
	}

	return bytes.Equal(zipSum, existingSum), nil
}

func calculateFileHash(reader io.ReadCloser) ([]byte, error) {
	defer reader.Close()

	hash := sha1.New()
	_, err := io.Copy(hash, reader)
	if err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

type Game struct {
	Name       string `yaml:"name"`
	Url        string `yaml:"url"`
	Version    string `yaml:"version"`
	VersionUrl string `yaml:"version_url"`
	PatchUrl   string `yaml:"patch_url"`
}

type Mod struct {
	Name       string  `yaml:"name"`
	Version    *string `yaml:"version"`
	VersionUrl *string `yaml:"version_url"`
	PatchUrl   string  `yaml:"patch_url"`
}

type Config struct {
	Game Game  `yaml:"game"`
	Mods []Mod `yaml:"mods"`
}

func update() error {
	var err error

	path, err := os.Executable()
	if err != nil {
		return err
	}

	err = os.Chdir(filepath.Dir(path))
	if err != nil {
		return err
	}

	yamlFile, err := os.ReadFile("updater.yaml")
	if err != nil {
		return err
	}

	config := Config{}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return err
	}

	err = download(config.Game.VersionUrl, "_version.txt")
	if err != nil {
		return err
	}

	remoteVersion, err := readVersion("_version.txt")
	if err != nil {
		return err
	}

	if !fileExists("Game.ini") {
		err = downloadBaseGame(config.Game, remoteVersion)
		if err != nil {
			return err
		}
	}

	var version string

	// If a patch was applied all followup patches need to be reapplied on top.
	forceUpdate := false

	version, err = attemptGameUpdate(config.Game, remoteVersion)
	if err != nil {
		return err
	}

	if version != config.Game.Version {
		forceUpdate = true
	}

	config.Game.Version = version

	for _, patch := range config.Mods {
		if patch.VersionUrl == nil || patch.Version == nil || forceUpdate {
			err = alwaysUpdate(patch)
			if err != nil {
				return err
			}

			forceUpdate = true
		} else {
			version, err = attemptUpdateUsingVersionFile(patch)
			if err != nil {
				return err
			}

			if version != *patch.Version {
				forceUpdate = true
			}

			*patch.Version = version
		}
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}

	err = os.WriteFile("updater.yaml", data, 0644)
	if err != nil {
		return err
	}

	fmt.Println("Closing in 3 seconds")
	time.Sleep(3 * time.Second)

	return nil
}

func downloadBaseGame(game Game, remoteVersion string) error {
	fmt.Println("Downloading base game " + game.Name + "...")

	if baseVersion(game.Version) != baseVersion(remoteVersion) {
		return errors.New("The latest version of " + game.Name + " is " + remoteVersion + " while this executable is for " + game.Version + ".")
	}

	err := applyPatch(game.Url)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Done!")
	fmt.Println()

	return nil
}

func attemptGameUpdate(game Game, remoteVersion string) (string, error) {
	var err error

	currentVersion := game.Version

	fmt.Println("Updating " + game.Name + "...")
	if currentVersion != remoteVersion {
		fmt.Println("Version " + currentVersion + " is outdated")
		fmt.Println("Latest version is " + remoteVersion)

		if baseVersion(currentVersion) != baseVersion(remoteVersion) {
			return "", errors.New("The latest version of " + game.Name + " needs to be downloaded manually.")
		}

		err = applyPatch(game.PatchUrl)
		if err != nil {
			return "", err
		}

		fmt.Println()
	} else {
		fmt.Println("Version " + currentVersion + " is up to date")
		fmt.Println()
	}

	return remoteVersion, nil
}

func alwaysUpdate(mod Mod) error {
	fmt.Println("Downloading latest mod for " + mod.Name + "...")

	err := applyPatch(mod.PatchUrl)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Done!")
	fmt.Println()

	return nil
}

func attemptUpdateUsingVersionFile(mod Mod) (string, error) {
	var err error

	err = download(*mod.VersionUrl, "_version.txt")
	if err != nil {
		return "", err
	}

	remoteVersion, err := readVersion("_version.txt")
	if err != nil {
		return "", err
	}

	currentVersion := *mod.Version

	fmt.Println("Updating " + mod.Name + "...")
	if currentVersion != remoteVersion {
		fmt.Println("Version " + currentVersion + " is outdated")
		fmt.Println("Downloading version " + remoteVersion)

		err = applyPatch(mod.PatchUrl)
		if err != nil {
			return "", err
		}

		fmt.Println()
	} else {
		fmt.Println("Version " + currentVersion + " is up to date")
		fmt.Println()
	}

	return remoteVersion, nil
}

func applyPatch(url string) error {
	var err error

	err = download(url, "_patch.zip")
	if err != nil {
		return err
	}

	err = extractArchive()
	if err != nil {
		return err
	}

	err = os.Remove("_patch.zip")
	return err
}

func baseVersion(version string) string {
	index := strings.LastIndex(version, ".")

	if index == -1 {
		return version
	}

	return version[:index] + ".0"
}

func main() {
	err := update()

	if fileExists("_version.txt") {
		_ = os.Remove("_version.txt")
	}

	if fileExists("_patch.zip") {
		_ = os.Remove("_patch.zip")
	}

	if err != nil {
		fmt.Printf("Error: %s", err)
		time.Sleep(5 * time.Second)
	}
}
