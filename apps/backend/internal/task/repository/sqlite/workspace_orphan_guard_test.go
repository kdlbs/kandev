package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func seedOrphanGuardParentAndChild(t *testing.T, repo *Repository, parentID, childID string, parentArchived bool) {
	t.Helper()
	ctx := context.Background()
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
		if err := repo.ArchiveTask(ctx, parentID); err != nil {
			t.Fatalf("ArchiveTask(parent): %v", err)
		}
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: childID, ParentID: parentID, WorkspaceID: wsID, WorkflowID: "wf-" + parentID, WorkflowStepID: "step",
		Title: "Child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}
}

func TestSetTaskWorkspaceMetadataIfUnchanged_UncontendedMarkLands(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-mark-parent", "owg-mark-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	guard := models.ObservedWorkspaceGuard(map[string]interface{}{"mode": "inherit_parent"})
	guard.RequireParentArchivedID = parentID
	guard.RequireParentID = parentID
	guard.RequireTaskNotArchived = true
	value := map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	}

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, value)
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
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
}

func TestSetTaskWorkspaceMetadataIfUnchanged_UncontendedClearLands(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-clear-parent", "owg-clear-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	markGuard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireTaskNotArchived: true,
	}
	marked := map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	}
	if landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, markGuard, marked); err != nil {
		t.Fatalf("mark: %v", err)
	} else if !landed {
		t.Fatal("setup failure: initial mark did not land")
	}
	if _, err := repo.UnarchiveTask(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTask: %v", err)
	}

	clearGuard := models.ObservedWorkspaceGuard(marked)
	clearGuard.RequireParentUnarchivedID = parentID
	cleared := map[string]interface{}{"mode": "inherit_parent"}

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, clearGuard, cleared)
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged (clear): %v", err)
	}
	if !landed {
		t.Fatal("uncontended clear did not land, want landed == true")
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	ws, _ := child.Metadata["workspace"].(map[string]interface{})
	if _, ok := ws["orphaned"]; ok {
		t.Fatalf("workspace.orphaned still present after clear: %v", ws)
	}
}

// The CAS must be type-aware: a stored non-string orphaned_parent_id (task
// metadata is user-writable) collapses to "" on the SQL side exactly as the
// Go comma-ok assertion does, so a guard built from that same assertion
// still matches instead of losing the guard on every boot.
func TestSetTaskWorkspaceMetadataIfUnchanged_NonStringStoredClaimStillMatches(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-nonstring-parent", "owg-nonstring-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	// Simulate a hand-written non-string orphaned_parent_id via the plain
	// metadata setter (bypassing the guard entirely, as a PATCH would).
	if err := repo.SetTaskMetadataKey(ctx, childID, "workspace", map[string]interface{}{
		"mode": "inherit_parent", "orphaned_parent_id": 42,
	}); err != nil {
		t.Fatalf("seed non-string claim: %v", err)
	}

	guard := models.OrphanWriteGuard{
		ExpectedOrphanedParentID: "", // comma-ok assertion on a number yields ""
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

// RequireParentID closes the window where ReparentDirectChildren (a bare
// UPDATE with no metadata touch) moves a child off the archived parent
// between selection and write.
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireParentID_LosesGuardAfterReparent(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, otherParentID, childID = "owg-reparent-parent", "owg-reparent-other", "owg-reparent-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)
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

// The CAS itself, not just the Require* clauses, must reject a losing
// writer: a guard built from a stale (pre-mark) observation must not land
// over an already-marked row, and the stored marker must survive untouched.
// Every other "loses guard" test in this file loses on a Require* clause —
// this is the only one that isolates the CAS comparison, which a guard
// missing every Require* clause (an already-marked row's first-time-mark
// stamp arriving late) would still have to reject on its own.
func TestSetTaskWorkspaceMetadataIfUnchanged_StaleClaimLosesGuardAndLeavesMarkerUntouched(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-staleclaim-parent", "owg-staleclaim-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	markGuard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireTaskNotArchived: true,
	}
	marked := map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	}
	if landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, markGuard, marked); err != nil {
		t.Fatalf("setup mark: %v", err)
	} else if !landed {
		t.Fatal("setup failure: initial mark did not land")
	}

	// A second writer that read the child before the mark above landed
	// builds its guard from a stale, unmarked observation (ExpectedOrphanedParentID
	// == ""). Every Require* clause it carries independently holds (same
	// archived parent, same parent_id, child not archived), so only the CAS
	// comparison itself can stop this write from overwriting the marker
	// above with a different orphaned_parent_id.
	staleGuard := models.OrphanWriteGuard{
		ExpectedOrphanedParentID: "", ExpectedMode: "inherit_parent",
		RequireParentArchivedID: parentID, RequireParentID: parentID, RequireTaskNotArchived: true,
	}
	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, staleGuard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": "late-writer-race", "orphaned_at": "2026-09-05T00:00:01Z",
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("stale-claim write landed over an existing marker, want the CAS to reject it")
	}

	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	ws, _ := child.Metadata["workspace"].(map[string]interface{})
	if got, _ := ws["orphaned_parent_id"].(string); got != parentID {
		t.Fatalf("orphaned_parent_id = %q after a losing writer, want untouched %q", got, parentID)
	}
}

// AC-005.2: a concurrent mode change must not be lost. The guard is built
// from the workspace map as read; if a second writer changes mode before
// this write lands, the CAS must reject it and the concurrently written
// mode must survive rather than being silently reverted.
func TestSetTaskWorkspaceMetadataIfUnchanged_ConcurrentModeChangeLosesGuardAndModeSurvives(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-modechange-parent", "owg-modechange-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	// Guard captured from the workspace map as read, before any concurrent
	// writer touches it — mirrors models.ObservedWorkspaceGuard's contract.
	guard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireTaskNotArchived: true,
	}

	// A concurrent writer (standing in for the delete path's normalize, or
	// the generic metadata PATCH surface) flips mode off inherit_parent
	// between this caller's read and its write.
	if err := repo.SetTaskMetadataKey(ctx, childID, "workspace", map[string]interface{}{"mode": "shared_group"}); err != nil {
		t.Fatalf("simulate concurrent mode change: %v", err)
	}

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true,
		"orphaned_reason": "parent_archived", "orphaned_parent_id": parentID, "orphaned_at": "2026-09-05T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("mark landed after a concurrent mode change, want the CAS to reject it")
	}

	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	ws, _ := child.Metadata["workspace"].(map[string]interface{})
	if mode, _ := ws["mode"].(string); mode != "shared_group" {
		t.Fatalf("workspace.mode = %q after a losing writer, want the concurrently written %q preserved", mode, "shared_group")
	}
	if _, ok := ws["orphaned"]; ok {
		t.Fatalf("workspace.orphaned present after a write that should have lost its guard: %v", ws)
	}
}

// RequireParentArchivedID must not match a task that exists and is
// archived, but lives in a DIFFERENT workspace — the mark-side mirror of
// RequireParentUnarchivedID_DoesNotMatchAcrossWorkspaces below, closing the
// same class of gap on the EXISTS clause RVW-F2's fix added to the stamping
// side. RequireParentID is deliberately omitted here so the archived-branch
// EXISTS clause is the only thing that can reject the write, isolating it
// from the plain parent_id column check.
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireParentArchivedID_DoesNotMatchAcrossWorkspaces(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const childID, otherTaskID = "owg-archcrossws-child", "owg-archcrossws-other-ws-task"

	seedWorkspace(t, repo, "ws-owg-archcrossws-child")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owg-archcrossws-child", WorkspaceID: "ws-owg-archcrossws-child", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: childID, WorkspaceID: "ws-owg-archcrossws-child", WorkflowID: "wf-owg-archcrossws-child", WorkflowStepID: "step",
		Title: "Child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	// Exists and archived, but in a different workspace.
	seedWorkspace(t, repo, "ws-owg-archcrossws-other")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owg-archcrossws-other", WorkspaceID: "ws-owg-archcrossws-other", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: otherTaskID, WorkspaceID: "ws-owg-archcrossws-other", WorkflowID: "wf-owg-archcrossws-other", WorkflowStepID: "step",
		Title: "Archived task in another workspace", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(other-workspace task): %v", err)
	}
	if err := repo.ArchiveTask(ctx, otherTaskID); err != nil {
		t.Fatalf("ArchiveTask(other-workspace task): %v", err)
	}

	guard := models.ObservedWorkspaceGuard(map[string]interface{}{"mode": "inherit_parent"})
	guard.RequireParentArchivedID = otherTaskID

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": otherTaskID,
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("mark landed against a parent that exists and is archived only in a different workspace, want zero rows matched")
	}
}

// RequireParentUnarchivedID protects the clearing side: a parent re-archived
// after selection must not have a fresh, valid marker stripped.
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireParentUnarchivedID_LosesGuardAfterReArchive(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-rearchive-parent", "owg-rearchive-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, false)

	clearGuard := models.OrphanWriteGuard{ExpectedMode: "inherit_parent", RequireParentUnarchivedID: parentID}

	if err := repo.ArchiveTask(ctx, parentID); err != nil {
		t.Fatalf("ArchiveTask(parent): %v", err)
	}

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, clearGuard, map[string]interface{}{"mode": "inherit_parent"})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("clear landed after the parent was re-archived, want zero rows matched")
	}
}

// RequireParentUnarchivedID must not match a task that exists and is
// unarchived, but lives in a DIFFERENT workspace: orphaned_parent_id is
// user-writable through the generic metadata PATCH surface, so an unscoped
// EXISTS here turns the guard into a cross-workspace existence/archived-state
// oracle (a user can point their own task's claim at any task ID and read the
// answer back off their own task's workspace_orphaned boolean).
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireParentUnarchivedID_DoesNotMatchAcrossWorkspaces(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const childID, otherTaskID = "owg-crossws-child", "owg-crossws-other-ws-task"

	seedWorkspace(t, repo, "ws-owg-crossws-child")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owg-crossws-child", WorkspaceID: "ws-owg-crossws-child", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: childID, WorkspaceID: "ws-owg-crossws-child", WorkflowID: "wf-owg-crossws-child", WorkflowStepID: "step",
		Title: "Child", Priority: "medium",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{
			"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": otherTaskID,
		}},
	}); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	// Exists, unarchived, and unrelated — the shape a user could name via the
	// generic metadata PATCH surface without ever having access to it.
	seedWorkspace(t, repo, "ws-owg-crossws-other")
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-owg-crossws-other", WorkspaceID: "ws-owg-crossws-other", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: otherTaskID, WorkspaceID: "ws-owg-crossws-other", WorkflowID: "wf-owg-crossws-other", WorkflowStepID: "step",
		Title: "Unrelated task in another workspace", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(other-workspace task): %v", err)
	}

	guard := models.ObservedWorkspaceGuard(map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": otherTaskID,
	})
	guard.RequireParentUnarchivedID = otherTaskID

	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{"mode": "inherit_parent"})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("clear landed against a task that exists only in a different workspace, want zero rows matched")
	}
}

// RequireNoOwnEnvironment protects mark site 2 (and the repair): a child
// that acquires its own task_environments row between the Go lookup and the
// write must not be stamped.
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireNoOwnEnvironment_LosesGuardOnceEnvironmentExists(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-ownenv-parent", "owg-ownenv-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		TaskID: childID, ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	guard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireNoOwnEnvironment: true, RequireTaskNotArchived: true,
	}
	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": parentID,
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("mark landed for a child that acquired its own environment, want zero rows matched")
	}
}

// RequireTaskNotArchived protects every stamping path: a child archived
// between selection and write must not be stamped.
func TestSetTaskWorkspaceMetadataIfUnchanged_RequireTaskNotArchived_LosesGuardOnceChildArchived(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	const parentID, childID = "owg-childarchived-parent", "owg-childarchived-child"
	seedOrphanGuardParentAndChild(t, repo, parentID, childID, true)

	if err := repo.ArchiveTask(ctx, childID); err != nil {
		t.Fatalf("ArchiveTask(child): %v", err)
	}

	guard := models.OrphanWriteGuard{
		ExpectedMode: "inherit_parent", RequireParentArchivedID: parentID,
		RequireParentID: parentID, RequireTaskNotArchived: true,
	}
	landed, err := repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, guard, map[string]interface{}{
		"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": parentID,
	})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged: %v", err)
	}
	if landed {
		t.Fatal("mark landed for an archived child, want zero rows matched")
	}

	// The clearing paths must NOT set RequireTaskNotArchived: a clear must
	// still land on an archived child (AC-003.9a).
	clearGuard := models.OrphanWriteGuard{ExpectedMode: "inherit_parent", RequireParentUnarchivedID: parentID}
	if _, err := repo.UnarchiveTask(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTask(parent): %v", err)
	}
	landed, err = repo.SetTaskWorkspaceMetadataIfUnchanged(ctx, childID, clearGuard, map[string]interface{}{"mode": "inherit_parent"})
	if err != nil {
		t.Fatalf("SetTaskWorkspaceMetadataIfUnchanged (clear on archived child): %v", err)
	}
	if !landed {
		t.Fatal("clear on an archived child did not land, want landed == true (AC-003.9a)")
	}
}
