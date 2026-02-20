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

type OpenRouterModel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ContextLength int  `json:"context_length"`
	Pricing     struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

func fetchOpenRouterModels(apiKey string) ([]map[string]interface{}, error) {
	req, _ := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []map[string]interface{}
	for _, m := range result.Data {
		isFree := m.Pricing.Prompt == "0" || m.Pricing.Prompt == "0.0" || strings.HasSuffix(m.ID, ":free")
		models = append(models, map[string]interface{}{
			"id":     m.ID,
			"name":   m.Name,
			"free":   isFree,
		})
	}
	return models, nil
}


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
	apiKey = strings.TrimSpace(apiKey)

	// Validate key exists via auth check — no credits needed
	req, _ := http.NewRequest("GET", "https://openrouter.ai/api/v1/auth/key", nil)
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return false, "Invalid API key"
	}
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Sprintf("API error %d: %s", resp.StatusCode, truncate(string(b), 120))
	}

	// If free model — skip the chat call entirely
	isFree := strings.HasSuffix(model, ":free")
	if isFree {
		return true, "Key valid — free model selected, no credits needed"
	}

	// Paid model — do a real test call
	body := `{"model":"` + model + `","messages":[{"role":"user","content":"hi"}],"max_tokens":5}`
	req2, _ := http.NewRequest("POST", "https://openrouter.ai/api/v1/chat/completions",
		strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+apiKey)
	req2.Header.Set("Content-Type", "application/json")

	resp2, err := httpClient.Do(req2)
	if err != nil {
		return false, "Connection failed: " + err.Error()
	}
	defer resp2.Body.Close()

	if resp2.StatusCode == 200 {
		return true, "Connected — model " + model + " is available"
	}
	b, _ := io.ReadAll(resp2.Body)
	return false, fmt.Sprintf("API error %d: %s", resp2.StatusCode, truncate(string(b), 120))
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
	body := fmt.Sprintf(`{"chat_id":"%s","text":"🟢 Ping from claw-setup!\n\nYour PicoClaw agent is configured and ready."}`, chatID)
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

