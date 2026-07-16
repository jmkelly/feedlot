package handler

import (
	"html/template"
	"log"
	"net/http"
	"strings"

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
		data.Settings["ai_provider"] = "openai"
		data.Settings["model_name"] = "gpt-4o-mini"
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

	tmpl := template.Must(template.New("settings").Parse(settingsPageTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render settings: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
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
		provider = "openai"
	}
	if modelName == "" {
		modelName = "gpt-4o-mini"
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

	tmpl := template.Must(template.New("settings").Parse(settingsPageTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render settings: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) renderSettingsWithError(w http.ResponseWriter, errMsg string) {
	data := settingsData{
		Settings:  make(map[string]string),
		SaveError: errMsg,
	}

	tmpl := template.Must(template.New("settings").Parse(settingsPageTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render settings error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const settingsPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Settings</title>
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="bg-stone-50 text-stone-900 antialiased min-h-screen">
  <nav class="bg-white border-b border-stone-200 shadow-sm">
    <div class="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between h-14">
        <div class="flex items-center gap-3">
          <span class="text-2xl">🐄</span>
          <h1 class="text-lg font-bold text-amber-800">Feedlot</h1>
        </div>
        <a href="/" class="text-stone-500 hover:text-stone-700 text-sm font-medium">&larr; Dashboard</a>
      </div>
    </div>
  </nav>

  <main class="max-w-3xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
    <h2 class="text-xl font-semibold text-stone-800 mb-6">Settings</h2>

    {{if .SaveError}}<div class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4 text-sm">{{.SaveError}}</div>{{end}}
    {{if .SaveSuccess}}<div class="bg-green-50 border border-green-200 text-green-700 px-4 py-3 rounded-lg mb-4 text-sm">{{.SaveSuccess}}</div>{{end}}

    <form action="/settings" method="POST" class="bg-white rounded-xl shadow-sm border border-stone-200 p-6 space-y-5">
      <!-- AI Provider -->
      <div>
        <label for="ai_provider" class="block text-sm font-medium text-stone-700 mb-1">AI Provider</label>
        <select id="ai_provider" name="ai_provider"
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none bg-white">
          <option value="openai" {{if eq .Settings.ai_provider "openai"}}selected{{end}}>OpenAI</option>
          <option value="anthropic" {{if eq .Settings.ai_provider "anthropic"}}selected{{end}}>Anthropic</option>
          <option value="ollama" {{if eq .Settings.ai_provider "ollama"}}selected{{end}}>Ollama (local)</option>
          <option value="custom" {{if eq .Settings.ai_provider "custom"}}selected{{end}}>Custom (OpenAI-compatible)</option>
        </select>
      </div>

      <!-- API Key -->
      <div>
        <label for="api_key" class="block text-sm font-medium text-stone-700 mb-1">API Key</label>
        <input type="password" id="api_key" name="api_key" placeholder="sk-..." value=""
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        <p class="text-xs text-stone-400 mt-1">Leave blank to keep existing key</p>
      </div>

      <!-- Model Name -->
      <div>
        <label for="model_name" class="block text-sm font-medium text-stone-700 mb-1">Model Name</label>
        <input type="text" id="model_name" name="model_name" value="{{.Settings.model_name}}"
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        <p class="text-xs text-stone-400 mt-1">e.g., gpt-4o-mini, claude-3-haiku-20240307, llama3</p>
      </div>

      <!-- Base URL -->
      <div>
        <label for="base_url" class="block text-sm font-medium text-stone-700 mb-1">Base URL (optional)</label>
        <input type="url" id="base_url" name="base_url" value="{{.Settings.base_url}}" placeholder="https://api.openai.com/v1"
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        <p class="text-xs text-stone-400 mt-1">For custom/self-hosted endpoints</p>
      </div>

      <!-- Summary Length -->
      <div>
        <label for="summary_length" class="block text-sm font-medium text-stone-700 mb-1">Summary Length</label>
        <select id="summary_length" name="summary_length"
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none bg-white">
          <option value="short" {{if eq .Settings.summary_length "short"}}selected{{end}}>Short (1-2 sentences)</option>
          <option value="medium" {{if eq .Settings.summary_length "medium"}}selected{{end}}>Medium (short paragraph)</option>
          <option value="long" {{if eq .Settings.summary_length "long"}}selected{{end}}>Long (detailed)</option>
        </select>
      </div>

      <!-- Summary Language -->
      <div>
        <label for="summary_language" class="block text-sm font-medium text-stone-700 mb-1">Summary Language</label>
        <select id="summary_language" name="summary_language"
          class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none bg-white">
          <option value="english" {{if eq .Settings.summary_language "english"}}selected{{end}}>English</option>
          <option value="original" {{if eq .Settings.summary_language "original"}}selected{{end}}>Same as article</option>
          <option value="spanish" {{if eq .Settings.summary_language "spanish"}}selected{{end}}>Spanish</option>
          <option value="french" {{if eq .Settings.summary_language "french"}}selected{{end}}>French</option>
          <option value="german" {{if eq .Settings.summary_language "german"}}selected{{end}}>German</option>
          <option value="japanese" {{if eq .Settings.summary_language "japanese"}}selected{{end}}>Japanese</option>
        </select>
      </div>

      <div class="pt-2">
        <button type="submit"
          class="w-full sm:w-auto px-6 py-2 bg-amber-600 hover:bg-amber-700 text-white font-medium rounded-lg transition-colors">
          Save Settings
        </button>
      </div>
    </form>
  </main>
</body>
</html>`
