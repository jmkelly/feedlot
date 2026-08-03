package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/james/feedlot/internal/ai"
)

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	userID := GetUserID(r)

	feedIDStr := r.URL.Query().Get("feed_id")
	var feedID *int64
	if feedIDStr != "" {
		id, err := strconv.ParseInt(feedIDStr, 10, 64)
		if err != nil {
			log.Printf("invalid feed_id parameter %q: %v", feedIDStr, err)
		} else {
			feedID = &id
		}
	}

	unreadOnly := r.URL.Query().Get("unread") == "1"

	articles, err := h.DB.GetArticlesByUserID(userID, feedID, unreadOnly, 0, 0)
	if err != nil {
		log.Printf("list articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Articles":     articles,
		"FeedID":       feedIDStr,
		"UnreadOnly":   unreadOnly,
		"Limit":        0,
		"Offset":       0,
		"FilterParams": filterParams(r),
	}

	if err := articleListTmpl.Execute(w, data); err != nil {
		log.Printf("render articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) ShowArticle(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)
	articleIDStr := chi.URLParam(r, "id")

	articleID, err := strconv.ParseInt(articleIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	article, err := h.DB.GetArticleByID(articleID, userID)
	if err != nil {
		log.Printf("get article %d: %v", articleID, err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Article": article,
	}

	if err := articleDetailTmpl.Execute(w, data); err != nil {
		log.Printf("render article: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) ToggleRead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	userID := GetUserID(r)
	articleIDStr := chi.URLParam(r, "id")

	articleID, err := strconv.ParseInt(articleIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	article, err := h.DB.ToggleArticleRead(articleID, userID)
	if err != nil {
		log.Printf("toggle article read: %v", err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// When the unread-only filter is active, a post that is now read must
	// disappear from the view: swap the card away (empty main content) and
	// refresh the sidebar OOB. If nothing unread remains, the server swaps in
	// the "all caught up" empty state instead.
	if r.URL.Query().Get("unread") == "1" && article.IsRead {
		h.renderUnreadRemoval(w, userID, r.URL.Query().Get("feed_id"))
		return
	}

	// Render the updated card (primary HTMX swap target)
	// hx-target="closest .card" hx-swap="outerHTML" on the toggle button
	data := map[string]any{
		"Article":      *article,
		"FilterParams": filterParams(r),
	}
	if err := articleCardTmpl.Execute(w, data); err != nil {
		log.Printf("render article card: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Render the OOB feed sidebar with authoritative unread counts from the server.
	// HTMX will process this as an out-of-band swap and update #feed-sidebar-inner,
	// keeping unread badges in sync without any client-side arithmetic.
	feeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get feeds for oob: %v", err)
		return
	}
	feedData := map[string]any{
		"Feeds": feeds,
	}
	if err := feedSidebarOOBTmpl.Execute(w, feedData); err != nil {
		log.Printf("render feed sidebar oob: %v", err)
	}
}

// renderUnreadRemoval writes the response for a read action performed while the
// unread-only filter is active: the card is swapped away (empty main content,
// which htmx replaces the card with under hx-swap="outerHTML"), or the
// "all caught up" empty state when no unread articles remain in the current
// view. The feed sidebar is refreshed alongside via an OOB swap so unread
// counts stay authoritative.
func (h *Handler) renderUnreadRemoval(w http.ResponseWriter, userID int64, feedIDStr string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// A non-empty feed_id that fails to parse intentionally falls back to nil
	// (all feeds): the dashboard list applies the same ParseInt (feeds.go), so
	// the count and the displayed view always agree. The empty response body is
	// the card-removal signal under hx-swap="outerHTML"; sidebar OOB failures
	// log and degrade silently, matching the ToggleRead OOB path.
	var feedID *int64
	if feedIDStr != "" {
		if fid, err := strconv.ParseInt(feedIDStr, 10, 64); err == nil {
			feedID = &fid
		}
	}

	remaining, err := h.DB.CountUnreadArticles(userID, feedID)
	if err != nil {
		log.Printf("count unread articles: %v", err)
	} else if remaining == 0 {
		data := map[string]any{"UnreadOnly": true}
		if err := emptyStateTmpl.Execute(w, data); err != nil {
			log.Printf("render empty state: %v", err)
		}
	}

	feeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get feeds for oob: %v", err)
		return
	}
	if err := feedSidebarOOBTmpl.Execute(w, map[string]any{"Feeds": feeds}); err != nil {
		log.Printf("render feed sidebar oob: %v", err)
	}
}

func (h *Handler) SummarizeArticle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	userID := GetUserID(r)
	articleIDStr := chi.URLParam(r, "id")

	articleID, err := strconv.ParseInt(articleIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	article, err := h.DB.GetArticleByID(articleID, userID)
	if err != nil {
		log.Printf("get article %d for summarize: %v", articleID, err)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Load user settings for AI config
	settings, err := h.DB.GetUserSettings(userID)
	if err != nil || settings == nil {
		log.Printf("get user settings for summarization: %v", err)
		http.Error(w, "AI not configured — go to Settings", http.StatusBadRequest)
		return
	}

	apiKey := ""
	if settings.APIKeyEncrypted != nil {
		if h.EncryptionKey != "" {
			decrypted, err := Decrypt(*settings.APIKeyEncrypted, []byte(h.EncryptionKey))
			if err != nil {
				log.Printf("decrypt API key: %v", err)
			} else {
				apiKey = string(decrypted)
			}
		} else {
			apiKey = *settings.APIKeyEncrypted
		}
	}

	// Fallback to global OpenCode Go key
	if apiKey == "" && settings.AIProvider == "opencode-go" && h.OpenCodeGoKey != "" {
		apiKey = h.OpenCodeGoKey
	}

	if apiKey == "" {
		http.Error(w, "AI API key not configured — go to Settings", http.StatusBadRequest)
		return
	}

	// Summarize the whole page the feed item points at (Hacker News items only
	// carry a "Comments" link, not the article), falling back to the feed's
	// own content when the page can't be fetched.
	articleText, err := h.articleTextForSummary(r.Context(), article, true)
	if err != nil {
		log.Printf("summarize article %d: %v", articleID, err)
		http.Error(w, "Article has no content", http.StatusBadRequest)
		return
	}

	baseURL := ""
	if settings.BaseURL != nil {
		baseURL = *settings.BaseURL
	}

	req := ai.SummaryRequest{
		Provider:    settings.AIProvider,
		APIKey:      apiKey,
		Model:       settings.ModelName,
		BaseURL:     baseURL,
		ArticleText: articleText,
		Length:      settings.SummaryLength,
		SummaryLang: settings.SummaryLanguage,
		Context:     r.Context(),
	}

	summary, err := h.Summarizer.Summarize(req)
	if err != nil {
		log.Printf("summarize article %d: %v", articleID, err)
		http.Error(w, "Summarization failed", http.StatusInternalServerError)
		return
	}

	// Save summary to DB
	if err := h.DB.UpdateArticleSummary(articleID, summary); err != nil {
		log.Printf("save summary for article %d: %v", articleID, err)
		http.Error(w, "Failed to save summary", http.StatusInternalServerError)
		return
	}

	// Re-render the updated card (same as ToggleRead pattern). FilterParams
	// keeps the unread/feed filter state flowing through the re-rendered
	// card's buttons.
	article.Summary = &summary
	data := map[string]any{
		"Article":      *article,
		"FilterParams": filterParams(r),
	}
	if err := articleCardTmpl.Execute(w, data); err != nil {
		log.Printf("render article card after summarize: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
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

	// Scroll-past-read under the unread-only filter: the card is swapped away
	// via the same server-driven removal as the toggle (see renderUnreadRemoval).
	if r.URL.Query().Get("unread") == "1" {
		h.renderUnreadRemoval(w, userID, r.URL.Query().Get("feed_id"))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

const articleCardTemplate = `
<div class="card{{if not .Article.IsRead}} is-unread{{end}}{{if .Article.IsRead}} is-read{{end}}" data-article-id="{{.Article.ID}}" data-feed-id="{{.Article.FeedID}}">
  <div class="card__main">
    <h3 class="card__title">
      {{if .Article.URL}}<a href="{{deref .Article.URL}}" target="_blank" rel="noopener noreferrer">{{.Article.Title}}</a>{{else}}{{.Article.Title}}{{end}}
    </h3>
    {{if .Article.Author}}<p class="card__meta">{{deref .Article.Author}}</p>{{end}}
    {{if .Article.Summary}}<p class="card__summary">{{deref .Article.Summary}}</p>{{end}}
  </div>
  <div class="card__actions">
    <button hx-post="/articles/{{.Article.ID}}/summarize{{.FilterParams}}" hx-target="closest .card" hx-swap="outerHTML"
      class="card__summarize" title="Generate AI summary">
      <span class="card__summarize-icon">✦</span>
    </button>
    <button hx-post="/articles/{{.Article.ID}}/toggle{{.FilterParams}}" hx-target="closest .card" hx-swap="outerHTML"
      class="card__read" title="{{if .Article.IsRead}}Mark unread{{else}}Mark read{{end}}"
      data-read="{{.Article.IsRead}}">
      {{if not .Article.IsRead}}●{{else}}○{{end}}
    </button>
  </div>
</div>
`

const articleDetailTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Article.Title}} — Feedlot</title>
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
  <nav class="topbar" style="position:static">
    <div class="topbar__brand">
      <span class="topbar__mark">🐄</span>
      <span class="topbar__name">Feedlot</span>
      <span class="topbar__tag">Field Station</span>
    </div>
    <div class="topbar__spacer"></div>
    <div class="topbar__actions">
      <button class="btn btn--ghost" id="theme-toggle" title="Toggle light/dark theme" aria-label="Toggle light/dark theme"></button>
      <a href="/" class="btn btn--ghost">&larr; Back to dashboard</a>
    </div>
  </nav>
  <div class="detailwrap">
    <a href="/" class="detail__back">&larr; Back to dashboard</a>
    <h1 class="detail__title">{{.Article.Title}}</h1>
    <div class="detail__meta">
      {{if .Article.Author}}<span>{{deref .Article.Author}}</span>{{end}}
      <button hx-post="/articles/{{.Article.ID}}/toggle" hx-target="closest .article-card" hx-swap="outerHTML"
        class="btn btn--ghost" style="color:var(--brick);padding:.3rem .6rem">
        {{if .Article.IsRead}}Mark unread{{else}}Mark read{{end}}
      </button>
      {{if .Article.URL}}<a href="{{deref .Article.URL}}" target="_blank" rel="noopener noreferrer" class="btn btn--ghost" style="color:var(--brick);padding:.3rem .6rem">View original ↗</a>{{end}}
    </div>
    {{if .Article.Summary}}
    <div class="summary-box">
      <div class="summary-box__h">Summary</div>
      <p>{{deref .Article.Summary}}</p>
    </div>
    {{end}}
    {{if .Article.Content}}
    <div class="prose">
      {{deref .Article.Content}}
    </div>
    {{end}}
  </div>
</body>
</html>
`
