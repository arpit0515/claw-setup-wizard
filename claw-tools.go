package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ── Tool registry ─────────────────────────────────────────────────────────────
//
// Single source of truth for what tools exist and where they live.
// When you add a new tool to claw-tools.dev, add one entry here.
// The wizard will automatically show it, let users install it, and
// generate the right MCP config snippet.

type ClawTool struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Icon        string   `json:"icon"`
	Status      string   `json:"status"`              // "available" | "coming-soon"
	MCPTools    []string `json:"mcp_tools"`           // exposed MCP tool names
	HTTPPort    int      `json:"http_port"`           // port the tool server runs on
	Dir         string   `json:"dir"`                 // path inside claw-tools.dev repo
	ReqAuth     []string `json:"requires_auth,omitempty"` // e.g. ["google_oauth2"]
	Installed   bool     `json:"installed"`
}

// ── Add new tools here as you build them in claw-tools.dev ───────────────────

var clawToolRegistry = []ClawTool{
	{
		ID:          "gmail",
		Name:        "Gmail Connector",
		Description: "Read and search Gmail. Powers email summaries and morning briefings.",
		Icon:        "📧",
		Status:      "coming-soon",
		MCPTools:    []string{"gmail_list", "gmail_search", "gmail_get"},
		HTTPPort:    3101,
		Dir:         "tools/gmail",
		ReqAuth:     []string{"google_oauth2"},
	},
	{
		ID:          "gcal",
		Name:        "Google Calendar",
		Description: "Today's schedule and upcoming events. Essential for morning briefings.",
		Icon:        "📅",
		Status:      "coming-soon",
		MCPTools:    []string{"gcal_today", "gcal_upcoming", "gcal_get"},
		HTTPPort:    3102,
		Dir:         "tools/gcal",
		ReqAuth:     []string{"google_oauth2"},
	},
	{
		ID:          "outlook",
		Name:        "Outlook / Exchange",
		Description: "Microsoft Graph API connector for Outlook mail and Exchange calendar.",
		Icon:        "📨",
		Status:      "coming-soon",
		MCPTools:    []string{"outlook_list", "outlook_calendar"},
		HTTPPort:    3103,
		Dir:         "tools/outlook",
		ReqAuth:     []string{"microsoft_oauth2"},
	},
	{
		ID:          "weather",
		Name:        "Weather",
		Description: "Current conditions and daily forecasts for morning briefing context.",
		Icon:        "🌤️",
		Status:      "coming-soon",
		MCPTools:    []string{"weather_now", "weather_forecast"},
		HTTPPort:    3104,
		Dir:         "tools/weather",
	},
	{
		ID:          "deals",
		Name:        "Grocery Deals",
		Description: "Weekly flyer deals from nearby stores. Let your agent plan smarter shopping.",
		Icon:        "🛒",
		Status:      "coming-soon",
		MCPTools:    []string{"deals_search", "deals_nearby"},
		HTTPPort:    3105,
		Dir:         "tools/deals",
	},
}

// ── Paths ─────────────────────────────────────────────────────────────────────

const clawToolsRepo = "https://github.com/arpit0515/claw-tools.dev"

func clawRepoDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claw", "tools-repo")
}

func clawToolDir(toolID string) string {
	for _, t := range clawToolRegistry {
		if t.ID == toolID {
			return filepath.Join(clawRepoDir(), t.Dir)
		}
	}
	return ""
}

// isToolInstalled checks for a .installed marker (written after a successful
// go build) or the compiled binary itself. This avoids rebuilding on every
// wizard open and works even if the binary was renamed.
func isToolInstalled(toolID string) bool {
	dir := clawToolDir(toolID)
	if dir == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, ".installed")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, toolID)); err == nil {
		return true
	}
	return false
}

func countInstalledTools() int {
	count := 0
	for _, t := range clawToolRegistry {
		if isToolInstalled(t.ID) {
			count++
		}
	}
	return count
}

// ── Repo management ───────────────────────────────────────────────────────────

func ensureClawRepo() error {
	repoDir := clawRepoDir()
	gitDir := filepath.Join(repoDir, ".git")

	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(repoDir), 0755)
		out, err := exec.Command("git", "clone", "--depth=1", "--quiet", clawToolsRepo, repoDir).CombinedOutput()
		if err != nil {
			return fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(out)))
		}
	} else {
		// Pull latest — non-fatal, use what we have if this fails
		cmd := exec.Command("git", "pull", "--quiet")
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("git pull warning: %s\n", strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// ── /api/claw-tools-list ─────────────────────────────────────────────────────
// Returns the full tool registry with live installed status.
// Clones claw-tools.dev on first call, pulls on subsequent calls.

func handleClawToolsList(w http.ResponseWriter, r *http.Request) {
	if _, err := exec.LookPath("git"); err != nil {
		errorResponse(w, "git is not installed — install git to use ClawTools")
		return
	}
	if _, err := exec.LookPath("go"); err != nil {
		errorResponse(w, "Go is not installed — install Go 1.21+ to use ClawTools")
		return
	}

	if err := ensureClawRepo(); err != nil {
		errorResponse(w, "Could not fetch ClawTools: "+err.Error())
		return
	}

	tools := make([]ClawTool, len(clawToolRegistry))
	copy(tools, clawToolRegistry)
	for i := range tools {
		tools[i].Installed = isToolInstalled(tools[i].ID)
	}

	jsonResponse(w, map[string]interface{}{
		"ok":    true,
		"tools": tools,
	})
}

// ── ToolResult — returned per tool after an install attempt ──────────────────

type ToolResult struct {
	OK      bool     `json:"ok"`
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Dir     string   `json:"dir,omitempty"`
	Error   string   `json:"error,omitempty"`
	ReqAuth []string `json:"requires_auth,omitempty"`
}

// ── /api/install-claw-tools ──────────────────────────────────────────────────
// Body: tool_ids = JSON array e.g. ["gmail","gcal"]
// Pulls repo, builds each tool binary, writes .installed marker.
// Returns granular per-tool results so the UI can show individual status.

func handleInstallClawTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(10 << 20)

	var toolIDs []string
	if err := json.Unmarshal([]byte(r.FormValue("tool_ids")), &toolIDs); err != nil || len(toolIDs) == 0 {
		errorResponse(w, "tool_ids must be a non-empty JSON array of tool ID strings")
		return
	}

	if err := ensureClawRepo(); err != nil {
		errorResponse(w, "Could not sync ClawTools repo: "+err.Error())
		return
	}

	var results []ToolResult
	var installed, failed []string

	for _, id := range toolIDs {
		res := installOneTool(id)
		results = append(results, res)
		if res.OK {
			installed = append(installed, id)
		} else {
			failed = append(failed, id)
		}
	}

	jsonResponse(w, map[string]interface{}{
		"ok":         len(installed) > 0,
		"results":    results,
		"installed":  installed,
		"failed":     failed,
		"mcp_config": buildMCPConfig(installed),
	})
}

// installOneTool compiles one tool and writes a .installed marker on success.
func installOneTool(id string) ToolResult {
	var tool *ClawTool
	for i := range clawToolRegistry {
		if clawToolRegistry[i].ID == id {
			tool = &clawToolRegistry[i]
			break
		}
	}
	if tool == nil {
		return ToolResult{OK: false, ID: id, Name: id, Error: "unknown tool ID"}
	}

	toolDir := filepath.Join(clawRepoDir(), tool.Dir)

	if _, err := os.Stat(filepath.Join(toolDir, "go.mod")); os.IsNotExist(err) {
		return ToolResult{
			OK: false, ID: id, Name: tool.Name,
			Error: "not in repo yet — update ClawTools and retry",
		}
	}

	cmd := exec.Command("go", "build", "-o", id, ".")
	cmd.Dir = toolDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return ToolResult{
			OK: false, ID: id, Name: tool.Name, Dir: toolDir,
			Error: strings.TrimSpace(string(out)),
		}
	}

	// Write marker so isToolInstalled() returns true without rechecking the binary
	os.WriteFile(filepath.Join(toolDir, ".installed"), []byte("ok"), 0644)

	return ToolResult{
		OK:      true,
		ID:      id,
		Name:    tool.Name,
		Dir:     toolDir,
		ReqAuth: tool.ReqAuth,
	}
}

// ── MCP config snippet ────────────────────────────────────────────────────────
// Generates the mcpServers block users paste into Cursor / Claude Code / Windsurf.
// Uses the compiled binary directly — no "go run" at agent startup.

func buildMCPConfig(toolIDs []string) string {
	if len(toolIDs) == 0 {
		return ""
	}
	servers := map[string]interface{}{}
	for _, id := range toolIDs {
		dir := clawToolDir(id)
		if dir == "" {
			continue
		}
		servers["claw-"+id] = map[string]interface{}{
			"command": filepath.Join(dir, id),
			"args":    []string{"--mode", "mcp"},
		}
	}
	b, _ := json.MarshalIndent(map[string]interface{}{"mcpServers": servers}, "", "  ")
	return string(b)
}