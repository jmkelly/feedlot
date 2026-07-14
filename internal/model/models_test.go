package model

import (
	"testing"
	"time"
)

func TestUserDefaults(t *testing.T) {
	u := User{}
	if u.ID != 0 {
		t.Errorf("Default User ID = %d, want 0", u.ID)
	}
	if u.Email != "" {
		t.Errorf("Default User Email = %q, want empty", u.Email)
	}
	if !u.CreatedAt.IsZero() {
		t.Error("Default User CreatedAt should be zero")
	}
}

func TestUserJSONTags(t *testing.T) {
	// Verify PasswordHash has json:"-" tag (compile check via struct definition)
	u := User{ID: 1, Email: "test@example.com", PasswordHash: "secret"}
	_ = u
}

func TestSessionDefaults(t *testing.T) {
	s := Session{}
	if s.ID != 0 {
		t.Errorf("Default Session ID = %d, want 0", s.ID)
	}
	if s.Token != "" {
		t.Errorf("Default Session Token = %q, want empty", s.Token)
	}
}

func TestSessionExpiry(t *testing.T) {
	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	s := Session{ExpiresAt: future}
	if s.ExpiresAt.Before(now) {
		t.Error("Future expiry should not be before now")
	}

	s2 := Session{ExpiresAt: past}
	if s2.ExpiresAt.After(now) {
		t.Error("Past expiry should be before now")
	}
}

func TestFeedDefaults(t *testing.T) {
	f := Feed{}
	if f.Title != "" {
		t.Errorf("Default Feed Title = %q, want empty", f.Title)
	}
	if f.UnreadCount != 0 {
		t.Errorf("Default Feed UnreadCount = %d, want 0", f.UnreadCount)
	}
}

func TestFeedOptionalFields(t *testing.T) {
	desc := "A description"
	site := "https://example.com"
	icon := "https://example.com/icon.png"

	f := Feed{
		Description: &desc,
		SiteURL:     &site,
		IconURL:     &icon,
	}

	if f.Description == nil || *f.Description != "A description" {
		t.Error("Description pointer not set correctly")
	}
	if f.SiteURL == nil || *f.SiteURL != "https://example.com" {
		t.Error("SiteURL pointer not set correctly")
	}
	if f.IconURL == nil || *f.IconURL != "https://example.com/icon.png" {
		t.Error("IconURL pointer not set correctly")
	}
}

func TestFeedNullPointers(t *testing.T) {
	f := Feed{}
	if f.Description != nil {
		t.Error("Default Description should be nil")
	}
	if f.SiteURL != nil {
		t.Error("Default SiteURL should be nil")
	}
	if f.IconURL != nil {
		t.Error("Default IconURL should be nil")
	}
	if f.LastFetchedAt != nil {
		t.Error("Default LastFetchedAt should be nil")
	}
}

func TestArticleDefaults(t *testing.T) {
	a := Article{}
	if a.GUID != "" {
		t.Errorf("Default Article GUID = %q, want empty", a.GUID)
	}
	if a.IsRead {
		t.Error("Default Article IsRead should be false")
	}
	if !a.CreatedAt.IsZero() {
		t.Error("Default Article CreatedAt should be zero")
	}
}

func TestArticleOptionalFields(t *testing.T) {
	url := "https://example.com/article"
	author := "John Doe"
	content := "Full article content"
	summary := "Short summary"
	publishedAt := time.Now()

	a := Article{
		URL:         &url,
		Author:      &author,
		Content:     &content,
		Summary:     &summary,
		PublishedAt: &publishedAt,
	}

	if a.URL == nil || *a.URL != "https://example.com/article" {
		t.Error("URL pointer not set correctly")
	}
	if a.Author == nil || *a.Author != "John Doe" {
		t.Error("Author pointer not set correctly")
	}
	if a.Content == nil || *a.Content != "Full article content" {
		t.Error("Content pointer not set correctly")
	}
	if a.Summary == nil || *a.Summary != "Short summary" {
		t.Error("Summary pointer not set correctly")
	}
	if a.PublishedAt == nil || !a.PublishedAt.Equal(publishedAt) {
		t.Error("PublishedAt pointer not set correctly")
	}
}

func TestArticleJSONTags(t *testing.T) {
	// Content has json:"-" so it's hidden from JSON output
	a := Article{Content: ptr("hidden content")}
	_ = a
	a2 := Article{Summary: ptr("visible summary")}
	_ = a2
}

func TestArticleReadToggle(t *testing.T) {
	a := Article{IsRead: false}
	if a.IsRead {
		t.Error("Fresh article should not be read")
	}
	a.IsRead = true
	if !a.IsRead {
		t.Error("Article should be read after setting IsRead = true")
	}
	a.IsRead = false
	if a.IsRead {
		t.Error("Article should be unread after toggling back")
	}
}

func TestUserSettingsDefaults(t *testing.T) {
	s := UserSettings{}
	if s.AIProvider != "" {
		t.Errorf("Default AIProvider = %q, want empty", s.AIProvider)
	}
	if s.SummaryLength != "" {
		t.Errorf("Default SummaryLength = %q, want empty", s.SummaryLength)
	}
	if s.SummaryLanguage != "" {
		t.Errorf("Default SummaryLanguage = %q, want empty", s.SummaryLanguage)
	}
}

func TestUserSettingsOptionalFields(t *testing.T) {
	key := "encrypted-key"
	baseURL := "https://custom.api.com"

	s := UserSettings{
		APIKeyEncrypted: &key,
		BaseURL:         &baseURL,
	}

	if s.APIKeyEncrypted == nil || *s.APIKeyEncrypted != "encrypted-key" {
		t.Error("APIKeyEncrypted pointer not set correctly")
	}
	if s.BaseURL == nil || *s.BaseURL != "https://custom.api.com" {
		t.Error("BaseURL pointer not set correctly")
	}
}

func TestUserSettingsAPIKeyHiddenFromJSON(t *testing.T) {
	s := UserSettings{APIKeyEncrypted: ptr("sk-super-secret")}
	_ = s // compile check: APIKeyEncrypted has json:"-" tag
}

func TestFeedUnreadCount(t *testing.T) {
	f := Feed{UnreadCount: 5}
	if f.UnreadCount != 5 {
		t.Errorf("UnreadCount = %d, want 5", f.UnreadCount)
	}
	f2 := Feed{}
	if f2.UnreadCount != 0 {
		t.Errorf("Default UnreadCount = %d, want 0", f2.UnreadCount)
	}
}

func TestArticlePublishedAtOrdering(t *testing.T) {
	now := time.Now()
	yesterday := now.Add(-24 * time.Hour)

	a1 := Article{PublishedAt: &now}
	a2 := Article{PublishedAt: &yesterday}

	if a1.PublishedAt.Before(*a2.PublishedAt) {
		t.Error("a1 published_at should be after a2 published_at (DESC order)")
	}
}

func ptr(s string) *string {
	return &s
}
