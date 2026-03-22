package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ── Types ─────────────────────────────────────────────────────────────────────

type WeatherLocation struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label"`
}

type GeoResult struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	Admin1    string  `json:"admin1"`
}

type GeoResponse struct {
	Results []GeoResult `json:"results"`
}

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
	soulPath := getSoulPath()
	os.MkdirAll(filepath.Dir(soulPath), 0755)
	if err := os.WriteFile(soulPath, []byte(content), 0644); err != nil {
		errorResponse(w, "Failed to write SOUL.md: "+err.Error())
		return
	}

	// Generate IDENTITY.md, USER.md, HEARTBEAT.md now that we know the owner.
	// AGENTS.md and TOOLS.md will be written (with tool details) after OAuth connects.
	// writeIfAbsent ensures user edits are never overwritten on re-runs.
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

func installSystemdService() (bool, string) {
	picocławPath, err := exec.LookPath("picoclaw")
	if err != nil {
		return false, "picoclaw not found in PATH"
	}
	home, _ := os.UserHomeDir()
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)
	serviceContent := fmt.Sprintf("[Unit]\nDescription=PicoClaw AI Agent\nAfter=network.target\n\n[Service]\nType=simple\nExecStart=%s gateway\nRestart=on-failure\nRestartSec=5\nWorkingDirectory=%s\nEnvironment=HOME=%s\n\n[Install]\nWantedBy=default.target\n", picocławPath, home, home)
	servicePath := filepath.Join(serviceDir, "picoclaw.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return false, "Failed to write service file: " + err.Error()
	}
	for _, cmd := range [][]string{{"systemctl", "--user", "daemon-reload"}, {"systemctl", "--user", "enable", "picoclaw"}, {"systemctl", "--user", "start", "picoclaw"}} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return false, strings.TrimSpace(string(out))
		}
	}
	return true, "Service installed and started"
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

func weatherConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config", "weather.json")
}

func weatherBinaryPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "tools", "weather", "weather-mcp")
}

func weatherToolDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "tools", "weather")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func loadWeatherLocation() (*WeatherLocation, error) {
	data, err := os.ReadFile(weatherConfigPath())
	if err != nil {
		return nil, err
	}
	var loc WeatherLocation
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

func saveWeatherLocation(loc WeatherLocation) error {
	path := weatherConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(loc, "", "  ")
	return os.WriteFile(path, data, 0600)
}

func geocodeCity(city string) (*WeatherLocation, error) {
	apiURL := "https://geocoding-api.open-meteo.com/v1/search?name=" +
		url.QueryEscape(city) + "&count=1&language=en&format=json"
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("geocoding request failed: %w", err)
	}
	defer resp.Body.Close()

	var geo GeoResponse
	if err := json.NewDecoder(resp.Body).Decode(&geo); err != nil {
		return nil, fmt.Errorf("geocoding parse failed: %w", err)
	}
	if len(geo.Results) == 0 {
		return nil, fmt.Errorf("location not found: %s", city)
	}
	r := geo.Results[0]
	label := r.Name
	if r.Admin1 != "" {
		label += ", " + r.Admin1
	}
	if r.Country != "" {
		label += ", " + r.Country
	}
	return &WeatherLocation{Lat: r.Latitude, Lon: r.Longitude, Label: label}, nil
}

func weatherBinaryInstalled() bool {
	_, err := os.Stat(weatherBinaryPath())
	return err == nil
}

func weatherLocationSet() bool {
	_, err := loadWeatherLocation()
	return err == nil
}

// registerWeatherMCPTool writes the claw-weather entry into ~/.picoclaw/config.json
// under tools.mcp.servers so PicoClaw's agent picks it up on next restart.
func registerWeatherMCPTool() error {
	cfg := readConfig()

	// Ensure tools.mcp.servers map exists
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]interface{})
	}
	mcp, ok := cfg.Tools["mcp"].(map[string]interface{})
	if !ok {
		mcp = make(map[string]interface{})
		cfg.Tools["mcp"] = mcp
	}
	servers, ok := mcp["servers"].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
		mcp["servers"] = servers
	}

	binPath := weatherBinaryPath()
	servers["claw-weather"] = map[string]interface{}{
		"command": binPath,
		"args":    []string{"--mode", "mcp"},
		"cwd":     weatherToolDir(),
	}

	writeConfig(cfg)
	return nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /api/weather/location
// Body (form): city=Mississauga  OR  lat=43.5&lon=-79.6&label=Custom
// GET  /api/weather/location  → returns stored location or 404
func handleWeatherLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		loc, err := loadWeatherLocation()
		if err != nil {
			http.Error(w, `{"error":"not set"}`, http.StatusNotFound)
			return
		}
		jsonResponse(w, map[string]interface{}{
			"ok":    true,
			"lat":   loc.Lat,
			"lon":   loc.Lon,
			"label": loc.Label,
		})
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(1 << 20)

	city := strings.TrimSpace(r.FormValue("city"))
	latStr := strings.TrimSpace(r.FormValue("lat"))
	lonStr := strings.TrimSpace(r.FormValue("lon"))
	label := strings.TrimSpace(r.FormValue("label"))

	var loc *WeatherLocation

	if city != "" {
		var err error
		loc, err = geocodeCity(city)
		if err != nil {
			errorResponse(w, err.Error())
			return
		}
	} else if latStr != "" && lonStr != "" {
		var lat, lon float64
		fmt.Sscanf(latStr, "%f", &lat)
		fmt.Sscanf(lonStr, "%f", &lon)
		if label == "" {
			label = fmt.Sprintf("%.4f, %.4f", lat, lon)
		}
		loc = &WeatherLocation{Lat: lat, Lon: lon, Label: label}
	} else {
		errorResponse(w, "provide city name or lat+lon")
		return
	}

	if err := saveWeatherLocation(*loc); err != nil {
		errorResponse(w, "Failed to save location: "+err.Error())
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ok":    true,
		"lat":   loc.Lat,
		"lon":   loc.Lon,
		"label": loc.Label,
	})
}

// POST /api/weather/install
// Builds the weather-mcp binary from source (go build) and registers it in agent config.
func handleWeatherInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	toolDir := weatherToolDir()
	binPath := weatherBinaryPath()

	// ── Step 1: find source ───────────────────────────────────────────────────
	// Source is expected at one of these locations (local dev or installed repo)
	home, _ := os.UserHomeDir()
	candidateSrcDirs := []string{
		filepath.Join(home, "claw-tools.dev", "tools", "weather"),
		filepath.Join(home, "clawtools", "tools", "weather"),
		"/opt/claw-tools/tools/weather",
	}
	srcDir := ""
	for _, d := range candidateSrcDirs {
		if _, err := os.Stat(filepath.Join(d, "main.go")); err == nil {
			srcDir = d
			break
		}
	}

	if srcDir == "" {
		// ── Fallback: download release binary ─────────────────────────────────
		arch := runtime.GOARCH // "arm64", "amd64", "arm"
		goos := runtime.GOOS   // "linux", "darwin"
		assetName := fmt.Sprintf("weather-mcp_%s_%s", goos, arch)
		downloadURL := fmt.Sprintf(
			"https://github.com/arpit0515/claw-tools.dev/releases/latest/download/%s",
			assetName,
		)
		os.MkdirAll(toolDir, 0755)
		out, err := runCommand("wget", "-L", "-q", "-O", binPath, downloadURL)
		if err != nil {
			// wget not available — try curl
			out, err = runCommand("curl", "-L", "-s", "-o", binPath, downloadURL)
		}
		if err != nil {
			errorResponse(w, "Source not found locally and download failed: "+strings.TrimSpace(out))
			return
		}
		os.Chmod(binPath, 0755)
	} else {
		// ── Build from source ─────────────────────────────────────────────────
		os.MkdirAll(toolDir, 0755)

		goPath, err := exec.LookPath("go")
		if err != nil {
			errorResponse(w, "go not found in PATH — install Go 1.21+ to build from source")
			return
		}

		cmd := exec.Command(goPath, "build", "-o", binPath, ".")
		cmd.Dir = srcDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			errorResponse(w, "Build failed: "+strings.TrimSpace(string(out)))
			return
		}
	}

	// ── Step 2: verify binary runs ────────────────────────────────────────────
	if _, err := os.Stat(binPath); err != nil {
		errorResponse(w, "Binary not found after build/download")
		return
	}

	// ── Step 3: register in agent config ─────────────────────────────────────
	if err := registerWeatherMCPTool(); err != nil {
		errorResponse(w, "Binary built but failed to register in agent config: "+err.Error())
		return
	}

	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"message": "weather-mcp installed and registered",
		"path":    binPath,
	})
}

// GET /api/weather/forecast
// Proxies a request to the local weather-mcp HTTP server (port 3104)
// or calls Open-Meteo directly using the stored location.
func handleWeatherForecast(w http.ResponseWriter, r *http.Request) {
	loc, err := loadWeatherLocation()
	if err != nil {
		errorResponse(w, "No location set — set a location first")
		return
	}

	// Try local weather-mcp HTTP server first
	localURL := fmt.Sprintf("http://localhost:3104/weather/forecast?lat=%.4f&lon=%.4f", loc.Lat, loc.Lon)
	resp, err := http.Get(localURL)
	if err == nil {
		defer resp.Body.Close()
		w.Header().Set("Content-Type", "application/json")
		var body map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&body) == nil {
			body["ok"] = true
			json.NewEncoder(w).Encode(body)
			return
		}
	}

	// Fallback: call Open-Meteo directly and return raw forecast JSON
	forecastURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&hourly=temperature_2m,precipitation_probability,weathercode"+
			"&current_weather=true&temperature_unit=celsius&forecast_days=1",
		loc.Lat, loc.Lon,
	)
	fResp, err := http.Get(forecastURL)
	if err != nil {
		errorResponse(w, "Could not fetch forecast: "+err.Error())
		return
	}
	defer fResp.Body.Close()

	var raw map[string]interface{}
	if err := json.NewDecoder(fResp.Body).Decode(&raw); err != nil {
		errorResponse(w, "Could not parse forecast response")
		return
	}
	raw["ok"] = true
	raw["location"] = loc.Label
	raw["lat"] = loc.Lat
	raw["lon"] = loc.Lon
	jsonResponse(w, raw)
}
