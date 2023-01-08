package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
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

type Config struct {
	VersionUrl *string `yaml:"versionUrl"`
	PatchUrl   string  `yaml:"patchUrl"`
}

func update() error {
	var err error

	yamlFile, err := os.ReadFile("gogoat.yaml")
	if err != nil {
		return err
	}

	config := Config{}
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		return err
	}

	if config.VersionUrl == nil {
		return alwaysUpdate(config)
	}

	return attemptUpdateUsingVersionFile(config)
}

func alwaysUpdate(config Config) error {
	fmt.Println("Downloading latest patch...")
	fmt.Println()

	err := applyPatch(config)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	fmt.Println("Closing in 3 seconds")
	time.Sleep(3 * time.Second)

	return nil
}

func attemptUpdateUsingVersionFile(config Config) error {
	var err error

	err = download(*config.VersionUrl, "_version")
	if err != nil {
		return err
	}

	var currentVersion string
	if fileExists("version") {
		currentVersion, err = readVersion("version")
		if err != nil {
			return err
		}
	} else {
		currentVersion = ""
	}

	remoteVersion, err := readVersion("_version")
	if err != nil {
		return err
	}

	if currentVersion != remoteVersion {
		fmt.Println("Version " + currentVersion + " is outdated")
		fmt.Println("Downloading version " + remoteVersion)
		fmt.Println()

		err = applyPatch(config)
		if err != nil {
			return err
		}
		err = os.Remove("version")
		if err != nil {
			return err
		}
		err = os.Rename("_version", "version")
		if err != nil {
			return err
		}

		fmt.Println()
		fmt.Println("Updated to version " + remoteVersion)
	} else {
		err = os.Remove("_version")
		if err != nil {
			return err
		}

		fmt.Println("Version " + currentVersion + " is up to date")
	}

	fmt.Println("Closing in 3 seconds")
	time.Sleep(3 * time.Second)

	return nil
}

func applyPatch(config Config) error {
	var err error

	err = download(config.PatchUrl, "_patch.zip")
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

func main() {
	err := update()
	if err != nil {
		fmt.Printf("Error: %s", err)
	}
}
