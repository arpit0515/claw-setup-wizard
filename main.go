package main

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
)

//go:embed templates/*
var templateFiles embed.FS
var tmpl *template.Template

//go:embed static
var staticFiles embed.FS

func main() {
	freePort(3000)
	freePort(3455) // OAuth callback port

	initDirs()

	var err error
	tmpl, err = template.ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatal("Could not load templates:", err)
	}
	staticFS, _ := fs.Sub(staticFiles, "static")

	mux := http.NewServeMux()

	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// ── Existing routes ───────────────────────────────────────────────────────
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/system-check", handleSystemCheck)
	mux.HandleFunc("/api/validate-llm", handleValidateLLM)
	mux.HandleFunc("/api/validate-telegram", handleValidateTelegram)
	mux.HandleFunc("/api/save-telegram-user", handleSaveTelegramUser)
	mux.HandleFunc("/api/ping-telegram", handlePingTelegram)
	mux.HandleFunc("/api/generate-soul", handleGenerateSoul)
	mux.HandleFunc("/api/save-soul", handleSaveSoul)
	mux.HandleFunc("/api/install-service", handleInstallService)
	mux.HandleFunc("/api/health", handleHealth)
	mux.HandleFunc("/api/install-picoclaw", handleInstallPicoclaw)
	mux.HandleFunc("/api/models", handleGetModels)

	// ── Tools & OAuth routes ──────────────────────────────────────────────────
	mux.HandleFunc("/api/tools", handleGetTools)
	mux.HandleFunc("/api/tools/refresh", handleToolsRefresh)
	mux.HandleFunc("/api/oauth/start", handleOAuthStart)
	mux.HandleFunc("/api/oauth/status", handleOAuthStatus)
	mux.HandleFunc("/api/oauth/accounts", handleOAuthAccounts)
	mux.HandleFunc("/api/oauth/revoke", handleOAuthRevoke)
	mux.HandleFunc("/api/oauth/creds-status", handleCredsStatus)
	mux.HandleFunc("/oauth/callback", handleOAuthCallback)

	mux.HandleFunc("/api/restart-service", handleRestartService)
	mux.HandleFunc("/api/local-ip", handleLocalIP)

	mux.HandleFunc("/api/weather/status", handleWeatherStatus)
	mux.HandleFunc("/api/weather/location", handleWeatherLocation)
	mux.HandleFunc("/api/weather/install", handleWeatherInstall)
	mux.HandleFunc("/api/weather/forecast", handleWeatherForecast)

	// ── PicoClaw version & update routes ──────────────────────────────────────
	mux.HandleFunc("/api/picoclaw/version", handlePicoClawVersion)
	mux.HandleFunc("/api/picoclaw/update", handlePicoClawUpdate)

	// ── Uninstall ─────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/uninstall", handleUninstall)

	ip := getLocalIP()
	fmt.Println(" *** claw-setup is running **** ")
	fmt.Println("--------------------------------")
	fmt.Println("  Local: http://localhost:3000  ")
	fmt.Printf(" Network: http://%s:3000\n", ip)
	fmt.Println("--------------------------------")
	fmt.Println(" Open either address in browser ")
	fmt.Println("    Press Ctrl + C to stop      ")
	log.Fatal(http.ListenAndServe("0.0.0.0:3000", mux))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl.ExecuteTemplate(w, "index.html", nil)
}

func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok &&
			!ipnet.IP.IsLoopback() &&
			ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "localhost"
}

func runCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
