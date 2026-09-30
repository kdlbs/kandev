package provideraccess

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newGrantTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", filepath.Join(t.TempDir(), "provider-access.db")+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	conn.SetMaxOpenConns(8)
	store, err := NewStore(conn)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func testGrant(id string) Grant {
	return Grant{
		ID: id, GrantScope: GrantScope{
			PluginInstallationID: "installation-1", PluginID: "coordinator",
			WorkspaceID: "workspace-1", ConversationKey: "coordinator",
			TargetTaskID: "target-1", RepositoryID: "repo-1", Provider: "github",
			Purpose: "actions_write",
		}, CreatedByUserID: "admin-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
}

func TestStoreReplaceGrantRevokesPreviousGeneration(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	first := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &first); err != nil {
		t.Fatalf("replace first grant: %v", err)
	}
	if first.Generation != 1 {
		t.Fatalf("first generation = %d, want 1", first.Generation)
	}
	second := testGrant("grant-2")
	if err := store.ReplaceGrant(ctx, &second); err != nil {
		t.Fatalf("replace second grant: %v", err)
	}
	if second.Generation != 2 {
		t.Fatalf("second generation = %d, want 2", second.Generation)
	}
	previous, err := store.GetGrant(ctx, first.ID)
	if err != nil || previous == nil || previous.RevokedAt == nil {
		t.Fatalf("previous grant = %+v, err = %v, want revoked", previous, err)
	}
	active, err := store.GetActiveGrant(ctx, second.Scope())
	if err != nil || active == nil || active.ID != second.ID {
		t.Fatalf("active grant = %+v, err = %v, want second grant", active, err)
	}
}

func TestStoreRevokeGrantFencesActiveScope(t *testing.T) {
	store := newGrantTestStore(t)
	ctx := context.Background()
	grant := testGrant("grant-1")
	if err := store.ReplaceGrant(ctx, &grant); err != nil {
		t.Fatal(err)
	}
	if err := store.RevokeGrant(ctx, "other-workspace", grant.ID, time.Now().UTC()); err == nil {
		t.Fatal("cross-workspace revocation succeeded")
	}
	if err := store.RevokeGrant(ctx, grant.WorkspaceID, grant.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	active, err := store.GetActiveGrant(ctx, grant.Scope())
	if err != nil || active != nil {
		t.Fatalf("active grant = %+v, err = %v, want none", active, err)
	}
}

func TestStoreConcurrentGrantReplacementHasOneActiveGeneration(t *testing.T) {
	store := newGrantTestStore(t)
	const count = 8
	var wg sync.WaitGroup
	results := make(chan Grant, count)
	errors := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			grant := testGrant(fmt.Sprintf("grant-%d", i))
			if err := store.ReplaceGrant(context.Background(), &grant); err != nil {
				errors <- err
				return
			}
			results <- grant
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("replace grant: %v", err)
	}
	if t.Failed() {
		return
	}
	close(results)
	seen := map[int64]bool{}
	for grant := range results {
		if seen[grant.Generation] {
			t.Fatalf("duplicate generation %d", grant.Generation)
		}
		seen[grant.Generation] = true
	}
	if len(seen) != count {
		t.Fatalf("generations = %d, want %d", len(seen), count)
	}
	active, err := store.GetActiveGrant(context.Background(), testGrant("sample").Scope())
	if err != nil || active == nil || active.Generation != count {
		t.Fatalf("active grant = %+v, err = %v, want generation %d", active, err, count)
	}
}
