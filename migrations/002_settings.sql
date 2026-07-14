CREATE TABLE IF NOT EXISTS user_settings (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    ai_provider TEXT DEFAULT 'openai',
    api_key_encrypted TEXT,
    model_name TEXT DEFAULT 'gpt-4o-mini',
    base_url TEXT,
    summary_length TEXT DEFAULT 'short',
    summary_language TEXT DEFAULT 'english',
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
