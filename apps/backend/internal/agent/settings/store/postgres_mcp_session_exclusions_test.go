package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6
func TestPostgresMCPSessionExclusionsMixedWriters(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(4)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	origin, rc, def := seedSessionExclusion(t, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	barrier, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = barrier.Rollback() }()
	if err := lockMCPSelectionOwner(ctx, barrier, origin); err != nil {
		t.Fatal(err)
	}
	revision := int64(1)
	empty := []string{}
	conditions := []mcpconfig.SelectionCondition{{ExpectedRevision: &revision}, {ExpectedRevision: &revision, ExcludedDefinitionIDs: &empty, ExclusionContext: &rc}}
	results := make(chan error, 2)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(2)
	for _, condition := range conditions {
		go func() {
			ready.Done()
			<-start
			_, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, []string{def.ID}, condition)
			results <- err
		}()
	}
	ready.Wait()
	close(start)
	for {
		var waiting int
		if err := db.GetContext(ctx, &waiting, `SELECT COUNT(DISTINCT pid) FROM pg_locks WHERE NOT granted AND pid <> pg_backend_pid()`); err != nil {
			t.Fatal(err)
		}
		if waiting >= 2 {
			break
		}
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
	}
	if err := barrier.Commit(); err != nil {
		t.Fatal(err)
	}
	a, b := <-results, <-results
	valid := (a == nil && errors.Is(b, mcpconfig.ErrMCPSelectionConflict)) || (b == nil && errors.Is(a, mcpconfig.ErrMCPSelectionConflict))
	if !valid {
		t.Fatalf("writers: %v / %v", a, b)
	}
	before, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.ExcludedDefinitionIDs) != len(before.ExcludedDefinitionIDs) || legacy.State.DesiredRevision != 3 {
		t.Fatalf("legacy save lost exclusions or revision: %#v", legacy)
	}
	reopened, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := reopened.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil || after.State.DesiredRevision != 3 {
		t.Fatalf("replay failed: %#v %v", after, err)
	}
	// Keep only an exclusion so workspace cleanup must discover its session.
	revision = 3
	excluded := []string{def.ID}
	if _, err := reopened.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &excluded, ExclusionContext: &rc}); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DeleteMCPWorkspaceData(ctx, def.WorkspaceID); err != nil {
		t.Fatal(err)
	}
	after, err = reopened.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil || len(after.ExcludedDefinitionIDs) != 0 || after.State != nil {
		t.Fatalf("cleanup failed: %#v %v", after, err)
	}
}
