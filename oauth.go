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
	oauthConfigURL    = "https://claw-tools.dev/api/oauth-config"
	oauthCallbackPort = "3455"
)

// clawAPISecret is injected at build time:
//
//	go build -ldflags "-X main.clawAPISecret=your-secret-here"
//
// Never hardcode the real value here.
var clawAPISecret = ""

// ── Types ─────────────────────────────────────────────────────────────────────

type ClawTool struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Dir          string   `json:"dir"`
	Status       string   `json:"status"`
	RequiresAuth []string `json:"requires_auth"`
	MCPTools     []string `json:"mcp_tools"`
	HTTPPort     int      `json:"http_port"`
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

// fetchOAuthConfig fetches client_id + client_secret from claw-tools.dev/api/oauth-config.
// The secret is never stored on disk — used in memory during the auth flow only.
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
		// ok
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

// GET /api/tools
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
		Accounts  []ConnectedAccount `json:"accounts,omitempty"`
	}

	result := make([]toolWithStatus, 0, len(tools))
	for _, t := range tools {
		ts := toolWithStatus{ClawTool: t}
		if len(t.RequiresAuth) > 0 && t.RequiresAuth[0] == "google_oauth2" {
			ts.Accounts = accounts
			ts.Connected = len(accounts) > 0
		}
		result = append(result, ts)
	}
	jsonResponse(w, map[string]interface{}{"ok": true, "tools": result})
}

// POST /api/oauth/start
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

// GET /oauth/callback — Google redirects here
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

// GET /api/oauth/status — poll after /api/oauth/start
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
		jsonResponse(w, map[string]interface{}{
			"ok": true, "status": "connected",
			"email": email, "message": "Account connected: " + email,
		})
	case err := <-activeFlow.errCh:
		activeFlow = nil
		errorResponse(w, "OAuth failed: "+err.Error())
	default:
		jsonResponse(w, map[string]interface{}{"ok": true, "status": "pending"})
	}
}

// GET /api/oauth/accounts
func handleOAuthAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := listConnectedAccounts()
	if err != nil {
		errorResponse(w, err.Error())
		return
	}
	jsonResponse(w, map[string]interface{}{"ok": true, "accounts": accounts})
}

// POST /api/oauth/revoke
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

// GET /api/oauth/creds-status — verify Vercel endpoint is reachable
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
    p{color:#8b90b0;font-size:14px;margin:0}
  </style>
</head>
<body>
  <div class="card">
    <div class="icon">🦞</div>
    <h1>Account connected!</h1>
    <p>You can close this tab and return to the setup wizard.</p>
  </div>
</body>
</html>`
}
