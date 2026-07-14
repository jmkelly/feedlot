package handler

import (
	"html/template"
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

	feeds, err := h.DB.GetUserFeeds(userID)
	if err != nil {
		log.Printf("get user feeds: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check for feed_id query parameter
	feedIDStr := r.URL.Query().Get("feed_id")
	var articles []model.Article
	var selectedFeedID *int64

	if feedIDStr != "" {
		fid, err := strconv.ParseInt(feedIDStr, 10, 64)
		if err == nil {
			selectedFeedID = &fid
		}
	}

	articles, err = h.DB.GetArticlesByUserID(userID, selectedFeedID)
	if err != nil {
		log.Printf("get articles: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := map[string]any{
		"Feeds":    feeds,
		"Articles": articles,
		"FeedID":   feedIDStr,
	}

	tmpl := template.Must(template.New("dashboard").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"timeAgo": func(t interface{}) string {
			return "recently"
		},
	}).Parse(dashboardTemplate))
	if err := tmpl.Execute(w, data); err != nil {
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

	tmpl := template.Must(template.New("feed-list").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(feedListTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render feed list: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
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

	// Check if feed already exists for any user
	existing, _ := h.DB.GetFeedByURL(feedURL)
	if existing != nil {
		// Feed already subscribed — check if by this user
		if existing.UserID == userID {
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
	settings, _ := h.DB.GetUserSettings(userID)

	// Count new articles
	newCount := 0

	for _, article := range result.Articles {
		article.FeedID = feedID

		stored, err := h.DB.CreateArticle(article)
		if err != nil {
			// Duplicate — skip
			continue
		}
		newCount++

		// Run AI summarization on new articles with content
		if stored.Content != nil && *stored.Content != "" {
			summary, err := summarizer.Summarize(buildSummaryRequest(settings, *stored.Content))
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

	tmpl := template.Must(template.New("article-list").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"timeAgo": func(t interface{}) string {
			return "recently"
		},
	}).Parse(articleListTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render article list: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func buildSummaryRequest(settings *model.UserSettings, content string) ai.SummaryRequest {
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
			req.APIKey = *settings.APIKeyEncrypted
		}
	}

	if req.Provider == "" {
		req.Provider = "openai"
	}
	if req.Model == "" {
		req.Model = "gpt-4o-mini"
	}

	return req
}

// ─── Inline Templates ──────────────────────────────────────────────────────

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Dashboard</title>
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="bg-stone-50 text-stone-900 antialiased">
  <nav class="bg-white border-b border-stone-200 shadow-sm">
    <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
      <div class="flex items-center justify-between h-14">
        <div class="flex items-center gap-3">
          <span class="text-2xl">🐄</span>
          <h1 class="text-lg font-bold text-amber-800">Feedlot</h1>
        </div>
        <div class="flex items-center gap-4">
          <a href="/settings" class="text-stone-500 hover:text-stone-700 text-sm font-medium">Settings</a>
          <form hx-post="/logout" hx-target="body" hx-swap="outerHTML" class="inline">
            <button type="submit" class="text-stone-500 hover:text-red-600 text-sm font-medium">Log out</button>
          </form>
        </div>
      </div>
    </div>
  </nav>

  <main class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
    <div class="flex gap-6">
      <!-- Sidebar: Feed List -->
      <aside id="feed-sidebar" class="w-72 flex-shrink-0">
        <div class="bg-white rounded-xl shadow-sm border border-stone-200 overflow-hidden" hx-get="/feeds" hx-trigger="load" hx-target="#feed-sidebar-inner">
          <div id="feed-sidebar-inner">
            <div class="p-4 border-b border-stone-100">
              <h2 class="text-sm font-semibold text-stone-500 uppercase tracking-wider">Feeds</h2>
            </div>
            {{range .Feeds}}
            <div class="feed-item px-4 py-3 border-b border-stone-50 hover:bg-stone-50 transition-colors {{if eq $.FeedID (printf "%d" .ID)}}bg-amber-50{{end}}">
              <a href="/?feed_id={{.ID}}" class="block"
                 hx-get="/?feed_id={{.ID}}" hx-target="#article-list" hx-push-url="true"
                 hx-indicator="#loading">
                <div class="flex items-center justify-between">
                  <span class="text-sm font-medium text-stone-800 truncate">{{.Title}}</span>
                  {{if gt .UnreadCount 0}}
                  <span class="inline-flex items-center justify-center px-2 py-0.5 text-xs font-bold text-amber-800 bg-amber-100 rounded-full">{{.UnreadCount}}</span>
                  {{end}}
                </div>
              </a>
              <div class="flex items-center gap-2 mt-1">
                <button hx-post="/feeds/{{.ID}}/refresh" hx-target="#article-list" hx-indicator="#loading"
                  class="text-xs text-stone-400 hover:text-amber-600 transition-colors" title="Refresh">
                  ↻
                </button>
                <button hx-delete="/feeds/{{.ID}}" hx-target="closest .feed-item" hx-swap="outerHTML swap:0.3s"
                  hx-confirm="Remove this feed?" class="text-xs text-stone-400 hover:text-red-500 transition-colors" title="Remove">
                  ✕
                </button>
              </div>
            </div>
            {{else}}
            <div class="p-6 text-center text-stone-400">
              <p class="text-sm">No feeds yet</p>
              <p class="text-xs mt-1">Add one below</p>
            </div>
            {{end}}
            <div class="p-4">
              <form hx-post="/feeds" hx-target="#feed-sidebar" hx-swap="outerHTML" class="flex gap-2">
                <input type="url" name="url" placeholder="RSS/Atom URL" required
                  class="flex-1 px-3 py-1.5 text-sm border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
                <button type="submit"
                  class="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-sm font-medium rounded-lg transition-colors whitespace-nowrap">
                  Add
                </button>
              </form>
            </div>
          </div>
        </div>
      </aside>

      <!-- Main: Article List -->
      <section id="article-list" class="flex-1 min-w-0">
        <div id="loading" class="htmx-indicator text-center py-4 text-stone-400 text-sm">Loading...</div>
        {{range .Articles}}
        <div class="article-card bg-white rounded-xl shadow-sm border border-stone-200 p-4 mb-3 {{if not .IsRead}}border-l-4 border-l-amber-500{{else}}opacity-75{{end}}">
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 min-w-0">
              <h3 class="text-base font-semibold {{if not .IsRead}}text-stone-900{{else}}text-stone-500{{end}} truncate">
                {{if .URL}}<a href="{{deref .URL}}" target="_blank" rel="noopener noreferrer" class="hover:text-amber-700 transition-colors">{{.Title}}</a>{{else}}{{.Title}}{{end}}
              </h3>
              {{if .Author}}<p class="text-xs text-stone-400 mt-0.5">{{deref .Author}}</p>{{end}}
              {{if .Summary}}<p class="text-sm text-stone-600 mt-2 line-clamp-3">{{deref .Summary}}</p>{{end}}
            </div>
            <button hx-post="/articles/{{.ID}}/toggle" hx-target="closest .article-card" hx-swap="outerHTML"
              class="flex-shrink-0 p-1.5 rounded-full {{if not .IsRead}}text-amber-500 hover:text-amber-700{{else}}text-stone-300 hover:text-stone-500{{end}} transition-colors"
              title="{{if .IsRead}}Mark unread{{else}}Mark read{{end}}">
              {{if not .IsRead}}●{{else}}○{{end}}
            </button>
          </div>
        </div>
        {{else}}
        <div class="text-center py-12">
          <p class="text-stone-400 text-lg">No articles yet</p>
          <p class="text-stone-300 text-sm mt-1">Add a feed or refresh existing ones</p>
        </div>
        {{end}}
      </section>
    </div>
  </main>
</body>
</html>`

const feedListTemplate = `<div class="bg-white rounded-xl shadow-sm border border-stone-200 overflow-hidden">
  <div class="p-4 border-b border-stone-100">
    <h2 class="text-sm font-semibold text-stone-500 uppercase tracking-wider">Feeds</h2>
  </div>
  {{range .Feeds}}
  <div class="feed-item px-4 py-3 border-b border-stone-50 hover:bg-stone-50 transition-colors">
    <a href="/?feed_id={{.ID}}" class="block"
       hx-get="/?feed_id={{.ID}}" hx-target="#article-list" hx-push-url="true"
       hx-indicator="#loading">
      <div class="flex items-center justify-between">
        <span class="text-sm font-medium text-stone-800 truncate">{{.Title}}</span>
        {{if gt .UnreadCount 0}}
        <span class="inline-flex items-center justify-center px-2 py-0.5 text-xs font-bold text-amber-800 bg-amber-100 rounded-full">{{.UnreadCount}}</span>
        {{end}}
      </div>
    </a>
    <div class="flex items-center gap-2 mt-1">
      <button hx-post="/feeds/{{.ID}}/refresh" hx-target="#article-list" hx-indicator="#loading"
        class="text-xs text-stone-400 hover:text-amber-600 transition-colors" title="Refresh">
        ↻
      </button>
      <button hx-delete="/feeds/{{.ID}}" hx-target="closest .feed-item" hx-swap="outerHTML swap:0.3s"
        hx-confirm="Remove this feed?" class="text-xs text-stone-400 hover:text-red-500 transition-colors" title="Remove">
        ✕
      </button>
    </div>
  </div>
  {{else}}
  <div class="p-6 text-center text-stone-400">
    <p class="text-sm">No feeds yet</p>
    <p class="text-xs mt-1">Add one below</p>
  </div>
  {{end}}
  <div class="p-4">
    <form hx-post="/feeds" hx-target="#feed-sidebar" hx-swap="outerHTML" class="flex gap-2">
      <input type="url" name="url" placeholder="RSS/Atom URL" required
        class="flex-1 px-3 py-1.5 text-sm border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
      <button type="submit"
        class="px-3 py-1.5 bg-amber-600 hover:bg-amber-700 text-white text-sm font-medium rounded-lg transition-colors whitespace-nowrap">
        Add
      </button>
    </form>
  </div>
</div>`

const articleListTemplate = `{{range .Articles}}
<div class="article-card bg-white rounded-xl shadow-sm border border-stone-200 p-4 mb-3 {{if not .IsRead}}border-l-4 border-l-amber-500{{else}}opacity-75{{end}}">
  <div class="flex items-start justify-between gap-3">
    <div class="flex-1 min-w-0">
      <h3 class="text-base font-semibold {{if not .IsRead}}text-stone-900{{else}}text-stone-500{{end}} truncate">
        {{if .URL}}<a href="{{deref .URL}}" target="_blank" rel="noopener noreferrer" class="hover:text-amber-700 transition-colors">{{.Title}}</a>{{else}}{{.Title}}{{end}}
      </h3>
      {{if .Author}}<p class="text-xs text-stone-400 mt-0.5">{{deref .Author}}</p>{{end}}
      {{if .Summary}}<p class="text-sm text-stone-600 mt-2 line-clamp-3">{{deref .Summary}}</p>{{end}}
    </div>
    <button hx-post="/articles/{{.ID}}/toggle" hx-target="closest .article-card" hx-swap="outerHTML"
      class="flex-shrink-0 p-1.5 rounded-full {{if not .IsRead}}text-amber-500 hover:text-amber-700{{else}}text-stone-300 hover:text-stone-500{{end}} transition-colors"
      title="{{if .IsRead}}Mark unread{{else}}Mark read{{end}}">
      {{if not .IsRead}}●{{else}}○{{end}}
    </button>
  </div>
</div>
{{else}}
<div class="text-center py-12">
  <p class="text-stone-400 text-lg">No articles yet</p>
  <p class="text-stone-300 text-sm mt-1">Add a feed or refresh existing ones</p>
</div>
{{end}}`
