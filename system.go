package main

import (
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	)

// SystemStatus that holds everything we check on the Pi
type SystemStatus struct {
	OS              string `json:"os"`
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
	HasClawTools    bool   `json:"has_claw_tools"`
	ClawToolsCount  int    `json:"claw_tools_count"`
	Checklist	struct	{
		System	bool	`json:"system"`
		Provider	bool	`json:"provider"`
		Telegram	bool	`json:"telegram"`
		Soul	bool	`json:"soul"`
		Tools   bool    `json:"tools"`
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

	// Detect OS
	osName, _ := runCommand("uname", "-s")
	osName = strings.TrimSpace(osName)
	if osName == "Darwin" {
		s.OS = "mac"
	} else {
		s.OS = "linux"
	}

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

	// RAM
	out, err = runCommand("free", "-h")
	if err == nil {
		lines := strings.Split(out, "\n")
		if len(lines) >= 2{
			fields := strings.Fields(lines[1])
			if len(fields) >=7 {
				s.RAM = fields[6] + " free of " + fields[1]
			}
		}
	}

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

	// Service status — launchctl on macOS, systemctl on Linux
	if s.OS == "mac" {
		out, err = runCommand("launchctl", "list", "com.picoclaw.agent")
		if err == nil && !strings.Contains(out, "Could not find service") {
			s.ServiceStatus = "active"
		} else {
			s.ServiceStatus = "inactive"
		}
	} else {
		out, err = runCommand("systemctl", "--user", "is-active", "picoclaw")
		if err == nil && strings.TrimSpace(out) == "active" {
			s.ServiceStatus = "active"
		} else {
			s.ServiceStatus = "inactive"
		}
	}

	// ClawTools
	s.ClawToolsCount = countInstalledTools()
	s.HasClawTools = s.ClawToolsCount > 0

	// Checklist
	s.Checklist.System = s.PicoclawInstalled
	s.Checklist.Provider = s.HasProvider
	s.Checklist.Telegram = s.HasTelegram
	s.Checklist.Soul = s.HasSoul
	s.Checklist.Tools = s.HasClawTools
	s.Checklist.Service = s.ServiceStatus == "active"

	return s
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