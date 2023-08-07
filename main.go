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

type Game struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
}

type Patch struct {
	Name       string  `yaml:"name"`
	Version    *string `yaml:"version"`
	VersionUrl *string `yaml:"version_url"`
	PatchUrl   string  `yaml:"patch_url"`
}

type Config struct {
	Game    Game    `yaml:"game"`
	Patches []Patch `yaml:"patches"`
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

	if !fileExists("Game.ini") {
		err = downloadBaseGame(config.Game)
		if err != nil {
			return err
		}
	}

	// If a patch was applied all followup patches need to be reapplied on top.
	forceUpdate := false
	for _, patch := range config.Patches {
		if patch.VersionUrl == nil || patch.Version == nil || forceUpdate {
			err = alwaysUpdate(patch)
			if err != nil {
				return err
			}

			forceUpdate = true
		} else {
			version, err := attemptUpdateUsingVersionFile(patch)
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

func downloadBaseGame(game Game) error {
	fmt.Println("Downloading base game " + game.Name + "...")

	err := applyPatch(game.Url)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	fmt.Println()

	return nil
}

func alwaysUpdate(patch Patch) error {
	fmt.Println("Downloading latest patch for " + patch.Name + "...")

	err := applyPatch(patch.PatchUrl)
	if err != nil {
		return err
	}

	fmt.Println("Done!")
	fmt.Println()

	return nil
}

func attemptUpdateUsingVersionFile(patch Patch) (string, error) {
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
		fmt.Println("Updating " + patch.Name + "...")
		fmt.Println("Version " + currentVersion + " is outdated")
		fmt.Println("Latest version is " + remoteVersion + " is outdated")

		if baseVersion(currentVersion) != baseVersion(remoteVersion) {
			return "", errors.New("The latest version of " + patch.Name + " is needs to be downloaded manually.")
		}

		fmt.Println("Downloading version " + remoteVersion)
		fmt.Println()

		err = applyPatch(patch.PatchUrl)
		if err != nil {
			return "", err
		}

		fmt.Println()
		fmt.Println("Updated to version " + remoteVersion)
		fmt.Println()
	} else {
		fmt.Println("Version " + currentVersion + " is up to date")
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

	if err != nil {
		fmt.Printf("Error: %s", err)
		time.Sleep(5 * time.Second)
	}
}
