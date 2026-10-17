package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/db"
)

func newNetworkLedgerStore(t *testing.T) *Store {
	t.Helper()
	conn := newSQLite(t)
	store, err := NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new storage store: %v", err)
	}
	return store
}

func TestNetworkLedgerFirstSightingIsSticky(t *testing.T) {
	store := newNetworkLedgerStore(t)
	ctx := context.Background()
	first := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(48 * time.Hour)

	entry, err := store.UpsertNetworkLedgerObserved(ctx, "net-1", "kd_task_default", first)
	if err != nil {
		t.Fatalf("first sighting: %v", err)
	}
	if !entry.FirstSeenAt.Equal(first) {
		t.Fatalf("first seen = %v, want %v", entry.FirstSeenAt, first)
	}
	entry, err = store.UpsertNetworkLedgerObserved(ctx, "net-1", "renamed", second)
	if err != nil {
		t.Fatalf("second sighting: %v", err)
	}
	if !entry.FirstSeenAt.Equal(first) {
		t.Fatalf("first seen drifted to %v; the stable-age clock must not reset", entry.FirstSeenAt)
	}
	if !entry.LastSeenAt.Equal(second) {
		t.Fatalf("last seen = %v, want %v", entry.LastSeenAt, second)
	}
	if entry.State != NetworkLedgerStateObserved {
		t.Fatalf("state = %s, want observed", entry.State)
	}
}

func TestNetworkLedgerMarkPersistsGraceDeadline(t *testing.T) {
	store := newNetworkLedgerStore(t)
	ctx := context.Background()
	if _, err := store.UpsertNetworkLedgerObserved(ctx, "net-1", "kd_task_default", time.Now()); err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	marked := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	deleteAfter := marked.Add(24 * time.Hour)

	entry, err := store.TransitionNetworkLedgerEntry(
		ctx, "net-1", NetworkLedgerStateMarked, marked, deleteAfter,
		json.RawMessage(`{"reason":"owning task finished"}`), "",
	)
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if entry.State != NetworkLedgerStateMarked || entry.DeleteAfter == nil ||
		!entry.DeleteAfter.Equal(deleteAfter) {
		t.Fatalf("entry = %#v, want marked with deadline %v", entry, deleteAfter)
	}

	// Restart survival: a fresh read returns the same row.
	reopened, err := store.GetNetworkLedgerEntry(ctx, "net-1")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.State != NetworkLedgerStateMarked {
		t.Fatalf("reopened state = %s, want marked persisted across restart", reopened.State)
	}
}

func TestNetworkLedgerRemovalIsTerminal(t *testing.T) {
	store := newNetworkLedgerStore(t)
	ctx := context.Background()
	at := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	entry, err := store.RecordNetworkLedgerRemoved(ctx, "net-1", "kd_task_default", at, "run-1")
	if err != nil {
		t.Fatalf("record removed: %v", err)
	}
	if entry.State != NetworkLedgerStateRemoved || entry.RemovedAt == nil {
		t.Fatalf("entry = %#v, want terminal removed row", entry)
	}
	// A later stray observation (e.g. a list raced with the removal) must not
	// resurrect the row.
	if _, err := store.UpsertNetworkLedgerObserved(ctx, "net-1", "kd_task_default", at); err != nil {
		t.Fatalf("stray observation: %v", err)
	}
	reopened, err := store.GetNetworkLedgerEntry(ctx, "net-1")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.State != NetworkLedgerStateRemoved {
		t.Fatalf("state = %s, want removed to stay terminal", reopened.State)
	}
	// Terminal rows never show up in the active listing.
	entries, err := store.ListNetworkLedgerEntries(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("active entries = %d, want 0 for a terminal record", len(entries))
	}
}

func TestNetworkLedgerSkipRecordsErrorAndKeepsObservation(t *testing.T) {
	store := newNetworkLedgerStore(t)
	ctx := context.Background()
	if _, err := store.UpsertNetworkLedgerObserved(ctx, "net-1", "kd_task_default", time.Now()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	entry, err := store.TransitionNetworkLedgerEntry(
		ctx, "net-1", NetworkLedgerStateSkipped, time.Time{}, time.Time{}, nil,
		"concurrent attachment detected on revalidation",
	)
	if err != nil {
		t.Fatalf("skip: %v", err)
	}
	if entry.State != NetworkLedgerStateSkipped ||
		entry.LastError != "concurrent attachment detected on revalidation" {
		t.Fatalf("entry = %#v, want skipped with the abort reason", entry)
	}
}

func TestNetworkLedgerInvalidTransitionRejected(t *testing.T) {
	store := newNetworkLedgerStore(t)
	ctx := context.Background()
	if _, err := store.TransitionNetworkLedgerEntry(
		ctx, "net-x", NetworkLedgerStateMarked, time.Time{}, time.Time{}, nil, "",
	); !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want validation error for mark without timestamps", err)
	}
	if _, err := store.TransitionNetworkLedgerEntry(
		ctx, "net-x", NetworkLedgerStateObserved, time.Now(), time.Now(), nil, "",
	); !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want validation error for observe with timestamps", err)
	}
}

func TestNetworkLedgerMissingEntryIsNotFound(t *testing.T) {
	store := newNetworkLedgerStore(t)
	if _, err := store.GetNetworkLedgerEntry(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
