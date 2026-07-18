# 🐄 Feedlot

> *A self-hosted RSS/Atom feed reader with AI-powered summarization.*

**Feedlot** subscribes to your favourite blogs, fetches full articles, and runs them through an LLM to generate short, digestible summaries. Scan dozens of posts in seconds — without ever leaving the trough.

<p align="center">
  <img src="screenshots/03-dashboard.png" alt="Feedlot dashboard" width="800" />
</p>

---

## ✨ Features

- **📡 RSS & Atom feeds** — Subscribe to any feed. Feedlot fetches full article content on ingest and keeps polling in the background.
- **🤖 AI summarization** — Articles are passed through your choice of LLM (OpenAI, Anthropic, Ollama, or any OpenAI-compatible endpoint) for concise, readable summaries.
- **✅ Read/unread tracking** — One-click toggling with zero page reloads via HTMX. Mark individual articles or bulk-toggle at the feed level.
- **🌗 Dark mode** — System-aware dark/light theme toggle.
- **📱 Mobile responsive** — Full UI works on phones, tablets, and desktops.
- **📂 OPML import** — Bring your existing subscriptions over in one click.
- **🔐 Simple auth** — Email + password login keeps your feeds private.
- **⚙️ Configurable AI** — Choose provider, model, base URL, summary length, and language from the Settings page.
- **🧪 Test connection** — Verify your AI provider credentials right from Settings.
- **🗄️ Admin logs** — Built-in log viewer for debugging feed fetches and AI calls.
- **🐳 Single binary** — Compiles to a ~19 MB static binary. Deploy anywhere.

---

## 📸 Screenshots

| | | |
|:---:|:---:|:---:|
| ![Login](screenshots/01-login.png) | ![Register](screenshots/02-register.png) | ![Dashboard](screenshots/03-dashboard.png) |
| **Login** | **Register** | **Dashboard** |
| ![Summarizing](screenshots/04-summarizing.png) | ![Settings](screenshots/05-settings.png) | ![Test Connection](screenshots/06-test-connection.png) |
| **AI Summarizing** | **Settings** | **Test Connection** |
| ![Dark Mode](screenshots/07-dark-mode.png) | ![Feed Filter](screenshots/08-feed-filter.png) | ![Mobile](screenshots/09-mobile.png) |
| **Dark Mode** | **Feed Filter** | **Mobile** |
| ![Mobile Articles](screenshots/10-mobile-articles.png) | ![Admin Logs](screenshots/11-admin-logs.png) | |
| **Mobile Articles** | **Admin Logs** | |

---

## 🧱 Tech Stack

| Layer | Choice |
|-------|--------|
| Language | **Go 1.25** |
| Router | [Chi](https://github.com/go-chi/chi) |
| Database | **SQLite** (via [mattn/go-sqlite3](https://github.com/mattn/go-sqlite3) + [sqlx](https://github.com/jmoiron/sqlx)) |
| Frontend | **HTML templates** + [HTMX](https://htmx.org/) + custom CSS |
| Auth | bcrypt passwords + JWT sessions |
| Feed parsing | [gofeed](https://github.com/mmcdole/gofeed) |
| AI calls | `net/http` — works with any OpenAI-compatible API |

---

## 🚀 Getting Started

### Prerequisites

- **Go 1.25+**
- **SQLite** (no separate install needed — uses CGo SQLite driver)

### Quick start

```bash
# Clone the repo
git clone https://github.com/james/feedlot.git
cd feedlot

# Build
go build -o feedlot .

# Run (generates random secrets for you)
FEEDLOT_JWT_SECRET="$(openssl rand -hex 32)" \
  FEEDLOT_ENCRYPTION_KEY="$(openssl rand -hex 32)" \
  ./feedlot
```

The server starts on **port 8080** by default. Open [http://localhost:8080](http://localhost:8080) and register an account.

### Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `FEEDLOT_PORT` | No | `8080` | HTTP listen port |
| `FEEDLOT_DB_PATH` | No | `./feedlot.db` | SQLite database path |
| `FEEDLOT_JWT_SECRET` | **Yes** | — | Secret for signing JWT tokens |
| `FEEDLOT_ENCRYPTION_KEY` | No | — | 32-byte hex key for encrypting API keys at rest |
| `FEEDLOT_REFRESH_INTERVAL` | No | `30m` | How often to poll feeds in background |
| `FEEDLOT_OPENCODE_GO_KEY` | No | — | Default API key for the built-in OpenCode Go provider (set in `.env`) |

Use a `.env` file in the project root to avoid typing these every time:

```bash
FEEDLOT_JWT_SECRET=<your random secret>
FEEDLOT_ENCRYPTION_KEY=<your random encryption key>
FEEDLOT_PORT=8090
```

### Test account

If the database ships with a test user, sign in with:

| Field | Value |
|-------|-------|
| Email | `test@example.com` |
| Password | `feedlot123` |

Otherwise, just **register** from the login page — it takes two seconds.

---

## 🧠 How summarization works

1. Feedlot fetches the RSS/Atom feed and stores the full article content.
2. A background goroutine picks up un-summarized articles and sends the text to your configured LLM.
3. The LLM returns a short summary, stored alongside the article.
4. The dashboard shows the summary inline — no need to open the full article.

You can also trigger summarization manually per-article with the **"Summarize"** button, or bulk-re-summarize.

### Supported AI providers

Any OpenAI-compatible endpoint works. Tested with:

- **OpenAI** (GPT-4o, GPT-4o-mini)
- **Anthropic** (via OpenAI-compatible proxies)
- **Ollama** (local, e.g. `http://localhost:11434/v1`)
- **OpenCode Go** (built-in default)
- Any other provider exposing a `/v1/chat/completions` endpoint

Configure provider, API key (encrypted at rest), model, base URL, summary length, and summary language in **Settings**.

---

## 📁 Project structure

```
feedlot/
├── main.go                  # Entry point, router, migrations
├── internal/
│   ├── ai/                  # LLM summarization client
│   ├── auth/                # JWT + bcrypt auth
│   ├── db/                  # SQLite database layer
│   ├── feeds/               # RSS/Atom fetcher & background refresher
│   ├── handler/             # HTTP handlers & HTML templates
│   └── model/               # Data models & user settings
├── static/
│   ├── css/app.css          # Application styles (light + dark)
│   ├── js/app.js            # Minimal vanilla JS
│   └── favicon.svg          # Cow favicon
├── migrations/              # SQL migration files
└── screenshots/             # Screenshots for docs
```

---

## 📄 License

MIT — do whatever you want. Feed your brain.
