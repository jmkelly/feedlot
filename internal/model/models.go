package model

import "time"

type User struct {
	ID           int64     `db:"id" json:"id"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

type Session struct {
	ID        int64     `db:"id" json:"id"`
	UserID    int64     `db:"user_id" json:"user_id"`
	Token     string    `db:"token" json:"token"`
	ExpiresAt time.Time `db:"expires_at" json:"expires_at"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Feed struct {
	ID            int64      `db:"id" json:"id"`
	UserID        int64      `db:"user_id" json:"user_id"`
	Title         string     `db:"title" json:"title"`
	Description   *string    `db:"description" json:"description,omitempty"`
	FeedURL       string     `db:"feed_url" json:"feed_url"`
	SiteURL       *string    `db:"site_url" json:"site_url,omitempty"`
	IconURL       *string    `db:"icon_url" json:"icon_url,omitempty"`
	LastFetchedAt *time.Time `db:"last_fetched_at" json:"last_fetched_at,omitempty"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
	UnreadCount   int        `db:"unread_count" json:"unread_count,omitempty"`
}

type LogEntry struct {
	ID        int64     `db:"id" json:"id"`
	Level     string    `db:"level" json:"level"`
	Message   string    `db:"message" json:"message"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Article struct {
	ID          int64      `db:"id" json:"id"`
	FeedID      int64      `db:"feed_id" json:"feed_id"`
	GUID        string     `db:"guid" json:"guid"`
	Title       string     `db:"title" json:"title"`
	URL         *string    `db:"url" json:"url,omitempty"`
	Author      *string    `db:"author" json:"author,omitempty"`
	Content     *string    `db:"content" json:"-"`
	Summary     *string    `db:"summary" json:"summary,omitempty"`
	PublishedAt *time.Time `db:"published_at" json:"published_at,omitempty"`
	IsRead      bool       `db:"is_read" json:"is_read"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}
