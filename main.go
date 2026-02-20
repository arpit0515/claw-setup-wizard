package main
import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strings"
	)

//go:embed templates/*
var templateFiles embed.FS
var tmpl *template.Template

func main() {
	var err error
	tmpl, err = template.ParseFS(templateFiles, "templates/*.html")
	if err != nil {
		log.Fatal("Could not load templates:", err)
	}

	mux := http.NewServeMux()
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

	ip := getLocalIP()
	fmt.Println(" *** claw-setup is running **** ")
	fmt.Println("--------------------------------")
	fmt.Println("  Local: http://localhost:3000  ")
	fmt.Println(" Network: http://%s:3000", ip)
	fmt.Println("--------------------------------")
	fmt.Println(" Open either address in browser ")
	fmt.Println("    Press Ctrl + C to stop      ")
	log.Fatal(http.ListenAndServe("0.0.0.0:3000", mux))
}

func handleIndex (w http.ResponseWriter, r *http.Request) {
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
