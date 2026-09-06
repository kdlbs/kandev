package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

// The guard's CAS clauses and the repair's two selections are written per
// dialect (jsonb_typeof/#>> on Postgres vs json_valid/json_extract on
// SQLite), so SQLite coverage says nothing about the PostgreSQL statements.
// These mirror the SQLite tests in workspace_orphan_guard_test.go and
// workspace_orphan_repair_test.go using the same package-level seed helpers.
// Skips unless KANDEV_TEST_POSTGRES_DSN is set.
//
// Seeding here archives the parent via a direct SQL UPDATE rather than
// repo.ArchiveTask: that method wraps a call into
// internal/orchestrator/messagequeue.PurgeTaskInTransaction, which was found
// during this testing pass to fail against a real Postgres instance
// ("commit unexpectedly resulted in rollback") even for a task with zero
// sessions and no orphan-marker involvement at all — a pre-existing gap in
// unrelated code, out of scope for this feature. The guard and repair SQL
// under test only ever consult tasks.archived_at IS NOT NULL, so setting it
// directly keeps these tests scoped to the diff under review.
func newRepoForOrphanPostgresTests(t *testing.T) *Repository {
	t.Helper()
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	return repo
}

func archiveTaskDirect(t *testing.T, repo *Repository, ctx context.Context, id string) {
	t.Helper()
	if _, err := repo.db.ExecContext(ctx, `UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = '`+id+`'`); err != nil {
		t.Fatalf("archiveTaskDirect(%s): %v", id, err)
	}
}

func seedOrphanGuardParentAndChildPG(t *testing.T, repo *Repository, ctx context.Context, parentID, childID string, parentArchived bool) {
	t.Helper()
	wsID := "ws-" + parentID
	seedWorkspace(t, repo, wsID)
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-" + parentID, WorkspaceID: wsID, Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: parentID, WorkspaceID: wsID, WorkflowID: "wf-" + parentID, WorkflowStepID: "step",
		Title: "Parent", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(parent): %v", err)
	}
	if parentArchived {
		archiveTaskDirect(t, repo, ctx, parentID)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: childID, ParentID: parentID, WorkspaceID: wsID, WorkflowID: "wf-" + parentID, WorkflowStepID: "step",
		Title: "Child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}
}

func TestPostgresSetTaskWorkspaceMetadataIfUnchanged_UncontendedMarkAndClearLand(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	const parentID, childID = "pg-owg-parent", "pg-owg-child"
	seedOrphanGuardParentAndChildPG(t, repo, ctx, parentID, childID, true)

	guard := models.ObservedWorkspaceGuard(map[string]interface{}{"mode": "inherit_parent"})
	guard.RequireParentArchivedID = parentID
	guard.RequireParentID = parentID
	guard.RequireTaskNotArchived = true
	marked := map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	}

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, marked)
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged (mark): %v", err)
	}
	if !landed {
		t.Fatal("uncontended mark did not land, want landed == true")
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	ws, _ := child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := ws["orphaned"].(bool); !orphaned {
		t.Fatalf("workspace.orphaned = %v, want true", ws["orphaned"])
	}

	if _, err := repo.UnarchiveTask(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTask: %v", err)
	}
	clearGuard := models.ObservedWorkspaceGuard(marked)
	clearGuard.RequireParentUnarchivedID = parentID
	cleared := map[string]interface{}{"mode": "inherit_parent"}

	landed, err = repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, clearGuard, cleared)
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged (clear): %v", err)
	}
	if !landed {
		t.Fatal("uncontended clear did not land, want landed == true")
	}
	child, err = repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	ws, _ = child.Metadata["workspace"].(map[string]interface{})
	if _, ok := ws["orphaned"]; ok {
		t.Fatalf("workspace.orphaned still present after clear: %v", ws)
	}
}

// The CAS must be type-aware on Postgres too: a stored non-string
// orphaned_parent_id collapses to "" via jsonb_typeof(...) = 'string' exactly
// as the SQLite json_type(...) = 'text' branch does, so a guard built from
// the Go comma-ok assertion still matches instead of losing the guard on
// every boot against a Postgres-backed install.
func TestPostgresSetTaskWorkspaceMetadataIfUnchanged_NonStringStoredClaimStillMatches(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	const parentID, childID = "pg-owg-nonstring-parent", "pg-owg-nonstring-child"
	seedOrphanGuardParentAndChildPG(t, repo, ctx, parentID, childID, true)

	if err := repo.SetTaskMetadataKey(ctx, childID, "workspace", map[string]interface{}{
		"mode": "inherit_parent", "orphaned_parent_id": 42,
	}); err != nil {
		t.Fatalf("seed non-string claim: %v", err)
	}

	guard := models.OrphanWriteGuard{
		ExpectedOrphanedParentID: "",
		ExpectedMode:             "inherit_parent",
		RequireParentArchivedID:  parentID,
		RequireParentID:          parentID,
		RequireTaskNotArchived:   true,
	}
	value := map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	}
	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, value)
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if !landed {
		t.Fatal("guard lost against a non-string stored claim, want it to match empty-string on both sides")
	}
}

// RequireParentID/RequireNoOwnEnvironment losses must also hold against the
// Postgres jsonb statement, not just the SQLite json1 one.
func TestPostgresSetTaskWorkspaceMetadataIfUnchanged_GuardLossesHold(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	const parentID, otherParentID, childID = "pg-owg-loss-parent", "pg-owg-loss-other", "pg-owg-loss-child"
	seedOrphanGuardParentAndChildPG(t, repo, ctx, parentID, childID, true)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: otherParentID, WorkspaceID: "ws-" + parentID, WorkflowID: "wf-" + parentID, WorkflowStepID: "step",
		Title: "Other root", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(other root): %v", err)
	}

	guard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireTaskNotArchived: true,
	}
	if err := repo.ReparentDirectChildren(ctx, parentID, otherParentID); err != nil {
		t.Fatalf("ReparentDirectChildren: %v", err)
	}
	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": parentID,
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("mark landed after the child was reparented away, want zero rows matched")
	}
}

func TestPostgresListOrphanRepairCandidates_FindsUnmarkedInheritParentChildOfArchivedParent(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "pg-ws-repair-1")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "pg-wf-repair-1", WorkspaceID: "pg-ws-repair-1", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "pg-repair-parent", "", "pg-ws-repair-1", nil, false, "")
	archiveTaskDirect(t, repo, ctx, "pg-repair-parent")
	seedRepairTask(t, repo, "pg-repair-child", "pg-repair-parent", "pg-ws-repair-1",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, false, "")
	seedRepairTask(t, repo, "pg-repair-ephemeral", "pg-repair-parent", "pg-ws-repair-1",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, true, "")

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].TaskID != "pg-repair-child" {
		t.Fatalf("candidates = %+v, want exactly pg-repair-child (ephemeral excluded)", candidates)
	}
	if candidates[0].Workspace["mode"] != "inherit_parent" {
		t.Fatalf("candidate workspace = %+v", candidates[0].Workspace)
	}
}

// Unlike SQLite's json_valid-guarded selections, Postgres has no equivalent
// guard here (the startup-repair design accepts this residual explicitly:
// "no writer produces malformed non-empty text there", same reasoning as
// CountMalformedTaskMetadata's dialect skip). A malformed metadata row on
// Postgres therefore surfaces as a query error rather than being silently
// excluded — this pins that accepted, documented dialect asymmetry rather
// than assuming the SQLite skip-and-continue behavior also holds here. The
// service layer above this repository method already tolerates a failed
// selection without aborting the other repair pass (see
// handoff_workspace_orphan_repair_test.go), so this residual is bounded.
func TestPostgresListOrphanRepairCandidates_MalformedMetadataRowErrorsRatherThanSkips(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "pg-ws-repair-2")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "pg-wf-repair-2", WorkspaceID: "pg-ws-repair-2", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "pg-repair2-parent", "", "pg-ws-repair-2", nil, false, "")
	archiveTaskDirect(t, repo, ctx, "pg-repair2-parent")
	seedRepairTask(t, repo, "pg-repair2-good-child", "pg-repair2-parent", "pg-ws-repair-2",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, false, "")
	seedRepairTask(t, repo, "pg-repair2-malformed-child", "pg-repair2-parent", "pg-ws-repair-2", nil, false, "")
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = '{oops' WHERE id = 'pg-repair2-malformed-child'`); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	if _, err := repo.ListOrphanRepairCandidates(ctx); err == nil {
		t.Fatal("want an error against malformed metadata on Postgres (no json_valid guard), got nil")
	}
}

func TestPostgresListStaleOrphanMarkers_FindsMarkerNamingUnarchivedParent(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "pg-ws-stale-1")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "pg-wf-stale-1", WorkspaceID: "pg-ws-stale-1", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "pg-stale1-parent", "", "pg-ws-stale-1", nil, false, "")
	seedRepairTask(t, repo, "pg-stale1-child", "pg-stale1-parent", "pg-ws-stale-1", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "pg-stale1-parent"},
	}, false, "")
	// A still-archived claim must be left alone.
	seedRepairTask(t, repo, "pg-stale1-archived-parent", "", "pg-ws-stale-1", nil, false, "")
	archiveTaskDirect(t, repo, ctx, "pg-stale1-archived-parent")
	seedRepairTask(t, repo, "pg-stale1-archived-child", "pg-stale1-archived-parent", "pg-ws-stale-1", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "pg-stale1-archived-parent"},
	}, false, "")

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 1 || markers[0].TaskID != "pg-stale1-child" {
		t.Fatalf("markers = %+v, want exactly pg-stale1-child", markers)
	}
}

// Postgres' tasks.metadata column can still hold non-JSON text (it is typed
// TEXT, cast to jsonb on read), but CountMalformedTaskMetadata deliberately
// short-circuits to 0 there rather than probing with json_valid (SQLite-only
// function) — this pins that dialect contract rather than assuming it.
func TestPostgresCountMalformedTaskMetadata_AlwaysZero(t *testing.T) {
	repo := newRepoForOrphanPostgresTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "pg-ws-malformed-count")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "pg-wf-malformed-count", WorkspaceID: "pg-ws-malformed-count", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "pg-mc-notjson", "", "pg-ws-malformed-count", nil, false, "")
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = 'not json at all' WHERE id = 'pg-mc-notjson'`); err != nil {
		t.Fatal(err)
	}

	count, err := repo.CountMalformedTaskMetadata(ctx)
	if err != nil {
		t.Fatalf("CountMalformedTaskMetadata: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (Postgres path is a deliberate no-op)", count)
	}
}
