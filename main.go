package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func download(url string, file string) error {
	response, err := http.Get(url)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, response.Body)
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

		if !strings.HasPrefix(file.Name, "Scripts/") {
			fmt.Println("Extracting " + file.Name)
		}

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

type Patch struct {
	Version    *string `yaml:"version"`
	VersionUrl *string `yaml:"versionUrl"`
	PatchUrl   string  `yaml:"patchUrl"`
}

type Config struct {
	Patches map[string]Patch `yaml:"patches"`
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

	for name, patch := range config.Patches {
		if patch.VersionUrl == nil || patch.Version == nil {
			err = alwaysUpdate(name, patch)
		} else {
			*patch.Version, err = attemptUpdateUsingVersionFile(name, patch)
		}

		if err != nil {
			return err
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

func alwaysUpdate(name string, patch Patch) error {
	fmt.Println("Downloading latest patch for " + name + "...")
	fmt.Println()

	err := applyPatch(patch)
	if err != nil {
		return err
	}

	fmt.Println("Done!")

	return nil
}

func attemptUpdateUsingVersionFile(name string, patch Patch) (string, error) {
	var err error

	err = download(*patch.VersionUrl, "_version.txt")
	if err != nil {
		return "", err
	}

	remoteVersion, err := readVersion("_version.txt")
	if err != nil {
		return "", err
	}

	currentVersion := *patch.Version

	if currentVersion != remoteVersion {
		fmt.Println("Updating " + name + "...")
		fmt.Println("Version " + currentVersion + " is outdated")
		fmt.Println("Latest version is " + remoteVersion + " is outdated")

		if baseVersion(currentVersion) != baseVersion(remoteVersion) {
			return "", errors.New("The latest version of " + name + " is needs to be downloaded manually.")
		}

		fmt.Println("Downloading version " + remoteVersion)
		fmt.Println()

		err = applyPatch(patch)
		if err != nil {
			return "", err
		}

		fmt.Println()
		fmt.Println("Updated to version " + remoteVersion)
	} else {
		fmt.Println("Version " + currentVersion + " is up to date")
	}

	return remoteVersion, nil
}

func applyPatch(patch Patch) error {
	var err error

	err = download(patch.PatchUrl, "_patch.zip")
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
	if err != nil {
		if fileExists("_version.txt") {
			_ = os.Remove("_version.txt")
		}

		fmt.Printf("Error: %s", err)
		time.Sleep(5 * time.Second)
	}
}
