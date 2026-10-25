package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedRepairTask(t *testing.T, repo *Repository, id, parentID, wsID string, metadata map[string]interface{}, ephemeral bool, origin string) {
	t.Helper()
	task := &models.Task{
		ID: id, ParentID: parentID, WorkspaceID: wsID, WorkflowID: "wf-" + wsID, WorkflowStepID: "step",
		Title: "Task", Priority: "medium", Metadata: metadata, IsEphemeral: ephemeral, Origin: origin,
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask(%s): %v", id, err)
	}
}

func TestListOrphanRepairCandidates_FindsUnmarkedInheritParentChildOfArchivedParent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-repair-1")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-repair-1", WorkspaceID: "ws-repair-1", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair-parent", "", "ws-repair-1", nil, false, "")
	if err := repo.ArchiveTask(ctx, "repair-parent"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	seedRepairTask(t, repo, "repair-child", "repair-parent", "ws-repair-1",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, false, "")

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %d, want 1: %+v", len(candidates), candidates)
	}
	if candidates[0].TaskID != "repair-child" || candidates[0].ParentID != "repair-parent" {
		t.Fatalf("candidate = %+v, want repair-child/repair-parent", candidates[0])
	}
	if candidates[0].Workspace["mode"] != "inherit_parent" {
		t.Fatalf("candidate workspace = %+v", candidates[0].Workspace)
	}
}

func TestListOrphanRepairCandidates_ExcludesAlreadyMarked(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-repair-2")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-repair-2", WorkspaceID: "ws-repair-2", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair2-parent", "", "ws-repair-2", nil, false, "")
	if err := repo.ArchiveTask(ctx, "repair2-parent"); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair2-child", "repair2-parent", "ws-repair-2", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "repair2-parent"},
	}, false, "")

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 (already marked): %+v", len(candidates), candidates)
	}
}

// Matches the producer's own ListChildren filter: ephemeral and
// automation-origin children must not be candidates (AC-003.2 requires the
// repair and the producer to mark the same population).
func TestListOrphanRepairCandidates_ExcludesEphemeralAndAutomationOrigin(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-repair-3")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-repair-3", WorkspaceID: "ws-repair-3", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair3-parent", "", "ws-repair-3", nil, false, "")
	if err := repo.ArchiveTask(ctx, "repair3-parent"); err != nil {
		t.Fatal(err)
	}
	inheritParent := map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}
	seedRepairTask(t, repo, "repair3-ephemeral", "repair3-parent", "ws-repair-3", inheritParent, true, "")
	seedRepairTask(t, repo, "repair3-automation", "repair3-parent", "ws-repair-3", inheritParent, false, models.TaskOriginAutomationRun)

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 (ephemeral/automation excluded): %+v", len(candidates), candidates)
	}
}

func TestListOrphanRepairCandidates_ExcludesOwnEnvironment(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-repair-4")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-repair-4", WorkspaceID: "ws-repair-4", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair4-parent", "", "ws-repair-4", nil, false, "")
	if err := repo.ArchiveTask(ctx, "repair4-parent"); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair4-child", "repair4-parent", "ws-repair-4",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, false, "")
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		TaskID: "repair4-child", ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidates = %d, want 0 (own environment excluded): %+v", len(candidates), candidates)
	}
}

// A malformed metadata row that is a live child of an archived parent must
// not abort the whole selection (json_valid term order).
func TestListOrphanRepairCandidates_SkipsMalformedMetadataRowWithoutAborting(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-repair-5")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-repair-5", WorkspaceID: "ws-repair-5", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair5-parent", "", "ws-repair-5", nil, false, "")
	if err := repo.ArchiveTask(ctx, "repair5-parent"); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "repair5-good-child", "repair5-parent", "ws-repair-5",
		map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}, false, "")
	seedRepairTask(t, repo, "repair5-malformed-child", "repair5-parent", "ws-repair-5", nil, false, "")
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = '{oops' WHERE id = 'repair5-malformed-child'`); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	candidates, err := repo.ListOrphanRepairCandidates(ctx)
	if err != nil {
		t.Fatalf("ListOrphanRepairCandidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].TaskID != "repair5-good-child" {
		t.Fatalf("candidates = %+v, want exactly repair5-good-child", candidates)
	}
}

func TestListStaleOrphanMarkers_FindsMarkerNamingUnarchivedParent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-stale-1")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-1", WorkspaceID: "ws-stale-1", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale1-parent", "", "ws-stale-1", nil, false, "")
	seedRepairTask(t, repo, "stale1-child", "stale1-parent", "ws-stale-1", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "stale1-parent"},
	}, false, "")

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 1 || markers[0].TaskID != "stale1-child" {
		t.Fatalf("markers = %+v, want exactly stale1-child", markers)
	}
}

func TestListStaleOrphanMarkers_LeavesMarkerNamingStillArchivedParent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-stale-2")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-2", WorkspaceID: "ws-stale-2", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale2-parent", "", "ws-stale-2", nil, false, "")
	if err := repo.ArchiveTask(ctx, "stale2-parent"); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale2-child", "stale2-parent", "ws-stale-2", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "stale2-parent"},
	}, false, "")

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 0 {
		t.Fatalf("markers = %+v, want none (parent still archived)", markers)
	}
}

// AC-003.9a: the clearing pass must consider archived children too — an
// archived task's marker has no other revisit path.
func TestListStaleOrphanMarkers_IncludesArchivedChild(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-stale-3")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-3", WorkspaceID: "ws-stale-3", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale3-parent", "", "ws-stale-3", nil, false, "")
	seedRepairTask(t, repo, "stale3-child", "stale3-parent", "ws-stale-3", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "stale3-parent"},
	}, false, "")
	if err := repo.ArchiveTask(ctx, "stale3-child"); err != nil {
		t.Fatalf("ArchiveTask(child): %v", err)
	}

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 1 || markers[0].TaskID != "stale3-child" {
		t.Fatalf("markers = %+v, want exactly stale3-child (archived)", markers)
	}
}

// A malformed metadata row ANYWHERE in tasks must not abort the clearing
// selection's much wider blast radius (rule 2: join key and workspace
// projection both wrapped).
// orphaned_parent_id is user-writable through the generic metadata PATCH
// surface, so a claim naming a task that exists and is unarchived only in a
// DIFFERENT workspace must not be treated as a stale marker to clear — doing
// so would let a caller read that task's existence/archived-state back off
// their own task's workspace_orphaned boolean.
func TestListStaleOrphanMarkers_LeavesMarkerNamingUnarchivedTaskInDifferentWorkspace(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-stale-crossws-child")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-crossws-child", WorkspaceID: "ws-stale-crossws-child", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale-crossws-child", "", "ws-stale-crossws-child", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "stale-crossws-other-ws-task"},
	}, false, "")

	seedWorkspace(t, repo, "ws-stale-crossws-other")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-crossws-other", WorkspaceID: "ws-stale-crossws-other", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale-crossws-other-ws-task", "", "ws-stale-crossws-other", nil, false, "")

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 0 {
		t.Fatalf("markers = %+v, want none (named task exists only in a different workspace)", markers)
	}
}

func TestListStaleOrphanMarkers_SkipsMalformedMetadataRowAnywhere(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-stale-4")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-stale-4", WorkspaceID: "ws-stale-4", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "stale4-parent", "", "ws-stale-4", nil, false, "")
	seedRepairTask(t, repo, "stale4-child", "stale4-parent", "ws-stale-4", map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "stale4-parent"},
	}, false, "")
	// A malformed row unrelated to the orphan predicate at all.
	seedRepairTask(t, repo, "stale4-unrelated", "", "ws-stale-4", nil, false, "")
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = 'not json at all' WHERE id = 'stale4-unrelated'`); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	markers, err := repo.ListStaleOrphanMarkers(ctx)
	if err != nil {
		t.Fatalf("ListStaleOrphanMarkers: %v", err)
	}
	if len(markers) != 1 || markers[0].TaskID != "stale4-child" {
		t.Fatalf("markers = %+v, want exactly stale4-child", markers)
	}
}

func TestCountMalformedTaskMetadata(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-malformed-count")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-ws-malformed-count", WorkspaceID: "ws-malformed-count", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	seedRepairTask(t, repo, "mc-null", "", "ws-malformed-count", nil, false, "")
	seedRepairTask(t, repo, "mc-empty", "", "ws-malformed-count", nil, false, "")
	seedRepairTask(t, repo, "mc-nullstring", "", "ws-malformed-count", nil, false, "")
	seedRepairTask(t, repo, "mc-valid", "", "ws-malformed-count", map[string]interface{}{"workspace": map[string]interface{}{"mode": "new_workspace"}}, false, "")
	seedRepairTask(t, repo, "mc-oops", "", "ws-malformed-count", nil, false, "")
	seedRepairTask(t, repo, "mc-notjson", "", "ws-malformed-count", nil, false, "")

	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = '' WHERE id = 'mc-empty'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = 'null' WHERE id = 'mc-nullstring'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = '{oops' WHERE id = 'mc-oops'`); err != nil {
		t.Fatal(err)
	}
	if _, err := r_execRaw(repo, ctx, `UPDATE tasks SET metadata = 'not json at all' WHERE id = 'mc-notjson'`); err != nil {
		t.Fatal(err)
	}

	count, err := repo.CountMalformedTaskMetadata(ctx)
	if err != nil {
		t.Fatalf("CountMalformedTaskMetadata: %v", err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2 (the two genuinely corrupt rows)", count)
	}
}

// r_execRaw is a test-only escape hatch to corrupt a row's metadata column
// directly, simulating data that predates strict JSON writes.
func r_execRaw(repo *Repository, ctx context.Context, query string) (int64, error) {
	result, err := repo.db.ExecContext(ctx, query)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
