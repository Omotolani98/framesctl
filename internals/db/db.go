// Package db handles database configuration
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	mu sync.RWMutex

	instance     *sql.DB
	databasePath string
)

func Init(ctx context.Context, path string) error {
	mu.Lock()
	defer mu.Unlock()

	if instance != nil {
		if databasePath != path {
			return fmt.Errorf(
				"database already initialized at %q",
				databasePath,
			)
		}

		return nil
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()

		return fmt.Errorf("ping sqlite database: %w", err)
	}

	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	}

	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()

			return fmt.Errorf("execute %q: %w", pragma, err)
		}
	}

	if err := migrate(ctx, db); err != nil {
		_ = db.Close()

		return err
	}

	instance = db
	databasePath = path

	return nil
}

func migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS uploads (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			filename TEXT NOT NULL,
			public_key TEXT NOT NULL UNIQUE,
			public_url TEXT NOT NULL,
			bucket TEXT NOT NULL,
			object_key TEXT NOT NULL,
			location TEXT NOT NULL,
			etag TEXT NOT NULL,
			content_length INTEGER NOT NULL,
			content_type TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	}

	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("run migration: %w", err)
		}
	}

	return nil
}

func Conn() (*sql.DB, error) {
	mu.RLock()
	defer mu.RUnlock()

	if instance == nil {
		return nil, errors.New("database has not been initialized")
	}

	return instance, nil
}

func MustConn() *sql.DB {
	db, err := Conn()
	if err != nil {
		panic(err)
	}

	return db
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		return nil
	}

	err := instance.Close()

	instance = nil
	databasePath = ""

	return err
}
