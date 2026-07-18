package handler

import (
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/james/feedlot/internal/ai"
	"github.com/james/feedlot/internal/feeds"
	"github.com/james/feedlot/internal/model"
)

var summarizer = ai.New()

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	// Check for feed_id query parameter
	feedIDStr := r.URL.Query().Get("feed_id")
	var selectedFeedID *int64

	if feedIDStr != "" {
		fid, err := strconv.ParseInt(feedIDStr, 10, 64)
		if err == nil {
			selectedFeedID = &fid
		}
	}

	articles, err := h.DB.GetArticlesByUserID(userID, selectedFeedID)
	if err != nil {
		log.Printf("get articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If this is an HTMX request (e.g. clicking a feed in the sidebar),
	// return only the article list fragment — not the full dashboard page.
	// This avoids the hx-select nesting bug where the entire <section id="article-list">
	// from the full page gets nested inside the existing one.
	if r.Header.Get("HX-Request") == "true" {
		data := map[string]any{
			"Articles": articles,
		}
		if err := articleListTmpl.Execute(w, data); err != nil {
			log.Printf("render article list: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	feeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get user feeds: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Load settings for AI status indicator
	settings, err := h.DB.GetUserSettings(userID)
	if err != nil {
		log.Printf("get user settings: %v", err)
	}
	settingsProvider := "not configured"
	settingsModel := ""
	settingsConfigured := false
	if settings != nil {
		settingsConfigured = settings.APIKeyEncrypted != nil && *settings.APIKeyEncrypted != ""
		settingsProvider = settings.AIProvider
		settingsModel = settings.ModelName
	}

	data := map[string]any{
		"Feeds":              feeds,
		"Articles":           articles,
		"FeedID":             feedIDStr,
		"SettingsProvider":   settingsProvider,
		"SettingsModel":      settingsModel,
		"SettingsConfigured": settingsConfigured,
	}

	if err := dashboardTmpl.Execute(w, data); err != nil {
		log.Printf("render dashboard: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) ListFeeds(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	feedsList, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("list feeds: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Feeds": feedsList,
	}

	if err := feedListTmpl.Execute(w, data); err != nil {
		log.Printf("render feed list: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// ─── OPML Import ─────────────────────────────────────────────────────────

// opmlDocument represents the root of an OPML 1.0/2.0 file.
type opmlDocument struct {
	XMLName xml.Name   `xml:"opml"`
	Version string     `xml:"version,attr"`
	Head    opmlHead   `xml:"head"`
	Body    opmlBody   `xml:"body"`
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

type opmlFeedEntry struct {
	Title   string
	FeedURL string
	SiteURL string
}

// parseOPML reads an OPML file and extracts all feed entries from nested outlines.
func parseOPML(r io.Reader) ([]opmlFeedEntry, error) {
	var doc opmlDocument
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode opml: %w", err)
	}

	var entries []opmlFeedEntry
	collectFeedEntries(doc.Body.Outlines, &entries)
	return entries, nil
}

// collectFeedEntries recursively flattens nested <outline> elements.
func collectFeedEntries(outlines []opmlOutline, entries *[]opmlFeedEntry) {
	for _, ol := range outlines {
		if ol.XMLURL != "" {
			title := ol.Title
			if title == "" {
				title = ol.Text
			}
			*entries = append(*entries, opmlFeedEntry{
				Title:   title,
				FeedURL: ol.XMLURL,
				SiteURL: ol.HTMLURL,
			})
		}
		if len(ol.Outlines) > 0 {
			collectFeedEntries(ol.Outlines, entries)
		}
	}
}

func (h *Handler) ImportOPML(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	// Max 10MB upload
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("opml_file")
	if err != nil {
		http.Error(w, "OPML file is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	entries, err := parseOPML(file)
	if err != nil {
		log.Printf("parse opml: %v", err)
		http.Error(w, "Failed to parse OPML file", http.StatusBadRequest)
		return
	}

	if len(entries) == 0 {
		http.Error(w, "No feeds found in OPML file", http.StatusBadRequest)
		return
	}

	// Get existing user feeds to skip duplicates
	existingFeeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get existing feeds: %v", err)
	}
	existingURLs := make(map[string]bool, len(existingFeeds))
	for _, f := range existingFeeds {
		existingURLs[f.FeedURL] = true
	}

	imported := 0
	skipped := 0
	var errs []string

	for _, entry := range entries {
		if existingURLs[entry.FeedURL] {
			skipped++
			continue
		}

		result, err := feeds.FetchFeed(entry.FeedURL, userID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.FeedURL, err))
			continue
		}

		// Use OPML title if available (often more descriptive), fall back to fetched title
		if entry.Title != "" {
			result.Feed.Title = entry.Title
		}
		if entry.SiteURL != "" && result.Feed.SiteURL == nil {
			result.Feed.SiteURL = &entry.SiteURL
		}

		feed, err := h.DB.CreateFeed(result.Feed)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", entry.FeedURL, err))
			continue
		}

		// Store articles from the fetched feed
		for _, article := range result.Articles {
			article.FeedID = feed.ID
			if _, err := h.DB.CreateArticle(article); err != nil {
				log.Printf("skip article during OPML import (likely duplicate): %v", err)
				continue
			}
		}

		imported++
	}

	log.Printf("OPML import for user %d: %d imported, %d skipped, %d errors", userID, imported, skipped, len(errs))
	for _, e := range errs {
		log.Printf("  OPML import error: %s", e)
	}

	// Re-render the feed list sidebar
	h.ListFeeds(w, r)
}

func (h *Handler) AddFeed(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	feedURL := strings.TrimSpace(r.FormValue("url"))
	if feedURL == "" {
		http.Error(w, "Feed URL is required", http.StatusBadRequest)
		return
	}

	// Check if feed already exists for this user
	userFeeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get user feeds: %v", err)
	}
	for _, f := range userFeeds {
		if f.FeedURL == feedURL {
			http.Error(w, "Feed already subscribed", http.StatusConflict)
			return
		}
	}

	// Fetch feed to get metadata
	result, err := feeds.FetchFeed(feedURL, userID)
	if err != nil {
		log.Printf("fetch feed %s: %v", feedURL, err)
		http.Error(w, "Failed to fetch feed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Create feed in database
	feed, err := h.DB.CreateFeed(result.Feed)
	if err != nil {
		log.Printf("create feed: %v", err)
		http.Error(w, "Failed to save feed", http.StatusInternalServerError)
		return
	}

	// Store new articles
	for _, article := range result.Articles {
		article.FeedID = feed.ID
		_, err := h.DB.CreateArticle(article)
		if err != nil {
			// Skip duplicates (GUID constraint)
			log.Printf("skip article (likely duplicate): %v", err)
			continue
		}
	}

	// Return the updated feed list
	h.ListFeeds(w, r)
}

func (h *Handler) RemoveFeed(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	feedIDStr := chi.URLParam(r, "id")

	feedID, err := strconv.ParseInt(feedIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	if err := h.DB.DeleteFeed(feedID, userID); err != nil {
		log.Printf("delete feed: %v", err)
		http.Error(w, "Failed to remove feed", http.StatusInternalServerError)
		return
	}

	// Return empty response (HTMX will remove the element)
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) RefreshFeed(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	feedIDStr := chi.URLParam(r, "id")

	feedID, err := strconv.ParseInt(feedIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid feed ID", http.StatusBadRequest)
		return
	}

	feed, err := h.DB.GetFeedByID(feedID, userID)
	if err != nil {
		log.Printf("get feed by id: %v", err)
		http.Error(w, "Feed not found", http.StatusNotFound)
		return
	}

	// Fetch new articles
	result, err := feeds.FetchFeed(feed.FeedURL, userID)
	if err != nil {
		log.Printf("fetch feed %s: %v", feed.FeedURL, err)
		http.Error(w, "Failed to refresh feed", http.StatusInternalServerError)
		return
	}

	// Get user settings for AI provider
	settings, err := h.DB.GetUserSettings(userID)
	if err != nil {
		log.Printf("get user settings: %v", err)
	}

	// Count new articles
	newCount := 0

	for _, article := range result.Articles {
		article.FeedID = feedID

		stored, err := h.DB.CreateArticle(article)
		if err != nil {
			log.Printf("skip article during refresh (likely duplicate): %v", err)
			continue
		}
		newCount++

		// Run AI summarization on new articles with content
		if stored.Content != nil && *stored.Content != "" {
			summary, err := summarizer.Summarize(h.buildSummaryRequest(settings, *stored.Content))
			if err != nil {
				log.Printf("summarize article %d: %v", stored.ID, err)
				continue
			}
			stored.Summary = &summary
			if err := h.DB.UpdateArticleSummary(stored.ID, summary); err != nil {
				log.Printf("update article summary: %v", err)
			}
		}
	}

	// Update last fetched time
	_ = h.DB.UpdateFeedLastFetched(feedID, time.Now())

	// Return article list
	h.ListArticlesByFeed(w, r)
}

func (h *Handler) ListArticlesByFeed(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	feedIDStr := r.URL.Query().Get("feed_id")

	var articles []model.Article
	var err error

	if feedIDStr != "" {
		fid, convErr := strconv.ParseInt(feedIDStr, 10, 64)
		if convErr == nil {
			articles, err = h.DB.GetArticlesByFeedID(fid)
		} else {
			articles, err = h.DB.GetArticlesByUserID(userID, nil)
		}
	} else {
		articles, err = h.DB.GetArticlesByUserID(userID, nil)
	}

	if err != nil {
		log.Printf("list articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Articles": articles,
	}

	if err := articleListTmpl.Execute(w, data); err != nil {
		log.Printf("render article list: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) buildSummaryRequest(settings *model.UserSettings, content string) ai.SummaryRequest {
	req := ai.SummaryRequest{
		ArticleText: content,
		Length:      "short",
		SummaryLang: "english",
	}

	if settings != nil {
		req.Provider = settings.AIProvider
		req.Model = settings.ModelName
		req.Length = settings.SummaryLength
		req.SummaryLang = settings.SummaryLanguage
		if settings.BaseURL != nil {
			req.BaseURL = *settings.BaseURL
		}
		if settings.APIKeyEncrypted != nil {
			// Decrypt if encryption key is configured
			if h.EncryptionKey != "" {
				decrypted, err := Decrypt(*settings.APIKeyEncrypted, []byte(h.EncryptionKey))
				if err != nil {
					log.Printf("decrypt API key: %v", err)
					req.APIKey = *settings.APIKeyEncrypted // fallback to stored value
				} else {
					req.APIKey = string(decrypted)
				}
			} else {
				req.APIKey = *settings.APIKeyEncrypted
			}
		}
	}

	if req.Provider == "" {
		req.Provider = "opencode-go"
	}
	if req.Model == "" {
		switch req.Provider {
		case "opencode-go":
			req.Model = "deepseek-v4-flash"
		default:
			req.Model = "gpt-4o-mini"
		}
	}

	// Fallback to global OpenCode Go key if user hasn't set one
	if req.APIKey == "" && req.Provider == "opencode-go" && h.OpenCodeGoKey != "" {
		req.APIKey = h.OpenCodeGoKey
	}

	return req
}

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	idStr := chi.URLParam(r, "id")
	
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}
	
	if err := h.DB.MarkArticleRead(id, userID); err != nil {
		log.Printf("mark article read: %v", err)
	}
	
	w.WriteHeader(http.StatusNoContent)
}

// ─── Inline Templates ──────────────────────────────────────────────────────

const dashboardTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Dashboard</title>
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
      <button class="chip" id="scroll-read-toggle" aria-pressed="true" title="Mark articles read as you scroll past them">
        <span class="chip__dot"></span> Auto-read
      </button>
      <button class="btn btn--ghost" id="mark-all-read" title="Mark all visible articles read">Mark read</button>
      <span class="ai-status" title="AI: {{.SettingsProvider}} / {{.SettingsModel}}">
        <span class="ai-status__dot{{if .SettingsConfigured}} ai-status__dot--on{{end}}"></span>
        <span class="ai-status__label">{{.SettingsProvider}}</span>
      </span>
      <a href="/settings" class="btn btn--ghost">Settings</a>
      <a href="/admin/logs" class="btn btn--ghost">Admin</a>
      <form hx-post="/logout" hx-target="body" hx-swap="outerHTML" class="flex">
        <button type="submit" class="btn btn--ghost">Log out</button>
      </form>
    </div>
    <div class="progress"><div class="progress__bar" id="progress-bar"></div></div>
  </nav>

  <main class="layout">
    <aside id="feed-sidebar" class="sidebar">
      <div class="panel" hx-get="/feeds" hx-trigger="load" hx-target="#feed-sidebar-inner">
        <div id="feed-sidebar-inner">
          <div class="panel__head"><span class="panel__title"><b>≡</b> Pen</span></div>
          {{range .Feeds}}
          <div class="feed{{if eq $.FeedID (printf "%d" .ID)}} feed--active{{end}}" data-feed-id="{{.ID}}">
            <a href="/?feed_id={{.ID}}" class="feed__row"
               hx-get="/?feed_id={{.ID}}" hx-target="#article-list" hx-push-url="true"
               hx-indicator="#loading" hx-trigger="click[preventDefault]">
              <span class="feed__title">{{.Title}}</span>
              {{if gt .UnreadCount 0}}<span class="ear-tag" data-count="{{.UnreadCount}}">{{.UnreadCount}}</span>{{end}}
            </a>
            <div class="feed__tools">
              <button hx-post="/feeds/{{.ID}}/refresh" hx-target="#article-list" hx-indicator="#loading" class="tool" title="Refresh">↻</button>
              <button hx-delete="/feeds/{{.ID}}" hx-target="closest .feed" hx-swap="outerHTML swap:0.3s"
                hx-confirm="Remove this feed?" class="tool tool--del" title="Remove">✕</button>
            </div>
          </div>
          {{else}}
          <div class="empty">
            <p class="empty__t">No feeds yet</p>
            <p class="empty__s">Add one below</p>
          </div>
          {{end}}
          <div class="add-feed">
            <form hx-post="/feeds" hx-target="#feed-sidebar" hx-swap="outerHTML" class="flex gap-2">
              <input type="url" name="url" placeholder="RSS / Atom URL" required class="input">
              <button type="submit" class="btn btn--primary btn--mini">Add</button>
            </form>
          </div>
          <div class="add-feed add-feed--opml">
            <form hx-post="/feeds/import" hx-target="#feed-sidebar" hx-swap="outerHTML" hx-encoding="multipart/form-data">
              <label class="label-mono">Import OPML</label>
              <div class="flex gap-2">
                <input type="file" name="opml_file" accept=".opml,.xml" required class="file">
                <button type="submit" class="btn btn--ghost btn--mini">Import</button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </aside>

    <section id="article-list" class="stream">
      <div id="loading" class="htmx-indicator loading">Checking pens...</div>
      {{range .Articles}}
      <div class="card{{if not .IsRead}} is-unread{{end}}{{if .IsRead}} is-read{{end}}" data-article-id="{{.ID}}" data-feed-id="{{.FeedID}}">
        <div class="card__main">
          <h3 class="card__title">
            {{if .URL}}<a href="{{deref .URL}}" target="_blank" rel="noopener noreferrer">{{.Title}}</a>{{else}}{{.Title}}{{end}}
          </h3>
          <p class="card__meta">
            {{if .Author}}<span>{{deref .Author}}</span>{{end}}
            {{if .PublishedAt}}<span>{{timeAgo .PublishedAt}}</span>{{end}}
            <span class="card__pen">#{{.FeedID}}</span>
          </p>
          {{if .Summary}}<p class="card__summary">{{deref .Summary}}</p>{{end}}
        </div>
        <button hx-post="/articles/{{.ID}}/toggle" hx-target="closest .card" hx-swap="outerHTML"
          class="card__read" title="{{if .IsRead}}Mark unread{{else}}Mark read{{end}}"
          data-read="{{.IsRead}}">
          {{if not .IsRead}}●{{else}}○{{end}}
        </button>
      </div>
      {{else}}
      <div class="empty">
        <div class="empty__mark">🐄</div>
        <p class="empty__t">No articles yet</p>
        <p class="empty__s">Add a feed or refresh existing ones</p>
      </div>
      {{end}}
    </section>
  </main>
</body>
</html>
`

const feedListTemplate = `
<div class="panel">
  <div class="panel__head"><span class="panel__title"><b>≡</b> Pen</span></div>
  {{range .Feeds}}
  <div class="feed" data-feed-id="{{.ID}}">
    <a href="/?feed_id={{.ID}}" class="feed__row"
       hx-get="/?feed_id={{.ID}}" hx-target="#article-list" hx-push-url="true"
       hx-indicator="#loading" hx-trigger="click[preventDefault]">
      <span class="feed__title">{{.Title}}</span>
      {{if gt .UnreadCount 0}}<span class="ear-tag" data-count="{{.UnreadCount}}">{{.UnreadCount}}</span>{{end}}
    </a>
    <div class="feed__tools">
      <button hx-post="/feeds/{{.ID}}/refresh" hx-target="#article-list" hx-indicator="#loading" class="tool" title="Refresh">↻</button>
      <button hx-delete="/feeds/{{.ID}}" hx-target="closest .feed" hx-swap="outerHTML swap:0.3s"
        hx-confirm="Remove this feed?" class="tool tool--del" title="Remove">✕</button>
    </div>
  </div>
  {{else}}
  <div class="empty">
    <p class="empty__t">No feeds yet</p>
    <p class="empty__s">Add one below</p>
  </div>
  {{end}}
  <div class="add-feed">
    <form hx-post="/feeds" hx-target="#feed-sidebar" hx-swap="outerHTML" class="flex gap-2">
      <input type="url" name="url" placeholder="RSS / Atom URL" required class="input">
      <button type="submit" class="btn btn--primary btn--mini">Add</button>
    </form>
  </div>
  <div class="add-feed add-feed--opml">
    <form hx-post="/feeds/import" hx-target="#feed-sidebar" hx-swap="outerHTML" hx-encoding="multipart/form-data">
      <label class="label-mono">Import OPML</label>
      <div class="flex gap-2">
        <input type="file" name="opml_file" accept=".opml,.xml" required class="file">
        <button type="submit" class="btn btn--ghost btn--mini">Import</button>
      </div>
    </form>
  </div>
</div>
`

const articleListTemplate = `
{{range .Articles}}
<div class="card{{if not .IsRead}} is-unread{{end}}{{if .IsRead}} is-read{{end}}" data-article-id="{{.ID}}" data-feed-id="{{.FeedID}}">
  <div class="card__main">
    <h3 class="card__title">
      {{if .URL}}<a href="{{deref .URL}}" target="_blank" rel="noopener noreferrer">{{.Title}}</a>{{else}}{{.Title}}{{end}}
    </h3>
    <p class="card__meta">
      {{if .Author}}<span>{{deref .Author}}</span>{{end}}
      <span class="card__pen">#{{.FeedID}}</span>
    </p>
    {{if .Summary}}<p class="card__summary">{{deref .Summary}}</p>{{end}}
  </div>
  <button hx-post="/articles/{{.ID}}/toggle" hx-target="closest .card" hx-swap="outerHTML"
    class="card__read" title="{{if .IsRead}}Mark unread{{else}}Mark read{{end}}"
    data-read="{{.IsRead}}">
    {{if not .IsRead}}●{{else}}○{{end}}
  </button>
</div>
{{else}}
<div class="empty">
  <div class="empty__mark">🐄</div>
  <p class="empty__t">No articles yet</p>
  <p class="empty__s">Add a feed or refresh existing ones</p>
</div>
{{end}}
`
