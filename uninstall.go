package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// handleUninstall performs a full teardown of everything the wizard installed.
// Stops services, removes binaries, wipes workspace and config.
// Returns a JSON summary of each step so the UI can show what was done.
func handleUninstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var steps []map[string]interface{}
	allOK := true

	record := func(label string, ok bool, detail string) {
		if !ok {
			allOK = false
		}
		steps = append(steps, map[string]interface{}{
			"label":  label,
			"ok":     ok,
			"detail": detail,
		})
	}

	home, _ := os.UserHomeDir()

	// ── 1. Stop and disable all claw services ────────────────────────────────

	services := []string{"picoclaw", "claw-gmail", "claw-gcal"}

	if runtime.GOOS == "darwin" {
		plistDir := filepath.Join(home, "Library", "LaunchAgents")
		for _, svc := range services {
			plistPath := filepath.Join(plistDir, "com."+svc+".agent.plist")
			exec.Command("launchctl", "unload", plistPath).Run()
			os.Remove(plistPath)
		}
		record("Stop services", true, "Unloaded and removed launchd plists")
	} else {
		var failed []string
		for _, svc := range services {
			exec.Command("systemctl", "--user", "stop", svc).Run()
			exec.Command("systemctl", "--user", "disable", svc).Run()
			unitPath := filepath.Join(home, ".config", "systemd", "user", svc+".service")
			if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
				failed = append(failed, svc)
			}
		}
		exec.Command("systemctl", "--user", "daemon-reload").Run()
		exec.Command("systemctl", "--user", "reset-failed").Run()
		if len(failed) > 0 {
			record("Stop & remove services", false, "Could not remove unit files for: "+strings.Join(failed, ", "))
		} else {
			record("Stop & remove services", true, "Stopped and removed: "+strings.Join(services, ", "))
		}
	}

	// ── 2. Remove picoclaw binary ─────────────────────────────────────────────

	picoclawPaths := []string{"/usr/local/bin/picoclaw", "/usr/bin/picoclaw"}
	if path, err := exec.LookPath("picoclaw"); err == nil {
		picoclawPaths = append(picoclawPaths, path)
	}

	removedBinary := false
	for _, p := range picoclawPaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Remove(p); err != nil {
			if out, err2 := runCommand("sudo", "rm", "-f", p); err2 != nil {
				record("Remove picoclaw binary", false, fmt.Sprintf("Could not remove %s: %s", p, out))
				continue
			}
		}
		removedBinary = true
	}
	if removedBinary {
		record("Remove picoclaw binary", true, "Removed from system")
	} else {
		record("Remove picoclaw binary", true, "Not found — already removed or never installed")
	}

	// ── 3. Wipe ~/.picoclaw entirely ──────────────────────────────────────────

	picoDir := filepath.Join(home, ".picoclaw")
	if _, err := os.Stat(picoDir); err == nil {
		if err := os.RemoveAll(picoDir); err != nil {
			record("Remove ~/.picoclaw", false, "Failed: "+err.Error())
		} else {
			record("Remove ~/.picoclaw", true, "Workspace, tokens, config, and tools removed")
		}
	} else {
		record("Remove ~/.picoclaw", true, "Already absent — nothing to remove")
	}

	// ── 4. Clean up empty systemd user directory ──────────────────────────────

	if runtime.GOOS != "darwin" {
		systemdUserDir := filepath.Join(home, ".config", "systemd", "user")
		if entries, err := os.ReadDir(systemdUserDir); err == nil && len(entries) == 0 {
			os.Remove(systemdUserDir)
			record("Clean systemd dir", true, "Removed empty directory")
		} else {
			record("Clean systemd dir", true, "Other services present — directory left intact")
		}
	}

	// ── 5. Done ───────────────────────────────────────────────────────────────

	message := "PicoClaw fully uninstalled — your Raspberry Pi is clean."
	if !allOK {
		message = "Uninstall completed with some warnings — check the steps below."
	}

	jsonResponse(w, map[string]interface{}{
		"ok":      allOK,
		"message": message,
		"steps":   steps,
	})
}
