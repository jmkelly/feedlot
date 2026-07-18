package handler

import (
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (h *Handler) ListArticles(w http.ResponseWriter, r *http.Request) {
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

	articles, err := h.DB.GetArticlesByUserID(userID, feedID)
	if err != nil {
		log.Printf("list articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Articles": articles,
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

	// Render the updated card (primary HTMX swap target)
	// hx-target="closest .card" hx-swap="outerHTML" on the toggle button
	data := map[string]any{
		"Article": *article,
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

const articleCardTemplate = `
<div class="card{{if not .Article.IsRead}} is-unread{{end}}{{if .Article.IsRead}} is-read{{end}}" data-article-id="{{.Article.ID}}" data-feed-id="{{.Article.FeedID}}">
  <div class="card__main">
    <h3 class="card__title">
      {{if .Article.URL}}<a href="{{deref .Article.URL}}" target="_blank" rel="noopener noreferrer">{{.Article.Title}}</a>{{else}}{{.Article.Title}}{{end}}
    </h3>
    {{if .Article.Author}}<p class="card__meta">{{deref .Article.Author}}</p>{{end}}
    {{if .Article.Summary}}<p class="card__summary">{{deref .Article.Summary}}</p>{{end}}
  </div>
  <button hx-post="/articles/{{.Article.ID}}/toggle" hx-target="closest .card" hx-swap="outerHTML"
    class="card__read" title="{{if .Article.IsRead}}Mark unread{{else}}Mark read{{end}}"
    data-read="{{.Article.IsRead}}">
    {{if not .Article.IsRead}}●{{else}}○{{end}}
  </button>
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
