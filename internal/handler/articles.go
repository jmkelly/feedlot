package handler

import (
	"html/template"
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
		if err == nil {
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

	tmpl := template.Must(template.New("articles").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(articleListTemplate))
	if err := tmpl.Execute(w, data); err != nil {
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
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	data := map[string]any{
		"Article": article,
	}

	tmpl := template.Must(template.New("article").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(articleDetailTemplate))
	if err := tmpl.Execute(w, data); err != nil {
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

	data := map[string]any{
		"Article": *article,
	}

	tmpl := template.Must(template.New("article-card").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(articleCardTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render article card: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const articleCardTemplate = `<div class="article-card bg-white rounded-xl shadow-sm border border-stone-200 p-4 mb-3 {{if not .Article.IsRead}}border-l-4 border-l-amber-500{{else}}opacity-75{{end}}">
  <div class="flex items-start justify-between gap-3">
    <div class="flex-1 min-w-0">
      <h3 class="text-base font-semibold {{if not .Article.IsRead}}text-stone-900{{else}}text-stone-500{{end}} truncate">
        {{if .Article.URL}}<a href="{{deref .Article.URL}}" target="_blank" rel="noopener noreferrer" class="hover:text-amber-700 transition-colors">{{.Article.Title}}</a>{{else}}{{.Article.Title}}{{end}}
      </h3>
      {{if .Article.Author}}<p class="text-xs text-stone-400 mt-0.5">{{deref .Article.Author}}</p>{{end}}
      {{if .Article.Summary}}<p class="text-sm text-stone-600 mt-2 line-clamp-3">{{deref .Article.Summary}}</p>{{end}}
    </div>
    <button hx-post="/articles/{{.Article.ID}}/toggle" hx-target="closest .article-card" hx-swap="outerHTML"
      class="flex-shrink-0 p-1.5 rounded-full {{if not .Article.IsRead}}text-amber-500 hover:text-amber-700{{else}}text-stone-300 hover:text-stone-500{{end}} transition-colors"
      title="{{if .Article.IsRead}}Mark unread{{else}}Mark read{{end}}">
      {{if not .Article.IsRead}}●{{else}}○{{end}}
    </button>
  </div>
</div>`

const articleDetailTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Article.Title}} — Feedlot</title>
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="bg-stone-50 text-stone-900 antialiased">
  <nav class="bg-white border-b border-stone-200 shadow-sm">
    <div class="max-w-3xl mx-auto px-4 py-3">
      <a href="/" class="text-stone-500 hover:text-stone-700 text-sm">&larr; Back to dashboard</a>
    </div>
  </nav>
  <article class="max-w-3xl mx-auto px-4 py-8">
    <h1 class="text-2xl font-bold text-stone-900">{{.Article.Title}}</h1>
    <div class="flex items-center gap-3 mt-2 text-sm text-stone-500">
      {{if .Article.Author}}<span>{{deref .Article.Author}}</span>{{end}}
      <button hx-post="/articles/{{.Article.ID}}/toggle" hx-target="closest .article-card" hx-swap="outerHTML"
        class="text-amber-600 hover:text-amber-700 font-medium">
        {{if .Article.IsRead}}Mark unread{{else}}Mark read{{end}}
      </button>
      {{if .Article.URL}}<a href="{{deref .Article.URL}}" target="_blank" rel="noopener noreferrer" class="text-amber-600 hover:text-amber-700">View original &nearr;</a>{{end}}
    </div>
    {{if .Article.Summary}}
    <div class="mt-6 bg-amber-50 border border-amber-200 rounded-xl p-4">
      <h2 class="text-sm font-semibold text-amber-800 uppercase tracking-wider mb-2">Summary</h2>
      <p class="text-stone-700">{{deref .Article.Summary}}</p>
    </div>
    {{end}}
    {{if .Article.Content}}
    <div class="mt-6 prose prose-stone max-w-none">
      {{deref .Article.Content}}
    </div>
    {{end}}
  </article>
</body>
</html>`
