package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	)

// SystemStatus that holds everything we check on the Pi
type SystemStatus struct {
	PicoclawInstalled	bool	`json:"picoclaw_installed"`
	PicoclawVersion	string	`json:"picoclaw_version"`
	DiskSpace	string	`json:"disk_space"`
	RAM	string	`json:"ram"`
	ConfigExists	bool	`json:"config_exists"`
	HasProvider	bool	`json:"has_provider"`
	HasTelegram	bool	`json:"has_telegram"`
	HasSoul	bool	`json:"has_soul"`
	ActiveModel     string `json:"active_model"`
	ActiveProvider  string `json:"active_provider"`
	TelegramToken   string `json:"telegram_token"`
	TelegramUser    string `json:"telegram_user"`
	ServiceStatus	string	`json:"service_status"`
	OS		string	`json:"os"`
	Checklist	struct	{
		System	bool	`json:"system"`
		Provider	bool	`json:"provider"`
		Telegram	bool	`json:"telegram"`
		Soul	bool	`json:"soul"`
		Service	bool	`json:"service"`
		}	`json:"checklist"`
	}

func handleSystemCheck(w http.ResponseWriter, r *http.Request) {
	status := buildSystemStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func buildSystemStatus() SystemStatus {
	var s SystemStatus
	// Check PicoClaw
	path, err := exec.LookPath("picoclaw")
	if err == nil && path != "" {
		s.PicoclawInstalled = true
		out, _ := runCommand("picoclaw", "version")
		if out == "" {
			out = "installed"
		}
		s.PicoclawVersion = out
	}

	// Disk Space
	out, err := runCommand("df", "-h", "/")
	if err == nil {
		lines := strings.Split(out, "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[1])
			if len(fields) >= 4 {
				s.DiskSpace = fields[3] + " free of " + fields[1]
			}
		}
	}

	// RAM — OS-aware
	s.RAM = getRAM()

	// Config
	configPath := getConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		s.ConfigExists = true
		cfg := readConfig()
		if len(cfg.Providers) > 0 {
			s.HasProvider = true
			for name := range cfg.Providers {
				s.ActiveProvider = name
				break
			}
		}
		// Read active model from agents.defaults
		if defaults, ok := cfg.Agents["defaults"].(map[string]interface{}); ok {
			if model, ok := defaults["model"].(string); ok && model != "" {
				s.ActiveModel = model
			}
			if provider, ok := defaults["provider"].(string); ok && provider != "" {
				s.ActiveProvider = provider
			}
		}

		if tg, ok := cfg.Channels["telegram"]; ok {
			if token, ok := tg["token"].(string); ok && token != "" {
				s.HasTelegram = true
				s.TelegramToken = token[:10] + "..." // masked for security
			}
			if users, ok := tg["allowFrom"].([]interface{}); ok && len(users) > 0 {
				if uid, ok := users[0].(string); ok {
					s.TelegramUser = uid
				}
			}
		}
	}
	//	if tg, ok := cfg.Channels["telegram"]; ok {
	//		if token, ok := tg["token"].(string); ok && token != "" {
	//			s.HasTelegram = true
	//		}
	//	}
	//}


	// Soul.md
	soulPath := getSoulPath()
	if _, err := os.Stat(soulPath); err == nil {
		s.HasSoul = true
	}

	// Service status — OS-aware
	s.ServiceStatus = getServiceStatus()

	// OS
	if runtime.GOOS == "darwin" {
		s.OS = "mac"
	} else {
		s.OS = "linux"
	}

	// Checklist
	s.Checklist.System = s.PicoclawInstalled
	s.Checklist.Provider = s.HasProvider
	s.Checklist.Telegram = s.HasTelegram
	s.Checklist.Soul = s.HasSoul
	s.Checklist.Service = s.ServiceStatus == "active"

	return s
}

// ------- Service Helpers -------

func getServiceStatus() string {
	if runtime.GOOS == "darwin" {
		// launchctl list returns a line with the label if loaded
		out, err := runCommand("launchctl", "list", "com.picoclaw.agent")
		if err == nil && !strings.Contains(out, "Could not find") && out != "" {
			return "active"
		}
		return "inactive"
	}
	// Linux systemd
	out, err := runCommand("systemctl", "--user", "is-active", "picoclaw")
	if err == nil && strings.TrimSpace(out) == "active" {
		return "active"
	}
	return "inactive"
}

// ------- RAM Helpers -------

func getRAM() string {
	if runtime.GOOS == "darwin" {
		return getMacRAM()
	}
	return getLinuxRAM()
}

// macOS: vm_stat for free pages + sysctl for total
func getMacRAM() string {
	// Total RAM via sysctl
	totalOut, err := runCommand("sysctl", "-n", "hw.memsize")
	if err != nil {
		return "unavailable"
	}
	totalBytes, err := strconv.ParseInt(strings.TrimSpace(totalOut), 10, 64)
	if err != nil {
		return "unavailable"
	}

	// Free pages via vm_stat
	vmOut, err := runCommand("vm_stat")
	if err != nil {
		return "unavailable"
	}

	var pageSize int64 = 4096
	var freePages, inactivePages int64
	for _, line := range strings.Split(vmOut, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Mach Virtual Memory Statistics") {
			// Extract page size: "page size of 4096 bytes"
			var ps int64
			if _, err := fmt.Sscanf(line, "Mach Virtual Memory Statistics: (page size of %d bytes)", &ps); err == nil {
				pageSize = ps
			}
		}
		var val int64
		if strings.HasPrefix(line, "Pages free:") {
			fmt.Sscanf(strings.TrimRight(strings.Split(line, ":")[1], "."), "%d", &val)
			freePages = val
		}
		if strings.HasPrefix(line, "Pages inactive:") {
			fmt.Sscanf(strings.TrimRight(strings.Split(line, ":")[1], "."), "%d", &val)
			inactivePages = val
		}
	}

	freeBytes := (freePages + inactivePages) * pageSize
	return formatBytes(freeBytes) + " free of " + formatBytes(totalBytes)
}

// Linux: read /proc/meminfo directly — works everywhere, no column guessing
func getLinuxRAM() string {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "unavailable"
	}

	var totalKB, availKB int64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		val, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = val
		case "MemAvailable:":
			availKB = val
		}
	}

	if totalKB == 0 {
		return "unavailable"
	}
	return formatBytes(availKB*1024) + " free of " + formatBytes(totalKB*1024)
}

func formatBytes(b int64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if b >= gb {
		return fmt.Sprintf("%.1fGB", float64(b)/float64(gb))
	}
	return fmt.Sprintf("%.0fMB", float64(b)/float64(mb))
}

// ------- Config Helpers -------

type PicoConfig struct {
	Agents	map[string]interface{}	`json:"agents,omitempty"`
	Providers	map[string]map[string]interface{}	`json:"providers,omitempty"`
	Channels	map[string]map[string]interface{}	`json:"channels,omitempty"`
	Tools	map[string]interface{}	`json:"tools,omitempty"`
}

func getConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "config.json")
}

func getSoulPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", "SOUL.md")
}

func readConfig() PicoConfig {
	var cfg PicoConfig
	data, err := os.ReadFile(getConfigPath())
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, &cfg)
	return cfg
}

func writeConfig(cfg PicoConfig) error {
	path := getConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.MarshalIndent(cfg, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func jsonResponse(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func errorResponse(w http.ResponseWriter, msg string) {
	jsonResponse(w, map[string]interface{}{
		"ok":	false,
		"message":	msg,
	})
}

func okResponse(w http.ResponseWriter, msg string, extra map[string]interface{}) {
	resp := map[string]interface{}{
		"ok":	true,
		"message":	msg,
	}
	for k, v := range extra {
		resp[k] = v
	}
	jsonResponse(w, resp)
}
