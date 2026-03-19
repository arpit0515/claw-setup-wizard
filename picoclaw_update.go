package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const picoClawReleasesAPI = "https://api.github.com/repos/sipeed/picoclaw/releases/latest"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// ── Arch detection ────────────────────────────────────────────────────────────

func picoClawArch() string {
	arch := runtime.GOARCH
	os_ := runtime.GOOS
	switch {
	case os_ == "linux" && arch == "arm64":
		return "Linux_arm64"
	case os_ == "linux" && arch == "arm":
		return "Linux_armv6"
	case os_ == "linux" && arch == "amd64":
		return "Linux_x86_64"
	case os_ == "darwin" && arch == "arm64":
		return "Darwin_arm64"
	case os_ == "darwin" && arch == "amd64":
		return "Darwin_x86_64"
	default:
		return "Linux_arm64" // Pi default
	}
}

// ── Current version ───────────────────────────────────────────────────────────

func picoClawCurrentVersion() string {
	out, err := exec.Command("picoclaw", "--version").Output()
	if err != nil {
		out2, err2 := exec.Command("picoclaw", "version").Output()
		if err2 != nil {
			return ""
		}
		out = out2
	}
	// Strip ANSI escape codes
	text := strings.TrimSpace(string(out))
	// Find version pattern vX.Y.Z
	for _, word := range strings.Fields(text) {
		clean := strings.Map(func(r rune) rune {
			if r >= 32 && r < 127 {
				return r
			}
			return -1
		}, word)
		if strings.HasPrefix(clean, "v") && strings.Count(clean, ".") >= 1 {
			return clean
		}
	}
	return ""
}

// ── Latest release ────────────────────────────────────────────────────────────

func fetchLatestRelease() (*githubRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", picoClawReleasesAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "claw-setup-wizard")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach GitHub: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API error: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, err
	}
	return &release, nil
}

// ── Download and install ──────────────────────────────────────────────────────

func installPicoClawRelease(release *githubRelease) error {
	arch := picoClawArch()
	tarName := "picoclaw_" + arch + ".tar.gz"

	// Find matching asset
	downloadURL := ""
	for _, a := range release.Assets {
		if a.Name == tarName {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		// Fallback to predictable URL pattern
		downloadURL = fmt.Sprintf(
			"https://github.com/sipeed/picoclaw/releases/latest/download/%s", tarName)
	}

	// Download
	tmpTar := filepath.Join(os.TempDir(), tarName)
	fmt.Fprintf(os.Stderr, "picoclaw-update: downloading %s\n", downloadURL)

	if err := downloadFile(downloadURL, tmpTar); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmpTar)

	// Extract binary
	tmpDir, err := os.MkdirTemp("", "picoclaw-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTar(tmpTar, tmpDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}

	// Find the binary
	binaryName := "picoclaw"
	extractedBin := filepath.Join(tmpDir, binaryName)
	if _, err := os.Stat(extractedBin); os.IsNotExist(err) {
		// Some archives have a subdir
		entries, _ := os.ReadDir(tmpDir)
		for _, e := range entries {
			candidate := filepath.Join(tmpDir, e.Name(), binaryName)
			if _, err := os.Stat(candidate); err == nil {
				extractedBin = candidate
				break
			}
		}
	}

	// Install to /usr/local/bin
	finalPath := "/usr/local/bin/picoclaw"
	if _, err := exec.LookPath("sudo"); err == nil {
		cmd := exec.Command("sudo", "mv", extractedBin, finalPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("install failed: %s", string(out))
		}
		exec.Command("sudo", "chmod", "+x", finalPath).Run()
	} else {
		if err := os.Rename(extractedBin, finalPath); err != nil {
			return fmt.Errorf("could not move binary: %w", err)
		}
	}

	fmt.Fprintf(os.Stderr, "picoclaw-update: installed %s at %s\n", release.TagName, finalPath)
	return nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func extractTar(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Only extract the picoclaw binary
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if name != "picoclaw" {
			continue
		}
		outPath := filepath.Join(destDir, name)
		out, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		io.Copy(out, tr)
		out.Close()
		break
	}
	return nil
}

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

// GET /api/picoclaw/version — returns current and latest version
func handlePicoClawVersion(w http.ResponseWriter, r *http.Request) {
	current := picoClawCurrentVersion()

	release, err := fetchLatestRelease()
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"ok":      true,
			"current": current,
			"latest":  "",
			"error":   err.Error(),
		})
		return
	}

	upToDate := current == release.TagName
	jsonResponse(w, map[string]interface{}{
		"ok":               true,
		"current":          current,
		"latest":           release.TagName,
		"up_to_date":       upToDate,
		"update_available": !upToDate && current != "",
	})
}

// POST /api/picoclaw/update — download and install latest PicoClaw
func handlePicoClawUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	release, err := fetchLatestRelease()
	if err != nil {
		errorResponse(w, "Could not fetch latest release: "+err.Error())
		return
	}

	current := picoClawCurrentVersion()
	if current == release.TagName {
		okResponse(w, "Already on latest version: "+release.TagName, nil)
		return
	}

	// Run in background — update can take a while
	go func() {
		if err := installPicoClawRelease(release); err != nil {
			fmt.Fprintf(os.Stderr, "picoclaw-update: failed: %v\n", err)
			return
		}
		// Restart service after update
		runCommand("systemctl", "--user", "restart", "picoclaw")
		fmt.Fprintf(os.Stderr, "picoclaw-update: service restarted\n")
	}()

	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"message": fmt.Sprintf("Updating from %s to %s in background...", current, release.TagName),
		"version": release.TagName,
	})
}
