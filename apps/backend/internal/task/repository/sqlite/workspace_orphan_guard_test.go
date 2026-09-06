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
