package db

import (
	"fmt"
	"time"

	"github.com/james/feedlot/internal/model"
)

// ─── Users ────────────────────────────────────────────────────────────────

func (db *DB) CreateUser(email, passwordHash string) (*model.User, error) {
	u := &model.User{}
	err := db.Get(u, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, password_hash, created_at`,
		email, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByEmail(email string) (*model.User, error) {
	u := &model.User{}
	err := db.Get(u, `SELECT id, email, password_hash, created_at FROM users WHERE email = $1`, email)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByID(id int64) (*model.User, error) {
	u := &model.User{}
	err := db.Get(u, `SELECT id, email, password_hash, created_at FROM users WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// ─── Sessions ─────────────────────────────────────────────────────────────

func (db *DB) CreateSession(userID int64, token string, expiresAt time.Time) (*model.Session, error) {
	s := &model.Session{}
	err := db.Get(s, `INSERT INTO sessions (user_id, token, expires_at) VALUES ($1, $2, $3)
		RETURNING id, user_id, token, expires_at, created_at`,
		userID, token, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return s, nil
}

func (db *DB) GetSessionByToken(token string) (*model.Session, error) {
	s := &model.Session{}
	err := db.Get(s, `SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE token = $1 AND expires_at > CURRENT_TIMESTAMP`,
		token)
	if err != nil {
		return nil, fmt.Errorf("get session by token: %w", err)
	}
	return s, nil
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (db *DB) DeleteUserSessions(userID int64) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// ─── Feeds ────────────────────────────────────────────────────────────────

func (db *DB) CreateFeed(feed *model.Feed) (*model.Feed, error) {
	f := &model.Feed{}
	err := db.Get(f, `INSERT INTO feeds (user_id, title, description, feed_url, site_url, icon_url)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, user_id, title, description, feed_url, site_url, icon_url, last_fetched_at, created_at`,
		feed.UserID, feed.Title, feed.Description, feed.FeedURL, feed.SiteURL, feed.IconURL)
	if err != nil {
		return nil, fmt.Errorf("create feed: %w", err)
	}
	return f, nil
}

func (db *DB) GetUserFeeds(userID int64) ([]model.Feed, error) {
	var feeds []model.Feed
	err := db.Select(&feeds, `SELECT f.id, f.user_id, f.title, f.description, f.feed_url, f.site_url, f.icon_url,
		f.last_fetched_at, f.created_at,
		COALESCE((SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id AND a.is_read = 0), 0) AS unread_count
		FROM feeds f WHERE f.user_id = $1 ORDER BY f.title`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user feeds: %w", err)
	}
	if feeds == nil {
		feeds = []model.Feed{}
	}
	return feeds, nil
}

func (db *DB) GetFeedByID(id, userID int64) (*model.Feed, error) {
	f := &model.Feed{}
	err := db.Get(f, `SELECT f.id, f.user_id, f.title, f.description, f.feed_url, f.site_url, f.icon_url,
		f.last_fetched_at, f.created_at,
		COALESCE((SELECT COUNT(*) FROM articles a WHERE a.feed_id = f.id AND a.is_read = 0), 0) AS unread_count
		FROM feeds f WHERE f.id = $1 AND f.user_id = $2`, id, userID)
	if err != nil {
		return nil, fmt.Errorf("get feed by id: %w", err)
	}
	return f, nil
}

func (db *DB) DeleteFeed(id, userID int64) error {
	_, err := db.Exec(`DELETE FROM feeds WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	return nil
}

func (db *DB) UpdateFeedLastFetched(id int64, t time.Time) error {
	_, err := db.Exec(`UPDATE feeds SET last_fetched_at = $1 WHERE id = $2`, t, id)
	if err != nil {
		return fmt.Errorf("update feed last_fetched: %w", err)
	}
	return nil
}

func (db *DB) GetFeedByURL(feedURL string) (*model.Feed, error) {
	f := &model.Feed{}
	err := db.Get(f, `SELECT id, user_id, title, description, feed_url, site_url, icon_url,
		last_fetched_at, created_at FROM feeds WHERE feed_url = $1`, feedURL)
	if err != nil {
		return nil, fmt.Errorf("get feed by URL: %w", err)
	}
	return f, nil
}

// ─── Articles ─────────────────────────────────────────────────────────────

func (db *DB) CreateArticle(a *model.Article) (*model.Article, error) {
	art := &model.Article{}
	err := db.Get(art, `INSERT INTO articles (feed_id, guid, title, url, author, content, summary, published_at, is_read)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, feed_id, guid, title, url, author, content, summary, published_at, is_read, created_at`,
		a.FeedID, a.GUID, a.Title, a.URL, a.Author, a.Content, a.Summary, a.PublishedAt, a.IsRead)
	if err != nil {
		return nil, fmt.Errorf("create article: %w", err)
	}
	return art, nil
}

func (db *DB) GetArticlesByFeedID(feedID int64) ([]model.Article, error) {
	var articles []model.Article
	err := db.Select(&articles, `SELECT id, feed_id, guid, title, url, author, content, summary, published_at, is_read, created_at
		FROM articles WHERE feed_id = $1 ORDER BY COALESCE(published_at, created_at) DESC`, feedID)
	if err != nil {
		return nil, fmt.Errorf("get articles by feed id: %w", err)
	}
	if articles == nil {
		articles = []model.Article{}
	}
	return articles, nil
}

func (db *DB) GetArticlesByUserID(userID int64, feedID *int64) ([]model.Article, error) {
	var articles []model.Article
	var err error

	if feedID != nil {
		err = db.Select(&articles, `SELECT a.id, a.feed_id, a.guid, a.title, a.url, a.author,
			NULL AS content, a.summary, a.published_at, a.is_read, a.created_at
			FROM articles a
			JOIN feeds f ON f.id = a.feed_id
			WHERE f.user_id = $1 AND a.feed_id = $2
			ORDER BY COALESCE(a.published_at, a.created_at) DESC`, userID, *feedID)
	} else {
		err = db.Select(&articles, `SELECT a.id, a.feed_id, a.guid, a.title, a.url, a.author,
			NULL AS content, a.summary, a.published_at, a.is_read, a.created_at
			FROM articles a
			JOIN feeds f ON f.id = a.feed_id
			WHERE f.user_id = $1
			ORDER BY COALESCE(a.published_at, a.created_at) DESC`, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("get articles by user id: %w", err)
	}
	if articles == nil {
		articles = []model.Article{}
	}
	return articles, nil
}

func (db *DB) ToggleArticleRead(id, userID int64) (*model.Article, error) {
	a := &model.Article{}
	err := db.Get(a, `UPDATE articles SET is_read = NOT is_read
		WHERE id = $1 AND feed_id IN (SELECT id FROM feeds WHERE user_id = $2)
		RETURNING id, feed_id, guid, title, url, author, content, summary, published_at, is_read, created_at`,
		id, userID)
	if err != nil {
		return nil, fmt.Errorf("toggle article read: %w", err)
	}
	return a, nil
}

func (db *DB) GetArticleByID(id, userID int64) (*model.Article, error) {
	a := &model.Article{}
	err := db.Get(a, `SELECT a.id, a.feed_id, a.guid, a.title, a.url, a.author,
		a.content, a.summary, a.published_at, a.is_read, a.created_at
		FROM articles a
		JOIN feeds f ON f.id = a.feed_id
		WHERE a.id = $1 AND f.user_id = $2`, id, userID)
	if err != nil {
		return nil, fmt.Errorf("get article by id: %w", err)
	}
	return a, nil
}

func (db *DB) UpdateArticleSummary(id int64, summary string) error {
	_, err := db.Exec(`UPDATE articles SET summary = $1 WHERE id = $2`, summary, id)
	if err != nil {
		return fmt.Errorf("update article summary: %w", err)
	}
	return nil
}

func (db *DB) DeleteArticlesByFeedID(feedID int64) error {
	_, err := db.Exec(`DELETE FROM articles WHERE feed_id = $1`, feedID)
	if err != nil {
		return fmt.Errorf("delete articles by feed id: %w", err)
	}
	return nil
}

// ─── Settings ─────────────────────────────────────────────────────────────

func (db *DB) GetUserSettings(userID int64) (*model.UserSettings, error) {
	s := &model.UserSettings{}
	err := db.Get(s, `SELECT user_id, ai_provider, api_key_encrypted, model_name, base_url, summary_length, summary_language, updated_at
		FROM user_settings WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user settings: %w", err)
	}
	return s, nil
}

func (db *DB) UpsertUserSettings(s *model.UserSettings) error {
	_, err := db.Exec(`INSERT INTO user_settings (user_id, ai_provider, api_key_encrypted, model_name, base_url, summary_length, summary_language, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP)
		ON CONFLICT(user_id) DO UPDATE SET
			ai_provider = EXCLUDED.ai_provider,
			api_key_encrypted = EXCLUDED.api_key_encrypted,
			model_name = EXCLUDED.model_name,
			base_url = EXCLUDED.base_url,
			summary_length = EXCLUDED.summary_length,
			summary_language = EXCLUDED.summary_language,
			updated_at = CURRENT_TIMESTAMP`,
		s.UserID, s.AIProvider, s.APIKeyEncrypted, s.ModelName, s.BaseURL, s.SummaryLength, s.SummaryLanguage)
	if err != nil {
		return fmt.Errorf("upsert user settings: %w", err)
	}
	return nil
}
