package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultTimeout   = 60 * time.Second
	maxContentLength = 8000
	maxRetries       = 3
)

type Summarizer struct {
	client *http.Client
}

func New() *Summarizer {
	return &Summarizer{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

type SummaryRequest struct {
	Provider      string // openai, anthropic, ollama, custom
	APIKey        string
	BaseURL       string
	Model         string
	ArticleText   string
	Length        string // short, medium, long
	SummaryLang   string // english, original
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

func (s *Summarizer) Summarize(req SummaryRequest) (string, error) {
	if req.ArticleText == "" {
		return "", fmt.Errorf("empty article text")
	}

	// Truncate content
	text := req.ArticleText
	if len(text) > maxContentLength {
		text = text[:maxContentLength]
	}

	// Determine max tokens based on length setting
	maxTokens := 150
	switch req.Length {
	case "medium":
		maxTokens = 300
	case "long":
		maxTokens = 500
	}

	// Build prompt
	langInstruction := ""
	if req.SummaryLang != "" && req.SummaryLang != "original" {
		langInstruction = fmt.Sprintf(" Write the summary in %s.", req.SummaryLang)
	}

	systemPrompt := fmt.Sprintf("Summarize the following article concisely. Return only the summary text, no preamble.%s", langInstruction)
	userPrompt := fmt.Sprintf("Article:\n\n%s\n\nSummary:", text)

	// Route to provider
	switch strings.ToLower(req.Provider) {
	case "openai", "custom":
		return s.callOpenAICompatible(req, systemPrompt, userPrompt, maxTokens)
	case "anthropic":
		return s.callAnthropic(req, systemPrompt, userPrompt, maxTokens)
	case "ollama":
		return s.callOllama(req, systemPrompt, userPrompt, maxTokens)
	default:
		return s.callOpenAICompatible(req, systemPrompt, userPrompt, maxTokens)
	}
}

func (s *Summarizer) callOpenAICompatible(req SummaryRequest, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	apiReq := openaiRequest{
		Model: req.Model,
		Messages: []openaiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.3,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		resp, err := s.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		var apiResp openaiResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			continue
		}

		if len(apiResp.Choices) == 0 {
			lastErr = fmt.Errorf("no choices in response")
			continue
		}

		summary := strings.TrimSpace(apiResp.Choices[0].Message.Content)
		if summary == "" {
			lastErr = fmt.Errorf("empty summary")
			continue
		}

		return summary, nil
	}

	return "", fmt.Errorf("summarization failed after %d retries: %w", maxRetries, lastErr)
}

type anthropicRequest struct {
	Model       string          `json:"model"`
	MaxTokens   int             `json:"max_tokens"`
	System      string          `json:"system,omitempty"`
	Messages    []openaiMessage `json:"messages"`
	Temperature float64         `json:"temperature,omitempty"`
}

type anthropicContent struct {
	Text string `json:"text"`
}

type anthropicMessage struct {
	Role    string             `json:"role"`
	Content []anthropicContent `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
}

func (s *Summarizer) callAnthropic(req SummaryRequest, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	apiReq := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    systemPrompt,
		Messages: []openaiMessage{
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.3,
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if len(apiResp.Content) == 0 {
		return "", fmt.Errorf("no content in Anthropic response")
	}

	return strings.TrimSpace(apiResp.Content[0].Text), nil
}

type ollamaRequest struct {
	Model       string          `json:"model"`
	Messages    []openaiMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Options     map[string]any  `json:"options,omitempty"`
}

type ollamaResponse struct {
	Message openaiMessage `json:"message"`
}

func (s *Summarizer) callOllama(req SummaryRequest, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	apiReq := ollamaRequest{
		Model: req.Model,
		Messages: []openaiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream: false,
		Options: map[string]any{
			"temperature": 0.3,
			"num_predict": maxTokens,
		},
	}

	body, err := json.Marshal(apiReq)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp ollamaResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return strings.TrimSpace(apiResp.Message.Content), nil
}
