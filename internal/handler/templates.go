package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"
)

// timeAgo returns a relative time string like "2 hours ago" or "3 days ago".
// It accepts *time.Time (from model) or time.Time.
func timeAgo(t interface{}) string {
	var tm time.Time
	switch v := t.(type) {
	case *time.Time:
		if v == nil {
			return ""
		}
		tm = *v
	case time.Time:
		tm = v
	default:
		return ""
	}

	diff := time.Since(tm)
	if diff < 0 {
		return "just now"
	}

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < 2*time.Minute:
		return "1 minute ago"
	case diff < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(diff.Minutes()))
	case diff < 2*time.Hour:
		return "1 hour ago"
	case diff < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(diff.Hours()))
	case diff < 2*24*time.Hour:
		return "1 day ago"
	case diff < 30*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(diff.Hours()/24))
	case diff < 12*30*24*time.Hour:
		months := int(diff.Hours() / (24 * 30))
		if months == 1 {
			return "1 month ago"
		}
		return fmt.Sprintf("%d months ago", months)
	default:
		years := int(diff.Hours() / (24 * 365))
		if years == 1 {
			return "1 year ago"
		}
		return fmt.Sprintf("%d years ago", years)
	}
}

// Pre-parsed templates — parsed once at package init instead of on every request.
var (
	authTmpl = template.Must(template.New("auth").Parse(authPageTemplate))

	// The sidebar defines are shared by three parse trees: the full dashboard
	// page, the /feeds swap fragment (outerHTML of the whole aside), and the
	// out-of-band badge refresh (innerHTML of #feed-sidebar-inner).
	sidebarDefs = sidebarInnerBodyDef + sidebarInnerDef + sidebarFragmentDef

	dashboardTmpl = template.Must(template.New("dashboard").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"timeAgo": timeAgo,
		"add":     func(a, b int) int { return a + b },
	}).Parse(sidebarDefs + searchBarDef + dashboardTemplate))

	feedListTmpl = template.Must(template.New("feed-list").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(sidebarDefs))

	feedSidebarOOBTmpl = template.Must(template.New("feed-sidebar-oob").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(sidebarInnerBodyDef + feedSidebarOOBTemplate))

	emptyStateTmpl = template.Must(template.New("empty-state").Parse(emptyStateTemplate))

	articleListTmpl = template.Must(template.New("article-list").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"timeAgo": timeAgo,
		"add":     func(a, b int) int { return a + b },
	}).Parse(searchBarDef + articleListTemplate))

	loadMoreTmpl = template.Must(template.New("load-more").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"timeAgo": timeAgo,
		"add":     func(a, b int) int { return a + b },
	}).Parse(loadMoreTemplate))

	articleCardTmpl = template.Must(template.New("article-card").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(articleCardTemplate))

	articleDetailTmpl = template.Must(template.New("article").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(articleDetailTemplate))

	settingsTmpl = template.Must(template.New("settings").Parse(settingsPageTemplate))

	adminTmpl = template.Must(template.New("admin").Funcs(template.FuncMap{
		"sub": func(a, b int) int { return a - b },
		"add": func(a, b int) int { return a + b },
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(adminPageTemplate))
)

// filterParams returns the query-string suffix (with a leading "?") that
// preserves the current dashboard filter state — unread-only toggle, feed
// selection, and the fuzzy-search query — across HTMX requests like the
// read-toggle and summarize buttons. Returns "" when no filter is active.
func filterParams(r *http.Request) string {
	q := r.URL.Query()
	vals := url.Values{}
	if q.Get("unread") == "1" {
		vals.Set("unread", "1")
	}
	if fid := q.Get("feed_id"); fid != "" {
		vals.Set("feed_id", fid)
	}
	if s := q.Get("q"); s != "" {
		vals.Set("q", s)
	}
	if len(vals) == 0 {
		return ""
	}
	return "?" + vals.Encode()
}

// clearHref builds the "clear feed filter" link used in the stream heading,
// preserving the unread toggle and the active search. Computed server-side
// because conditional query-string segments inside an href make html/template's
// URL escaping context ambiguous.
func clearHref(unreadOnly bool, query string) template.URL {
	href := "/"
	if unreadOnly {
		href += "?unread=1"
	}
	if query != "" {
		if unreadOnly {
			href += "&"
		} else {
			href += "?"
		}
		href += "q=" + url.QueryEscape(query)
	}
	return template.URL(href)
}

// searchQueryFragments returns the pre-escaped query fragments that preserve
// an active search across links, as trusted template.URL values so they pass
// through html/template unmodified (the built-in urlquery func can't be used
// inside non-URL attributes like hx-get, and plain strings get re-escaped in
// href URL contexts). QueryAmp is for URLs that already carry a query string
// ("&q=..."), QueryQS for bare paths ("?q=..."). Both are empty when no
// search is active.
func searchQueryFragments(query string) (qs, amp template.URL) {
	if query == "" {
		return "", ""
	}
	q := "q=" + url.QueryEscape(query)
	return template.URL("?" + q), template.URL("&" + q)
}

// searchBarDef is the fuzzy-search row rendered at the top of the article
// stream. It lives inside #article-list so every list swap re-renders it
// with fresh filter state; hidden inputs carry the current feed/unread
// filters so a search composes with them (the visible input's value is
// included automatically because it is the htmx trigger). app.js restores
// focus to the input across swaps and highlights matching terms.
const searchBarDef = `{{define "search-bar"}}
<div class="searchbar">
  <input type="hidden" name="feed_id" value="{{.FeedID}}">
  <input type="hidden" name="unread" value="{{if .UnreadOnly}}1{{end}}">
  <span class="searchbar__icon" aria-hidden="true">⌕</span>
  <input type="search" id="article-search" name="q" value="{{.Query}}"
         placeholder="Search posts ( / )" autocomplete="off" spellcheck="false"
         aria-label="Search posts"
         hx-get="/" hx-trigger="input changed delay:250ms"
         hx-target="#article-list" hx-push-url="true"
         hx-indicator="#loading" hx-include="closest .searchbar">
  {{if .Query}}<a class="searchbar__clear" href="/?feed_id={{.FeedID}}{{if .UnreadOnly}}&unread=1{{end}}" title="Clear search" aria-label="Clear search">✕</a>{{end}}
</div>
{{end}}`

// emptyStateTemplate is the "all caught up" placeholder swapped in place of a
// card when a read action empties the unread-only view.
const emptyStateTemplate = `
<div class="empty">
  <div class="empty__mark">🐄</div>
  <p class="empty__t">All caught up</p>
  <p class="empty__s">No unread articles in this view</p>
</div>
`

const adminPageTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Admin</title>
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
      <a href="/" class="btn btn--ghost">&larr; Dashboard</a>
    </div>
  </nav>

  <main style="max-width:64rem;margin:0 auto;padding:2rem 1.1rem">
    <h2 style="font-family:var(--font-display);font-size:1.4rem;font-weight:700;margin:0 0 .25rem">Admin</h2>
    <p style="font-size:.75rem;color:var(--text-faint);margin:0 0 1rem">{{.Total}} log entries</p>

    <div class="panel" style="padding:0;overflow:hidden">
      <div style="overflow-x:auto">
        <table style="width:100%;border-collapse:collapse;font-family:var(--font-mono);font-size:.72rem;line-height:1.4">
          <thead>
            <tr style="background:var(--bg-hover);text-align:left">
              <th style="padding:.4rem .6rem;border-bottom:1px solid var(--border);color:var(--text-faint);font-weight:600;width:5rem">Time</th>
              <th style="padding:.4rem .6rem;border-bottom:1px solid var(--border);color:var(--text-faint);font-weight:600;width:3.5rem">Level</th>
              <th style="padding:.4rem .6rem;border-bottom:1px solid var(--border);color:var(--text-faint);font-weight:600">Message</th>
            </tr>
          </thead>
          <tbody>
            {{range .Logs}}
            <tr style="border-bottom:1px solid var(--border)">
              <td style="padding:.3rem .6rem;white-space:nowrap;color:var(--text-faint)">{{.CreatedAt}}</td>
              <td style="padding:.3rem .6rem"><span style="background:var(--bg-hover);padding:.1rem .35rem;border-radius:3px;font-size:.65rem">{{.Level}}</span></td>
              <td style="padding:.3rem .6rem;word-break:break-all">{{.Message}}</td>
            </tr>
            {{else}}
            <tr><td colspan="3" style="padding:2rem;text-align:center;color:var(--text-faint)">No log entries yet</td></tr>
            {{end}}
          </tbody>
        </table>
      </div>
    </div>

    {{if gt .TotalPages 1}}
    <div style="display:flex;gap:.35rem;align-items:center;justify-content:center;margin-top:1rem">
      {{if gt .Page 1}}
      <a href="/admin/logs?page={{sub .Page 1}}" class="btn btn--ghost btn--mini">&larr; Newer</a>
      {{end}}
      <span style="font-size:.75rem;color:var(--text-faint)">Page {{.Page}} / {{.TotalPages}}</span>
      {{if lt .Page .TotalPages}}
      <a href="/admin/logs?page={{add .Page 1}}" class="btn btn--ghost btn--mini">Older &rarr;</a>
      {{end}}
    </div>
    {{end}}
  </main>
</body>
</html>
`
