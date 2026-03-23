package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func handleValidateLLM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	provider := strings.TrimSpace(r.FormValue("provider"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	model := strings.TrimSpace(r.FormValue("model"))
	if provider == "" || model == "" {
		errorResponse(w, "provider and model are required")
		return
	}
	if apiKey == "" {
		cfg := readConfig()
		if p, ok := cfg.Providers[provider]; ok {
			apiKey, _ = p["api_key"].(string)
		}
	}
	if apiKey == "" {
		errorResponse(w, "No API key provided and none saved for this provider")
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
		cfg.Providers[provider] = map[string]interface{}{"api_key": apiKey}
		if provider == "openrouter" {
			cfg.Providers[provider]["api_base"] = "https://openrouter.ai/api/v1"
		}
		cfg.Agents["defaults"] = map[string]interface{}{"model": model}
		modelEntry := map[string]interface{}{"model_name": model, "model": provider + "/" + model, "api_key": apiKey}
		if provider == "openrouter" {
			modelEntry["api_base"] = "https://openrouter.ai/api/v1"
		}
		cfg.ModelList = []map[string]interface{}{modelEntry}
		writeConfig(cfg)
	}
	jsonResponse(w, map[string]interface{}{"ok": ok, "message": msg})
}

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
		cfg.Channels["telegram"] = map[string]interface{}{"enabled": true, "token": token}
		writeConfig(cfg)
	}
	jsonResponse(w, map[string]interface{}{"ok": ok, "message": msg, "username": username})
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
	jsonResponse(w, map[string]interface{}{"ok": ok, "message": msg})
}

func handleGenerateSoul(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)
	answers := SoulAnswers{Name: r.FormValue("name"), UserName: r.FormValue("user_name"), Role: r.FormValue("role"), Expertise: r.FormValue("expertise"), Style: r.FormValue("style"), Goals: r.FormValue("goals"), Dislikes: r.FormValue("dislikes"), Decisions: r.FormValue("decisions")}
	soul := generateSoulMD(answers)
	jsonResponse(w, map[string]interface{}{"ok": true, "soul": soul})
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

	// Inject tool routing rules if not already present
	content = ensureToolRoutingRules(content)

	soulPath := getSoulPath()
	os.MkdirAll(filepath.Dir(soulPath), 0755)
	if err := os.WriteFile(soulPath, []byte(content), 0644); err != nil {
		errorResponse(w, "Failed to write SOUL.md: "+err.Error())
		return
	}

	agentName, ownerName := resolveAgentIdentity()
	go writeWorkspaceFiles(nil, agentName, ownerName)

	okResponse(w, "SOUL.md saved to "+soulPath, nil)
}

func handleInstallService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	ok, msg := installSystemdService()
	jsonResponse(w, map[string]interface{}{"ok": ok, "message": msg})
}

func handleRestartService(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var out string
	var err error
	if runtime.GOOS == "darwin" {
		plistPath := os.ExpandEnv("$HOME/Library/LaunchAgents/com.picoclaw.agent.plist")
		exec.Command("launchctl", "unload", plistPath).Run()
		out, err = runCommand("launchctl", "load", plistPath)
	} else {
		out, err = runCommand("systemctl", "--user", "restart", "picoclaw")
	}
	if err != nil {
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = "Restart failed: " + err.Error()
		}
		errorResponse(w, msg)
		return
	}
	okResponse(w, "Agent restarted", nil)
}

func handleLocalIP(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]interface{}{"ip": getLocalIP()})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	status := buildSystemStatus()
	cfg := readConfig()
	model := ""
	if defaults, ok := cfg.Agents["defaults"].(map[string]interface{}); ok {
		model, _ = defaults["model"].(string)
	}
	botUsername := ""
	if tg, ok := cfg.Channels["telegram"]; ok {
		if token, ok := tg["token"].(string); ok && token != "" {
			_, _, botUsername = validateTelegramToken(token)
		}
	}
	jsonResponse(w, map[string]interface{}{"status": status, "model": model, "bot_username": botUsername, "uptime": getUptime()})
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
	out, err := runCommand("uname", "-m")
	if err != nil {
		errorResponse(w, "Could not detect architecture")
		return
	}
	var picoArch string
	switch strings.TrimSpace(out) {
	case "aarch64":
		picoArch = "arm64"
	case "armv7l", "armv6l":
		picoArch = "armv6"
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
	if _, err = runCommand("wget", "-L", "-q", "-O", tmpTar, url); err != nil {
		errorResponse(w, "Download failed: "+err.Error())
		return
	}
	os.MkdirAll(tmpDir, 0755)
	if _, err = runCommand("tar", "-xzf", tmpTar, "-C", tmpDir); err != nil {
		errorResponse(w, "Extract failed: "+err.Error())
		return
	}
	if _, err = runCommand("sudo", "mv", tmpDir+"/picoclaw", finalPath); err != nil {
		errorResponse(w, "Could not find picoclaw binary in archive: "+err.Error())
		return
	}
	os.Remove(tmpTar)
	os.RemoveAll(tmpDir)
	path, err := exec.LookPath("picoclaw")
	if err != nil || path == "" {
		errorResponse(w, "Installed but not found in PATH — restart the wizard")
		return
	}
	okResponse(w, "PicoClaw installed at "+path, nil)
}

func handleGetModels(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20)
	provider := strings.TrimSpace(r.FormValue("provider"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))
	if provider != "openrouter" {
		errorResponse(w, "provider required")
		return
	}
	if apiKey == "" {
		cfg := readConfig()
		if p, ok := cfg.Providers[provider]; ok {
			apiKey, _ = p["api_key"].(string)
		}
	}
	if apiKey == "" {
		errorResponse(w, "No API key provided and none saved for this provider")
		return
	}
	models, err := fetchOpenRouterModels(apiKey)
	if err != nil {
		errorResponse(w, "Failed to fetch models: "+err.Error())
		return
	}
	jsonResponse(w, map[string]interface{}{"ok": true, "models": models})
}

// ── Paths ─────────────────────────────────────────────────────────────────────

func weatherBinaryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "tools", "weather", "weather-mcp")
}

func weatherToolDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "tools", "weather")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func weatherBinaryInstalled() bool {
	_, err := os.Stat(weatherBinaryPath())
	return err == nil
}

func weatherLocationSet() bool {
	_, err := loadWeatherLocation()
	return err == nil
}
