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

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
func TestPostgresMCPSelectionConditionalWriters(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	db.SetMaxOpenConns(4)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []mcpconfig.SelectionScope{mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTaskSession} {
		t.Run(string(scope), func(t *testing.T) {
			origin := mcpconfig.SelectionOrigin{Scope: scope, WorkspaceID: def.WorkspaceID, OwnerID: "owner"}
			empty := []string{}
			revision := int64(0)
			condition := mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &empty}
			if scope == mcpconfig.SelectionScopeTaskSession {
				condition = mcpconfig.SelectionCondition{ExpectedRevision: &revision}
			}
			barrier, err := db.BeginTxx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = barrier.Rollback() }()
			if err := lockMCPDefinition(ctx, barrier, def.WorkspaceID, def.ID); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			var ready sync.WaitGroup
			ready.Add(2)
			for range 2 {
				go func() {
					ready.Done()
					<-start
					_, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, []string{def.ID}, condition)
					results <- err
				}()
			}
			ready.Wait()
			close(start)
			// Both writers must reach a database lock before either may commit.
			// Without owner admission, both pass the empty-snapshot condition.
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
			first, second := <-results, <-results
			valid := (first == nil && errors.Is(second, mcpconfig.ErrMCPSelectionConflict)) || (second == nil && errors.Is(first, mcpconfig.ErrMCPSelectionConflict))
			if !valid {
				t.Fatalf("writers: %v, %v", first, second)
			}
			snapshot, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
			if err != nil || len(snapshot.DefinitionIDs) != 1 {
				t.Fatalf("snapshot %#v: %v", snapshot, err)
			}
			if scope == mcpconfig.SelectionScopeTaskSession && snapshot.State.DesiredRevision != 1 {
				t.Fatalf("revision=%d", snapshot.State.DesiredRevision)
			}
		})
	}
}

func TestPostgresMCPSelectionImportAndDeletion(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	def := testMCPDefinition()
	profileID := seedAgentProfile(t, repo, "Imported", "import-owner")
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeProfile, WorkspaceID: def.WorkspaceID, OwnerID: profileID}
	if err := repo.ImportLegacyMCPProfileWorkspace(ctx, def.WorkspaceID, profileID, nil, []string{def.ID}, mcpconfig.LegacyImportState{WorkspaceID: def.WorkspaceID, ProfileID: profileID, Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	empty := []string{}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &empty}); !errors.Is(err, mcpconfig.ErrMCPSelectionConflict) {
		t.Fatalf("overwrote import: %v", err)
	}
	if err := repo.DeleteMCPServerDefinitionWithSelections(ctx, def.WorkspaceID, def.ID, def.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, []string{def.ID}, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &empty}); !errors.Is(err, mcpconfig.ErrMCPSelectionWorkspaceMismatch) {
		t.Fatalf("recreated removed definition: %v", err)
	}
}
