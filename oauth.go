package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	toolsRegistryURL  = "https://raw.githubusercontent.com/arpit0515/claw-tools.dev/refs/heads/main/tools.json"
	toolsRepoCloneURL = "https://github.com/arpit0515/claw-tools.dev.git"
	oauthConfigURL    = "https://claw-tools.dev/api/oauth-config"
	oauthCallbackPort = "3455"
)

var clawAPISecret = ""

// ── Types ─────────────────────────────────────────────────────────────────────

type ClawTool struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Dir          string   `json:"dir"`
	SkillMD      string   `json:"skill_md"`
	Status       string   `json:"status"`
	RequiresAuth []string `json:"requires_auth"`
	MCPTools     []string `json:"mcp_tools"`
	HTTPPort     int      `json:"http_port"`
	ServiceStart string   `json:"service_start"`
	HealthURL    string   `json:"health_url"`
	Category     string   `json:"category"`
}

type ConnectedAccount struct {
	Email     string    `json:"email"`
	Provider  string    `json:"provider"`
	AddedAt   time.Time `json:"added_at"`
	TokenFile string    `json:"token_file"`
}

// ── Paths ─────────────────────────────────────────────────────────────────────

func tokensDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "tokens")
}

func tokenFileForEmail(email string) string {
	safe := strings.ReplaceAll(email, "/", "_")
	return filepath.Join(tokensDir(), safe+".enc")
}

// clawToolsRepoDir is where the claw-tools.dev git repo is cloned.
// Kept separate from the tools themselves so the .git folder stays out of the way.
func clawToolsRepoDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".picoclaw", "workspace", ".claw-tools-repo")
}

// toolsRepoDir returns the tools/ subfolder inside the cloned repo.
// Binaries are built here — e.g. workspace/.claw-tools-repo/tools/gmail/gmail-mcp
// t.Dir in tools.json should be just "gmail", "gcal" etc — no "tools/" prefix needed.
func toolsRepoDir() string {
	return filepath.Join(clawToolsRepoDir(), "tools")
}

// toolBinaryPath returns the path to the compiled binary for a tool.
// e.g. ~/claw-tools.dev/tools/gmail/gmail-mcp
func toolBinaryPath(t ClawTool) string {
	return filepath.Join(toolsRepoDir(), t.Dir, t.ID+"-mcp")
}

// ── Encryption ────────────────────────────────────────────────────────────────

func machineKey() []byte {
	hostname, _ := os.Hostname()
	hash := sha256.Sum256([]byte("claw-token-key:" + hostname))
	return hash[:]
}

func encryptToken(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(machineKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func decryptToken(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(machineKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], nil)
}

// ── Token storage ─────────────────────────────────────────────────────────────

func saveOAuthToken(email string, tok *oauth2.Token) error {
	os.MkdirAll(tokensDir(), 0700)
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	enc, err := encryptToken(data)
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFileForEmail(email), enc, 0600)
}

func loadOAuthToken(email string) (*oauth2.Token, error) {
	enc, err := os.ReadFile(tokenFileForEmail(email))
	if err != nil {
		return nil, err
	}
	data, err := decryptToken(enc)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	return &tok, json.Unmarshal(data, &tok)
}

func deleteOAuthToken(email string) error {
	return os.Remove(tokenFileForEmail(email))
}

// ── Account listing ───────────────────────────────────────────────────────────

func listConnectedAccounts() ([]ConnectedAccount, error) {
	entries, err := os.ReadDir(tokensDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []ConnectedAccount{}, nil
		}
		return nil, err
	}
	accounts := []ConnectedAccount{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".enc") {
			continue
		}
		email := strings.TrimSuffix(e.Name(), ".enc")
		info, _ := e.Info()
		addedAt := time.Time{}
		if info != nil {
			addedAt = info.ModTime()
		}
		accounts = append(accounts, ConnectedAccount{
			Email:     email,
			Provider:  "google",
			AddedAt:   addedAt,
			TokenFile: filepath.Join(tokensDir(), e.Name()),
		})
	}
	return accounts, nil
}

// ── Tools registry ────────────────────────────────────────────────────────────

func fetchToolsRegistry() ([]ClawTool, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(toolsRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("could not fetch tools registry: %w", err)
	}
	defer resp.Body.Close()
	var tools []ClawTool
	if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
		return nil, fmt.Errorf("invalid tools registry: %w", err)
	}
	return tools, nil
}

// ── OAuth config from Vercel ──────────────────────────────────────────────────

type oauthConfigResp struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

func fetchOAuthConfig() (*oauthConfigResp, error) {
	req, err := http.NewRequest("GET", oauthConfigURL, nil)
	if err != nil {
		return nil, err
	}
	if clawAPISecret != "" {
		req.Header.Set("x-claw-secret", clawAPISecret)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("could not reach oauth config server — check internet connection: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 401:
		return nil, fmt.Errorf("unauthorized — re-run install.sh to get the latest binary")
	case 429:
		return nil, fmt.Errorf("rate limit exceeded — try again later")
	case 200:
	default:
		return nil, fmt.Errorf("oauth config server error: %d", resp.StatusCode)
	}

	var cfg oauthConfigResp
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid oauth config response: %w", err)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("incomplete oauth config from server")
	}
	return &cfg, nil
}

func newGoogleOAuthConfig(scopes ...string) (*oauth2.Config, error) {
	cfg, err := fetchOAuthConfig()
	if err != nil {
		return nil, err
	}
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  "http://localhost:" + oauthCallbackPort + "/oauth/callback",
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}, nil
}

// ── OAuth helpers ─────────────────────────────────────────────────────────────

func fetchGoogleUserEmail(tok *oauth2.Token) (string, error) {
	client := oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(tok))
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}
	if info.Email == "" {
		return "", fmt.Errorf("empty email in userinfo response")
	}
	return info.Email, nil
}

func randomOAuthState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}

// ── Active OAuth flow state ───────────────────────────────────────────────────

type oauthFlowState struct {
	state  string
	cfg    *oauth2.Config
	codeCh chan string
	errCh  chan error
	srv    *http.Server
}

var activeFlow *oauthFlowState

// ── HTTP Handlers ─────────────────────────────────────────────────────────────

func handleToolsRefresh(w http.ResponseWriter, r *http.Request) {
	go autoConfigureFromRegistry()
	okResponse(w, "Tools refreshed — SKILL.md and MCP config updated", nil)
}

func handleGetTools(w http.ResponseWriter, r *http.Request) {
	tools, err := fetchToolsRegistry()
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	accounts, _ := listConnectedAccounts()

	type toolWithStatus struct {
		ClawTool
		Connected bool               `json:"connected"`
		Installed bool               `json:"installed"`
		Accounts  []ConnectedAccount `json:"accounts,omitempty"`
	}

	result := make([]toolWithStatus, 0, len(tools))
	for _, t := range tools {
		ts := toolWithStatus{ClawTool: t}
		if _, err := os.Stat(toolBinaryPath(t)); err == nil {
			ts.Installed = true
		}
		if len(t.RequiresAuth) > 0 && t.RequiresAuth[0] == "google_oauth2" {
			ts.Accounts = accounts
			ts.Connected = len(accounts) > 0
		}
		result = append(result, ts)
	}
	jsonResponse(w, map[string]interface{}{"ok": true, "tools": result})
}

func handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	scopes := []string{
		"https://www.googleapis.com/auth/gmail.readonly",
		"https://www.googleapis.com/auth/calendar.readonly",
		"https://www.googleapis.com/auth/userinfo.email",
	}

	cfg, err := newGoogleOAuthConfig(scopes...)
	if err != nil {
		errorResponse(w, err.Error())
		return
	}

	state := randomOAuthState()
	authURL := cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":" + oauthCallbackPort, Handler: mux}

	activeFlow = &oauthFlowState{state: state, cfg: cfg, codeCh: codeCh, errCh: errCh, srv: srv}
	mux.HandleFunc("/oauth/callback", handleOAuthCallback)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	openBrowser(authURL)
	jsonResponse(w, map[string]interface{}{
		"ok":       true,
		"auth_url": authURL,
		"message":  "Browser opened — authorize in the browser to continue",
	})
}

func handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if activeFlow == nil {
		http.Error(w, "No active OAuth flow", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("state") != activeFlow.state {
		http.Error(w, "Invalid state", http.StatusBadRequest)
		activeFlow.errCh <- fmt.Errorf("state mismatch")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "No code received", http.StatusBadRequest)
		activeFlow.errCh <- fmt.Errorf("no code: %s", r.URL.Query().Get("error"))
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, oauthSuccessPage())
	activeFlow.codeCh <- code
}

func handleOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if activeFlow == nil {
		jsonResponse(w, map[string]interface{}{"ok": false, "status": "no_flow"})
		return
	}
	select {
	case code := <-activeFlow.codeCh:
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		tok, err := activeFlow.cfg.Exchange(ctx, code)
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutCancel()
		activeFlow.srv.Shutdown(shutCtx)
		activeFlow = nil
		if err != nil {
			errorResponse(w, "Token exchange failed: "+err.Error())
			return
		}
		email, err := fetchGoogleUserEmail(tok)
		if err != nil {
			errorResponse(w, "Could not fetch account email: "+err.Error())
			return
		}
		if err := saveOAuthToken(email, tok); err != nil {
			errorResponse(w, "Could not save token: "+err.Error())
			return
		}
		// Write google_credentials.json so ClawTools can refresh tokens independently
		if err := writeGoogleCredentials(); err != nil {
			fmt.Fprintf(os.Stderr, "oauth: warning — could not write credentials file: %v\n", err)
		}
		// Clone + build + configure in background so OAuth response is instant
		go autoConfigureFromRegistry()
		jsonResponse(w, map[string]interface{}{
			"ok":      true,
			"status":  "connected",
			"email":   email,
			"message": "Account connected: " + email + " — installing tools in background...",
		})
	case err := <-activeFlow.errCh:
		activeFlow = nil
		errorResponse(w, "OAuth failed: "+err.Error())
	default:
		jsonResponse(w, map[string]interface{}{"ok": true, "status": "pending"})
	}
}

func handleOAuthAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := listConnectedAccounts()
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	jsonResponse(w, map[string]interface{}{"ok": true, "accounts": accounts})
}

func handleOAuthRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	r.ParseMultipartForm(1 << 20)
	email := r.FormValue("email")
	if email == "" {
		errorResponse(w, "email is required")
		return
	}
	tok, err := loadOAuthToken(email)
	if err == nil {
		token := tok.AccessToken
		if tok.RefreshToken != "" {
			token = tok.RefreshToken
		}
		http.Get("https://oauth2.googleapis.com/revoke?token=" + token)
	}
	if err := deleteOAuthToken(email); err != nil {
		errorResponse(w, "Could not delete token: "+err.Error())
		return
	}
	okResponse(w, "Account "+email+" removed", nil)
}

func handleCredsStatus(w http.ResponseWriter, r *http.Request) {
	_, err := fetchOAuthConfig()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	jsonResponse(w, map[string]interface{}{
		"ok":     err == nil,
		"exists": err == nil,
		"error":  errMsg,
	})
}

// ── Tool installation ─────────────────────────────────────────────────────────


// copyFile copies src to dst, making dst executable.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src %s: %w", src, err)
	}
	defer in.Close()
	os.Remove(dst)
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dst %s: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	if err := os.Chmod(dst, 0755); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	// Verify the file actually landed
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		return fmt.Errorf("post-copy verify failed for %s", dst)
	}
	return nil
}

// ensureToolsRepo clones claw-tools.dev if not present, or pulls latest if it is.
func ensureToolsRepo() error {
	repoDir := clawToolsRepoDir()

	if _, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil {
		fmt.Fprintf(os.Stderr, "tools-repo: pulling latest...\n")
		cmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "tools-repo: pull warning: %s\n", strings.TrimSpace(string(out)))
		}
		return nil
	}

	os.MkdirAll(filepath.Dir(repoDir), 0755)
	fmt.Fprintf(os.Stderr, "tools-repo: cloning %s to %s...\n", toolsRepoCloneURL, repoDir)
	cmd := exec.Command("git", "clone", "--depth=1", toolsRepoCloneURL, repoDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(out)))
	}
	fmt.Fprintf(os.Stderr, "tools-repo: cloned to %s\n", repoDir)
	return nil
}

// buildTool compiles the tool binary. Skips if binary is already newer than source.
func buildTool(t ClawTool, goBin string) error {
	toolDir := filepath.Join(toolsRepoDir(), t.Dir)
	binaryPath := toolBinaryPath(t)

	// Skip rebuild if binary is newer than all .go source files
	if binInfo, err := os.Stat(binaryPath); err == nil {
		entries, _ := filepath.Glob(filepath.Join(toolDir, "*.go"))
		needRebuild := false
		for _, src := range entries {
			if srcInfo, err := os.Stat(src); err == nil && srcInfo.ModTime().After(binInfo.ModTime()) {
				needRebuild = true
				break
			}
		}
		if !needRebuild {
			fmt.Fprintf(os.Stderr, "build: %s-%s already up to date\n", t.ID, "mcp")
			return nil
		}
	}

	fmt.Fprintf(os.Stderr, "build: compiling %s-mcp...\n", t.ID)
	cmd := exec.Command(goBin, "build", "-o", binaryPath, ".")
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build failed for %s: %s", t.ID, strings.TrimSpace(string(out)))
	}
	os.Chmod(binaryPath, 0755)
	fmt.Fprintf(os.Stderr, "build: %s-mcp ready at %s\n", t.ID, binaryPath)
	return nil
}

// ── Auto-configure ────────────────────────────────────────────────────────────


// writeGoogleCredentials writes google_credentials.json so ClawTools can
// initialize their own OAuth client to refresh tokens independently.
// The file format matches what Google's OAuth2 library expects.
func writeGoogleCredentials() error {
	cfg, err := fetchOAuthConfig()
	if err != nil {
		return fmt.Errorf("could not fetch oauth config: %w", err)
	}
	home, _ := os.UserHomeDir()
	credsPath := filepath.Join(home, ".picoclaw", "config", "google_credentials.json")
	os.MkdirAll(filepath.Dir(credsPath), 0755)

	creds := map[string]interface{}{
		"installed": map[string]interface{}{
			"client_id":                   cfg.ClientID,
			"client_secret":               cfg.ClientSecret,
			"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
			"token_uri":                   "https://oauth2.googleapis.com/token",
			"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
			"redirect_uris":               []string{"http://localhost"},
		},
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(credsPath, data, 0600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "oauth: wrote google_credentials.json to %s\n", credsPath)
	return nil
}

// autoConfigureFromRegistry is the single source of truth for wiring ClawTools
// into PicoClaw. Flow:
//  1. Fetch tools.json from GitHub
//  2. Clone / pull claw-tools.dev repo
//  3. Build each available tool binary (skip if already current)
//  4. Write config.json pointing to compiled binaries (not go run)
//  5. Write SKILL.md
//  6. Install / restart systemd services
//  7. Restart PicoClaw
func autoConfigureFromRegistry() {
	fmt.Fprintf(os.Stderr, "auto-configure: starting...\n")

	// Always ensure credentials file exists — needed by ClawTools to refresh tokens
	if err := writeGoogleCredentials(); err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: warning — could not write credentials: %v\n", err)
	}

	tools, err := fetchToolsRegistry()
	if err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: could not fetch tools registry: %v\n", err)
		return
	}

	accounts, _ := listConnectedAccounts()
	home, _ := os.UserHomeDir()

	// Step 1 — Ensure repo is present and up to date
	if err := ensureToolsRepo(); err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: repo setup failed: %v\n", err)
		return
	}

	// Find Go binary
	goBin := "/usr/local/go/bin/go"
	if _, err := os.Stat(goBin); err != nil {
		if path, err := exec.LookPath("go"); err == nil {
			goBin = path
		} else {
			fmt.Fprintf(os.Stderr, "auto-configure: go binary not found\n")
			return
		}
	}

	cfg := readConfig()
	if cfg.Tools == nil {
		cfg.Tools = make(map[string]interface{})
	}

	// Ensure workspace is explicitly set in agents.defaults — PicoClaw needs this
	// to resolve the safety guard path check correctly.
	if cfg.Agents == nil {
		cfg.Agents = make(map[string]interface{})
	}
	defaults, _ := cfg.Agents["defaults"].(map[string]interface{})
	if defaults == nil {
		defaults = make(map[string]interface{})
	}
	defaults["workspace"] = filepath.Join(home, ".picoclaw", "workspace")
	cfg.Agents["defaults"] = defaults

	mcpServers := map[string]interface{}{}
	configuredTools := []ClawTool{}
	newlyInstalled := []ClawTool{} // only ping for tools installed this run

	for _, t := range tools {
		if t.Status != "available" {
			continue
		}
		if len(t.RequiresAuth) > 0 && len(accounts) == 0 {
			continue
		}

		binaryPath := toolBinaryPath(t)
		alreadyBuilt := func() bool {
			_, err := os.Stat(binaryPath)
			return err == nil
		}()

		// Build binary — skip if already current
		if err := buildTool(t, goBin); err != nil {
			fmt.Fprintf(os.Stderr, "auto-configure: build failed for %s: %v — skipping\n", t.ID, err)
			continue
		}

		toolDir := filepath.Join(toolsRepoDir(), t.Dir)

		// cwd must be inside workspace so PicoClaw's safety guard allows execution
		workspaceBinDir := filepath.Join(home, ".picoclaw", "workspace", "bin")
		mcpServers["claw-"+t.ID] = map[string]interface{}{
			"command": filepath.Join(workspaceBinDir, t.ID+"-mcp"),
			"args":    []string{"--mode", "mcp"},
			"cwd":     workspaceBinDir,
		}

		installToolService(t.ID, t.HTTPPort, binaryPath, toolDir, home)
		configuredTools = append(configuredTools, t)

		// Only ping on first install, not every re-run
		if !alreadyBuilt {
			newlyInstalled = append(newlyInstalled, t)
		}
	}

	// Write at root level as mcpServers — this is the key PicoClaw actually reads
	cfg.MCPServers = mcpServers

	if err := writeConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: could not write config: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "auto-configure: config.json updated — %d MCP servers\n", len(mcpServers))

	autoWriteSkillFile(configuredTools, home)

	// Generate all missing workspace markdown files
	agentName, ownerName := resolveAgentIdentity()
	writeWorkspaceFiles(configuredTools, agentName, ownerName)

	runCommand("systemctl", "--user", "restart", "picoclaw")
	fmt.Fprintf(os.Stderr, "auto-configure: picoclaw restarted\n")

	// Only ping for tools that were just built for the first time
	for _, t := range newlyInstalled {
		sendToolConnectedPing(t)
	}
}


// ── Skill file ────────────────────────────────────────────────────────────────

func autoWriteSkillFile(tools []ClawTool, home string) {
	skillDir := filepath.Join(home, ".picoclaw", "workspace", "skills", "clawtools")
	os.MkdirAll(skillDir, 0755)

	// Copy binaries into workspace/bin — PicoClaw's exec sandbox only allows
	// execution of files inside the workspace directory. Binaries are built inside
	// .claw-tools-repo which is inside workspace, but the sandbox uses path prefix
	// matching so hidden dirs (.claw-tools-repo) may be blocked. workspace/bin is
	// the guaranteed-safe location. Copy on every run so it stays in sync.
	workspaceBin := filepath.Join(home, ".picoclaw", "workspace", "bin")
	os.MkdirAll(workspaceBin, 0755)
	for _, t := range tools {
		src := toolBinaryPath(t)
		dst := filepath.Join(workspaceBin, t.ID+"-mcp")
		if err := copyFile(src, dst); err != nil {
			fmt.Fprintf(os.Stderr, "skill: could not copy %s binary to workspace/bin: %v (src=%s)\n", t.ID, err, src)
			continue
		}
		fmt.Fprintf(os.Stderr, "skill: copied %s-mcp → workspace/bin/\n", t.ID)
	}

	// Use absolute paths — PicoClaw's WorkingDirectory is $HOME, not the workspace,
	// so relative ./bin/ paths resolve to ~/bin/ which doesn't exist.
	gmailBin := filepath.Join(home, ".picoclaw", "workspace", "bin", "gmail-mcp")
	gcalBin := filepath.Join(home, ".picoclaw", "workspace", "bin", "gcal-mcp")

	skill := fmt.Sprintf(`# ClawTools — Gmail & Google Calendar

You have direct access to Gmail and Google Calendar via local binaries.
Use the exec tool to call them. Always use -jc flags for compact JSON output.

## How to call tools

Use the exec tool with these exact commands:

### Gmail

List recent emails:
` + "```" + `
exec: %s -mode mcp <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gmail_list","arguments":{"max_results":5}}}
EOF
` + "```" + `

Search emails:
` + "```" + `
exec: %s -mode mcp <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gmail_search","arguments":{"query":"is:unread","max_results":5}}}
EOF
` + "```" + `

### Google Calendar

Today's events:
` + "```" + `
exec: %s -mode mcp <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gcal_today","arguments":{}}}
EOF
` + "```" + `

Upcoming events (next 7 days):
` + "```" + `
exec: %s -mode mcp <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gcal_upcoming","arguments":{"days":7}}}
EOF
` + "```" + `

## Rules
- ALWAYS use exec with the commands above when asked about emails or calendar
- NEVER say you don't have access to emails or calendar
- NEVER install or suggest porteden or any other email tool
- Parse the JSON result and present it in a readable format
- If result is empty array, say "No emails found" or "No events today"
`, gmailBin, gmailBin, gcalBin, gcalBin)

	path := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(skill), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: could not write SKILL.md: %v\n", err)
		return
	}
	fmt.Fprintf(os.Stderr, "auto-configure: SKILL.md written to %s\n", path)
}

// ── Systemd service ───────────────────────────────────────────────────────────

// installToolService writes a systemd user service that runs the compiled binary
// in HTTP mode. Uses the binary directly — no go run.
func installToolService(id string, port int, binaryPath string, toolDir string, home string) {
	serviceDir := filepath.Join(home, ".config", "systemd", "user")
	os.MkdirAll(serviceDir, 0755)

	content := fmt.Sprintf(`[Unit]
Description=ClawTools %s
After=network.target picoclaw.service

[Service]
Type=simple
ExecStart=%s --mode http --port %d
WorkingDirectory=%s
Restart=on-failure
RestartSec=5
Environment=HOME=%s

[Install]
WantedBy=default.target
`, id, binaryPath, port, toolDir, home)

	serviceName := "claw-" + id
	path := filepath.Join(serviceDir, serviceName+".service")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "auto-configure: could not write service %s: %v\n", serviceName, err)
		return
	}

	runCommand("systemctl", "--user", "daemon-reload")
	runCommand("systemctl", "--user", "enable", serviceName)
	runCommand("systemctl", "--user", "restart", serviceName)
	fmt.Fprintf(os.Stderr, "auto-configure: service %s installed\n", serviceName)
}

// ── Telegram ping ─────────────────────────────────────────────────────────────

func sendToolConnectedPing(t ClawTool) {
	cfg := readConfig()
	tg, ok := cfg.Channels["telegram"]
	if !ok {
		return
	}
	token, _ := tg["token"].(string)
	users, _ := tg["allowFrom"].([]interface{})
	if token == "" || len(users) == 0 {
		return
	}
	chatID, _ := users[0].(string)
	if chatID == "" {
		return
	}
	msg := fmt.Sprintf("🦞 *%s* is now connected!\n\n_%s_", t.Name, t.Description)
	body := fmt.Sprintf(`{"chat_id":"%s","text":"%s","parse_mode":"Markdown"}`,
		chatID, strings.ReplaceAll(msg, `"`, `\"`))
	req, err := http.NewRequest("POST",
		"https://api.telegram.org/bot"+token+"/sendMessage",
		strings.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	httpClient.Do(req)
}

// ── Success page ──────────────────────────────────────────────────────────────

func oauthSuccessPage() string {
	return `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>ClawTools — Connected</title>
  <style>
    body{font-family:-apple-system,sans-serif;background:#0f1117;color:#e8eaf6;
         display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
    .card{background:#1a1d27;border:1px solid #2e3350;border-radius:12px;
          padding:40px 48px;text-align:center;max-width:400px}
    .icon{font-size:48px;margin-bottom:16px}
    h1{font-size:22px;font-weight:600;margin:0 0 8px;color:#00d4aa}
    p{color:#8b90b0;font-size:14px;margin:0 0 20px}
    .timer-ring{position:relative;width:48px;height:48px;margin:0 auto}
    .timer-ring svg{transform:rotate(-90deg)}
    .timer-ring circle{fill:none;stroke:#2e3350;stroke-width:4}
    .timer-ring circle.progress{stroke:#00d4aa;stroke-dasharray:126;stroke-dashoffset:0;
      transition:stroke-dashoffset 1s linear;stroke-linecap:round}
    .timer-num{position:absolute;inset:0;display:flex;align-items:center;justify-content:center;
      font-size:16px;font-weight:700;color:#00d4aa}
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">🦞</div>
    <h1>Account connected!</h1>
    <p>Closing this tab automatically...</p>
    <div class="timer-ring">
      <svg width="48" height="48" viewBox="0 0 48 48">
        <circle cx="24" cy="24" r="20"/>
        <circle class="progress" id="ring" cx="24" cy="24" r="20"/>
      </svg>
      <div class="timer-num" id="num">10</div>
    </div>
  </div>
  <script>
    const circ = 2 * Math.PI * 20;
    const ring = document.getElementById('ring');
    const num  = document.getElementById('num');
    ring.style.strokeDasharray = circ;
    ring.style.strokeDashoffset = 0;
    let left = 10;
    const tick = () => {
      left--;
      num.textContent = left;
      ring.style.strokeDashoffset = circ * (1 - left / 10);
      if (left <= 0) {
        // Try window.close() first (works if opened via window.open)
        // Fall back to a blank page — browser won't block that
        try { window.close(); } catch(e) {}
        setTimeout(() => { window.location.replace('about:blank'); }, 200);
      }
    };
    setInterval(tick, 1000);
  </script>
</body>
</html>`
}
