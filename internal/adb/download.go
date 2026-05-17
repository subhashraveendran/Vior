package adb

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// Google's official platform-tools download URLs.
	macURL     = "https://dl.google.com/android/repository/platform-tools-latest-darwin.zip"
	linuxURL   = "https://dl.google.com/android/repository/platform-tools-latest-linux.zip"
	windowsURL = "https://dl.google.com/android/repository/platform-tools-latest-windows.zip"
)

// viorDir returns the Vior app support directory (~/.vior/).
func viorDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".vior")
}

// bundledADBPath returns the path to the bundled ADB binary if it exists.
func bundledADBPath() string {
	dir := viorDir()
	if dir == "" {
		return ""
	}
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	path := filepath.Join(dir, "platform-tools", name)
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	return path
}

// isBundled returns true if the given path is inside Vior's directory.
func isBundled(path string) bool {
	dir := viorDir()
	if dir == "" {
		return false
	}
	return strings.HasPrefix(path, dir)
}

// downloadPlatformTools downloads Google's platform-tools to ~/.vior/platform-tools/.
func downloadPlatformTools() error {
	dir := viorDir()
	if dir == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	// Pick URL for current OS.
	var url string
	switch runtime.GOOS {
	case "darwin":
		url = macURL
	case "linux":
		url = linuxURL
	case "windows":
		url = windowsURL
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	log.Printf("Downloading Android platform-tools from %s ...", url)

	// Create temp file for download.
	tmpFile, err := os.CreateTemp("", "vior-platform-tools-*.zip")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Download.
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("download write failed: %w", err)
	}
	tmpFile.Close()

	// Extract to ~/.vior/.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create vior dir: %w", err)
	}

	// Remove old platform-tools if exists.
	ptDir := filepath.Join(dir, "platform-tools")
	os.RemoveAll(ptDir)

	if err := unzip(tmpFile.Name(), dir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Make adb executable.
	adbName := "adb"
	if runtime.GOOS == "windows" {
		adbName = "adb.exe"
	}
	adbBin := filepath.Join(ptDir, adbName)
	if err := os.Chmod(adbBin, 0755); err != nil {
		return fmt.Errorf("chmod adb: %w", err)
	}

	log.Printf("ADB installed to %s", adbBin)
	return nil
}

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)

		// Prevent zip slip.
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(path, f.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(path), 0755)

		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
