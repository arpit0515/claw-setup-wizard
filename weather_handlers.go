package main

// weather_handlers.go
// Drop this file into ~/Desktop/claw-setup-wizard/ replacing the existing one.
// Routes are already registered in main.go — no changes needed there.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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

// ── Paths ─────────────────────────────────────────────────────────────────────

func weatherBinPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", "bin", "weather-mcp")
}

func weatherBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", "bin")
}

func weatherSrcDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", ".claw-tools-repo", "tools", "weather")
}

func weatherConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config", "weather.json")
}

func weatherSkillDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", "skills", "claw-weather")
}

func weatherForecastScript() string {
	return filepath.Join(weatherBinDir(), "get_weather_forecast.sh")
}

func weatherNowScript() string {
	return filepath.Join(weatherBinDir(), "get_weather_now.sh")
}

// ── Location helpers ──────────────────────────────────────────────────────────

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

// ── Shell scripts ─────────────────────────────────────────────────────────────

func writeWeatherScripts() error {
	binDir := weatherBinDir()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	forecast := fmt.Sprintf("#!/bin/bash\ncurl -s http://localhost:3104/weather/forecast\n")
	if err := os.WriteFile(weatherForecastScript(), []byte(forecast), 0755); err != nil {
		return fmt.Errorf("failed to write forecast script: %w", err)
	}

	now := fmt.Sprintf("#!/bin/bash\ncurl -s http://localhost:3104/weather/now\n")
	if err := os.WriteFile(weatherNowScript(), []byte(now), 0755); err != nil {
		return fmt.Errorf("failed to write now script: %w", err)
	}

	return nil
}

// ── Skill SKILL.md ────────────────────────────────────────────────────────────

func writeWeatherSkill(locationLabel string) error {
	skillDir := weatherSkillDir()
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	workspaceDir := filepath.Join(home, ".picoclaw", "workspace")

	content := fmt.Sprintf(`---
name: claw-weather
description: "Get current weather and full day forecast for %s. Use for ANY weather question. Location is already saved — never ask the user. Always use exec commands below, never web_fetch."
user-invocable: true
---

# Claw Weather

Use the EXACT exec commands below. Do not use web_fetch or curl inline.
Default location is already set: %s
Working directory is %s

## Get full day forecast (morning / afternoon / evening + rain chance)
exec: %s

## Get current conditions right now
exec: %s

## Rules
- Use ONLY these exact exec commands
- working_dir MUST be %s
- The forecast returns JSON with a "summary" field — present that directly to the user
- NEVER ask for location — it is already configured
- NEVER use web_fetch, wttr.in, or openmeteo CLI
`,
		locationLabel,
		locationLabel,
		workspaceDir,
		weatherForecastScript(),
		weatherNowScript(),
		workspaceDir,
	)

	skillPath := filepath.Join(skillDir, "SKILL.md")
	return os.WriteFile(skillPath, []byte(content), 0644)
}

// ── Systemd service ───────────────────────────────────────────────────────────

func installWeatherService() error {
	home, _ := os.UserHomeDir()
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)

	content := fmt.Sprintf(`[Unit]
Description=ClawTools Weather MCP
After=network.target

[Service]
Type=simple
ExecStart=%s --mode http --port 3104
Restart=on-failure
RestartSec=5
WorkingDirectory=%s
Environment=HOME=%s

[Install]
WantedBy=default.target
`, weatherBinPath(), weatherBinDir(), home)

	servicePath := filepath.Join(serviceDir, "claw-weather.service")
	if err := os.WriteFile(servicePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	for _, cmd := range [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "claw-weather"},
		{"systemctl", "--user", "start", "claw-weather"},
	} {
		if out, err := runCommand(cmd[0], cmd[1:]...); err != nil {
			return fmt.Errorf("%s failed: %s", strings.Join(cmd, " "), out)
		}
	}
	return nil
}

func weatherServiceRunning() bool {
	out, err := runCommand("systemctl", "--user", "is-active", "claw-weather")
	return err == nil && strings.TrimSpace(out) == "active"
}

// ── PATH injection ────────────────────────────────────────────────────────────

func ensureBinInPath() {
	home, _ := os.UserHomeDir()
	binDir := weatherBinDir()
	rcPath := filepath.Join(home, ".bashrc")
	marker := "# claw-tools bin"

	data, _ := os.ReadFile(rcPath)
	if strings.Contains(string(data), marker) {
		return
	}

	entry := fmt.Sprintf("\n%s\nexport PATH=\"%s:$PATH\"\n", marker, binDir)
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(entry)
}

// ── Updated installSystemdService (replaces the one in handlers.go) ───────────
// DELETE the existing installSystemdService() from handlers.go
// and use this one instead — it injects PATH so the agent can find weather-mcp.

func installSystemdService() (bool, string) {
	piocławPath, err := runCommand("which", "picoclaw")
	if err != nil || piocławPath == "" {
		return false, "picoclaw not found in PATH"
	}
	home, _ := os.UserHomeDir()
	binDir := weatherBinDir()
	currentPath := "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin"
	fullPath := binDir + ":" + currentPath

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
Environment=PATH=%s

[Install]
WantedBy=default.target
`, strings.TrimSpace(piocławPath), home, home, fullPath)

	servicePath := filepath.Join(serviceDir, "picoclaw.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return false, "Failed to write service file: " + err.Error()
	}
	for _, cmd := range [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "picoclaw"},
		{"systemctl", "--user", "start", "picoclaw"},
	} {
		if out, err := runCommand(cmd[0], cmd[1:]...); err != nil {
			return false, strings.TrimSpace(out)
		}
	}
	return true, "Service installed and started"
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GET /api/weather/status
func handleWeatherStatus(w http.ResponseWriter, r *http.Request) {
	_, binErr := os.Stat(weatherBinPath())
	binInstalled := binErr == nil
	svcRunning := weatherServiceRunning()

	_, forecastScriptErr := os.Stat(weatherForecastScript())
	_, skillErr := os.Stat(filepath.Join(weatherSkillDir(), "SKILL.md"))

	loc, locErr := loadWeatherLocation()

	resp := map[string]interface{}{
		"ok":              true,
		"bin_installed":   binInstalled,
		"svc_running":     svcRunning,
		"scripts_written": forecastScriptErr == nil,
		"skill_written":   skillErr == nil,
		"location_set":    locErr == nil,
	}
	if loc != nil {
		resp["label"] = loc.Label
		resp["lat"] = loc.Lat
		resp["lon"] = loc.Lon
	}
	jsonResponse(w, resp)
}

// GET/POST /api/weather/location
func handleWeatherLocation(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		loc, err := loadWeatherLocation()
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			jsonResponse(w, map[string]interface{}{"ok": false, "error": "not set"})
			return
		}
		jsonResponse(w, map[string]interface{}{
			"ok": true, "lat": loc.Lat, "lon": loc.Lon, "label": loc.Label,
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

	// Update skill with new location label
	writeWeatherSkill(loc.Label)

	jsonResponse(w, map[string]interface{}{
		"ok": true, "lat": loc.Lat, "lon": loc.Lon, "label": loc.Label,
	})
}

// POST /api/weather/install
// 1. Copies binary from repo to workspace/bin/
// 2. Writes shell scripts (get_weather_forecast.sh, get_weather_now.sh)
// 3. Writes skills/claw-weather/SKILL.md
// 4. Installs claw-weather.service (starts on boot)
// 5. Ensures PATH in ~/.bashrc
// 6. Restarts picoclaw so it picks up the new skill
func handleWeatherInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	binPath := weatherBinPath()
	binDir := weatherBinDir()
	os.MkdirAll(binDir, 0755)

	// ── Step 1: get the binary ────────────────────────────────────────────────
	repoBin := filepath.Join(weatherSrcDir(), "weather-mcp")
	if _, err := os.Stat(repoBin); err == nil {
		if out, err := runCommand("cp", repoBin, binPath); err != nil {
			errorResponse(w, "Failed to copy binary: "+out)
			return
		}
		os.Chmod(binPath, 0755)
	} else if _, err := os.Stat(binPath); err != nil {
		errorResponse(w, "weather-mcp binary not found — run install.sh first to build it")
		return
	}

	// ── Step 2: write shell scripts ───────────────────────────────────────────
	if err := writeWeatherScripts(); err != nil {
		errorResponse(w, "Failed to write scripts: "+err.Error())
		return
	}

	// ── Step 3: write SKILL.md ────────────────────────────────────────────────
	loc, _ := loadWeatherLocation()
	locationLabel := "your default location"
	if loc != nil {
		locationLabel = loc.Label
	}
	if err := writeWeatherSkill(locationLabel); err != nil {
		errorResponse(w, "Failed to write skill: "+err.Error())
		return
	}

	// ── Step 4: install systemd service ──────────────────────────────────────
	if err := installWeatherService(); err != nil {
		// Non-fatal — binary + skill still work
		jsonResponse(w, map[string]interface{}{
			"ok":      true,
			"message": "weather-mcp installed (systemd setup failed: " + err.Error() + " — start manually with: weather-mcp --mode http &)",
			"path":    binPath,
			"warning": err.Error(),
		})
		return
	}

	// ── Step 5: ensure PATH in ~/.bashrc ─────────────────────────────────────
	ensureBinInPath()

	// ── Step 6: restart picoclaw to load the new skill ────────────────────────
	runCommand("systemctl", "--user", "restart", "picoclaw")

	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"message": "weather-mcp installed, skill registered, service started, agent restarted",
		"path":    binPath,
	})
}

// GET /api/weather/forecast
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
		var body map[string]interface{}
		if json.NewDecoder(resp.Body).Decode(&body) == nil {
			body["ok"] = true
			jsonResponse(w, body)
			return
		}
	}

	// Fallback: call Open-Meteo directly
	forecastURL := fmt.Sprintf(
		"https://api.open-meteo.com/v1/forecast?latitude=%.4f&longitude=%.4f"+
			"&hourly=temperature_2m,apparent_temperature,precipitation_probability,weathercode,windspeed_10m,relativehumidity_2m"+
			"&current_weather=true&temperature_unit=celsius&windspeed_unit=kmh&forecast_days=1",
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
