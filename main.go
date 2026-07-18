package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"github.com/james/feedlot/internal/auth"
	"github.com/james/feedlot/internal/db"
	"github.com/james/feedlot/internal/handler"
)

func main() {
	// ─── Load .env ─────────────────────────────────────────────────────────
	_ = godotenv.Load()

	// ─── Configuration ─────────────────────────────────────────────────────
	port := getEnv("FEEDLOT_PORT", "8080")
	dbPath := getEnv("FEEDLOT_DB_PATH", "./feedlot.db")
	jwtSecret := getEnv("FEEDLOT_JWT_SECRET", "")
	encryptionKey := getEnv("FEEDLOT_ENCRYPTION_KEY", "")
	opencodeGoKey := getEnv("FEEDLOT_OPENCODE_GO_KEY", "")

	if jwtSecret == "" {
		log.Fatal("FEEDLOT_JWT_SECRET environment variable is required")
	}

	// ─── Database ──────────────────────────────────────────────────────────
	log.Printf("Opening database at %s", dbPath)
	database, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// Run migrations
	migrations := []string{
		migration001,
		migration002,
		migration003,
	}
	if err := database.Migrate(migrations); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Database migrations complete")

	// Redirect all log output to both stdout and the database
	db.NewLogWriter(database)

	// ─── Dependencies ──────────────────────────────────────────────────────
	authService := auth.New(jwtSecret)
	h := handler.New(database, authService, encryptionKey, opencodeGoKey)

	// ─── Router ────────────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(60 * time.Second))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "HX-Request", "HX-Current-URL", "HX-Target", "HX-Trigger"},
		ExposedHeaders:   []string{"HX-Redirect"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Static files
	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// ─── Routes ────────────────────────────────────────────────────────────

	// Auth (public)
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)
	r.Get("/register", h.RegisterPage)
	r.Post("/register", h.Register)

	// Auth (authenticated)
	r.Group(func(r chi.Router) {
		r.Use(h.RequireAuth)

		r.Post("/logout", h.Logout)
		r.Get("/", h.Dashboard)
		r.Get("/feeds", h.ListFeeds)
		r.Post("/feeds", h.AddFeed)
		r.Post("/feeds/import", h.ImportOPML)
		r.Delete("/feeds/{id}", h.RemoveFeed)
		r.Post("/feeds/{id}/refresh", h.RefreshFeed)
		r.Get("/articles", h.ListArticles)
		r.Get("/articles/{id}", h.ShowArticle)
		r.Post("/articles/{id}/toggle", h.ToggleRead)
		r.Post("/articles/{id}/read", h.MarkRead)
		r.Get("/settings", h.SettingsPage)
		r.Post("/settings", h.SaveSettings)
		r.Post("/settings/test", h.TestSettings)
		r.Post("/settings/models", h.ListModelsHandler)
		r.Get("/admin/logs", h.AdminLogs)
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// ─── Start Server ─────────────────────────────────────────────────────
	addr := ":" + port
	log.Printf("Feedlot starting on %s", addr)
	log.Printf("Dashboard: http://localhost%s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 120 * time.Second, // Allow long AI summarization requests
		IdleTimeout:  60 * time.Second,
	}

	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

const migration001 = `CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token TEXT UNIQUE NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS feeds (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    feed_url TEXT UNIQUE NOT NULL,
    site_url TEXT,
    icon_url TEXT,
    last_fetched_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS articles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    feed_id INTEGER NOT NULL REFERENCES feeds(id) ON DELETE CASCADE,
    guid TEXT UNIQUE NOT NULL,
    title TEXT NOT NULL,
    url TEXT,
    author TEXT,
    content TEXT,
    summary TEXT,
    published_at DATETIME,
    is_read BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_articles_feed_id ON articles(feed_id);
CREATE INDEX IF NOT EXISTS idx_articles_is_read ON articles(is_read);
CREATE INDEX IF NOT EXISTS idx_articles_guid ON articles(guid);
CREATE INDEX IF NOT EXISTS idx_feeds_user_id ON feeds(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);`

const migration003 = `CREATE TABLE IF NOT EXISTS log_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    level TEXT NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const migration002 = `CREATE TABLE IF NOT EXISTS user_settings (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ai_provider TEXT DEFAULT 'opencode-go',
    api_key_encrypted TEXT,
    model_name TEXT DEFAULT 'deepseek-v4-flash',
    base_url TEXT,
    summary_length TEXT DEFAULT 'short',
    summary_language TEXT DEFAULT 'english',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`
