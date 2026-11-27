package inventoryrepair

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.4
// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.5
// @covers AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.6
func TestApplyCleanupPreservesConflictDiagnosticEvidence(t *testing.T) {
	f := cleanupFixture(t)
	diagnostic := "worktree cleanup inspection failed at commit_lookup (commit_mismatch); " +
		"worktree_id=wt comparison=recorded_commit_vs_current_ref action=compare_recorded_commit"
	execSQL(t, f.db, `UPDATE task_resource_cleanup_jobs SET last_error=? WHERE id='job'`, diagnostic)
	// Keep one independent worktree in the snapshot that is absent from disk and
	// has no active inventory. It must remain protected and recorded as absent.
	var snapshot map[string]json.RawMessage
	var worktrees []map[string]any
	var stored string
	if err := f.db.QueryRow(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id='job'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(stored), &snapshot); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(snapshot["worktrees"], &worktrees); err != nil {
		t.Fatal(err)
	}
	absentPath := filepath.Join(filepath.Dir(f.plan.Repairs[0].Path), "absent-wt")
	worktrees = append(worktrees, map[string]any{
		"id": "wt-absent", "task_id": "owner", "task_environment_id": "env",
		"repository_id": "repo", "repository_path": filepath.Dir(absentPath),
		"path": absentPath, "branch": "feature/absent", "status": "active",
	})
	encoded, err := json.Marshal(worktrees)
	if err != nil {
		t.Fatal(err)
	}
	snapshot["worktrees"] = encoded
	storedBytes, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	execSQL(t, f.db, `UPDATE task_resource_cleanup_jobs SET resource_snapshot=? WHERE id='job'`, string(storedBytes))
	p := observedPlan(t, f)
	var originalSnapshot, originalError string
	if err := f.db.QueryRow(
		`SELECT resource_snapshot,last_error FROM task_resource_cleanup_jobs WHERE id='job'`,
	).Scan(&originalSnapshot, &originalError); err != nil {
		t.Fatal(err)
	}
	if originalError != diagnostic {
		t.Fatalf("preview lost the conflict diagnostic: %q", originalError)
	}

	if err := repairWithIsolatedHost(p, "apply"); err != nil {
		t.Fatal(err)
	}

	var state, preservedSnapshot, preservedError string
	if err := f.db.QueryRow(
		`SELECT state,resource_snapshot,last_error FROM task_resource_cleanup_jobs WHERE id='job'`,
	).Scan(&state, &preservedSnapshot, &preservedError); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" {
		t.Fatalf("predecessor state = %q, want cancelled", state)
	}
	if preservedSnapshot != originalSnapshot {
		t.Fatal("predecessor snapshot changed during explicit reconciliation")
	}
	if preservedError != diagnostic {
		t.Fatalf("predecessor diagnostic = %q, want %q", preservedError, diagnostic)
	}

	var successor string
	if err := f.db.QueryRow(
		`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE state='pending'`,
	).Scan(&successor); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(successor, `"inventory_repair"`) {
		t.Fatalf("successor lacks linked supersession evidence: %s", successor)
	}
	if !strings.Contains(successor, `"previously-removed"`) {
		t.Fatalf("successor lost independent progress evidence: %s", successor)
	}
	var successorSnapshot map[string]json.RawMessage
	if err := json.Unmarshal([]byte(successor), &successorSnapshot); err != nil {
		t.Fatal(err)
	}
	var repairEvidence struct {
		AbsentWorktreeIDs []string `json:"absent_worktree_ids"`
	}
	if err := json.Unmarshal(successorSnapshot["inventory_repair"], &repairEvidence); err != nil {
		t.Fatal(err)
	}
	if len(repairEvidence.AbsentWorktreeIDs) != 1 || repairEvidence.AbsentWorktreeIDs[0] != "wt-absent" {
		t.Fatalf("independent absent worktree was not protected: %+v", repairEvidence.AbsentWorktreeIDs)
	}
}
