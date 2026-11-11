package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1
func TestPostgresMCPSelectionConditionIgnoresNumericCollation(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var hasICU bool
	if err := db.GetContext(ctx, &hasICU, `SELECT EXISTS (SELECT 1 FROM pg_collation WHERE collprovider = 'i')`); err != nil {
		t.Fatal(err)
	}
	if !hasICU {
		t.Skip("PostgreSQL server has no ICU collation support")
	}
	for _, query := range []string{
		`CREATE COLLATION mcp_selection_numeric (provider = icu, locale = 'und-u-kn')`,
		`ALTER TABLE repository_mcp_selections ALTER COLUMN mcp_server_id TYPE TEXT COLLATE mcp_selection_numeric`,
	} {
		if _, err := db.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	ids := []string{"10a00000-0000-4000-8000-000000000000", "2a000000-0000-4000-8000-000000000000"}
	for index, id := range ids {
		def := testMCPDefinition()
		def.ID = id
		def.RuntimeName = []string{"ten", "two"}[index]
		def.NormalizedRuntimeName = def.RuntimeName
		if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
			t.Fatal(err)
		}
	}
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeRepository, WorkspaceID: "workspace-1", OwnerID: "owner"}
	initial, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, mcpconfig.SelectionCondition{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(initial.DefinitionIDs, []string{ids[1], ids[0]}) {
		t.Fatalf("numeric collation did not change the database order: %v", initial.DefinitionIDs)
	}
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, ids, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &ids}); err != nil {
		t.Fatalf("unchanged membership was rejected under numeric collation: %v", err)
	}
	stale := ids[:1]
	if _, err := repo.ReplaceMCPSelectionsConditional(ctx, origin, nil, mcpconfig.SelectionCondition{ExpectedDefinitionIDs: &stale}); !errors.Is(err, mcpconfig.ErrMCPSelectionConflict) {
		t.Fatalf("changed membership did not conflict: %v", err)
	}
	latest, err := repo.ReadMCPSelectionSnapshot(ctx, origin)
	if err != nil || !slices.Equal(latest.DefinitionIDs, initial.DefinitionIDs) {
		t.Fatalf("condition changed the saved membership: %#v %v", latest, err)
	}
}
