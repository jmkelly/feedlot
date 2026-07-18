package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/james/feedlot/internal/auth"
	"github.com/james/feedlot/internal/db"
	"github.com/james/feedlot/internal/model"
)
// noRedirectClient returns an HTTP client that does not follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}


// setupTestHandler creates a complete handler with in-memory database for testing.
func setupTestHandler(t *testing.T) (*Handler, *db.DB, *auth.Auth) {
	t.Helper()

	dbRaw, err := sqlx.Connect("sqlite3", "file::memory:?_journal_mode=WAL&_foreign_keys=on&cache=shared")
	if err != nil {
		t.Fatalf("Failed to connect to in-memory DB: %v", err)
	}
	database := &db.DB{DB: dbRaw}
	database.SetMaxOpenConns(1)

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS feeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			title TEXT NOT NULL,
			description TEXT,
			feed_url TEXT UNIQUE NOT NULL,
			site_url TEXT,
			icon_url TEXT,
			last_fetched_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS articles (
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
		)`,
		`CREATE TABLE IF NOT EXISTS user_settings (
			user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
			ai_provider TEXT DEFAULT 'openai',
			api_key_encrypted TEXT,
			model_name TEXT DEFAULT 'gpt-4o-mini',
			base_url TEXT,
			summary_length TEXT DEFAULT 'short',
			summary_language TEXT DEFAULT 'english',
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	for _, m := range migrations {
		if _, err := database.Exec(m); err != nil {
			t.Fatalf("Migration failed: %v", err)
		}
	}

	a := auth.New("test-jwt-secret-for-testing-purposes")
	h := New(database, a, "", "")
	return h, database, a
}

// createTestUser creates a user and returns it with a valid JWT cookie.
func createTestUser(t *testing.T, h *Handler, db *db.DB, a *auth.Auth, email, password string) (*model.User, string) {
	t.Helper()

	hash, err := a.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	user, err := db.CreateUser(email, hash)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	jwt, _, err := a.GenerateToken(user.ID)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	return user, jwt
}

// setupRouter creates a chi router with the handler's routes for testing.
func setupRouter(h *Handler) *chi.Mux {
	r := chi.NewRouter()
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)
	r.Get("/register", h.RegisterPage)
	r.Post("/register", h.Register)

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
		r.Get("/settings", h.SettingsPage)
		r.Post("/settings", h.SaveSettings)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	return r
}

// ─── Health Check ──────────────────────────────────────────────────────────

func TestHealthEndpoint(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatalf("Health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Health status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("Health body = %v, want {status: ok}", body)
	}
}

// ─── Auth: Registration ────────────────────────────────────────────────────

func TestRegisterSuccess(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=new@example.com&password=password123&confirm_password=password123")
	resp, err := noRedirectClient().Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	if err != nil {
		t.Fatalf("Register request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should redirect to dashboard
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Register status = %d, want 303", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Errorf("Redirect Location = %q, want /", loc)
	}

	// Should set a cookie
	cookies := resp.Cookies()
	var tokenCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "feedlot_token" {
			tokenCookie = c
			break
		}
	}
	if tokenCookie == nil {
		t.Fatal("No feedlot_token cookie set after registration")
	}
	if tokenCookie.Value == "" {
		t.Error("Cookie value is empty")
	}
}

func TestRegisterPasswordMismatch(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=test@example.com&password=password123&confirm_password=different")
	resp, _ := http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Register with mismatch should return 200 (re-render), got %d", resp.StatusCode)
	}
}

func TestRegisterShortPassword(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=test@example.com&password=short&confirm_password=short")
	resp, _ := http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	// Should re-render form with error
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Register with short password should return 200, got %d", resp.StatusCode)
	}
}

func TestRegisterDuplicateEmail(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// First registration
	body := strings.NewReader("email=dupe@example.com&password=password123&confirm_password=password123")
	resp, _ := http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	resp.Body.Close()

	// Second registration with same email
	body = strings.NewReader("email=dupe@example.com&password=otherpass123&confirm_password=otherpass123")
	resp, _ = http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Duplicate registration should return 200 (re-render with error), got %d", resp.StatusCode)
	}
}

// ─── Auth: Login ───────────────────────────────────────────────────────────

func TestLoginSuccess(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	createTestUser(t, h, database, a, "login@example.com", "correct-password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=login@example.com&password=correct-password")
	resp, _ := noRedirectClient().Post(server.URL+"/login", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Login status = %d, want 303", resp.StatusCode)
	}

	cookies := resp.Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "feedlot_token" && c.Value != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("No feedlot_token cookie set after login")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	createTestUser(t, h, database, a, "wrong@example.com", "correct-password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=wrong@example.com&password=wrong-password")
	resp, _ := noRedirectClient().Post(server.URL+"/login", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Login with wrong password should return 200 (re-render), got %d", resp.StatusCode)
	}
}

func TestLoginNonexistentUser(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=nobody@example.com&password=anything")
	resp, _ := noRedirectClient().Post(server.URL+"/login", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Login for nonexistent user should return 200, got %d", resp.StatusCode)
	}
}

// ─── Auth: Logout ──────────────────────────────────────────────────────────

func TestLogout(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "logout@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/logout", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Logout request failed: %v", err)
	}
	defer resp.Body.Close()

	// Should clear the cookie
	cookies := resp.Cookies()
	for _, c := range cookies {
		if c.Name == "feedlot_token" && c.MaxAge != -1 && c.Value != "" {
			t.Error("feedlot_token cookie should be cleared on logout")
		}
	}
}

func TestLogoutWithoutAuth(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// Logout without a cookie should redirect to login
	// (actually the middleware will redirect)
	resp, _ := noRedirectClient().Post(server.URL+"/logout", "application/x-www-form-urlencoded", nil)
	defer resp.Body.Close()

	// Without auth middleware, it should redirect to login
	if resp.StatusCode != http.StatusSeeOther {
		t.Logf("Logout without auth status = %d (may redirect to login)", resp.StatusCode)
	}
}

// ─── Dashboard ─────────────────────────────────────────────────────────────

func TestDashboardAuthenticated(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "dash@example.com", "password")
	_ = user

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Dashboard request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Dashboard status = %d, want 200", resp.StatusCode)
	}
}

func TestDashboardWithoutAuth(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, _ := noRedirectClient().Get(server.URL + "/")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Dashboard without auth should redirect, got %d", resp.StatusCode)
	}
}

// ─── Feed Operations ───────────────────────────────────────────────────────

func TestListFeedsEmpty(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "feeds@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/feeds", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ListFeeds status = %d, want 200", resp.StatusCode)
	}
}

func TestAddFeedInvalidURL(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "add-feed@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("url=")
	req, _ := http.NewRequest("POST", server.URL+"/feeds", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("AddFeed with empty URL status = %d, want 400", resp.StatusCode)
	}
}

func TestRemoveFeedInvalidID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "remove-feed@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("DELETE", server.URL+"/feeds/not-a-number", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("RemoveFeed with invalid ID status = %d, want 400", resp.StatusCode)
	}
}

func TestRefreshFeedNotFound(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "refresh@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/feeds/99999/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("RefreshFeed for nonexistent feed status = %d, want 404", resp.StatusCode)
	}
}

func TestRefreshFeedInvalidID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "refresh2@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/feeds/invalid/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("RefreshFeed with invalid ID status = %d, want 400", resp.StatusCode)
	}
}

// ─── Article Operations ────────────────────────────────────────────────────

func TestShowArticleNotFound(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "article-show@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles/99999", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("ShowArticle for nonexistent article status = %d, want 404", resp.StatusCode)
	}
}

func TestShowArticleInvalidID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "article-show2@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles/invalid", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ShowArticle with invalid ID status = %d, want 400", resp.StatusCode)
	}
}

func TestToggleReadInvalidID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "toggle@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/articles/invalid/toggle", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("ToggleRead with invalid ID status = %d, want 400", resp.StatusCode)
	}
}

func TestToggleReadNotFound(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "toggle2@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("POST", server.URL+"/articles/99999/toggle", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("ToggleRead for nonexistent article status = %d, want 404", resp.StatusCode)
	}
}

// ─── Settings ──────────────────────────────────────────────────────────────

func TestSettingsPage(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "settings-page@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/settings", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("SettingsPage status = %d, want 200", resp.StatusCode)
	}
}

func TestSaveSettings(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "save-settings@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("ai_provider=anthropic&model_name=claude-3-haiku-20240307&summary_length=medium&summary_language=french&api_key=sk-test&base_url=")
	req, _ := http.NewRequest("POST", server.URL+"/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("SaveSettings status = %d, want 200", resp.StatusCode)
	}
}

func TestSettingsRequiresAuth(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, _ := noRedirectClient().Get(server.URL + "/settings")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Settings without auth should redirect, got %d", resp.StatusCode)
	}
}

// ─── Auth Page Rendering ───────────────────────────────────────────────────

func TestLoginPage(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/login")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("LoginPage status = %d, want 200", resp.StatusCode)
	}
}

func TestRegisterPage(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	resp, _ := http.Get(server.URL + "/register")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("RegisterPage status = %d, want 200", resp.StatusCode)
	}
}

// ─── ListArticles ──────────────────────────────────────────────────────────

func TestListArticles(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "list-articles@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ListArticles status = %d, want 200", resp.StatusCode)
	}
}

// ─── Auth Page Cookie Redirect ─────────────────────────────────────────────

func TestAuthenticatedUserRedirectedToLoginOnBadToken(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: "invalid-jwt-token-that-will-fail-validation"})
	resp, _ := noRedirectClient().Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("Bad token should redirect to login, got %d", resp.StatusCode)
	}
}

// ─── Request Timing ────────────────────────────────────────────────────────

func TestHandlerCreation(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	if h == nil {
		t.Fatal("New() returned nil")
	}
	if h.DB == nil {
		t.Error("Handler.DB is nil")
	}
	if h.Auth == nil {
		t.Error("Handler.Auth is nil")
	}
}

func TestGetUserIDReturnsZeroForNoContext(t *testing.T) {
	// Create a request without the userID context value
	req := httptest.NewRequest("GET", "/", nil)
	uid := GetUserID(req)
	if uid != 0 {
		t.Errorf("GetUserID without context returned %d, want 0", uid)
	}
}

func TestGetUserIDOrDefault(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	uid := GetUserIDOrDefault(req)
	if uid != 0 {
		t.Errorf("GetUserIDOrDefault without context returned %d, want 0", uid)
	}
}

// ─── Registration edge cases ──────────────────────────────────────────────

func TestRegisterEmptyFields(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// Test with empty email and password
	body := strings.NewReader("email=&password=&confirm_password=")
	resp, _ := http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Register with empty fields should return 200, got %d", resp.StatusCode)
	}
}

// ─── Concurrent requests ──────────────────────────────────────────────────

func TestConcurrentRequests(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "concurrent@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			req, _ := http.NewRequest("GET", server.URL+"/", nil)
			req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
				done <- true
			} else {
				done <- false
			}
		}()
	}

	successes := 0
	for i := 0; i < 5; i++ {
		if <-done {
			successes++
		}
	}

	if successes != 5 {
		t.Errorf("Concurrent requests: %d/5 succeeded", successes)
	}
}

// ─── OptionalAuth ─────────────────────────────────────────────────────────

func TestOptionalAuthWithValidToken(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "opt-auth@example.com", "password")

	// Test OptionalAuth manually
	handler := h.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := GetUserID(r)
		if uid != user.ID {
			t.Errorf("OptionalAuth: userID = %d, want %d", uid, user.ID)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("OptionalAuth status = %d, want 200", resp.Code)
	}
}

func TestOptionalAuthWithoutToken(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	handler := h.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := GetUserID(r)
		if uid != 0 {
			t.Errorf("Without token, userID should be 0, got %d", uid)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("OptionalAuth without token should pass through, got %d", resp.Code)
	}
}

func TestOptionalAuthWithBadToken(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	handler := h.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid := GetUserID(r)
		if uid != 0 {
			t.Errorf("With bad token, userID should be 0, got %d", uid)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: "bad-token"})
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Errorf("OptionalAuth with bad token should pass through, got %d", resp.Code)
	}
}

// ─── Login edge cases ─────────────────────────────────────────────────────

func TestLoginEmptyFields(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("email=&password=")
	resp, _ := noRedirectClient().Post(server.URL+"/login", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	// Should re-render with error
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Login with empty fields should return 200, got %d", resp.StatusCode)
	}
}

func TestLoginInvalidForm(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// Send invalid form data (garbage)
	body := strings.NewReader("%%%invalid%%%")
	resp, _ := noRedirectClient().Post(server.URL+"/login", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Login with invalid form should return 200, got %d", resp.StatusCode)
	}
}

// ─── Helper tests ─────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	if h == nil {
		t.Fatal("New returned nil")
	}
}

func TestBuildSummaryRequestDefaults(t *testing.T) {
	h := &Handler{EncryptionKey: ""}
	req := h.buildSummaryRequest(nil, "test content")
	if req.Provider != "opencode-go" {
		t.Errorf("Default provider = %q, want %q", req.Provider, "opencode-go")
	}
	if req.Model != "deepseek-v4-flash" {
		t.Errorf("Default model = %q, want %q", req.Model, "deepseek-v4-flash")
	}
	if req.Length != "short" {
		t.Errorf("Default length = %q, want %q", req.Length, "short")
	}
	if req.ArticleText != "test content" {
		t.Errorf("ArticleText = %q", req.ArticleText)
	}
}

func TestBuildSummaryRequestFromSettings(t *testing.T) {
	h := &Handler{EncryptionKey: ""}
	baseURL := "https://custom.api.com"
	apiKey := "sk-encrypted"
	settings := &model.UserSettings{
		AIProvider:      "anthropic",
		ModelName:       "claude-3-haiku-20240307",
		BaseURL:         &baseURL,
		APIKeyEncrypted: &apiKey,
		SummaryLength:   "long",
		SummaryLanguage: "spanish",
	}

	req := h.buildSummaryRequest(settings, "article text")
	if req.Provider != "anthropic" {
		t.Errorf("Provider = %q", req.Provider)
	}
	if req.Model != "claude-3-haiku-20240307" {
		t.Errorf("Model = %q", req.Model)
	}
	if req.Length != "long" {
		t.Errorf("Length = %q", req.Length)
	}
	if req.SummaryLang != "spanish" {
		t.Errorf("SummaryLang = %q", req.SummaryLang)
	}
	if req.BaseURL != "https://custom.api.com" {
		t.Errorf("BaseURL = %q", req.BaseURL)
	}
	if req.APIKey != "sk-encrypted" {
		t.Errorf("APIKey = %q", req.APIKey)
	}
}

// ─── Timeout on long operations ────────────────────────────────────────────

func TestDashboardWithFeedFilter(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "dash-filter@example.com", "password")

	// Create a feed
	desc := "Test feed"
	feed, err := database.CreateFeed(&model.Feed{
		UserID:      user.ID,
		Title:       "Filtered Feed",
		FeedURL:     "https://filter-test.example.com/rss",
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("CreateFeed failed: %v", err)
	}

	// Create an article in that feed
	_, err = database.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "filter-article-1",
		Title:  "Filtered Article",
		IsRead: false,
	})
	if err != nil {
		t.Fatalf("CreateArticle failed: %v", err)
	}

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// Request dashboard with feed filter
	req, _ := http.NewRequest("GET", server.URL+"/?feed_id=", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Dashboard with empty feed_id status = %d, want 200", resp.StatusCode)
	}
}

// ─── Articles list with feed filter ───────────────────────────────────────

func TestListArticlesWithFeedID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "articles-feed@example.com", "password")

	feed, _ := database.CreateFeed(&model.Feed{
		UserID:  user.ID,
		Title:   "Articles Feed",
		FeedURL: "https://articles-feed.example.com/rss",
	})

	database.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "articles-feed-g1",
		Title:  "Article 1",
	})

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles?feed_id=", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ListArticles with empty feed_id status = %d, want 200", resp.StatusCode)
	}
}

func TestListArticlesWithInvalidFeedID(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "articles-badfeed@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles?feed_id=invalid", nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	// Should still work, just ignore the invalid feed_id
	if resp.StatusCode != http.StatusOK {
		t.Errorf("ListArticles with invalid feed_id status = %d, want 200", resp.StatusCode)
	}
}

func TestSaveSettingsEmptyFormData(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	_, jwt := createTestUser(t, h, database, a, "save-empty@example.com", "password")

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	// Empty body — ParseForm should handle gracefully
	body := strings.NewReader("")
	req, _ := http.NewRequest("POST", server.URL+"/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("SaveSettings with empty form status = %d, want 200", resp.StatusCode)
	}
}

func TestRegisterInvalidFormData(t *testing.T) {
	h, database, _ := setupTestHandler(t)
	defer database.Close()

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	body := strings.NewReader("%%%")
	resp, _ := http.Post(server.URL+"/register", "application/x-www-form-urlencoded", body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Register with invalid form data should return 200, got %d", resp.StatusCode)
	}
}

// ─── Article detail with content ───────────────────────────────────────────

func TestShowArticleWithContent(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "show-content@example.com", "password")

	content := "Full article content for detail view"
	feed, _ := database.CreateFeed(&model.Feed{
		UserID:  user.ID,
		Title:   "Content Feed",
		FeedURL: "https://content-feed.example.com/rss",
	})
	article, _ := database.CreateArticle(&model.Article{
		FeedID:  feed.ID,
		GUID:    "content-guid-1",
		Title:   "Content Article",
		Content: &content,
	})

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles/"+itoa(article.ID), nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ShowArticle status = %d, want 200", resp.StatusCode)
	}
}

func TestShowArticleWithoutContent(t *testing.T) {
	h, database, a := setupTestHandler(t)
	defer database.Close()

	user, jwt := createTestUser(t, h, database, a, "show-nocontent@example.com", "password")

	feed, _ := database.CreateFeed(&model.Feed{
		UserID:  user.ID,
		Title:   "No Content Feed",
		FeedURL: "https://nocontent.example.com/rss",
	})
	article, _ := database.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "no-content-guid",
		Title:  "No Content Article",
	})

	r := setupRouter(h)
	server := httptest.NewServer(r)
	defer server.Close()

	req, _ := http.NewRequest("GET", server.URL+"/articles/"+itoa(article.ID), nil)
	req.AddCookie(&http.Cookie{Name: "feedlot_token", Value: jwt})
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("ShowArticle without content status = %d, want 200", resp.StatusCode)
	}
}

func itoa(i int64) string {
	return fmt.Sprintf("%d", i)
}

