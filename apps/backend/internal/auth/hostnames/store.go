package hostnames

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const timestampLayout = "2006-01-02T15:04:05.000000000Z07:00"

const maxGetManyBatch = 900

// ErrNotFound reports that no cached hostname exists for an IP address.
var ErrNotFound = errors.New("hostname cache: not found")

// CacheEntry stores a cached hostname and the time it was resolved.
type CacheEntry struct {
	Hostname   string
	ResolvedAt *time.Time
}

// Cache reads and writes persisted hostname-resolution entries.
type Cache interface {
	GetMany(context.Context, []string) (map[string]CacheEntry, error)
	Get(context.Context, string) (CacheEntry, error)
	Set(context.Context, string, string, time.Time) error
}

// Store persists hostname-resolution entries in SQLite.
type Store struct {
	writer *sqlx.DB
	reader *sqlx.DB
}

// NewStore initializes hostname-cache storage over the supplied database handles.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	store := &Store{writer: writer, reader: reader}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

// initSchema creates the hostname cache table when it is absent.
func (s *Store) initSchema() error {
	_, err := s.writer.Exec(`CREATE TABLE IF NOT EXISTS auth_hostname_cache (
		ip TEXT PRIMARY KEY,
		hostname TEXT NOT NULL DEFAULT '',
		resolved_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create hostname cache: %w", err)
	}
	return nil
}

// FormatTimestamp serializes a resolution time for SQLite storage.
func FormatTimestamp(value time.Time) string { return value.UTC().Format(timestampLayout) }

// ParseTimestamp decodes a timestamp stored by FormatTimestamp.
func ParseTimestamp(value string) (time.Time, error) { return time.Parse(timestampLayout, value) }

// Get returns the cached hostname entry for one IP.
func (s *Store) Get(ctx context.Context, ip string) (CacheEntry, error) {
	entries, err := s.GetMany(ctx, []string{ip})
	if err != nil {
		return CacheEntry{}, err
	}
	entry, ok := entries[ip]
	if !ok {
		return CacheEntry{}, ErrNotFound
	}
	return entry, nil
}

// GetMany returns cached hostname entries for the requested IPs.
func (s *Store) GetMany(ctx context.Context, ips []string) (map[string]CacheEntry, error) {
	entries := make(map[string]CacheEntry)
	if len(ips) == 0 {
		return entries, nil
	}
	for start := 0; start < len(ips); start += maxGetManyBatch {
		end := min(start+maxGetManyBatch, len(ips))
		query, args, err := sqlx.In(`SELECT ip, hostname, resolved_at FROM auth_hostname_cache WHERE ip IN (?)`, ips[start:end])
		if err != nil {
			return nil, err
		}
		rows, err := s.reader.QueryxContext(ctx, s.reader.Rebind(query), args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var ip, hostname, resolvedAt string
			if err := rows.Scan(&ip, &hostname, &resolvedAt); err != nil {
				_ = rows.Close()
				return nil, err
			}
			parsed, err := ParseTimestamp(resolvedAt)
			if err != nil {
				_ = rows.Close()
				return nil, err
			}
			entries[ip] = CacheEntry{Hostname: hostname, ResolvedAt: &parsed}
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return entries, nil
}

// Set inserts or replaces a cached hostname entry.
func (s *Store) Set(ctx context.Context, ip, hostname string, resolvedAt time.Time) error {
	_, err := s.writer.ExecContext(ctx, s.writer.Rebind(`INSERT INTO auth_hostname_cache (ip, hostname, resolved_at)
		VALUES (?, ?, ?)
		ON CONFLICT(ip) DO UPDATE SET hostname = excluded.hostname, resolved_at = excluded.resolved_at`), ip, hostname, FormatTimestamp(resolvedAt))
	return err
}
