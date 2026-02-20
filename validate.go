package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

func validateLLMKey(provider, apiKey, model string) (bool, string) {
	switch provider {
	case "openrouter":
		return testOpenRouter(apiKey, model)
	case "anthropic":
		return testAnthropic(apiKey, model)
	case "gemini":
		return testGemini(apiKey, model)
	case "groq":
		return testGroq(apiKey, model)
	default:
		return false, "Unknown provider: " + provider
	}
}

func testOpenRouter(apiKey, model string) (bool, string) {
	body := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}],"max_tokens":5}`
	req, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, "Connected — model " + model + " is available"
	}
	b, _ := io.ReadAll(resp.Body)
	return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(b), 120))
}

func testAnthropic(apiKey, model string) (bool, string) {
	body := `{"model":"` + model + `","max_tokens":5,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages",
		strings.NewReader(body))
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, "Connected — model " + model + " is available"
	}
	b, _ := io.ReadAll(resp.Body)
	return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(b), 120))
}

func testGemini(apiKey, model string) (bool, string) {
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		model, apiKey)
	body := `{"contents":[{"parts":[{"text":"hi"}]}]}`
	req, _ := http.NewRequest("POST", url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, "Connected — model " + model + " is available"
	}
	b, _ := io.ReadAll(resp.Body)
	return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(b), 120))
}

func testGroq(apiKey, model string) (bool, string) {
	body := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}],"max_tokens":5}`
	req, _ := http.NewRequest("POST", "https://api.groq.com/openai/v1/chat/completions",
		strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return true, "Connected — model " + model + " is available"
	}
	b, _ := io.ReadAll(resp.Body)
	return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(b), 120))
}

func validateTelegramToken(token string) (bool, string, string) {
	resp, err := httpClient.Get("https://api.telegram.org/bot" + token + "/getMe")
	if err != nil {
		return false, "Connection failed: " + err.Error(), ""
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			Username string `json:"username"`
		} `json:"result"`
		Description string `json:"description"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.OK {
		return true, "Bot @" + result.Result.Username + " connected", result.Result.Username
	}
	return false, result.Description, ""
}

func sendTelegramPing(token, chatID string) (bool, string) {
	body := fmt.Sprintf(
		`{"chat_id":"%s","text":"🟢 *Ping from claw-setup\\!*\n\nYour PicoClaw agent is configured and ready\\.","parse_mode":"MarkdownV2"}`,
		chatID)
	req, _ := http.NewRequest("POST",
		"https://api.telegram.org/bot"+token+"/sendMessage",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	if result.OK {
		return true, "Ping sent — check your Telegram"
	}
	return false, result.Description + " (did you send /start to your bot?)"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

