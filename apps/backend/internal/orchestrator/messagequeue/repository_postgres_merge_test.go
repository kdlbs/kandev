package messagequeue

import (
	"context"
	"errors"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/testutil"
)

// newTestPostgresRepo opens an isolated Postgres schema (skipping unless
// KANDEV_TEST_POSTGRES_DSN is set) and constructs the repository over it. The
// queue repository's SQL is driver-agnostic (Rebind'd ? placeholders, no
// SQLite-only constructs), so the same implementation exercises the merge/drain
// ordering under a real shared-database server.
func newTestPostgresRepo(t *testing.T) Repository {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("NewSQLiteRepository(postgres): %v", err)
	}
	return repo
}

// TestPostgresRepository_MergeDrainOrdering_DrainWins asserts the drain-wins
// ordering of the merge/drain race: after the source drains, a merge reports
// ErrEntryNotFound and the target is untouched, so no content is lost.
func TestPostgresRepository_MergeDrainOrdering_DrainWins(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	if _, err := repo.TakeByID(ctx, "s1", source.ID); err != nil {
		t.Fatalf("drain source: %v", err)
	}
	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrEntryNotFound) {
		t.Fatalf("merge after drain error = %v, want ErrEntryNotFound", err)
	}
	kept, err := repo.ListBySession(ctx, "s1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(kept) != 1 {
		t.Fatalf("kept entries = %d, want 1", len(kept))
	}
	if kept[0].ID != target.ID || kept[0].Content != "first" {
		t.Errorf("kept entry = %s %q, want target %q", kept[0].ID, kept[0].Content, "first")
	}
}

// TestPostgresRepository_MergeDrainOrdering_MergeWins asserts the merge-wins
// ordering: the drain that follows picks up the merged entry with combined
// content, not the source.
func TestPostgresRepository_MergeDrainOrdering_MergeWins(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil, nil)
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil, nil)

	merged, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	drained, err := repo.TakeHead(ctx, "s1")
	if err != nil {
		t.Fatalf("drain after merge: %v", err)
	}
	if drained.ID != target.ID || drained.ID != merged.ID {
		t.Errorf("drained id = %s, want merged target %s", drained.ID, merged.ID)
	}
	if drained.Content != "first\n\nsecond" {
		t.Errorf("drained content = %q, want combined content", drained.Content)
	}
}

// TestPostgresRepository_MergeIntoAbove_ReferenceOverflow asserts the overflow
// rejection is atomic under Postgres too: neither row is touched and the queue
// count stays unchanged.
func TestPostgresRepository_MergeIntoAbove_ReferenceOverflow(t *testing.T) {
	repo := newTestPostgresRepo(t)
	ctx := context.Background()

	target := insertTestEntry(t, repo, "s1", "t1", "first", "user", nil,
		map[string]interface{}{MetadataEntityReferences: manyEntityRefsFrom(1, entityrefs.MaxReferencesPerMessage)})
	_ = target
	source := insertTestEntry(t, repo, "s1", "t1", "second", "user", nil,
		map[string]interface{}{MetadataEntityReferences: manyEntityRefsFrom(entityrefs.MaxReferencesPerMessage+1, entityrefs.MaxReferencesPerMessage)})

	if _, err := repo.MergeIntoAbove(ctx, "s1", source.ID, "user"); !errors.Is(err, ErrMergeReferenceOverflow) {
		t.Fatalf("merge error = %v, want ErrMergeReferenceOverflow", err)
	}
	count, _ := repo.CountBySession(ctx, "s1")
	if count != 2 {
		t.Errorf("count after rejected overflow merge = %d, want 2", count)
	}
}
