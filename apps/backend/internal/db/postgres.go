package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/stdlib"
)

// OpenPostgres opens a PostgreSQL database connection using pgx.
// If maxConns or minConns are 0, they default to 25 and 5 respectively.
func OpenPostgres(dsn string, maxConns, minConns int) (*sql.DB, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres configuration: %w", err)
	}
	db := sql.OpenDB(stdlib.GetConnector(*config, stdlib.OptionAfterConnect(func(_ context.Context, conn *pgx.Conn) error {
		return configurePostgresTimeTypes(conn.TypeMap())
	})))
	if db == nil {
		return nil, fmt.Errorf("failed to open postgres database")
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

// configurePostgresTimeTypes makes database/sql scans deterministic across
// hosts. pgx's default timestamptz and timestamp codecs use time.Local when no
// scan location is configured, even though Kandev stores both forms as UTC.
func configurePostgresTimeTypes(typeMap *pgtype.Map) error {
	for _, typeName := range []string{"timestamp", "timestamptz"} {
		registered, ok := typeMap.TypeForName(typeName)
		if !ok {
			return fmt.Errorf("postgres type map has no %s type", typeName)
		}
		replacement := *registered
		if typeName == "timestamp" {
			replacement.Codec = &pgtype.TimestampCodec{ScanLocation: time.UTC}
		} else {
			replacement.Codec = &pgtype.TimestamptzCodec{ScanLocation: time.UTC}
		}
		typeMap.RegisterType(&replacement)
	}
	return nil
}
