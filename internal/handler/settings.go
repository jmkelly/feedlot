package handler

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"

	"github.com/james/feedlot/internal/ai"
	"github.com/james/feedlot/internal/model"
)

type settingsData struct {
	Settings    map[string]string
	SaveError   string
	SaveSuccess string
}

func (h *Handler) SettingsPage(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	data := settingsData{
		Settings: make(map[string]string),
	}

	settings, err := h.DB.GetUserSettings(userID)
	if err != nil {
		// No settings yet — use defaults
		data.Settings["ai_provider"] = "opencode-go"
		data.Settings["model_name"] = "deepseek-v4-flash"
		data.Settings["summary_length"] = "short"
		data.Settings["summary_language"] = "english"
		data.Settings["base_url"] = ""
	} else {
		data.Settings["ai_provider"] = settings.AIProvider
		data.Settings["model_name"] = settings.ModelName
		data.Settings["summary_length"] = settings.SummaryLength
		data.Settings["summary_language"] = settings.SummaryLanguage
		if settings.BaseURL != nil {
			data.Settings["base_url"] = *settings.BaseURL
		} else {
			data.Settings["base_url"] = ""
		}
	}

	if err := settingsTmpl.Execute(w, data); err != nil {
		log.Printf("render settings: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) ListModelsHandler(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	provider := strings.TrimSpace(r.FormValue("ai_provider"))
	modelName := strings.TrimSpace(r.FormValue("model_name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	// Fall back to saved API key if form field is empty
	if apiKey == "" {
		settings, err := h.DB.GetUserSettings(userID)
		if err != nil {
			log.Printf("load settings for model list: %v", err)
		} else if settings.APIKeyEncrypted != nil {
			if h.EncryptionKey != "" {
				decrypted, err := Decrypt(*settings.APIKeyEncrypted, []byte(h.EncryptionKey))
				if err != nil {
					log.Printf("decrypt API key for model list: %v", err)
				} else {
					apiKey = string(decrypted)
				}
			} else {
				apiKey = *settings.APIKeyEncrypted
			}
		}
	}

	// Fallback to global OpenCode Go key
	if apiKey == "" && provider == "opencode-go" && h.OpenCodeGoKey != "" {
		apiKey = h.OpenCodeGoKey
	}

	req := ai.SummaryRequest{
		Provider: provider,
		APIKey:   apiKey,
		BaseURL:  baseURL,
		Context:  r.Context(),
	}

	models, err := h.Summarizer.ListModels(req)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err != nil {
		// Fall back to text input with error indicator
		fmt.Fprintf(w, `<div class="field">
  <label for="model_name">Model Name</label>
  <input type="text" id="model_name" name="model_name" value="%s" class="input" placeholder="%s">
  <p style="font-size:.7rem;color:var(--err);margin-top:.2rem">Could not fetch models: %s</p>
  <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">e.g. gpt-4o-mini, claude-3-haiku, llama3</p>
</div>`, html.EscapeString(modelName), html.EscapeString("Enter model name manually"), html.EscapeString(err.Error()))
		return
	}

	// Build select dropdown
	w.Write([]byte(`<div class="field">
  <label for="model_name">Model Name</label>
  <select id="model_name" name="model_name" class="select">`))
	for _, m := range models {
		selected := ""
		if m == modelName {
			selected = " selected"
		}
		fmt.Fprintf(w, `<option value="%s"%s>%s</option>`, html.EscapeString(m), selected, html.EscapeString(m))
	}
	w.Write([]byte(`</select>
  <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">Fetched from provider — pick a model</p>
</div>`))
}

func (h *Handler) TestSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	userID := GetUserID(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	provider := strings.TrimSpace(r.FormValue("ai_provider"))
	modelName := strings.TrimSpace(r.FormValue("model_name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	// If no API key in form, try to load existing saved key
	if apiKey == "" {
		settings, err := h.DB.GetUserSettings(userID)
		if err != nil {
			log.Printf("load settings for test: %v", err)
		} else if settings.APIKeyEncrypted != nil {
			if h.EncryptionKey != "" {
				decrypted, err := Decrypt(*settings.APIKeyEncrypted, []byte(h.EncryptionKey))
				if err != nil {
					log.Printf("decrypt API key for test: %v", err)
				} else {
					apiKey = string(decrypted)
				}
			} else {
				apiKey = *settings.APIKeyEncrypted
			}
		}
	}

	// Fallback to global OpenCode Go key
	if apiKey == "" && provider == "opencode-go" && h.OpenCodeGoKey != "" {
		apiKey = h.OpenCodeGoKey
	}

	if provider == "" {
		provider = "opencode-go"
	}
	if modelName == "" {
		switch provider {
		case "opencode-go":
			modelName = "deepseek-v4-flash"
		default:
			modelName = "gpt-4o-mini"
		}
	}

	req := ai.SummaryRequest{
		Provider: provider,
		APIKey:   apiKey,
		Model:    modelName,
		BaseURL:  baseURL,
		Context:  r.Context(),
	}

	err := h.Summarizer.TestConnection(req)

	if err != nil {
		w.Write([]byte(`<div class="alert alert--err" style="margin-top:.5rem">Test failed: ` + html.EscapeString(err.Error()) + `</div>`))
	} else {
		w.Write([]byte(`<div class="alert alert--ok" style="margin-top:.5rem">Connection successful! Using <strong>` + html.EscapeString(provider) + `</strong> / <strong>` + html.EscapeString(modelName) + `</strong></div>`))
	}
}

func (h *Handler) SaveSettings(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	if err := r.ParseForm(); err != nil {
		h.renderSettingsWithError(w, "Invalid form data")
		return
	}

	provider := strings.TrimSpace(r.FormValue("ai_provider"))
	modelName := strings.TrimSpace(r.FormValue("model_name"))
	baseURL := strings.TrimSpace(r.FormValue("base_url"))
	summaryLength := strings.TrimSpace(r.FormValue("summary_length"))
	summaryLanguage := strings.TrimSpace(r.FormValue("summary_language"))
	apiKey := strings.TrimSpace(r.FormValue("api_key"))

	if provider == "" {
		provider = "opencode-go"
	}
	if modelName == "" {
		switch provider {
		case "opencode-go":
			modelName = "deepseek-v4-flash"
		default:
			modelName = "gpt-4o-mini"
		}
	}
	if summaryLength == "" {
		summaryLength = "short"
	}
	if summaryLanguage == "" {
		summaryLanguage = "english"
	}

	var apiKeyPtr *string
	if apiKey != "" {
		if h.EncryptionKey != "" {
			encrypted, err := Encrypt([]byte(apiKey), []byte(h.EncryptionKey))
			if err != nil {
				log.Printf("encrypt API key: %v", err)
				h.renderSettingsWithError(w, "Failed to encrypt API key")
				return
			}
			apiKeyPtr = &encrypted
		} else {
			// No encryption key configured — store as-is (not recommended)
			log.Println("WARNING: FEEDLOT_ENCRYPTION_KEY not set, storing API key in plain text")
			apiKeyPtr = &apiKey
		}
	} else {
		// Retain existing key if the form field was left blank
		if existing, err := h.DB.GetUserSettings(userID); err != nil {
			log.Printf("load existing settings to retain API key: %v", err)
		} else {
			apiKeyPtr = existing.APIKeyEncrypted
		}
	}

	var baseURLPtr *string
	if baseURL != "" {
		baseURLPtr = &baseURL
	}

	s := &model.UserSettings{
		UserID:          userID,
		AIProvider:      provider,
		APIKeyEncrypted: apiKeyPtr,
		ModelName:       modelName,
		BaseURL:         baseURLPtr,
		SummaryLength:   summaryLength,
		SummaryLanguage: summaryLanguage,
	}

	err := h.DB.UpsertUserSettings(s)
	if err != nil {
		log.Printf("save settings: %v", err)
		h.renderSettingsWithError(w, "Failed to save settings")
		return
	}

	// Re-render with success message
	data := settingsData{
		Settings: map[string]string{
			"ai_provider":      provider,
			"model_name":       modelName,
			"base_url":         baseURL,
			"summary_length":   summaryLength,
			"summary_language": summaryLanguage,
		},
		SaveSuccess: "Settings saved successfully",
	}

	if err := settingsTmpl.Execute(w, data); err != nil {
		log.Printf("render settings: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) renderSettingsWithError(w http.ResponseWriter, errMsg string) {
	data := settingsData{
		Settings:  make(map[string]string),
		SaveError: errMsg,
	}

	if err := settingsTmpl.Execute(w, data); err != nil {
		log.Printf("render settings error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const settingsPageTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Settings</title>
  <script>(function(){var t=null;try{t=localStorage.getItem('feedlot:theme')}catch(e){}if(t!=='light'&&t!=='dark'){t=window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'}document.documentElement.setAttribute('data-theme',t)})();</script>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&family=Newsreader:ital,opsz,wght@0,6..72,400..700;1,6..72,400..600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
  <link rel="icon" type="image/svg+xml" href="/static/favicon.svg">
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body>
  <nav class="topbar">
    <div class="topbar__brand">
      <span class="topbar__mark">🐄</span>
      <span class="topbar__name">Feedlot</span>
      <span class="topbar__tag">Field Station</span>
    </div>
    <div class="topbar__spacer"></div>
    <div class="topbar__actions">
      <button class="btn btn--ghost" id="theme-toggle" title="Toggle light/dark theme" aria-label="Toggle light/dark theme"></button>
      <a href="/" class="btn btn--ghost">&larr; Dashboard</a>
      <a href="/admin/logs" class="btn btn--ghost">Admin</a>
    </div>
  </nav>

  <main style="max-width:44rem;margin:0 auto;padding:2rem 1.1rem">
    <h2 style="font-family:var(--font-display);font-size:1.4rem;font-weight:700;margin:0 0 1rem">Settings</h2>

    {{if .SaveError}}<div class="alert alert--err">{{.SaveError}}</div>{{end}}
    {{if .SaveSuccess}}<div class="alert alert--ok">{{.SaveSuccess}}</div>{{end}}

    <form action="/settings" method="POST" class="panel" style="padding:1.4rem">
      <div class="field">
        <label for="ai_provider">AI Provider</label>
        <select id="ai_provider" name="ai_provider" class="select"
          hx-post="/settings/models"
          hx-trigger="change"
          hx-target="#model-field-container"
          hx-include="form"
          hx-indicator="#model-spinner">
          <option value="openai" {{if eq .Settings.ai_provider "openai"}}selected{{end}}>OpenAI</option>
          <option value="anthropic" {{if eq .Settings.ai_provider "anthropic"}}selected{{end}}>Anthropic</option>
          <option value="ollama" {{if eq .Settings.ai_provider "ollama"}}selected{{end}}>Ollama (local)</option>
          <option value="opencode-go" {{if eq .Settings.ai_provider "opencode-go"}}selected{{end}}>OpenCode Go</option>
          <option value="custom" {{if eq .Settings.ai_provider "custom"}}selected{{end}}>Custom (OpenAI-compatible)</option>
        </select>
      </div>

      <div class="field">
        <label for="api_key">API Key</label>
        <input type="password" id="api_key" name="api_key" placeholder="sk-..." value="" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">Leave blank to keep existing key</p>
      </div>

      <div id="model-field-container" class="field">
        <label for="model_name">Model Name</label>
        <input type="text" id="model_name" name="model_name" value="{{.Settings.model_name}}" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">e.g. deepseek-v4-flash, deepseek-v4-pro, kimi-k3 <span id="model-spinner" class="htmx-indicator" style="color:var(--accent)">↻</span></p>
      </div>

      <div class="field">
        <label for="base_url">Base URL (optional)</label>
        <input type="url" id="base_url" name="base_url" value="{{.Settings.base_url}}" placeholder="https://api.openai.com/v1" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">For custom / self-hosted endpoints</p>
      </div>

      <div class="field">
        <label for="summary_length">Summary Length</label>
        <select id="summary_length" name="summary_length" class="select">
          <option value="short" {{if eq .Settings.summary_length "short"}}selected{{end}}>Short (1-2 sentences)</option>
          <option value="medium" {{if eq .Settings.summary_length "medium"}}selected{{end}}>Medium (short paragraph)</option>
          <option value="long" {{if eq .Settings.summary_length "long"}}selected{{end}}>Long (detailed)</option>
        </select>
      </div>

      <div class="field">
        <label for="summary_language">Summary Language</label>
        <select id="summary_language" name="summary_language" class="select">
          <option value="english" {{if eq .Settings.summary_language "english"}}selected{{end}}>English</option>
          <option value="original" {{if eq .Settings.summary_language "original"}}selected{{end}}>Same as article</option>
          <option value="spanish" {{if eq .Settings.summary_language "spanish"}}selected{{end}}>Spanish</option>
          <option value="french" {{if eq .Settings.summary_language "french"}}selected{{end}}>French</option>
          <option value="german" {{if eq .Settings.summary_language "german"}}selected{{end}}>German</option>
          <option value="japanese" {{if eq .Settings.summary_language "japanese"}}selected{{end}}>Japanese</option>
        </select>
      </div>

      <div style="margin-top:.5rem;display:flex;gap:.5rem;align-items:center">
        <button type="submit" class="btn btn--primary">Save Settings</button>
        <button type="button" class="btn btn--ghost" 
          hx-post="/settings/test" 
          hx-target="#test-result"
          hx-include="form"
          hx-indicator="#test-spinner">
          Test Connection
        </button>
        <span id="test-spinner" class="htmx-indicator" style="font-size:.75rem;color:var(--ink-faint)">Testing…</span>
      </div>
      <div id="test-result"></div>
    </form>
  </main>
</body>
</html>
`
