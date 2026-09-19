package hostnames

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	raw, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	store, err := NewStore(database, database)
	if err != nil {
		t.Fatalf("new hostname store: %v", err)
	}
	return store
}

// Reviewer-requested regression coverage for SQLite timestamp round trips.
func TestStoreSetAndGetRoundTrip(t *testing.T) {
	store := newTestStore(t)
	resolvedAt := time.Date(2026, 8, 16, 12, 0, 0, 123456789, time.UTC)

	if err := store.Set(context.Background(), "192.0.2.1", "mail.example.test", resolvedAt); err != nil {
		t.Fatalf("set hostname: %v", err)
	}
	entry, err := store.Get(context.Background(), "192.0.2.1")
	if err != nil {
		t.Fatalf("get hostname: %v", err)
	}
	if entry.Hostname != "mail.example.test" {
		t.Errorf("hostname = %q, want %q", entry.Hostname, "mail.example.test")
	}
	if entry.ResolvedAt == nil || !entry.ResolvedAt.Equal(resolvedAt) {
		t.Errorf("resolved at = %v, want %v", entry.ResolvedAt, resolvedAt)
	}
}

func TestStoreGetManyChunksLargeCacheLookup(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	ips := make([]string, 0, 33_000)
	for i := range cap(ips) {
		ips = append(ips, fmt.Sprintf("192.0.2.%d", i))
	}
	seeded := map[string]string{
		ips[0]:               "first.example.test",
		ips[maxGetManyBatch]: "second-batch.example.test",
		ips[len(ips)-1]:      "last.example.test",
	}
	resolvedAt := time.Date(2026, 8, 18, 12, 0, 0, 123456789, time.UTC)
	for ip, hostname := range seeded {
		if err := store.Set(ctx, ip, hostname, resolvedAt); err != nil {
			t.Fatalf("seed %s: %v", ip, err)
		}
	}

	entries, err := store.GetMany(ctx, ips)
	if err != nil {
		t.Fatalf("get many: %v", err)
	}
	if len(entries) != len(seeded) {
		t.Fatalf("entry count = %d, want %d", len(entries), len(seeded))
	}
	for ip, hostname := range seeded {
		entry, ok := entries[ip]
		if !ok || entry.Hostname != hostname {
			t.Errorf("entry %s = %#v, want hostname %q", ip, entry, hostname)
		}
	}
}
