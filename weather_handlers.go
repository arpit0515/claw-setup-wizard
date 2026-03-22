package main

// weather_handlers.go
// Drop this file into your claw-setup-wizard alongside handlers.go, system.go, etc.
// Register routes in main.go:
//   mux.HandleFunc("/api/weather/location", handleWeatherLocation)
//   mux.HandleFunc("/api/weather/install",  handleWeatherInstall)
//   mux.HandleFunc("/api/weather/forecast", handleWeatherForecast)
//   mux.HandleFunc("/api/weather/status",   handleWeatherStatus)

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
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

// ── Paths — match the actual Pi layout ───────────────────────────────────────
//
// Binary lives at:  ~/.picoclaw/workspace/bin/weather-mcp
// Source lives at:  ~/.picoclaw/workspace/.claw-tools-repo/tools/weather
// Location config:  ~/.picoclaw/config/weather.json
// Systemd service:  ~/.config/systemd/user/claw-weather.service

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

// ── PATH injection ────────────────────────────────────────────────────────────
// Ensures ~/.picoclaw/workspace/bin is in the user's PATH permanently
// by appending to ~/.bashrc if not already present. This is idempotent.

func ensureBinInPath() {
	home, _ := os.UserHomeDir()
	binDir := weatherBinDir()
	rcPath := filepath.Join(home, ".bashrc")

	line := fmt.Sprintf(`export PATH="%s:$PATH"`, binDir)
	marker := "# claw-tools bin"

	data, _ := os.ReadFile(rcPath)
	if strings.Contains(string(data), marker) {
		return // already added
	}

	entry := fmt.Sprintf("\n%s\n%s\n", marker, line)
	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString(entry)
}

// ── MCP registration ──────────────────────────────────────────────────────────
// Writes claw-weather into config.json under mcpServers so PicoClaw picks it up.
// Uses cfg.MCPServers (the field PicoClaw actually reads at runtime).

func registerWeatherMCPTool() error {
	toolsPath := filepath.Join(os.Getenv("HOME"),
		".picoclaw", "workspace", ".claw-tools-repo", "tools.json")

	data, err := os.ReadFile(toolsPath)
	if err != nil {
		return fmt.Errorf("tools.json not found: %w", err)
	}

	var tools []map[string]interface{}
	if err := json.Unmarshal(data, &tools); err != nil {
		return fmt.Errorf("invalid tools.json: %w", err)
	}

	// Find weather entry and set status to available
	for i, t := range tools {
		if t["id"] == "weather" {
			tools[i]["status"] = "available"
			tools[i]["service_start"] = weatherBinPath() + " --mode http --port 3104"
			break
		}
	}

	out, err := json.MarshalIndent(tools, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(toolsPath, out, 0644)
}

// ── Systemd service for weather-mcp ──────────────────────────────────────────
// Installs claw-weather.service so the HTTP server on :3104 starts on boot
// and stays running without any manual intervention.

func installWeatherService() error {
	home, _ := os.UserHomeDir()
	binPath := weatherBinPath()
	binDir := weatherBinDir()

	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)

	serviceContent := fmt.Sprintf(`[Unit]
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
`, binPath, binDir, home)

	servicePath := filepath.Join(serviceDir, "claw-weather.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write service file: %w", err)
	}

	cmds := [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "claw-weather"},
		{"systemctl", "--user", "start", "claw-weather"},
	}
	for _, cmd := range cmds {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return fmt.Errorf("%s failed: %s", strings.Join(cmd, " "), strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func weatherServiceRunning() bool {
	out, err := exec.Command("systemctl", "--user", "is-active", "claw-weather").Output()
	return err == nil && strings.TrimSpace(string(out)) == "active"
}

// ── installSystemdService patch ───────────────────────────────────────────────
// Replace your existing installSystemdService() with this version.
// It injects PATH into the picoclaw.service Environment= line so the agent
// process can find weather-mcp and other bin tools without manual PATH setup.

func installSystemdService() (bool, string) {
	piocławPath, err := exec.LookPath("picoclaw")
	if err != nil {
		return false, "picoclaw not found in PATH"
	}
	home, _ := os.UserHomeDir()
	binDir := weatherBinDir()

	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)

	// Inject bin dir into PATH so picoclaw agent subprocess can find weather-mcp
	currentPath := os.Getenv("PATH")
	fullPath := binDir + ":" + currentPath

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
`, piocławPath, home, home, fullPath)

	servicePath := filepath.Join(serviceDir, "picoclaw.service")
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return false, "Failed to write service file: " + err.Error()
	}
	for _, cmd := range [][]string{
		{"systemctl", "--user", "daemon-reload"},
		{"systemctl", "--user", "enable", "picoclaw"},
		{"systemctl", "--user", "start", "picoclaw"},
	} {
		if out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput(); err != nil {
			return false, strings.TrimSpace(string(out))
		}
	}
	return true, "Service installed and started"
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// GET /api/weather/status — returns binary installed, service running, location set
func handleWeatherStatus(w http.ResponseWriter, r *http.Request) {
	_, binErr := os.Stat(weatherBinPath())
	binInstalled := binErr == nil
	svcRunning := weatherServiceRunning()
	loc, locErr := loadWeatherLocation()

	resp := map[string]interface{}{
		"ok":            true,
		"bin_installed": binInstalled,
		"svc_running":   svcRunning,
		"location_set":  locErr == nil,
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
	jsonResponse(w, map[string]interface{}{
		"ok": true, "lat": loc.Lat, "lon": loc.Lon, "label": loc.Label,
	})
}

// POST /api/weather/install
// 1. Copies binary from the cloned repo bin dir (already built by install.sh)
// 2. Ensures ~/.bashrc has the bin dir in PATH
// 3. Registers MCP tool in config.json (mcpServers)
// 4. Installs + starts claw-weather.service
// 5. Restarts picoclaw.service so it picks up the new MCP entry
func handleWeatherInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	binPath := weatherBinPath()
	binDir := weatherBinDir()
	os.MkdirAll(binDir, 0755)

	// ── Step 1: get the binary ────────────────────────────────────────────────
	// Primary: already built binary in the cloned repo
	repoBin := filepath.Join(weatherSrcDir(), "weather-mcp")
	if _, err := os.Stat(repoBin); err == nil {
		// Copy from repo to workspace bin
		if out, err := runCommand("cp", repoBin, binPath); err != nil {
			errorResponse(w, "Failed to copy binary: "+out)
			return
		}
		os.Chmod(binPath, 0755)
	} else if _, err := os.Stat(binPath); err != nil {
		// Binary not in repo and not already in place — try building from source
		srcDir := weatherSrcDir()
		if _, err := os.Stat(filepath.Join(srcDir, "main.go")); err != nil {
			errorResponse(w, "weather-mcp source not found — run install.sh first")
			return
		}
		goPath, err := exec.LookPath("go")
		if err != nil {
			errorResponse(w, "go not found in PATH")
			return
		}
		cmd := exec.Command(goPath, "build", "-o", binPath, ".")
		cmd.Dir = srcDir
		if out, err := cmd.CombinedOutput(); err != nil {
			errorResponse(w, "Build failed: "+strings.TrimSpace(string(out)))
			return
		}
		os.Chmod(binPath, 0755)
	}
	// If binary already exists at binPath, nothing to do for step 1

	// ── Step 2: ensure PATH in ~/.bashrc ──────────────────────────────────────
	ensureBinInPath()

	// ── Step 3: register MCP tool in config.json ─────────────────────────────
	if err := registerWeatherMCPTool(); err != nil {
		errorResponse(w, "Failed to register MCP tool: "+err.Error())
		return
	}

	// ── Step 4: install + start claw-weather systemd service ─────────────────
	if err := installWeatherService(); err != nil {
		// Non-fatal — binary + MCP registration still worked
		jsonResponse(w, map[string]interface{}{
			"ok":      true,
			"message": "weather-mcp installed and registered (systemd setup failed: " + err.Error() + ")",
			"path":    binPath,
			"warning": err.Error(),
		})
		return
	}

	// ── Step 5: restart picoclaw so it loads the new mcpServers entry ─────────
	exec.Command("systemctl", "--user", "restart", "picoclaw").Run()

	jsonResponse(w, map[string]interface{}{
		"ok":      true,
		"message": "weather-mcp installed, service started, agent restarted",
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

	// Try local weather-mcp HTTP server first (port 3104)
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
