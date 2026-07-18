package ai

import (
	"bytes"
	"context"
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
	Provider    string // openai, anthropic, ollama, custom
	APIKey      string
	BaseURL     string
	Model       string
	ArticleText string
	Length      string // short, medium, long
	SummaryLang string // english, original
	Context     context.Context
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

// cleanSummary strips out thinking/reasoning artifacts that some models
// emit alongside the actual response.
//
// Some reasoning models (DeepSeek, etc.) output chain-of-thought in the
// `content` field despite being asked not to. This function detects common
// thinking patterns and extracts only the summary portion.
//
// The returned text is always whitespace-collapsed for consistency.
func cleanSummary(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// 1. Remove <think>...</think> blocks (DeepSeek R1, Qwen, etc.)
	for {
		start := strings.Index(s, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end < 0 {
			s = strings.TrimSpace(s[:start])
			break
		}
		s = strings.TrimSpace(s[:start] + s[start+end+len("</think>"):])
	}
	if s == "" {
		return s
	}

	// 2. Collapse whitespace to make paragraph detection reliable.
	s = collapseWhitespace(s)

	// 3. Split into paragraphs and detect thinking patterns.
	paragraphs := strings.Split(s, "\n")
	if len(paragraphs) <= 1 {
		return s // single paragraph — already collapsed
	}

	// Check if this looks like reasoning output:
	//   - Starts with "Thinking", "Let me", "First,", "I need to", etc.
	//   - Starts with a numbered list item ("1.", "2.", etc.)
	//   - Contains numbered items referencing "Analyze", "Request", "Constraint" etc.
	first := strings.TrimSpace(strings.ToLower(paragraphs[0]))

	hasThinkingPrefix := strings.HasPrefix(first, "thinking") ||
		strings.HasPrefix(first, "let me") ||
		strings.HasPrefix(first, "first,") ||
		strings.HasPrefix(first, "i need to") ||
		strings.HasPrefix(first, "i'll") ||
		strings.HasPrefix(first, "i will") ||
		strings.HasPrefix(first, "i should") ||
		strings.HasPrefix(first, "ok,") ||
		strings.HasPrefix(first, "okay,")
	hasNumberedList := false
	listItemCount := 0
	for _, p := range paragraphs {
		t := strings.TrimSpace(p)
		if len(t) > 2 && t[0] >= '1' && t[0] <= '9' && t[1] == '.' {
			listItemCount++
		}
	}
	if listItemCount >= 2 || (listItemCount >= 1 && hasThinkingPrefix) {
		hasNumberedList = true
	}

	if !hasThinkingPrefix && !hasNumberedList {
		return s // not obviously thinking — return collapsed as-is
	}

	// 4. The text looks like reasoning. Try to extract just the summary.
	// Strategy: look for the last paragraph that doesn't look like a
	// meta-analysis step (numbered, analyzing the task, etc.)
	// OR find a marker line that introduces the summary.

	// First pass: find the last paragraph that looks like actual content
	// (not a numbered step, not meta-commentary about the task).
	var summaryParagraphs []string
	for _, p := range paragraphs {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		// Skip numbered list items (e.g. "1. **Analyze...**")
		if len(t) > 2 && t[0] >= '1' && t[0] <= '9' && t[1] == '.' {
			continue
		}
		// Skip meta-instruction lines — use anchored checks to avoid
		// false positives on words like "review" in normal content.
		lower := strings.ToLower(t)
		if strings.Contains(lower, "analyze the request") ||
			strings.Contains(lower, "formulate response") ||
			strings.Contains(lower, "draft summary") ||
			strings.HasPrefix(lower, "review") ||
			strings.Contains(lower, "constraint") {
			continue
		}
		summaryParagraphs = append(summaryParagraphs, t)
	}

	if len(summaryParagraphs) > 0 {
		result := strings.Join(summaryParagraphs, " ")
		result = strings.TrimSpace(result)
		if result != "" {
			return result
		}
	}

	// Last resort: take the last paragraph that isn't a numbered item
	for i := len(paragraphs) - 1; i >= 0; i-- {
		t := strings.TrimSpace(paragraphs[i])
		if t == "" {
			continue
		}
		if len(t) > 2 && t[0] >= '1' && t[0] <= '9' && t[1] == '.' {
			continue
		}
		return t
	}

	return s
}

// collapseWhitespace normalizes line breaks and trims each line.
func collapseWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	var result []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			result = append(result, t)
		}
	}
	return strings.Join(result, "\n")
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

	systemPrompt := fmt.Sprintf("Summarize this concisely. Output the summary only.%s", langInstruction)
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

		httpReq, err := http.NewRequestWithContext(req.Context, "POST", url, bytes.NewReader(body))
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

		// Parse response — use openaiResponseFull (superset that captures both
		// content and reasoning_content, so it works with any provider).
		var apiResp openaiResponseFull
		if err := json.Unmarshal(respBody, &apiResp); err != nil {
			lastErr = fmt.Errorf("unmarshal response: %w", err)
			continue
		}

		if len(apiResp.Choices) > 0 {
			summary := cleanSummary(strings.TrimSpace(apiResp.Choices[0].Message.Content))
			if summary != "" {
				return summary, nil
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

	httpReq, err := http.NewRequestWithContext(req.Context, "POST", baseURL+"/messages", bytes.NewReader(body))
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
		return "", fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
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

	httpReq, err := http.NewRequestWithContext(req.Context, "GET", baseURL+"/models", nil)
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

	httpReq, err := http.NewRequestWithContext(req.Context, "GET", baseURL+"/models", nil)
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
		return nil, fmt.Errorf("anthropic API error (status %d): %s", resp.StatusCode, string(respBody))
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

	httpReq, err := http.NewRequestWithContext(req.Context, "GET", baseURL+"/api/tags", nil)
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
		return nil, fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
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
	Model    string          `json:"model"`
	Messages []openaiMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]any  `json:"options,omitempty"`
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

	httpReq, err := http.NewRequestWithContext(req.Context, "POST", baseURL+"/api/chat", bytes.NewReader(body))
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
		return "", fmt.Errorf("ollama API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp ollamaResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	return strings.TrimSpace(apiResp.Message.Content), nil
}
