package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSummarizeShort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Failed to decode request: %v", err)
		}
		if req.Model != "gpt-4o-mini" {
			t.Errorf("Model = %q, want %q", req.Model, "gpt-4o-mini")
		}
		if len(req.Messages) < 2 {
			t.Fatal("Expected at least 2 messages")
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("First message role = %q, want %q", req.Messages[0].Role, "system")
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "This is a test summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "This is a long article about something interesting. It has multiple sentences of content that should be summarized.",
		Length:      "short",
		SummaryLang: "english",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary != "This is a test summary." {
		t.Errorf("Summary = %q, want %q", summary, "This is a test summary.")
	}
}

func TestSummarizeMedium(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.MaxTokens != 2000 {
			t.Errorf("MaxTokens = %d for medium, want 2000", req.MaxTokens)
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Medium length summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Some article content.",
		Length:      "medium",
		SummaryLang: "english",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestSummarizeLong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.MaxTokens != 4000 {
			t.Errorf("MaxTokens = %d for long, want 4000", req.MaxTokens)
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Detailed long summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Content for long summary.",
		Length:      "long",
		SummaryLang: "english",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
}

func TestSummarizeEmptyArticle(t *testing.T) {
	s := New()
	_, err := s.Summarize(SummaryRequest{Provider: "openai", ArticleText: "",
		Context: context.Background()})
	if err == nil {
		t.Error("Summarize should fail for empty article text")
	}
}

func TestSummarizeTruncatesLongContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		json.NewDecoder(r.Body).Decode(&req)
		userMsg := req.Messages[len(req.Messages)-1].Content
		if len(userMsg) > maxContentLength+200 {
			t.Errorf("Content length = %d, should be truncated to ~%d", len(userMsg), maxContentLength)
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Summary of truncated content."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	longContent := make([]byte, maxContentLength*2)
	for i := range longContent {
		longContent[i] = 'a' + byte(i%26)
	}

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: string(longContent),
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
}

func TestSummarizeCustomProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer custom-key" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Custom provider summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "custom",
		APIKey:      "custom-key",
		BaseURL:     server.URL,
		Model:       "my-model",
		ArticleText: "Custom provider article.",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize with custom provider failed: %v", err)
	}
}

func TestSummarizeWithLanguage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Résumé en français."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "French article content here.",
		SummaryLang: "french",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestSummarizeEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: ""}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Some content.",
		Context:     context.Background()})
	if err == nil {
		t.Error("Summarize should fail when API returns empty summary")
	}
}

func TestDefaultProviderFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Default fallback summary."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "unknown-provider",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Default fallback test.",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary == "" {
		t.Error("Expected non-empty summary")
	}
}

func TestSummarizeRetriesOnHTTPError(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < maxRetries {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"server error"}`))
			return
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Summary after retry."}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Article that triggers retries.",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Summarize should succeed after retries: %v", err)
	}
	if summary != "Summary after retry." {
		t.Errorf("Summary = %q", summary)
	}
	if attempts < 2 {
		t.Errorf("Expected at least 2 attempts, got %d", attempts)
	}
}

func TestSummarizeAllRetriesExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "always down"})
	}))
	defer server.Close()

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		APIKey:      "test-key",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "Article that exhausts retries.",
		Context:     context.Background()})
	if err == nil {
		t.Error("Summarize should fail when all retries are exhausted")
	}
}

func TestSummarizeAnthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "ant-key" {
			t.Errorf("x-api-key = %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("anthropic-version = %q", r.Header.Get("anthropic-version"))
		}
		resp := anthropicResponse{
			Content: []anthropicContent{
				{Text: "Anthropic summary result."},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "anthropic",
		APIKey:      "ant-key",
		BaseURL:     server.URL,
		Model:       "claude-3-haiku-20240307",
		ArticleText: "Anthropic test article.",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Anthropic Summarize failed: %v", err)
	}
	if summary != "Anthropic summary result." {
		t.Errorf("Summary = %q", summary)
	}
}

func TestSummarizeOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Options["temperature"] != 0.3 {
			t.Errorf("Temperature = %v", req.Options["temperature"])
		}
		resp := ollamaResponse{
			Message: openaiMessage{Role: "assistant", Content: "Ollama summary."},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	summary, err := s.Summarize(SummaryRequest{
		Provider:    "ollama",
		BaseURL:     server.URL,
		Model:       "llama3",
		ArticleText: "Ollama article.",
		Context:     context.Background()})
	if err != nil {
		t.Fatalf("Ollama Summarize failed: %v", err)
	}
	if summary != "Ollama summary." {
		t.Errorf("Summary = %q", summary)
	}
}

func TestSummarizeNoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := New()
	_, err := s.Summarize(SummaryRequest{
		Provider:    "openai",
		BaseURL:     server.URL,
		Model:       "gpt-4o-mini",
		ArticleText: "No choices test.",
		Context:     context.Background()})
	if err == nil {
		t.Error("Summarize should fail when API returns no choices")
	}
}

func TestNewSummarizer(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.client == nil {
		t.Fatal("Summarizer client is nil")
	}
	if s.client.Timeout != defaultTimeout {
		t.Errorf("Timeout = %v, want %v", s.client.Timeout, defaultTimeout)
	}
}
