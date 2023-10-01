package main

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"github.com/bodgit/sevenzip"
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

func download(url string, file string, progress bool) error {
	var reader io.ReadCloser
	var size int64
	var err error

	if strings.HasPrefix(url, "https://mega.nz/") {
		url = strings.Replace(url, "#", "!", 1)
		url = strings.Replace(url, "/file/", "/#!", 1)

		var info *megadl.Info

		reader, info, err = megadl.Download(url)

		if err != nil {
			return err
		}

		size = int64(info.Size)
	} else {
		var response *http.Response
		response, err = http.Get(url)

		if err != nil {
			return err
		}

		reader = response.Body
		size = response.ContentLength
	}

	defer reader.Close()

	out, err := os.Create(file)
	if err != nil {
		return err
	}
	defer out.Close()

	var in io.Reader

	if progress {
		progressWriter := &progressWriter{
			totalSize:   size,
			downloaded:  0,
			lastPercent: -1,
		}
		in = io.TeeReader(reader, progressWriter)
	} else {
		in = reader
	}

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	if progress {
		fmt.Println()
	}

	return nil
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

func extractZipArchive() error {
	var err error
	reader, err := zip.OpenReader("_patch.zip")
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		err = os.MkdirAll(filepath.Dir(file.Name), os.ModePerm)
		if err != nil {
			return err
		}

		err = extractZipFile(file)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractSevenZipArchive(password *string) error {
	var err error
	var reader *sevenzip.ReadCloser

	if password == nil {
		reader, err = sevenzip.OpenReader("_patch.7z")
	} else {
		reader, err = sevenzip.OpenReaderWithPassword("_patch.7z", *password)
	}

	if err != nil {
		return err
	}

	defer reader.Close()

	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}

		err = os.MkdirAll(filepath.Dir(file.Name), os.ModePerm)
		if err != nil {
			return err
		}

		err = extractSevenZipFile(file)
		if err != nil {
			return err
		}
	}

	return nil
}

func extractZipFile(zipFile *zip.File) error {
	out, err := os.Create(zipFile.Name)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := zipFile.Open()
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = io.Copy(out, in)

	return err
}

func extractSevenZipFile(zipFile *sevenzip.File) error {
	out, err := os.Create(zipFile.Name)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := zipFile.Open()
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = io.Copy(out, in)

	return err
}

type Game struct {
	Name       string  `yaml:"name"`
	Url        string  `yaml:"url"`
	Version    string  `yaml:"version"`
	VersionUrl string  `yaml:"version_url"`
	PatchUrl   string  `yaml:"patch_url"`
	Password   *string `yaml:"password"`
}

type Mod struct {
	Name       string  `yaml:"name"`
	Version    *string `yaml:"version"`
	VersionUrl *string `yaml:"version_url"`
	PatchUrl   string  `yaml:"patch_url"`
	Password   *string `yaml:"password"`
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

	err = download(config.Game.VersionUrl, "_version.txt", false)
	if err != nil {
		return err
	}

	remoteVersion, err := readVersion("_version.txt")
	if err != nil {
		return err
	}

	if config.Game.Password == nil && strings.HasSuffix(config.Game.PatchUrl, ".7z") {
		fmt.Println("Provide password for base game:")

		password, err := readPassword()
		if err != nil {
			return err
		}

		config.Game.Password = &password
	}

	if !fileExists("version") {
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

	for _, mod := range config.Mods {
		if mod.Password == nil && strings.HasSuffix(mod.PatchUrl, ".7z") {
			fmt.Println("Provide password for mod " + mod.Name + ":")

			password, err := readPassword()
			if err != nil {
				return err
			}

			*mod.Password = password
		}

		if mod.VersionUrl == nil || mod.Version == nil || forceUpdate {
			err = alwaysUpdate(mod)
			if err != nil {
				return err
			}

			forceUpdate = true
		} else {
			version, err = attemptUpdateUsingVersionFile(mod)
			if err != nil {
				return err
			}

			if version != *mod.Version {
				forceUpdate = true
			}

			*mod.Version = version
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

func readPassword() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(input), nil
}

func downloadBaseGame(game Game, remoteVersion string) error {
	fmt.Println("Downloading base game " + game.Name + "...")

	if baseVersion(game.Version) != baseVersion(remoteVersion) {
		return errors.New("The latest version of " + game.Name + " is " + remoteVersion + " while this executable is for " + game.Version + ".")
	}

	err := applyPatch(game.Url, game.Password)
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

		err = applyPatch(game.PatchUrl, game.Password)
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

	err := applyPatch(mod.PatchUrl, mod.Password)
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

	err = download(*mod.VersionUrl, "_version.txt", false)
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

		err = applyPatch(mod.PatchUrl, mod.Password)
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

func applyPatch(url string, password *string) error {
	var name string
	var err error

	if strings.HasSuffix(url, ".7z") {
		name = "_patch.7z"
	} else {
		name = "_patch.zip"
	}

	err = download(url, name, true)
	if err != nil {
		return err
	}

	fmt.Println("Extracting archive...")

	if strings.HasSuffix(url, ".7z") {
		err = extractSevenZipArchive(password)
	} else {
		err = extractZipArchive()
	}

	if err != nil {
		return err
	}

	err = os.Remove(name)
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

	if fileExists("_patch.7z") {
		_ = os.Remove("_patch.7z")
	}

	if err != nil {
		fmt.Printf("Error: %s", err)
		time.Sleep(5 * time.Second)
	}
}
