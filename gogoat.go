package main

import (
	"archive/zip"
	"fmt"
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

func update() error {
	versionUrl := "https://www.rebornevo.com/downloads/rebornremote/Desolation6/version"
	patchUrl := "https://www.rebornevo.com/downloads/rebornremote/Desolation6/patch_24bs6P9uo5.zip"

	var err error
	err = download(versionUrl, "_version")
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

		err = download(patchUrl, "_patch.zip")
		if err != nil {
			return err
		}

		err = extractArchive()
		if err != nil {
			return err
		}

		err = os.Remove("_patch.zip")
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

func main() {
	err := update()
	if err != nil {
		fmt.Printf("Error: %s", err)
	}
}
