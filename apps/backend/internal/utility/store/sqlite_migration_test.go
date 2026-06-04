package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// openTestDB opens a fresh SQLite file under t.TempDir and registers cleanup.
func openTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	tmpDir := t.TempDir()
	conn, err := db.OpenSQLite(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() {
		_ = sqlxDB.Close()
	})
	return sqlxDB
}

// TestInitSchema_MigratesLegacyClaudeCodeAgentID guards against the #1269
// regression: a stale "claude-code" agent_id (and the empty agent_id seeded
// into pre-fix built-in rows) must be rewritten to the registered inference
// agent ID "claude-acp" on schema init, so the utility-agent dialog's model
// dropdown can resolve to a valid agent.
func TestInitSchema_MigratesLegacyClaudeCodeAgentID(t *testing.T) {
	sqlxDB := openTestDB(t)

	// Bootstrap the schema once so we can seed legacy rows directly.
	if _, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("initial schema init: %v", err)
	}

	now := time.Now().UTC()
	rows := []struct {
		id, agentID string
	}{
		{"legacy-claude-code", "claude-code"},
		{"legacy-empty", ""},
		{"untouched-amp", "amp"},
	}
	for _, r := range rows {
		if _, err := sqlxDB.Exec(`INSERT OR REPLACE INTO utility_agents
			(id, name, description, prompt, agent_id, model, builtin, enabled, created_at, updated_at)
			VALUES (?, ?, '', '', ?, '', 0, 1, ?, ?)`,
			r.id, r.id, r.agentID, now, now); err != nil {
			t.Fatalf("seed %s: %v", r.id, err)
		}
	}

	// Re-run schema init — this fires the agent_id backfill migration.
	if _, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("re-init schema: %v", err)
	}

	want := map[string]string{
		"legacy-claude-code": "claude-acp",
		"legacy-empty":       "claude-acp",
		"untouched-amp":      "amp",
	}
	for id, expected := range want {
		var got string
		if err := sqlxDB.QueryRowx(`SELECT agent_id FROM utility_agents WHERE id = ?`, id).
			Scan(&got); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if got != expected {
			t.Errorf("%s: agent_id = %q, want %q", id, got, expected)
		}
	}
}

// TestSeedBuiltinAgents_UsesClaudeACP pins the seed contract: every built-in
// row inserted on first run already references the registered inference
// agent, with no reliance on the dialog fallback or the backfill migration.
func TestSeedBuiltinAgents_UsesClaudeACP(t *testing.T) {
	sqlxDB := openTestDB(t)

	if _, err := newSQLiteRepositoryWithDB(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("schema init: %v", err)
	}

	ctx := context.Background()
	rowsxDB, err := sqlxDB.QueryxContext(ctx, `SELECT id, agent_id FROM utility_agents WHERE builtin = 1`)
	if err != nil {
		t.Fatalf("query builtins: %v", err)
	}
	t.Cleanup(func() {
		if err := rowsxDB.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	})

	var count int
	for rowsxDB.Next() {
		var id, agentID string
		if err := rowsxDB.Scan(&id, &agentID); err != nil {
			t.Fatalf("scan: %v", err)
		}
		count++
		if agentID != "claude-acp" {
			t.Errorf("builtin %s: agent_id = %q, want %q", id, agentID, "claude-acp")
		}
	}
	if err := rowsxDB.Err(); err != nil {
		t.Fatalf("rows iteration error: %v", err)
	}
	if count == 0 {
		t.Fatalf("expected at least one built-in row, found 0")
	}
}
