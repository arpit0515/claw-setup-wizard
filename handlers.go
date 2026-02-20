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
	r.ParseForm()
	provider := r.FormValue("provider")
	apiKey := r.FormValue("api_key")
	model := r.FormValue("model")

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
	r.ParseForm()
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
	r.ParseForm()
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
	r.ParseForm()
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
	r.ParseForm()

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
	r.ParseForm()
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

	ok, msg := installSystemdService()
	jsonResponse(w, map[string]interface{}{
		"ok":      ok,
		"message": msg,
	})
}

func installSystemdService() (bool, string) {
	picocławPath, err := exec.LookPath("picoclaw")
	if err != nil {
		return false, "picoclaw not found in PATH"
	}

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
			return false, strings.TrimSpace(string(out))
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

	var arch string
	switch strings.TrimSpace(out) {
	case "aarch64":
		arch = "arm64"
	case "armv7l":
		arch = "armv6l"
	case "x86_64":
		arch = "amd64"
	default:
		errorResponse(w, "Unsupported architecture: "+out)
		return
	}

	url := "https://github.com/sipeed/picoclaw/releases/latest/download/picoclaw-linux-" + arch
	tmpPath := "/tmp/picoclaw"
	finalPath := "/usr/local/bin/picoclaw"

	// Download
	_, err = runCommand("wget", "-q", "-O", tmpPath, url)
	if err != nil {
		errorResponse(w, "Download failed — check internet connection")
		return
	}

	// Make executable
	_, err = runCommand("chmod", "+x", tmpPath)
	if err != nil {
		errorResponse(w, "chmod failed: "+err.Error())
		return
	}

	// Move to bin (requires sudo)
	_, err = runCommand("sudo", "mv", tmpPath, finalPath)
	if err != nil {
		errorResponse(w, "Could not move to /usr/local/bin — try: sudo mv /tmp/picoclaw /usr/local/bin/picoclaw")
		return
	}

	// Verify
	path, err := exec.LookPath("picoclaw")
	if err != nil || path == "" {
		errorResponse(w, "Installed but not found in PATH — restart the wizard")
		return
	}

	okResponse(w, "PicoClaw installed at "+path, nil)
}
