// Package database provides PostgreSQL connection pooling and database operations.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kandev/kandev/internal/common/config"
)

// DB wraps a pgxpool.Pool and provides helper methods for database operations.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB creates a new database connection pool using the provided configuration.
// It builds the connection string from config, configures pool settings,
// establishes the connection, and verifies it with a ping.
func NewDB(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	connString := cfg.DSN()

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure pool settings
	poolConfig.MaxConns = int32(cfg.MaxConns)
	poolConfig.MinConns = int32(cfg.MinConns)

	// Set reasonable connection timeouts
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	// Create the connection pool
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{pool: pool}, nil
}

// Pool returns the underlying pgxpool.Pool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Close closes the connection pool.
func (db *DB) Close() {
	if db.pool != nil {
		db.pool.Close()
	}
}

// Ping verifies the database connection is still alive.
func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Exec executes a query that doesn't return rows.
func (db *DB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// Query executes a query that returns rows.
func (db *DB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns at most one row.
func (db *DB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// BeginTx starts a transaction with the given options.
func (db *DB) BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error) {
	return db.pool.BeginTx(ctx, opts)
}

// WithTx executes the given function within a transaction.
// If the function returns an error, the transaction is rolled back.
// If the function succeeds, the transaction is committed.
func (db *DB) WithTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// Rollback on panic
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx failed: %w, rollback failed: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// WithTxOptions executes the given function within a transaction with custom options.
// If the function returns an error, the transaction is rolled back.
// If the function succeeds, the transaction is committed.
func (db *DB) WithTxOptions(ctx context.Context, opts pgx.TxOptions, fn func(tx pgx.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			// Rollback on panic
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx failed: %w, rollback failed: %v", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

