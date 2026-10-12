package service

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newWaveIdentityTestRepo builds a minimal office repository (backed by
// the real task schema, since ListWaveMembers reads the tasks table
// taskrepo owns) for resolveWaveIdentity's own unit tests.
func newWaveIdentityTestRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := taskrepo.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("init task repo: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init office repo: %v", err)
	}
	return repo
}

func insertWaveIdentityTask(t *testing.T, repo *sqlite.Repository, ctx context.Context, id, parentID, state string) {
	t.Helper()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, title, parent_id, state, created_at, updated_at)
		VALUES (?, 'ws-1', 'Task', ?, ?, datetime('now'), datetime('now'))
	`, id, parentID, state); err != nil {
		t.Fatalf("insert task %s: %v", id, err)
	}
}

// TestResolveWaveIdentity_NonTerminalMemberReportsNotOK is
// AC-OFFICE-WAKE-WAVE-IDENTITY-002.15's regression test for P2/P3's
// shared terminality gate: resolveWaveIdentity must report ok=false (and
// derive no wave key/string at all) when any wave member has not yet
// reached a terminal state, exactly like P1's byte-identical gate in
// cascadeChildrenCompleted (reactivity.go). Before this test, only P1's
// arm of the same invariant was pinned — a regression here (e.g.
// dropping the state check, or loosening it to only COMPLETED) would ship
// P2/P3 dispatching a wave identity for a wave that has not actually
// finished.
func TestResolveWaveIdentity_NonTerminalMemberReportsNotOK(t *testing.T) {
	repo := newWaveIdentityTestRepo(t)
	ctx := context.Background()
	var log *logger.Logger

	const parentID = "parent-1"
	insertWaveIdentityTask(t, repo, ctx, parentID, "", "TODO")
	insertWaveIdentityTask(t, repo, ctx, parentID+"-child-0", parentID, "COMPLETED")
	insertWaveIdentityTask(t, repo, ctx, parentID+"-child-1", parentID, "IN_PROGRESS")

	waveKey, waveString, ok := resolveWaveIdentity(ctx, repo, parentID, log)
	if ok {
		t.Fatalf("ok = true, want false — child-1 is not terminal, so no wave identity is derivable")
	}
	if waveKey != "" || waveString != "" {
		t.Fatalf("waveKey=%q waveString=%q, want both empty when ok=false", waveKey, waveString)
	}
}

// TestResolveWaveIdentity_AllTerminalReportsOK is the control alongside
// TestResolveWaveIdentity_NonTerminalMemberReportsNotOK: once the
// remaining member also reaches a terminal state, resolveWaveIdentity
// derives a non-empty wave key/string and reports ok=true.
func TestResolveWaveIdentity_AllTerminalReportsOK(t *testing.T) {
	repo := newWaveIdentityTestRepo(t)
	ctx := context.Background()
	var log *logger.Logger

	const parentID = "parent-1"
	insertWaveIdentityTask(t, repo, ctx, parentID, "", "TODO")
	insertWaveIdentityTask(t, repo, ctx, parentID+"-child-0", parentID, "COMPLETED")
	insertWaveIdentityTask(t, repo, ctx, parentID+"-child-1", parentID, "CANCELLED")

	waveKey, waveString, ok := resolveWaveIdentity(ctx, repo, parentID, log)
	if !ok {
		t.Fatalf("ok = false, want true — every wave member is terminal (COMPLETED or CANCELLED)")
	}
	if waveKey == "" || waveString == "" {
		t.Fatalf("waveKey=%q waveString=%q, want both non-empty when ok=true", waveKey, waveString)
	}
}
