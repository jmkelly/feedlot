package handler

import "html/template"

// Pre-parsed templates — parsed once at package init instead of on every request.
var (
	authTmpl = template.Must(template.New("auth").Parse(authPageTemplate))

	dashboardTmpl = template.Must(template.New("dashboard").Funcs(template.FuncMap{
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

	feedListTmpl = template.Must(template.New("feed-list").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(feedListTemplate))

	feedSidebarOOBTmpl = template.Must(template.New("feed-sidebar-oob").Funcs(template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
	}).Parse(feedSidebarOOBTemplate))

	articleListTmpl = template.Must(template.New("article-list").Funcs(template.FuncMap{
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
