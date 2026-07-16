# Feedlot 🐄📡

> *A blog feed reader that chews up articles and spits out summaries.*

Feedlot is a custom-built, self-hosted blog feed reader with AI-powered summarization. Subscribe to RSS/Atom feeds, and Feedlot will fetch full articles, run them through your choice of an OpenAI-compatible AI model, and serve you short, digestible summaries — so you can scan dozens of posts in seconds without leaving the trough.

## Concept

- **Feed subscription** — Subscribe to any RSS or Atom feed. Feedlot fetches and stores the full article content.
- **AI summarization** — On ingest, each article is passed through an LLM (OpenAI, Anthropic via API, local Ollama, or any OpenAI-compatible endpoint) to generate a concise, readable summary. Think RAG-light: the model gets the full article text and returns a title-grade summary.
- **Read/unread tracking** — Posts can be toggled between read and unread with minimal friction (one click, keypress, or swipe). Designed for rapid scanning.
- **Authentication** — Simple email + password login to keep your feed private.
- **Settings page** — Configure your AI provider, API key, model name, base URL, summary length, and other preferences.

## Why "Feedlot"?

Because it's a place where feeds get fattened up before they reach you. And because the name is a little weird, a little funny, and just relevant enough.

## Language / Framework Suggestions

Not sure what to build this in? Here are three solid options, each with a different trade-off:

### 1. Python + FastAPI + HTMX + SQLite

| Layer | Choice |
|-------|--------|
| Backend | FastAPI (Python) |
| Frontend | HTMX + Tailwind CSS (minimal JS) |
| Database | SQLite (via SQLAlchemy) |
| Auth | FastAPI users + JWT |
| Feed parsing | feedparser |
| AI calls | httpx / openai Python library |

**Best for:** Rapid prototyping, solo dev, low operational complexity. Python has the richest AI/LLM ecosystem (LangChain, LlamaIndex, direct OpenAI SDK). HTMX keeps the frontend simple without a JS framework — perfect for fast read/unread toggling with zero page reloads. SQLite means zero infrastructure.

**Trade-off:** Not the best for massive scale (tens of thousands of feeds) or real-time concurrency, but for a personal/private reader it's ideal.

---

### 2. Go + Templ + SQLite + LiteStream

| Layer | Choice |
|-------|--------|
| Backend | Go (Chi or Gin router) |
| Frontend | Templ (type-safe HTML templates) + HTMX + Tailwind |
| Database | SQLite (via modernc.org/sqlite or mattn/go-sqlite3) |
| Auth | bcrypt + JWT, or plug in something like PocketBase |
| Feed parsing | gofeed |
| AI calls | net/http + custom OpenAI-compatible client |

**Best for:** Single binary deployment, performance, and long-term maintainability. Go compiles to a tiny static binary — deploy anywhere with no dependencies. Templ gives you type-safe HTML templates with Go. HTMX keeps interactivity snappy.

**Trade-off:** Fewer AI libraries — you'll write more HTTP plumbing for LLM calls. Less ecosystem than Python, but the result is a rock-solid, fast, minimal-dependency app.

---

### 3. TypeScript + Next.js (or SvelteKit) + SQLite (via Turso/LibSQL)

| Layer | Choice |
|-------|--------|
| Backend | Next.js API routes or SvelteKit |
| Frontend | React (Next.js) or Svelte (SvelteKit) |
| Database | Turso (LibSQL, edge-compatible SQLite) or better-sqlite3 |
| Auth | NextAuth.js / Lucia Auth |
| Feed parsing | rss-parser |
| AI calls | Vercel AI SDK or openai npm package |

**Best for:** Full-stack in one language, modern DX, potential edge deployment. One language (TypeScript) for frontend and backend. Next.js can deploy to Vercel's edge network. SvelteKit is lighter and more performant for reactive UIs like read/unread toggling.

**Trade-off:** More JavaScript tooling complexity. Node.js background tasks (feed polling) require extra setup (cron jobs, or in-process with BullMQ). The AI SDK integration is nice, but you're tied to a heavier dependency tree.

---

### Summary

| | Python | Go | TypeScript |
|---|---|---|---|
| **Prototyping speed** | ⚡⚡⚡ | ⚡⚡ | ⚡⚡ |
| **Performance** | 🐌 | 🚀 | 🚀 (edge) |
| **Deployment ease** | ⚡ (Docker) | ⚡⚡⚡ (binary) | ⚡⚡ (Node) |
| **AI ecosystem** | 🏆 best | 🟢 decent | 🟢 good |
| **Single dependency?** | Needs Python runtime | Static binary 🏆 | Needs Node |
| **Read/unread UX** | HTMX does the heavy lifting | HTMX + Templ | Native reactive (Svelte) |

**My personal pick:** If you want to ship fast and have the richest AI tooling, go **Python + FastAPI + HTMX**. If you want a forever-binary you can deploy on a $5 VPS and forget about, go **Go + Templ + HTMX**. If you want a polished reactive UI and don't mind the JS toolchain, go **SvelteKit** (TypeScript).

## Getting Started

### Prerequisites
- Go 1.25+
- SQLite

### Run locally

```bash
go build -o feedlot .
FEEDLOT_JWT_SECRET="$(openssl rand -hex 32)" \
  FEEDLOT_ENCRYPTION_KEY="$(openssl rand -hex 32)" \
  ./feedlot
```

The server starts on port 8080 by default. Set `FEEDLOT_PORT` to change it.

### Test account

If the database already has a test user, you can sign in with:

| Field | Value |
|-------|-------|
| Email | `test@example.com` |
| Password | `feedlot123` |

To create this account on a fresh database, just register from the login page.

### Access from other devices

Feedlot listens on all interfaces (`0.0.0.0`), so you can reach it from other devices on your LAN at `http://<your-ip>:8080`. Find your IP with `hostname -I`.

---

*Feedlot — fattening your brain, one feed at a time.*
