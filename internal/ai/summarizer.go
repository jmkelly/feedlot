package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
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

type openaiMessageFull struct {
	Role             string `json:"role"`
	Content          string `json:"content"`
	ReasoningContent string `json:"reasoning_content"`
}

type openaiChoiceFull struct {
	Message openaiMessageFull `json:"message"`
}

type openaiResponseFull struct {
	Choices []openaiChoiceFull `json:"choices"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
}

func (s *Summarizer) TestConnection(req SummaryRequest) error {
	req.ArticleText = "Respond with only the word OK."
	req.Length = "short"
	summary, err := s.Summarize(req)
	if err != nil {
		return err
	}
	if summary == "" {
		return fmt.Errorf("empty response from API")
	}
	return nil
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
	maxTokens := 1000
	switch req.Length {
	case "medium":
		maxTokens = 2000
	case "long":
		maxTokens = 4000
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
	case "openai", "custom", "opencode-go":
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
		if req.Provider == "opencode-go" {
			baseURL = "https://opencode.ai/zen/go/v1"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
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

	url := baseURL + "/chat/completions"

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if req.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
		}

		resp, err := s.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("http request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		// Try standard response first, then full struct for reasoning models
		var apiResp openaiResponse
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			continue
		}

		if len(apiResp.Choices) > 0 {
			summary := strings.TrimSpace(apiResp.Choices[0].Message.Content)
			if summary != "" {
				return summary, nil
			}
		}

		// Reasoning models may put content in reasoning_content
		var apiRespFull openaiResponseFull
		if err := json.Unmarshal(respBody, &apiRespFull); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			continue
		}

		if len(apiRespFull.Choices) > 0 {
			summary := strings.TrimSpace(apiRespFull.Choices[0].Message.Content)
			if summary != "" {
				return summary, nil
			}
			reasoning := strings.TrimSpace(apiRespFull.Choices[0].Message.ReasoningContent)
			if reasoning != "" {
				return reasoning, nil
			}
		}

		lastErr = fmt.Errorf("empty summary")
		continue
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

type openAIModel struct {
	ID string `json:"id"`
}

type openAIModelsResponse struct {
	Data []openAIModel `json:"data"`
}

type ollamaTagModel struct {
	Name string `json:"name"`
}

type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

func (s *Summarizer) ListModels(req SummaryRequest) ([]string, error) {
	switch strings.ToLower(req.Provider) {
	case "openai", "custom", "opencode-go":
		return s.listOpenAIModels(req)
	case "anthropic":
		return s.listAnthropicModels(req)
	case "ollama":
		return s.listOllamaModels(req)
	default:
		return nil, fmt.Errorf("unsupported provider for model listing")
	}
}

func (s *Summarizer) listOpenAIModels(req SummaryRequest) ([]string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		if req.Provider == "opencode-go" {
			baseURL = "https://opencode.ai/zen/go/v1"
		} else {
			baseURL = "https://api.openai.com/v1"
		}
	}

	httpReq, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp openAIModelsResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var models []string
	for _, m := range apiResp.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

func (s *Summarizer) listAnthropicModels(req SummaryRequest) ([]string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	httpReq, err := http.NewRequest("GET", baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("x-api-key", req.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp openAIModelsResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var models []string
	for _, m := range apiResp.Data {
		models = append(models, m.ID)
	}
	sort.Strings(models)
	return models, nil
}

func (s *Summarizer) listOllamaModels(req SummaryRequest) ([]string, error) {
	baseURL := strings.TrimRight(req.BaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	httpReq, err := http.NewRequest("GET", baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp ollamaTagsResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	var models []string
	for _, m := range apiResp.Models {
		models = append(models, m.Name)
	}
	sort.Strings(models)
	return models, nil
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
