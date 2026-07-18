package db

import (
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/james/feedlot/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()

	dbRaw, err := sqlx.Connect("sqlite3", "file::memory:?_journal_mode=WAL&_foreign_keys=on&cache=shared")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}
	db := &DB{dbRaw}
	db.SetMaxOpenConns(1)

	// Run migrations
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

	if err := db.Migrate(migrations); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	return db
}

func createTestUser(t *testing.T, db *DB, email, hash string) *model.User {
	t.Helper()
	u, err := db.CreateUser(email, hash)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	return u
}

func createTestFeed(t *testing.T, db *DB, userID int64, title, feedURL string) *model.Feed {
	t.Helper()
	f, err := db.CreateFeed(&model.Feed{
		UserID:  userID,
		Title:   title,
		FeedURL: feedURL,
	})
	if err != nil {
		t.Fatalf("CreateFeed failed: %v", err)
	}
	return f
}

// ─── User Tests ────────────────────────────────────────────────────────────

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	u, err := db.CreateUser("test@example.com", "hash123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if u.ID == 0 {
		t.Error("CreateUser returned user with ID 0")
	}
	if u.Email != "test@example.com" {
		t.Errorf("CreateUser returned email %q, want %q", u.Email, "test@example.com")
	}
	if u.PasswordHash != "hash123" {
		t.Errorf("CreateUser returned password_hash %q, want %q", u.PasswordHash, "hash123")
	}
	if u.CreatedAt.IsZero() {
		t.Error("CreateUser returned zero created_at")
	}
}

func TestCreateUserDuplicateEmail(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.CreateUser("dupe@example.com", "hash1")
	if err != nil {
		t.Fatalf("First CreateUser failed: %v", err)
	}

	_, err = db.CreateUser("dupe@example.com", "hash2")
	if err == nil {
		t.Error("CreateUser should fail for duplicate email")
	}
}

func TestGetUserByEmail(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	created := createTestUser(t, db, "findme@example.com", "hash")

	u, err := db.GetUserByEmail("findme@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if u.ID != created.ID {
		t.Errorf("GetUserByEmail returned ID %d, want %d", u.ID, created.ID)
	}
	if u.Email != "findme@example.com" {
		t.Errorf("GetUserByEmail returned email %q", u.Email)
	}
}

func TestGetUserByEmailNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.GetUserByEmail("nonexistent@example.com")
	if err == nil {
		t.Error("GetUserByEmail should fail for nonexistent email")
	}
}

func TestGetUserByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	created := createTestUser(t, db, "byid@example.com", "hash")

	u, err := db.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}
	if u.Email != "byid@example.com" {
		t.Errorf("GetUserByID returned email %q", u.Email)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.GetUserByID(99999)
	if err == nil {
		t.Error("GetUserByID should fail for nonexistent ID")
	}
}

// ─── Session Tests ─────────────────────────────────────────────────────────

func TestCreateSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "session@example.com", "hash")
	expires := time.Now().Add(24 * time.Hour)

	s, err := db.CreateSession(user.ID, "token-abc-123", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if s.ID == 0 {
		t.Error("CreateSession returned session with ID 0")
	}
	if s.UserID != user.ID {
		t.Errorf("CreateSession UserID = %d, want %d", s.UserID, user.ID)
	}
	if s.Token != "token-abc-123" {
		t.Errorf("CreateSession Token = %q", s.Token)
	}
}

func TestGetSessionByToken(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "session-get@example.com", "hash")
	expires := time.Now().Add(24 * time.Hour)
	_, err := db.CreateSession(user.ID, "valid-token", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	s, err := db.GetSessionByToken("valid-token")
	if err != nil {
		t.Fatalf("GetSessionByToken failed: %v", err)
	}
	if s.UserID != user.ID {
		t.Errorf("GetSessionByToken UserID = %d, want %d", s.UserID, user.ID)
	}
}

func TestGetSessionByTokenExpired(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "session-exp@example.com", "hash")
	expires := time.Now().Add(-24 * time.Hour) // in the past
	_, err := db.CreateSession(user.ID, "expired-token", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = db.GetSessionByToken("expired-token")
	if err == nil {
		t.Error("GetSessionByToken should fail for expired session")
	}
}

func TestGetSessionByTokenNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.GetSessionByToken("nonexistent-token")
	if err == nil {
		t.Error("GetSessionByToken should fail for nonexistent token")
	}
}

func TestDeleteSession(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "session-del@example.com", "hash")
	expires := time.Now().Add(24 * time.Hour)
	_, err := db.CreateSession(user.ID, "delete-token", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = db.DeleteSession("delete-token")
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	_, err = db.GetSessionByToken("delete-token")
	if err == nil {
		t.Error("GetSessionByToken should fail after DeleteSession")
	}
}

func TestDeleteUserSessions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "session-del-all@example.com", "hash")
	expires := time.Now().Add(24 * time.Hour)

	_, err := db.CreateSession(user.ID, "token-1", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	_, err = db.CreateSession(user.ID, "token-2", expires)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	err = db.DeleteUserSessions(user.ID)
	if err != nil {
		t.Fatalf("DeleteUserSessions failed: %v", err)
	}

	_, err = db.GetSessionByToken("token-1")
	if err == nil {
		t.Error("Session should be deleted after DeleteUserSessions")
	}
	_, err = db.GetSessionByToken("token-2")
	if err == nil {
		t.Error("Session should be deleted after DeleteUserSessions")
	}
}

// ─── Feed Tests ────────────────────────────────────────────────────────────

func TestCreateFeed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed@example.com", "hash")
	desc := "A test feed"
	siteURL := "https://example.com"
	iconURL := "https://example.com/icon.png"

	feed, err := db.CreateFeed(&model.Feed{
		UserID:      user.ID,
		Title:       "Test Feed",
		Description: &desc,
		FeedURL:     "https://example.com/rss",
		SiteURL:     &siteURL,
		IconURL:     &iconURL,
	})
	if err != nil {
		t.Fatalf("CreateFeed failed: %v", err)
	}

	if feed.ID == 0 {
		t.Error("CreateFeed returned feed with ID 0")
	}
	if feed.Title != "Test Feed" {
		t.Errorf("Feed Title = %q", feed.Title)
	}
	if feed.FeedURL != "https://example.com/rss" {
		t.Errorf("Feed FeedURL = %q", feed.FeedURL)
	}
	if feed.Description == nil || *feed.Description != "A test feed" {
		t.Error("Feed Description not set correctly")
	}
	if feed.SiteURL == nil || *feed.SiteURL != "https://example.com" {
		t.Error("Feed SiteURL not set correctly")
	}
	if feed.IconURL == nil || *feed.IconURL != "https://example.com/icon.png" {
		t.Error("Feed IconURL not set correctly")
	}
}

func TestCreateFeedDuplicateURL(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-dupe@example.com", "hash")

	_, err := db.CreateFeed(&model.Feed{
		UserID:  user.ID,
		Title:   "Feed 1",
		FeedURL: "https://example.com/rss",
	})
	if err != nil {
		t.Fatalf("First CreateFeed failed: %v", err)
	}

	_, err = db.CreateFeed(&model.Feed{
		UserID:  user.ID,
		Title:   "Feed 2",
		FeedURL: "https://example.com/rss",
	})
	if err == nil {
		t.Error("CreateFeed should fail for duplicate feed URL")
	}
}

func TestGetUserFeeds(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feeds-list@example.com", "hash")
	createTestFeed(t, db, user.ID, "Feed A", "https://example.com/a.rss")
	createTestFeed(t, db, user.ID, "Feed B", "https://example.com/b.rss")

	feeds, err := db.GetUserFeeds(user.ID)
	if err != nil {
		t.Fatalf("GetUserFeeds failed: %v", err)
	}
	if len(feeds) != 2 {
		t.Errorf("GetUserFeeds returned %d feeds, want 2", len(feeds))
	}
}

func TestGetUserFeedsEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feeds-empty@example.com", "hash")

	feeds, err := db.GetUserFeeds(user.ID)
	if err != nil {
		t.Fatalf("GetUserFeeds failed: %v", err)
	}
	if len(feeds) != 0 {
		t.Errorf("GetUserFeeds returned %d feeds for user with no feeds", len(feeds))
	}
}

func TestGetUserFeedsOwnership(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user1 := createTestUser(t, db, "user1@example.com", "hash")
	user2 := createTestUser(t, db, "user2@example.com", "hash")

	createTestFeed(t, db, user1.ID, "User1 Feed", "https://user1.example.com/rss")
	createTestFeed(t, db, user2.ID, "User2 Feed", "https://user2.example.com/rss")

	feeds1, _ := db.GetUserFeeds(user1.ID)
	if len(feeds1) != 1 || feeds1[0].UserID != user1.ID {
		t.Error("GetUserFeeds should only return feeds owned by the user")
	}
}

func TestGetFeedByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-get@example.com", "hash")
	created := createTestFeed(t, db, user.ID, "Specific Feed", "https://specific.example.com/rss")

	feed, err := db.GetFeedByID(created.ID, user.ID)
	if err != nil {
		t.Fatalf("GetFeedByID failed: %v", err)
	}
	if feed.Title != "Specific Feed" {
		t.Errorf("Feed Title = %q", feed.Title)
	}
}

func TestGetFeedByIDNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-notfound@example.com", "hash")

	_, err := db.GetFeedByID(99999, user.ID)
	if err == nil {
		t.Error("GetFeedByID should fail for nonexistent feed")
	}
}

func TestGetFeedByIDWrongUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user1 := createTestUser(t, db, "user1@example.com", "hash")
	user2 := createTestUser(t, db, "user2@example.com", "hash")
	feed := createTestFeed(t, db, user1.ID, "User1 Feed", "https://example.com/rss")

	_, err := db.GetFeedByID(feed.ID, user2.ID)
	if err == nil {
		t.Error("GetFeedByID should fail when requesting another user's feed")
	}
}

func TestDeleteFeed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-del@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Delete Me", "https://delete.example.com/rss")

	err := db.DeleteFeed(feed.ID, user.ID)
	if err != nil {
		t.Fatalf("DeleteFeed failed: %v", err)
	}

	_, err = db.GetFeedByID(feed.ID, user.ID)
	if err == nil {
		t.Error("GetFeedByID should fail after DeleteFeed")
	}
}

func TestDeleteFeedCascadesToArticles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-cascade@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Cascade Test", "https://cascade.example.com/rss")

	_, err := db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "guid-1",
		Title:  "Article 1",
	})
	if err != nil {
		t.Fatalf("CreateArticle failed: %v", err)
	}

	err = db.DeleteFeed(feed.ID, user.ID)
	if err != nil {
		t.Fatalf("DeleteFeed failed: %v", err)
	}

	articles, _ := db.GetArticlesByFeedID(feed.ID)
	if len(articles) != 0 {
		t.Error("Articles should be cascade-deleted when feed is deleted")
	}
}

func TestUpdateFeedLastFetched(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-fetch@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Fetch Test", "https://fetch.example.com/rss")

	now := time.Now()
	err := db.UpdateFeedLastFetched(feed.ID, now)
	if err != nil {
		t.Fatalf("UpdateFeedLastFetched failed: %v", err)
	}

	updated, err := db.GetFeedByID(feed.ID, user.ID)
	if err != nil {
		t.Fatalf("GetFeedByID failed: %v", err)
	}
	if updated.LastFetchedAt == nil {
		t.Fatal("LastFetchedAt should not be nil after update")
	}
	if !updated.LastFetchedAt.Equal(now) {
		t.Errorf("LastFetchedAt = %v, want %v", updated.LastFetchedAt, now)
	}
}

func TestGetFeedByURL(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "feed-url@example.com", "hash")
	createTestFeed(t, db, user.ID, "URL Feed", "https://urltest.example.com/rss")

	feed, err := db.GetFeedByURL("https://urltest.example.com/rss")
	if err != nil {
		t.Fatalf("GetFeedByURL failed: %v", err)
	}
	if feed.Title != "URL Feed" {
		t.Errorf("Feed Title = %q", feed.Title)
	}
}

func TestGetFeedByURLNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.GetFeedByURL("https://nonexistent.example.com/rss")
	if err == nil {
		t.Error("GetFeedByURL should fail for nonexistent URL")
	}
}

// ─── Article Tests ─────────────────────────────────────────────────────────

func TestCreateArticle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "article@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Article Feed", "https://article.example.com/rss")
	content := "Full article content here"
	summary := "A short summary"
	publishedAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)

	article, err := db.CreateArticle(&model.Article{
		FeedID:      feed.ID,
		GUID:        "guid-article-1",
		Title:       "Test Article",
		URL:         strPtr("https://example.com/article1"),
		Author:      strPtr("John Doe"),
		Content:     &content,
		Summary:     &summary,
		PublishedAt: &publishedAt,
		IsRead:      false,
	})
	if err != nil {
		t.Fatalf("CreateArticle failed: %v", err)
	}

	if article.ID == 0 {
		t.Error("CreateArticle returned article with ID 0")
	}
	if article.Title != "Test Article" {
		t.Errorf("Article Title = %q", article.Title)
	}
	if article.GUID != "guid-article-1" {
		t.Errorf("Article GUID = %q", article.GUID)
	}
	if article.IsRead {
		t.Error("New article should have IsRead = false")
	}
}

func TestCreateArticleDuplicateGUID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "article-dupe@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Dupe Feed", "https://dupe.example.com/rss")

	_, err := db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "dupe-guid",
		Title:  "Original",
	})
	if err != nil {
		t.Fatalf("First CreateArticle failed: %v", err)
	}

	_, err = db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "dupe-guid",
		Title:  "Duplicate",
	})
	if err == nil {
		t.Error("CreateArticle should fail for duplicate GUID")
	}
}

func TestGetArticlesByFeedID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "articles-list@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "List Feed", "https://list.example.com/rss")

	_, err := db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "g1", Title: "Article 1"})
	if err != nil {
		t.Fatalf("CreateArticle failed: %v", err)
	}
	_, err = db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "g2", Title: "Article 2"})
	if err != nil {
		t.Fatalf("CreateArticle failed: %v", err)
	}

	articles, err := db.GetArticlesByFeedID(feed.ID)
	if err != nil {
		t.Fatalf("GetArticlesByFeedID failed: %v", err)
	}
	if len(articles) != 2 {
		t.Errorf("Got %d articles, want 2", len(articles))
	}
}

func TestGetArticlesByFeedIDEmpty(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "empty-articles@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Empty Feed", "https://empty.example.com/rss")

	articles, err := db.GetArticlesByFeedID(feed.ID)
	if err != nil {
		t.Fatalf("GetArticlesByFeedID failed: %v", err)
	}
	if len(articles) != 0 {
		t.Errorf("Got %d articles for empty feed, want 0", len(articles))
	}
}

func TestGetArticlesByUserID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "user-articles@example.com", "hash")
	feed1 := createTestFeed(t, db, user.ID, "Feed 1", "https://f1.example.com/rss")
	feed2 := createTestFeed(t, db, user.ID, "Feed 2", "https://f2.example.com/rss")

	db.CreateArticle(&model.Article{FeedID: feed1.ID, GUID: "g1", Title: "A1"})
	db.CreateArticle(&model.Article{FeedID: feed1.ID, GUID: "g2", Title: "A2"})
	db.CreateArticle(&model.Article{FeedID: feed2.ID, GUID: "g3", Title: "A3"})

	articles, err := db.GetArticlesByUserID(user.ID, nil, false, 0, 0)
	if err != nil {
		t.Fatalf("GetArticlesByUserID failed: %v", err)
	}
	if len(articles) != 3 {
		t.Errorf("Got %d articles, want 3", len(articles))
	}
}

func TestGetArticlesByUserIDFilteredByFeed(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "user-filter@example.com", "hash")
	feed1 := createTestFeed(t, db, user.ID, "Feed 1", "https://f1.example.com/rss")
	feed2 := createTestFeed(t, db, user.ID, "Feed 2", "https://f2.example.com/rss")

	db.CreateArticle(&model.Article{FeedID: feed1.ID, GUID: "g1", Title: "A1"})
	db.CreateArticle(&model.Article{FeedID: feed2.ID, GUID: "g2", Title: "A2"})

	articles, err := db.GetArticlesByUserID(user.ID, &feed1.ID, false, 0, 0)
	if err != nil {
		t.Fatalf("GetArticlesByUserID failed: %v", err)
	}
	if len(articles) != 1 {
		t.Errorf("Got %d articles, want 1", len(articles))
	}
	if len(articles) > 0 && articles[0].Title != "A1" {
		t.Errorf("Got article %q, want %q", articles[0].Title, "A1")
	}
}

func TestToggleArticleRead(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "toggle@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Toggle Feed", "https://toggle.example.com/rss")
	article, _ := db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "toggle-guid",
		Title:  "Toggle Article",
		IsRead: false,
	})

	// Toggle to read
	toggled, err := db.ToggleArticleRead(article.ID, user.ID)
	if err != nil {
		t.Fatalf("ToggleArticleRead failed: %v", err)
	}
	if !toggled.IsRead {
		t.Error("Article should be read after toggle")
	}

	// Toggle back to unread
	toggled, err = db.ToggleArticleRead(article.ID, user.ID)
	if err != nil {
		t.Fatalf("ToggleArticleRead failed: %v", err)
	}
	if toggled.IsRead {
		t.Error("Article should be unread after second toggle")
	}
}

func TestToggleArticleReadWrongUser(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user1 := createTestUser(t, db, "user1@example.com", "hash")
	user2 := createTestUser(t, db, "user2@example.com", "hash")
	feed := createTestFeed(t, db, user1.ID, "Feed", "https://example.com/rss")
	article, _ := db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "wrong-user-guid",
		Title:  "Article",
	})

	_, err := db.ToggleArticleRead(article.ID, user2.ID)
	if err == nil {
		t.Error("ToggleArticleRead should fail for wrong user")
	}
}

func TestGetArticleByID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "article-get@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Get Feed", "https://get.example.com/rss")
	content := "Detailed content"
	created, _ := db.CreateArticle(&model.Article{
		FeedID:  feed.ID,
		GUID:    "get-guid",
		Title:   "Get Article",
		Content: &content,
	})

	article, err := db.GetArticleByID(created.ID, user.ID)
	if err != nil {
		t.Fatalf("GetArticleByID failed: %v", err)
	}
	if article.Title != "Get Article" {
		t.Errorf("Article Title = %q", article.Title)
	}
	if article.Content == nil || *article.Content != "Detailed content" {
		t.Error("Article content not retrieved correctly")
	}
}

func TestGetArticleByIDNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "article-notfound@example.com", "hash")

	_, err := db.GetArticleByID(99999, user.ID)
	if err == nil {
		t.Error("GetArticleByID should fail for nonexistent article")
	}
}

func TestUpdateArticleSummary(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "summary@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Summary Feed", "https://summary.example.com/rss")
	article, _ := db.CreateArticle(&model.Article{
		FeedID: feed.ID,
		GUID:   "summary-guid",
		Title:  "Summary Article",
	})

	err := db.UpdateArticleSummary(article.ID, "Updated summary text")
	if err != nil {
		t.Fatalf("UpdateArticleSummary failed: %v", err)
	}

	updated, _ := db.GetArticleByID(article.ID, user.ID)
	if updated.Summary == nil || *updated.Summary != "Updated summary text" {
		t.Errorf("Summary = %v, want %q", updated.Summary, "Updated summary text")
	}
}

func TestDeleteArticlesByFeedID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "delete-articles@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Delete Feed", "https://delete.example.com/rss")

	db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "d1", Title: "Delete 1"})
	db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "d2", Title: "Delete 2"})

	err := db.DeleteArticlesByFeedID(feed.ID)
	if err != nil {
		t.Fatalf("DeleteArticlesByFeedID failed: %v", err)
	}

	articles, _ := db.GetArticlesByFeedID(feed.ID)
	if len(articles) != 0 {
		t.Errorf("Got %d articles after delete, want 0", len(articles))
	}
}

// ─── Settings Tests ───────────────────────────────────────────────────────

func TestUpsertUserSettings(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "settings@example.com", "hash")

	s := &model.UserSettings{
		UserID:          user.ID,
		AIProvider:      "anthropic",
		ModelName:       "claude-3-haiku-20240307",
		SummaryLength:   "medium",
		SummaryLanguage: "spanish",
	}
	baseURL := "https://custom.api.com"
	s.BaseURL = &baseURL
	apiKey := "sk-encrypted"
	s.APIKeyEncrypted = &apiKey

	err := db.UpsertUserSettings(s)
	if err != nil {
		t.Fatalf("UpsertUserSettings failed: %v", err)
	}

	retrieved, err := db.GetUserSettings(user.ID)
	if err != nil {
		t.Fatalf("GetUserSettings failed: %v", err)
	}

	if retrieved.AIProvider != "anthropic" {
		t.Errorf("AIProvider = %q, want %q", retrieved.AIProvider, "anthropic")
	}
	if retrieved.ModelName != "claude-3-haiku-20240307" {
		t.Errorf("ModelName = %q", retrieved.ModelName)
	}
	if retrieved.SummaryLength != "medium" {
		t.Errorf("SummaryLength = %q", retrieved.SummaryLength)
	}
}

func TestUpsertUserSettingsUpdate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "settings-update@example.com", "hash")

	// Insert
	db.UpsertUserSettings(&model.UserSettings{
		UserID:          user.ID,
		AIProvider:      "openai",
		ModelName:       "gpt-4o-mini",
		SummaryLength:   "short",
		SummaryLanguage: "english",
	})

	// Upsert with changes
	db.UpsertUserSettings(&model.UserSettings{
		UserID:          user.ID,
		AIProvider:      "anthropic",
		ModelName:       "claude-3-haiku-20240307",
		SummaryLength:   "long",
		SummaryLanguage: "french",
	})

	retrieved, _ := db.GetUserSettings(user.ID)
	if retrieved.AIProvider != "anthropic" {
		t.Errorf("After upsert, AIProvider = %q, want %q", retrieved.AIProvider, "anthropic")
	}
}

func TestGetUserSettingsNotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, err := db.GetUserSettings(99999)
	if err == nil {
		t.Error("GetUserSettings should fail for user without settings")
	}
}

func TestUnreadCountInFeeds(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "unread@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Unread Feed", "https://unread.example.com/rss")

	db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "u1", Title: "Unread 1", IsRead: false})
	db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "u2", Title: "Unread 2", IsRead: false})
	db.CreateArticle(&model.Article{FeedID: feed.ID, GUID: "u3", Title: "Read 1", IsRead: true})

	feeds, _ := db.GetUserFeeds(user.ID)
	if len(feeds) != 1 {
		t.Fatalf("Got %d feeds, want 1", len(feeds))
	}
	if feeds[0].UnreadCount != 2 {
		t.Errorf("UnreadCount = %d, want 2", feeds[0].UnreadCount)
	}
}

func TestArticleContentNotExposedInList(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	user := createTestUser(t, db, "content-hide@example.com", "hash")
	feed := createTestFeed(t, db, user.ID, "Content Test", "https://content.example.com/rss")
	content := "Secret content that should not appear in list view"
	db.CreateArticle(&model.Article{
		FeedID:  feed.ID,
		GUID:    "content-guid",
		Title:   "Content Article",
		Content: &content,
	})

	articles, _ := db.GetArticlesByUserID(user.ID, nil, false, 0, 0)
	if len(articles) > 0 && articles[0].Content != nil {
		t.Error("Article content should NOT be populated in list queries (NULL AS content)")
	}
}

// ─── Helpers ───────────────────────────────────────────────────────────────

func strPtr(s string) *string {
	return &s
}
