package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
	"testing"
	"time"
)

func TestPostgresConfirmedMCPDelete(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repository, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo := repository
	ctx := context.Background()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	owners := map[mcpconfig.SelectionScope]string{
		mcpconfig.SelectionScopeProfile:    seedAgentProfile(t, repo, "MCP selected", "mcp-owner"),
		mcpconfig.SelectionScopeRepository: "repository", mcpconfig.SelectionScopeTask: "task", mcpconfig.SelectionScopeTaskSession: "session",
	}
	scopes := []mcpconfig.SelectionScope{mcpconfig.SelectionScopeProfile, mcpconfig.SelectionScopeRepository, mcpconfig.SelectionScopeTask, mcpconfig.SelectionScopeTaskSession}
	for _, scope := range scopes {
		if err := repo.ReplaceMCPSelections(ctx, scope, def.WorkspaceID, owners[scope], []string{def.ID}); err != nil {
			t.Fatal(err)
		}
	}
	service := mcpconfig.NewCatalogService(repo)
	service.SetSelectionRepository(repo)
	if err := service.Delete(ctx, def.WorkspaceID, def.ID, def.Revision+1, true); !errors.Is(err, mcpconfig.ErrMCPRevisionConflict) {
		t.Fatalf("stale deletion: %v", err)
	}
	for _, scope := range scopes {
		ids, err := repo.ListMCPSelections(ctx, scope, def.WorkspaceID, owners[scope])
		if err != nil || len(ids) != 1 {
			t.Fatal("stale deletion lost selection rows")
		}
	}
	if err := service.Delete(ctx, def.WorkspaceID, def.ID, def.Revision, true); err != nil {
		t.Fatal(err)
	}
	for _, scope := range scopes {
		ids, err := repo.ListMCPSelections(ctx, scope, def.WorkspaceID, owners[scope])
		if err != nil || len(ids) != 0 {
			t.Fatal("confirmed deletion left selection rows")
		}
	}
}

func TestPostgresMCPResolutionSnapshot(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	db := testutil.OpenIsolatedPostgres(t, dsn)
	repo, err := newSQLiteRepositoryWithDB(db, db, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	def := testMCPDefinition()
	if err := repo.CreateMCPServerDefinition(ctx, def); err != nil {
		t.Fatal(err)
	}
	if err := repo.ReplaceMCPSelectionsAndState(ctx, mcpconfig.SelectionScopeTaskSession, def.WorkspaceID, "s", []string{def.ID}, mcpconfig.SessionMCPSelectionState{DesiredRevision: 1}); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := db.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		t.Fatal(err)
	}
	raw, err := internaldb.OpenPostgres(testutil.PostgresDSNWithSearchPath(t, dsn, schema), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := raw.Close(); err != nil {
			t.Error(err)
		}
	}()
	other := sqlx.NewDb(raw, "pgx")
	lock, err := other.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := lock.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			t.Error(err)
		}
	}()
	if _, err := lock.ExecContext(ctx, "LOCK TABLE mcp_server_definitions IN ACCESS EXCLUSIVE MODE"); err != nil {
		t.Fatal(err)
	}
	var pid int
	if err := db.GetContext(ctx, &pid, "SELECT pg_backend_pid()"); err != nil {
		t.Fatal(err)
	}
	type result struct {
		input *mcpconfig.ResolutionInputs
		err   error
	}
	done := make(chan result, 1)
	go func() {
		x, e := repo.ReadMCPResolutionInputs(ctx, mcpconfig.ResolutionContext{WorkspaceID: def.WorkspaceID, SessionID: "s"})
		done <- result{x, e}
	}()
	for {
		var waiting bool
		err := lock.GetContext(ctx, &waiting, "SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE pid=$1 AND wait_event_type='Lock')", pid)
		if err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
	// The writer can commit while the earlier reader waits on its catalog lookup.
	if _, err := lock.ExecContext(ctx, "DELETE FROM task_session_mcp_selections WHERE task_session_id='s'"); err != nil {
		t.Fatal(err)
	}
	if _, err := lock.ExecContext(ctx, "UPDATE mcp_task_session_apply_state SET desired_revision=2 WHERE task_session_id='s'"); err != nil {
		t.Fatal(err)
	}
	if err := lock.Commit(); err != nil {
		t.Fatal(err)
	}
	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	if got.input.DesiredRevision != 1 || len(got.input.SessionSelectionIDs) != 1 || got.input.SessionSelectionIDs[0] != def.ID {
		t.Fatal("selection and revision did not retain the reader's original snapshot")
	}
	setup := &mcpconfig.MCPSetupSnapshot{Version: 1, Servers: []mcpconfig.MCPSetupServer{{DefinitionID: def.ID, Revision: def.Revision, Enabled: def.Enabled, DisplayName: def.DisplayName, RuntimeName: def.RuntimeName}}}
	if err := repo.SaveAcceptedMCPSetup(ctx, "s", got.input.DesiredRevision, "old-read-attempt", setup); err != nil {
		t.Fatal(err)
	}
	state, err := repo.GetMCPSelectionState(ctx, "s")
	if err != nil || state.DesiredRevision != 2 || state.AppliedRevision != 1 || state.ApplyState != mcpconfig.SessionMCPApplyStatePendingIdle {
		t.Fatal("accepting the frozen operation falsely applied a later save")
	}
	latest, err := repo.ReadMCPResolutionInputs(ctx, mcpconfig.ResolutionContext{WorkspaceID: def.WorkspaceID, SessionID: "s"})
	if err != nil || latest.DesiredRevision != 2 || len(latest.SessionSelectionIDs) != 0 {
		t.Fatal("a fresh resolution did not observe the committed later save")
	}
}
