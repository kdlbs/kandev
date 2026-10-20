package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// sessionIncarnation reads the incarnation a block must carry to be current
// for the session (an empty queue_incarnation_id falls back to the id).
func sessionIncarnation(t *testing.T, repo *Repository, sessionID string) string {
	t.Helper()
	var incarnation string
	if err := repo.db.QueryRow(repo.db.Rebind(
		`SELECT COALESCE(NULLIF(queue_incarnation_id, ''), id) FROM task_sessions WHERE id = ?`), sessionID,
	).Scan(&incarnation); err != nil {
		t.Fatalf("read incarnation %s: %v", sessionID, err)
	}
	return incarnation
}

func seedHarnessGeneration(t *testing.T, repo *Repository, sessionID, incarnation string, generation int64) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO harness_session_generations
			(session_id, incarnation_id, generation, native_session_id, created_at, committed_at)
		VALUES (?, ?, ?, 'native', ?, ?)`), sessionID, incarnation, generation, now, now); err != nil {
		t.Fatalf("seed generation %s/%d: %v", sessionID, generation, err)
	}
}

func seedRecoveryBlock(t *testing.T, repo *Repository, id, sessionID, incarnation string, generation int64, state string, at time.Time) {
	t.Helper()
	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO session_recovery_blocks
			(id, session_id, incarnation_id, expected_generation, reason, state, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`), id, sessionID, incarnation, generation, "reason-"+id, state, at, at); err != nil {
		t.Fatalf("seed recovery block %s: %v", id, err)
	}
}

// seedRecoverySession creates a task in workspace ws with one session at the
// given state and a committed generation 2, returning the incarnation.
func seedRecoverySession(t *testing.T, repo *Repository, taskID, sessionID, ws string, state models.TaskSessionState) string {
	t.Helper()
	seedBundleTask(t, repo, taskID, ws)
	seedBundleSessionWithState(t, repo, sessionID, taskID, state)
	incarnation := sessionIncarnation(t, repo, sessionID)
	seedHarnessGeneration(t, repo, sessionID, incarnation, 1)
	seedHarnessGeneration(t, repo, sessionID, incarnation, 2)
	return incarnation
}

func TestListOpenWorkspaceRecoveryBlocks_ListsOnlyCurrentOpenBlocks(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)

	inc := seedRecoverySession(t, repo, "task-R1", "sess-R1", "ws-R", models.TaskSessionStateWaitingForInput)
	seedRecoveryBlock(t, repo, "blk-current", "sess-R1", inc, 2, models.RecoveryBlockOpen, base)
	seedRecoveryBlock(t, repo, "blk-old-generation", "sess-R1", inc, 1, models.RecoveryBlockOpen, base)
	seedRecoveryBlock(t, repo, "blk-resolved", "sess-R1", inc, 2, "resolved", base.Add(time.Second))
	seedRecoveryBlock(t, repo, "blk-other-incarnation", "sess-R1", "stale-incarnation", 2, models.RecoveryBlockOpen, base)

	terminalInc := seedRecoverySession(t, repo, "task-R2", "sess-R2", "ws-R", models.TaskSessionStateFailed)
	seedRecoveryBlock(t, repo, "blk-terminal", "sess-R2", terminalInc, 2, models.RecoveryBlockOpen, base)

	archivedInc := seedRecoverySession(t, repo, "task-R3", "sess-R3", "ws-R", models.TaskSessionStateWaitingForInput)
	seedRecoveryBlock(t, repo, "blk-archived", "sess-R3", archivedInc, 2, models.RecoveryBlockOpen, base)
	if err := repo.ArchiveTask(ctx, "task-R3"); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	otherInc := seedRecoverySession(t, repo, "task-R4", "sess-R4", "ws-other", models.TaskSessionStateWaitingForInput)
	seedRecoveryBlock(t, repo, "blk-other-workspace", "sess-R4", otherInc, 2, models.RecoveryBlockOpen, base)

	blocks, hasMore, err := repo.ListOpenWorkspaceRecoveryBlocks(ctx, "ws-R", 50)
	if err != nil {
		t.Fatalf("ListOpenWorkspaceRecoveryBlocks: %v", err)
	}
	if hasMore || len(blocks) != 1 || blocks[0].BlockID != "blk-current" {
		t.Fatalf("blocks = %+v (hasMore=%v), want exactly blk-current", blocks, hasMore)
	}
	got := blocks[0]
	if got.TaskID != "task-R1" || got.SessionID != "sess-R1" || got.TaskTitle != "bundle task" ||
		got.SessionState != string(models.TaskSessionStateWaitingForInput) || got.Reason != "reason-blk-current" {
		t.Fatalf("block = %+v, want task-R1/sess-R1 identity and fields", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatalf("CreatedAt is zero")
	}
}

func TestListOpenWorkspaceRecoveryBlocks_LimitReportsMore(t *testing.T) {
	repo := newRepoForSessionTests(t)
	base := time.Now().UTC().Add(-time.Hour)
	inc := seedRecoverySession(t, repo, "task-L1", "sess-L1", "ws-L", models.TaskSessionStateRunning)
	seedRecoveryBlock(t, repo, "blk-a", "sess-L1", inc, 2, models.RecoveryBlockOpen, base)
	seedRecoveryBlock(t, repo, "blk-b", "sess-L1", inc, 2, models.RecoveryBlockOpen, base.Add(time.Minute))

	blocks, hasMore, err := repo.ListOpenWorkspaceRecoveryBlocks(context.Background(), "ws-L", 1)
	if err != nil {
		t.Fatalf("ListOpenWorkspaceRecoveryBlocks: %v", err)
	}
	if !hasMore || len(blocks) != 1 || blocks[0].BlockID != "blk-a" {
		t.Fatalf("blocks = %+v (hasMore=%v), want blk-a with more", blocks, hasMore)
	}
	if _, _, err := repo.ListOpenWorkspaceRecoveryBlocks(context.Background(), "ws-L", 0); err == nil {
		t.Fatalf("limit 0 accepted, want error")
	}
}
