#!/usr/bin/env python3
"""Transform Feedlot inline template constants — visual redesign."""

import re, sys

# Which template types to process
TEMPLATES = ['dashboard', 'feedList', 'articleList', 'articleCard', 'articleDetail', 'authPage', 'settingsPage']

def find_const_end(text, start):
    """Find closing backtick of a Go raw string literal starting at `start`."""
    # The opening \` is at start. Find the next \` on its own line.
    idx = start + 1
    while idx < len(text):
        if text[idx] == '`':
            # Backtick found — this is the closer
            return idx + 1
        idx += 1
    return -1

def replace_template(filepath, name, new_body):
    """Replace const `name`Template = \`...\` in file."""
    with open(filepath, 'r') as f:
        text = f.read()

    pattern = f'const {name}Template = `'
    start = text.find(pattern)
    if start == -1:
        print(f"  [SKIP] {name}Template not found in {filepath}")
        return False

    # Find opening backtick (right after = and optional space)
    needle = '= `'
    eq_pos = text.find(needle, start)
    if eq_pos == -1:
        print(f"  [SKIP] {name}Template const format unexpected")
        return False
    
    open_tick = eq_pos + len(needle) - 1  # point to the \`
    
    # Find closing backtick
    end = find_const_end(text, open_tick)
    if end == -1:
        print(f"  [SKIP] {name}Template closing backtick not found")
        return False
    
    # Build replacement
    replacement = f'= `\n{new_body}\n`'
    
    # Replace from `= \`` through the closing `` ` ``
    old = text[eq_pos:end]
    text = text[:eq_pos] + replacement + text[end:]
    
    with open(filepath, 'w') as f:
        f.write(text)
    
    print(f"  [OK] {name}Template replaced in {filepath}")
    return True

# ===== NEW TEMPLATE BODIES =====
# These are the HTML content BETWEEN the opening and closing backticks.
# They must NOT contain backticks themselves (Go raw string constraint).

NEW_DASHBOARD = '''<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Dashboard</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
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
      <button class="chip" id="scroll-read-toggle" aria-pressed="true" title="Mark articles read as you scroll past them">
        <span class="chip__dot"></span> Auto-read
      </button>
      <button class="btn btn--ghost" id="mark-all-read" title="Mark all visible articles read">Mark read</button>
      <a href="/settings" class="btn btn--ghost">Settings</a>
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
               hx-indicator="#loading">
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
</html>'''

NEW_FEEDLIST = '''<div class="panel">
  <div class="panel__head"><span class="panel__title"><b>≡</b> Pen</span></div>
  {{range .Feeds}}
  <div class="feed" data-feed-id="{{.ID}}">
    <a href="/?feed_id={{.ID}}" class="feed__row"
       hx-get="/?feed_id={{.ID}}" hx-target="#article-list" hx-push-url="true"
       hx-indicator="#loading">
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
</div>'''

NEW_ARTICLELIST = '''{{range .Articles}}
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
{{end}}'''

NEW_ARTICLECARD = '''<div class="card{{if not .Article.IsRead}} is-unread{{end}}{{if .Article.IsRead}} is-read{{end}}" data-article-id="{{.Article.ID}}" data-feed-id="{{.Article.FeedID}}">
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
</div>'''

NEW_ARTICLEDETAIL = '''<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{{.Article.Title}} — Feedlot</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
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
</html>'''

NEW_AUTHPAGE = '''<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — {{if eq .Page "login"}}Sign in{{else}}Register{{end}}</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="authwrap">
  <div class="authcard">
    <div class="authhero">
      <div class="authhero__mark">🐄</div>
      <h1 class="authhero__name">Feedlot</h1>
      <p class="authhero__sub">Chew through your feeds, one article at a time.</p>
    </div>
    <div class="authform">
      {{if .Error}}<div class="alert alert--err">{{.Error}}</div>{{end}}
      {{if eq .Page "login"}}
      <form hx-post="/login" hx-target="body" hx-swap="outerHTML">
        <h2>Sign in</h2>
        <div class="field">
          <label for="email">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required class="input" autofocus>
        </div>
        <div class="field">
          <label for="password">Password</label>
          <input type="password" id="password" name="password" required class="input">
        </div>
        <button type="submit" class="btn btn--primary w-full">Sign in</button>
        <p class="authswitch">Don't have an account? <a href="/register">Register</a></p>
      </form>
      {{else}}
      <form hx-post="/register" hx-target="body" hx-swap="outerHTML">
        <h2>Create account</h2>
        <div class="field">
          <label for="email">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required class="input" autofocus>
        </div>
        <div class="field">
          <label for="password">Password</label>
          <input type="password" id="password" name="password" required minlength="8" class="input">
        </div>
        <div class="field">
          <label for="confirm_password">Confirm password</label>
          <input type="password" id="confirm_password" name="confirm_password" required minlength="8" class="input">
        </div>
        <button type="submit" class="btn btn--primary w-full">Create account</button>
        <p class="authswitch">Already have an account? <a href="/login">Sign in</a></p>
      </form>
      {{end}}
    </div>
  </div>
</body>
</html>'''

NEW_SETTINGSPAGE = '''<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — Settings</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
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
      <a href="/" class="btn btn--ghost">&larr; Dashboard</a>
    </div>
  </nav>

  <main style="max-width:44rem;margin:0 auto;padding:2rem 1.1rem">
    <h2 style="font-family:var(--font-display);font-size:1.4rem;font-weight:700;margin:0 0 1rem">Settings</h2>

    {{if .SaveError}}<div class="alert alert--err">{{.SaveError}}</div>{{end}}
    {{if .SaveSuccess}}<div class="alert alert--ok">{{.SaveSuccess}}</div>{{end}}

    <form action="/settings" method="POST" class="panel" style="padding:1.4rem">
      <div class="field">
        <label for="ai_provider">AI Provider</label>
        <select id="ai_provider" name="ai_provider" class="select">
          <option value="openai" {{if eq .Settings.ai_provider "openai"}}selected{{end}}>OpenAI</option>
          <option value="anthropic" {{if eq .Settings.ai_provider "anthropic"}}selected{{end}}>Anthropic</option>
          <option value="ollama" {{if eq .Settings.ai_provider "ollama"}}selected{{end}}>Ollama (local)</option>
          <option value="custom" {{if eq .Settings.ai_provider "custom"}}selected{{end}}>Custom (OpenAI-compatible)</option>
        </select>
      </div>

      <div class="field">
        <label for="api_key">API Key</label>
        <input type="password" id="api_key" name="api_key" placeholder="sk-..." value="" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">Leave blank to keep existing key</p>
      </div>

      <div class="field">
        <label for="model_name">Model Name</label>
        <input type="text" id="model_name" name="model_name" value="{{.Settings.model_name}}" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">e.g. gpt-4o-mini, claude-3-haiku, llama3</p>
      </div>

      <div class="field">
        <label for="base_url">Base URL (optional)</label>
        <input type="url" id="base_url" name="base_url" value="{{.Settings.base_url}}" placeholder="https://api.openai.com/v1" class="input">
        <p style="font-size:.7rem;color:var(--text-faint);margin-top:.2rem">For custom / self-hosted endpoints</p>
      </div>

      <div class="field">
        <label for="summary_length">Summary Length</label>
        <select id="summary_length" name="summary_length" class="select">
          <option value="short" {{if eq .Settings.summary_length "short"}}selected{{end}}>Short (1-2 sentences)</option>
          <option value="medium" {{if eq .Settings.summary_length "medium"}}selected{{end}}>Medium (short paragraph)</option>
          <option value="long" {{if eq .Settings.summary_length "long"}}selected{{end}}>Long (detailed)</option>
        </select>
      </div>

      <div class="field">
        <label for="summary_language">Summary Language</label>
        <select id="summary_language" name="summary_language" class="select">
          <option value="english" {{if eq .Settings.summary_language "english"}}selected{{end}}>English</option>
          <option value="original" {{if eq .Settings.summary_language "original"}}selected{{end}}>Same as article</option>
          <option value="spanish" {{if eq .Settings.summary_language "spanish"}}selected{{end}}>Spanish</option>
          <option value="french" {{if eq .Settings.summary_language "french"}}selected{{end}}>French</option>
          <option value="german" {{if eq .Settings.summary_language "german"}}selected{{end}}>German</option>
          <option value="japanese" {{if eq .Settings.summary_language "japanese"}}selected{{end}}>Japanese</option>
        </select>
      </div>

      <div style="margin-top:.5rem">
        <button type="submit" class="btn btn--primary">Save Settings</button>
      </div>
    </form>
  </main>
</body>
</html>'''

# ===== RUN =====

files = {
    'internal/handler/feeds.go': [
        ('dashboard', NEW_DASHBOARD),
        ('feedList', NEW_FEEDLIST),
        ('articleList', NEW_ARTICLELIST),
    ],
    'internal/handler/articles.go': [
        ('articleCard', NEW_ARTICLECARD),
        ('articleDetail', NEW_ARTICLEDETAIL),
    ],
    'internal/handler/auth.go': [
        ('authPage', NEW_AUTHPAGE),
    ],
    'internal/handler/settings.go': [
        ('settingsPage', NEW_SETTINGSPAGE),
    ],
}

for filepath, templates in files.items():
    print(f"\nProcessing {filepath}:")
    for name, body in templates:
        replace_template(filepath, name, body)

print("\nDone. Templates updated.")
