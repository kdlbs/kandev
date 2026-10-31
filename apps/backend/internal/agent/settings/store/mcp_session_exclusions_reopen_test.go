package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.3
func TestMCPSessionExclusionDiskReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	open := func() (*sqlx.DB, *sqliteRepository) {
		t.Helper()
		db, err := sqlx.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		repo, err := newSQLiteRepositoryWithDB(db, db, nil)
		if err != nil {
			t.Fatal(err)
		}
		return db, repo
	}
	db, repo := open()
	origin, rc, _ := seedSessionExclusion(t, repo)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, reopened := open()
	resolver := mcpconfig.NewResolver(reopened, reopened)
	resolver.SetSecretResolver(func(context.Context, string, string) (string, error) { return "test-value", nil })
	result, err := resolver.Resolve(context.Background(), rc, mcpconfig.Policy{AllowHTTP: true})
	if err != nil || len(result.Servers) != 0 || result.DesiredRevision != 1 {
		t.Fatalf("reopen did not use exclusion: %#v %v", result, err)
	}
	revision := int64(1)
	empty := []string{}
	if _, err := reopened.ReplaceMCPSelectionsConditional(context.Background(), origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &empty, ExclusionContext: &rc}); err != nil {
		t.Fatal(err)
	}
	restored, err := resolver.Resolve(context.Background(), rc, mcpconfig.Policy{AllowHTTP: true})
	if err != nil || len(restored.Servers) != 1 {
		t.Fatalf("restore after reopen: %#v %v", restored, err)
	}
}

// @covers AC-AGENTS-SESSION-MCP-EXCLUSIONS-001.6
func TestMCPSessionExclusionRejectedCombinedWriteRollsBack(t *testing.T) {
	repo := newTestRepo(t).(*sqliteRepository)
	origin, rc, def := seedSessionExclusion(t, repo)
	ctx := context.Background()
	foreign := testMCPDefinition()
	foreign.ID = "foreign"
	foreign.WorkspaceID = "another-workspace"
	if err := repo.CreateMCPServerDefinition(ctx, foreign); err != nil {
		t.Fatal(err)
	}
	disabled := testMCPDefinition()
	disabled.ID = "disabled"
	disabled.RuntimeName = "disabled"
	disabled.NormalizedRuntimeName = "disabled"
	disabled.Enabled = false
	if err := repo.CreateMCPServerDefinition(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign.ID, disabled.ID} {
		revision := int64(1)
		empty := []string{}
		if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, []string{id}, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &empty, ExclusionContext: &rc}); err == nil {
			t.Fatal("invalid addition accepted")
		}
		snapshot, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
		if err != nil || snapshot.State.DesiredRevision != 1 || len(snapshot.DefinitionIDs) != 0 || len(snapshot.ExcludedDefinitionIDs) != 1 || snapshot.ExcludedDefinitionIDs[0] != def.ID {
			t.Fatalf("partial write: %#v %v", snapshot, err)
		}
	}
	revision := int64(1)
	ids := []string{foreign.ID}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedRevision: &revision, ExcludedDefinitionIDs: &ids, ExclusionContext: &rc}); err == nil {
		t.Fatal("foreign exclusion accepted")
	}
}
