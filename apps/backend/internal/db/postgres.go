package db

import (
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgres opens a PostgreSQL database connection using pgx.
// If maxConns or minConns are 0, they default to 25 and 5 respectively.
func OpenPostgres(dsn string, maxConns, minConns int) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres database: %w", err)
	}

	if maxConns <= 0 {
		maxConns = 25
	}
	if minConns <= 0 {
		minConns = 5
	}

	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(minConns)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	return db, nil
}
