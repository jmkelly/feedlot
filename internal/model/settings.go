package model

import "time"

type UserSettings struct {
	UserID          int64     `db:"user_id" json:"user_id"`
	AIProvider      string    `db:"ai_provider" json:"ai_provider"`
	APIKeyEncrypted *string   `db:"api_key_encrypted" json:"-"`
	ModelName       string    `db:"model_name" json:"model_name"`
	BaseURL         *string   `db:"base_url" json:"base_url,omitempty"`
	SummaryLength   string    `db:"summary_length" json:"summary_length"`
	SummaryLanguage string    `db:"summary_language" json:"summary_language"`
	UpdatedAt       time.Time `db:"updated_at" json:"updated_at"`
}
