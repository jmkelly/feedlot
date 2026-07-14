package db

import (
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sqlx.DB
}

func Open(dbPath string) (*DB, error) {
	db, err := sqlx.Connect("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("connect to sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite doesn't support concurrent writes

	return &DB{db}, nil
}

func (db *DB) Migrate(migrations []string) error {
	for i, m := range migrations {
		log.Printf("Running migration %d", i+1)
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
	}
	return nil
}
