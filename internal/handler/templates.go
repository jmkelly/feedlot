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
)
