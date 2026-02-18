package db

import "github.com/jmoiron/sqlx"

// Pool provides separate read and write database connections.
//
// For SQLite with WAL mode, this enables concurrent reads while serializing
// writes through a single connection. The writer pool uses MaxOpenConns(1) to
// avoid SQLITE_BUSY on write contention, while the reader pool allows multiple
// concurrent connections for SELECT queries.
//
// For PostgreSQL, both Writer and Reader return the same *sqlx.DB since pgx
// handles connection pooling internally.
type Pool struct {
	writer *sqlx.DB
	reader *sqlx.DB
}

// NewPool creates a Pool from separate writer and reader connections.
func NewPool(writer, reader *sqlx.DB) *Pool {
	return &Pool{writer: writer, reader: reader}
}

// Writer returns the connection pool used for INSERT, UPDATE, DELETE, and
// transactions. For SQLite this is limited to a single connection.
func (p *Pool) Writer() *sqlx.DB { return p.writer }

// Reader returns the connection pool used for SELECT queries. For SQLite
// this opens multiple read-only connections that can operate concurrently
// with the writer via WAL snapshots.
func (p *Pool) Reader() *sqlx.DB { return p.reader }

// Close closes both the writer and reader pools.
func (p *Pool) Close() error {
	wErr := p.writer.Close()
	// Avoid double-close when both pools share the same *sqlx.DB (Postgres).
	if p.reader != p.writer {
		if rErr := p.reader.Close(); rErr != nil && wErr == nil {
			return rErr
		}
	}
	return wErr
}
