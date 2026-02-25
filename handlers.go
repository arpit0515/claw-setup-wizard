package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ── LLM ──────────────────────────────────────────────────────────────────────

func handleValidateLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)

	// DEBGGING

	provider := strings.TrimSpace(r.FormValue("provider"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	model := strings.TrimSpace(r.FormValue("model"))

	if provider == "" || apiKey == "" || model == "" {
		errorResponse(w, "provider, api_key and model are required")
		return
	}

	ok, msg := validateLLMKey(provider, apiKey, model)
	if ok {
		cfg := readConfig()
		if cfg.Providers == nil {
			cfg.Providers = make(map[string]map[string]interface{})
		}
		if cfg.Agents == nil {
			cfg.Agents = make(map[string]interface{})
		}
		cfg.Providers[provider] = map[string]interface{}{
			"api_key": apiKey,
		}
		if provider == "openrouter" {
			cfg.Providers[provider]["api_base"] = "https://openrouter.ai/api/v1"
		}
		cfg.Agents["defaults"] = map[string]interface{}{
			"model": model,
		}
		writeConfig(cfg)
	}
	jsonResponse(w, map[string]interface{}{
		"ok":      ok,
		"message": msg,
	})
}

// ── Telegram ──────────────────────────────────────────────────────────────────

func handleValidateTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	token := r.FormValue("token")
	if token == "" {
		errorResponse(w, "token is required")
		return
	}

	ok, msg, username := validateTelegramToken(token)
	if ok {
		cfg := readConfig()
		if cfg.Channels == nil {
			cfg.Channels = make(map[string]map[string]interface{})
		}
		cfg.Channels["telegram"] = map[string]interface{}{
			"enabled": true,
			"token":   token,
		}
		writeConfig(cfg)
	}
	jsonResponse(w, map[string]interface{}{
		"ok":       ok,
		"message":  msg,
		"username": username,
	})
}

func handleSaveTelegramUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	userID := r.FormValue("user_id")
	if userID == "" {
		errorResponse(w, "user_id is required")
		return
	}

	cfg := readConfig()
	if cfg.Channels == nil {
		cfg.Channels = make(map[string]map[string]interface{})
	}
	if cfg.Channels["telegram"] == nil {
		cfg.Channels["telegram"] = make(map[string]interface{})
	}
	cfg.Channels["telegram"]["allowFrom"] = []string{userID}
	writeConfig(cfg)
	okResponse(w, "User ID saved", nil)
}

func handlePingTelegram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	chatID := r.FormValue("chat_id")
	if chatID == "" {
		errorResponse(w, "chat_id is required")
		return
	}

	cfg := readConfig()
	tg := cfg.Channels["telegram"]
	if tg == nil {
		errorResponse(w, "Telegram not configured yet")
		return
	}
	token, _ := tg["token"].(string)
	if token == "" {
		errorResponse(w, "No token found — complete token validation first")
		return
	}

	ok, msg := sendTelegramPing(token, chatID)
	jsonResponse(w, map[string]interface{}{
		"ok":      ok,
		"message": msg,
	})
}

// ── Soul ──────────────────────────────────────────────────────────────────────

func handleGenerateSoul(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)

	answers := SoulAnswers{
		Name:      r.FormValue("name"),
		UserName:  r.FormValue("user_name"),
		Role:      r.FormValue("role"),
		Expertise: r.FormValue("expertise"),
		Style:     r.FormValue("style"),
		Goals:     r.FormValue("goals"),
		Dislikes:  r.FormValue("dislikes"),
		Decisions: r.FormValue("decisions"),
	}

	soul := generateSoulMD(answers)
	jsonResponse(w, map[string]interface{}{
		"ok":   true,
		"soul": soul,
	})
}

func handleSaveSoul(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	content := r.FormValue("soul_content")
	if content == "" {
		errorResponse(w, "soul_content is required")
		return
	}

	soulPath := getSoulPath()
	os.MkdirAll(filepath.Dir(soulPath), 0755)
	err := os.WriteFile(soulPath, []byte(content), 0644)
	if err != nil {
		errorResponse(w, "Failed to write SOUL.md: "+err.Error())
		return
	}
	okResponse(w, "SOUL.md saved to "+soulPath, nil)
}

// ── Service ───────────────────────────────────────────────────────────────────

func handleInstallService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	ok, msg := installService()
	jsonResponse(w, map[string]interface{}{
		"ok":      ok,
		"message": msg,
	})
}

func installService() (bool, string) {
	picocławPath, err := exec.LookPath("picoclaw")
	if err != nil {
		return false, "picoclaw not found in PATH — install PicoClaw first"
	}

	osName, _ := runCommand("uname", "-s")
	osName = strings.TrimSpace(osName)

	if osName == "Darwin" {
		return installLaunchdService(picocławPath)
	}
	return installSystemdService(picocławPath)
}

// macOS: launchd plist in ~/Library/LaunchAgents
func installLaunchdService(picocławPath string) (bool, string) {
	home, _ := os.UserHomeDir()
	launchDir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(launchDir, 0755)

	logDir := filepath.Join(home, ".picoclaw", "logs")
	os.MkdirAll(logDir, 0755)

	plistContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.picoclaw.agent</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>gateway</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>WorkingDirectory</key>
  <string>%s</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>HOME</key>
    <string>%s</string>
  </dict>
  <key>StandardOutPath</key>
  <string>%s/picoclaw.log</string>
  <key>StandardErrorPath</key>
  <string>%s/picoclaw.err</string>
</dict>
</plist>
`, picocławPath, home, home, logDir, logDir)

	plistPath := filepath.Join(launchDir, "com.picoclaw.agent.plist")
	if err := os.WriteFile(plistPath, []byte(plistContent), 0644); err != nil {
		return false, "Failed to write plist: " + err.Error()
	}

	// Unload first in case it was already loaded, ignore error
	exec.Command("launchctl", "unload", plistPath).Run()

	out, err := exec.Command("launchctl", "load", plistPath).CombinedOutput()
	if err != nil {
		return false, "launchctl load failed: " + strings.TrimSpace(string(out))
	}
	return true, "Service installed — PicoClaw will start automatically on login"
}

// Linux: systemd user service
func installSystemdService(picocławPath string) (bool, string) {
	home, _ := os.UserHomeDir()
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)

	serviceContent := fmt.Sprintf(`[Unit]
Description=PicoClaw AI Agent
After=network.target

[Service]
Type=simple
ExecStart=%s gateway
Restart=on-failure
RestartSec=5
WorkingDirectory=%s
Environment=HOME=%s

[Install]
WantedBy=default.target
`, picocławPath, home, home)

	servicePath := filepath.Join(serviceDir, "picoclaw.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return false, "Failed to write service file: " + err.Error()
	}

	commands := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "picoclaw"},
		{"systemctl", "--user", "start", "picoclaw"},
	}
	for _, cmd := range commands {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = "command failed: " + strings.Join(cmd, " ")
			}
			return false, msg
		}
	}
	return true, "Service installed and started"
}

// ── Health ────────────────────────────────────────────────────────────────────

func handleHealth(w http.ResponseWriter, r *http.Request) {
	status := buildSystemStatus()
	cfg := readConfig()

	// Get current model
	model := ""
	if defaults, ok := cfg.Agents["defaults"].(map[string]interface{}); ok {
		model, _ = defaults["model"].(string)
	}

	// Get bot username
	botUsername := ""
	if tg, ok := cfg.Channels["telegram"]; ok {
		if token, ok := tg["token"].(string); ok && token != "" {
			_, _, botUsername = validateTelegramToken(token)
		}
	}

	jsonResponse(w, map[string]interface{}{
		"status":       status,
		"model":        model,
		"bot_username": botUsername,
		"uptime":       getUptime(),
	})
}

func getUptime() string {
	out, err := runCommand("uptime", "-p")
	if err != nil {
		return "unknown"
	}
	return out
}


func handleInstallPicoclaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	// Detect architecture
	out, err := runCommand("uname", "-m")
	if err != nil {
		errorResponse(w, "Could not detect architecture")
		return
	}

	var picoArch string
	switch strings.TrimSpace(out) {
	case "aarch64":
	    picoArch = "arm64"
	case "armv7l":
	    picoArch = "arm"
	case "x86_64":
	    picoArch = "x86_64"
	default:
	    errorResponse(w, "Unsupported architecture: "+out)
	    return
	}

	tarName := "picoclaw_Linux_" + picoArch + ".tar.gz"
	url := "https://github.com/sipeed/picoclaw/releases/latest/download/" + tarName
	tmpTar := "/tmp/" + tarName
	tmpDir := "/tmp/picoclaw-extract"
	finalPath := "/usr/local/bin/picoclaw"

	// Download with redirect follow
	_, err = runCommand("wget", "-L", "-q", "-O", tmpTar, url)
	if err != nil {
	    errorResponse(w, "Download failed: "+err.Error())
	    return
	}

	// Extract
	os.MkdirAll(tmpDir, 0755)
	_, err = runCommand("tar", "-xzf", tmpTar, "-C", tmpDir)
	if err != nil {
	    errorResponse(w, "Extract failed: "+err.Error())
	    return
	}

	// Find the binary inside extracted folder
	_, err = runCommand("sudo", "mv", tmpDir+"/picoclaw", finalPath)
	if err != nil {
	    // try root of extract dir
	    errorResponse(w, "Could not find picoclaw binary in archive: "+err.Error())
	    return
	}

	// Cleanup
	os.Remove(tmpTar)
	os.RemoveAll(tmpDir)

	// Verify
	path, err := exec.LookPath("picoclaw")
	if err != nil || path == "" {
		errorResponse(w, "Installed but not found in PATH — restart the wizard")
		return
	}

	okResponse(w, "PicoClaw installed at "+path, nil)
}


// ---- Handling Models

func handleGetModels(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	provider := strings.TrimSpace(r.FormValue("provider"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	if provider != "openrouter" || apiKey == "" {
		errorResponse(w, "provider and api_key required")
		return
	}

	models, err := fetchOpenRouterModels(apiKey)
	if err != nil {
		errorResponse(w, "Failed to fetch models: "+err.Error())
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ok":     true,
		"models": models,
	})
}
