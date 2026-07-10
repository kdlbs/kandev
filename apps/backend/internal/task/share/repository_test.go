package share

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	tmp := t.TempDir()
	rawDB, err := db.OpenSQLite(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	sqlxDB := sqlx.NewDb(rawDB, "sqlite3")
	repo, err := NewRepository(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func TestRepository_CreateAndGet(t *testing.T) {
	t.Parallel()
	r := newTestRepo(t)
	ctx := context.Background()

	in := &Share{
		ID:                "s-1",
		TaskSessionID:     "sess-1",
		Backend:           BackendGitHubGist,
		ExternalID:        "abc123",
		ExternalURL:       "https://gist.github.com/u/abc123",
		SnapshotSizeBytes: 1024,
	}
	if err := r.Create(ctx, in); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := r.GetByID(ctx, "s-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != in.ID || got.ExternalURL != in.ExternalURL || got.Backend != BackendGitHubGist {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt should default to non-zero")
	}
	if got.RevokedAt != nil {
		t.Fatalf("RevokedAt should be nil on fresh row, got %v", got.RevokedAt)
	}
}

func TestRepository_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	r := newTestRepo(t)
	_, err := r.GetByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepository_ListByTaskSession_FiltersAndOrders(t *testing.T) {
	t.Parallel()
	r := newTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC()
	rows := []*Share{
		{ID: "s-a", TaskSessionID: "sess-1", Backend: BackendGitHubGist, ExternalID: "a", ExternalURL: "u/a", CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "s-b", TaskSessionID: "sess-1", Backend: BackendGitHubGist, ExternalID: "b", ExternalURL: "u/b", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "s-c", TaskSessionID: "sess-2", Backend: BackendGitHubGist, ExternalID: "c", ExternalURL: "u/c", CreatedAt: now},
	}
	for _, row := range rows {
		if err := r.Create(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.ID, err)
		}
	}

	got, err := r.ListByTaskSession(ctx, "sess-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows for sess-1, got %d", len(got))
	}
	if got[0].ID != "s-b" || got[1].ID != "s-a" {
		t.Fatalf("expected newest-first ordering, got %s,%s", got[0].ID, got[1].ID)
	}
}

func TestRepository_MarkRevoked(t *testing.T) {
	t.Parallel()
	r := newTestRepo(t)
	ctx := context.Background()

	if err := r.Create(ctx, &Share{
		ID: "s-1", TaskSessionID: "sess-1", Backend: BackendGitHubGist,
		ExternalID: "x", ExternalURL: "u/x",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	revokedAt := time.Now().UTC()
	if err := r.MarkRevoked(ctx, "s-1", revokedAt); err != nil {
		t.Fatalf("first revoke: %v", err)
	}

	got, err := r.GetByID(ctx, "s-1")
	if err != nil {
		t.Fatalf("get after revoke: %v", err)
	}
	if got.RevokedAt == nil {
		t.Fatal("expected RevokedAt to be set")
	}

	// Second revoke is idempotent: returns nil and leaves the timestamp untouched.
	priorTs := *got.RevokedAt
	if err := r.MarkRevoked(ctx, "s-1", revokedAt.Add(time.Hour)); err != nil {
		t.Fatalf("second revoke should be idempotent, got %v", err)
	}
	got2, err := r.GetByID(ctx, "s-1")
	if err != nil {
		t.Fatalf("get after second revoke: %v", err)
	}
	if !got2.RevokedAt.Equal(priorTs) {
		t.Fatalf("revoked_at changed: was %v, now %v", priorTs, got2.RevokedAt)
	}
}

func TestRepository_MarkRevoked_NotFound(t *testing.T) {
	t.Parallel()
	r := newTestRepo(t)
	err := r.MarkRevoked(context.Background(), "missing", time.Now())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
