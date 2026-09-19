package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
)

// REGRESSION: HandoffService.ArchiveTaskTree is the archive path the WS
// kanban board action and the HTTP archive endpoint actually take whenever a
// HandoffService is wired (always, in production) — see wsArchiveTask /
// httpArchiveTask. Before this fix, only Service.ArchiveTask (reachable
// solely via the MCP archive_task_kandev tool) stamped the orphan marker on
// an unmaterialized inherit_parent child, so the primary user-facing archive
// entry points never marked anything.
func TestHandoffArchiveTaskTree_MarksUnmaterializedInheritParentChildOrphaned(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-handoff-archive-orphan-parent"
	const childID = "task-handoff-archive-orphan-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)

	if _, err := handoff.ArchiveTaskTree(ctx, parentID, false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}

	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if workspace == nil {
		t.Fatal("child metadata.workspace missing after archive")
	}
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatalf("child metadata.workspace.orphaned = %v, want true", workspace["orphaned"])
	}
	if reason, _ := workspace["orphaned_reason"].(string); reason != "parent_archived" {
		t.Fatalf("child metadata.workspace.orphaned_reason = %q, want %q", reason, "parent_archived")
	}
	if gotParentID, _ := workspace["orphaned_parent_id"].(string); gotParentID != parentID {
		t.Fatalf("child metadata.workspace.orphaned_parent_id = %q, want %q", gotParentID, parentID)
	}
	if _, ok := workspace["orphaned_at"].(string); !ok {
		t.Fatal("child metadata.workspace.orphaned_at missing after archive")
	}

	found := false
	for _, evt := range bus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["task_id"] == childID {
			found = true
		}
	}
	if !found {
		t.Fatal("no task.updated event published for orphaned child")
	}
}

// REGRESSION (round-trip, cascade-stamped root): the orphan marker was a
// one-way write with no code path that ever cleared it. Archiving a parent
// stamps metadata.workspace.orphaned=true/orphaned_reason="parent_archived"
// on its inherit_parent children, but the archive-time cleanup job that
// actually removes the parent's worktree runs asynchronously and can be
// cancelled by an unarchive before it ever fires (see
// resource_cleanup_jobs.go's cancelIfTaskUnarchived). Restoring the parent
// through HandoffService.UnarchiveTaskTree (the only unarchive entry point —
// see httpUnarchiveTask) must clear the marker on every affected child and
// publish task.updated so the UI drops the stale orphan banner.
func TestHandoffUnarchiveTaskTree_ClearsOrphanMarkerOnChildren(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-handoff-unarchive-orphan-parent"
	const childID = "task-handoff-unarchive-orphan-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)

	if _, err := handoff.ArchiveTaskTree(ctx, parentID, false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after archive: %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatal("setup failure: child was not marked orphaned by archive")
	}

	bus.ClearEvents()
	if _, err := handoff.UnarchiveTaskTree(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTaskTree: %v", err)
	}

	child, err = repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after unarchive: %v", err)
	}
	workspace, _ = child.Metadata["workspace"].(map[string]interface{})
	if workspace == nil {
		t.Fatal("child metadata.workspace missing after unarchive")
	}
	for _, key := range []string{"orphaned", "orphaned_reason", "orphaned_parent_id", "orphaned_at"} {
		if _, ok := workspace[key]; ok {
			t.Errorf("child metadata.workspace[%q] = %v, want cleared after parent unarchive", key, workspace[key])
		}
	}
	if workspace["mode"] != "inherit_parent" {
		t.Fatalf("child metadata.workspace.mode changed to %v, want unchanged %q", workspace["mode"], "inherit_parent")
	}

	found := false
	for _, evt := range bus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["task_id"] == childID {
			found = true
		}
	}
	if !found {
		t.Fatal("no task.updated event published for un-orphaned child")
	}
}

// REGRESSION (round-trip, manual root): a task archived via the MCP
// archive_task_kandev tool (Service.ArchiveTask, no cascade ID stamped)
// takes UnarchiveTaskTree's unarchiveManualRoot branch on restore, a
// separate code path from the cascade branch above. It must clear the same
// orphan marker.
func TestHandoffUnarchiveTaskTree_ManualRoot_ClearsOrphanMarkerOnChildren(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-handoff-unarchive-manual-orphan-parent"
	const childID = "task-handoff-unarchive-manual-orphan-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	if err := svc.ArchiveTask(ctx, parentID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after archive: %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatal("setup failure: child was not marked orphaned by archive")
	}
	root, err := repo.GetTask(ctx, parentID)
	if err != nil {
		t.Fatalf("GetTask(parent): %v", err)
	}
	if root.ArchivedByCascadeID != "" {
		t.Fatalf("setup failure: expected manual archive with no cascade id, got %q", root.ArchivedByCascadeID)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)
	bus.ClearEvents()

	if _, err := handoff.UnarchiveTaskTree(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTaskTree: %v", err)
	}

	child, err = repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after unarchive: %v", err)
	}
	workspace, _ = child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); orphaned {
		t.Fatal("child metadata.workspace.orphaned still true after manual-root unarchive")
	}
	for _, key := range []string{"orphaned_reason", "orphaned_parent_id", "orphaned_at"} {
		if _, ok := workspace[key]; ok {
			t.Errorf("child metadata.workspace[%q] still present after manual-root unarchive", key)
		}
	}

	found := false
	for _, evt := range bus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["task_id"] == childID {
			found = true
		}
	}
	if !found {
		t.Fatal("no task.updated event published for un-orphaned child (manual root path)")
	}
}

// REGRESSION: clearOrphanedInheritParentChildren used to call ListChildren,
// which (matching the sqlite ListChildren SQL filter, `archived_at IS
// NULL`) excludes archived rows. A child archived independently BETWEEN its
// parent's archive and the parent's unarchive was therefore invisible to
// the clear — and nothing else ever revisits it afterward: the child's own
// later unarchive scopes clearOrphanedInheritParentChildren to the CHILD's
// own children, never the child itself. The marker would survive
// permanently, falsely asserting parent_archived about a parent that is now
// restored. Fixed by switching the clear path to
// ListChildrenIncludingArchived (the mark path deliberately stays on
// ListChildren — see the comment on clearOrphanedInheritParentChildren).
func TestHandoffUnarchiveTaskTree_ClearsOrphanMarkerOnArchivedChild(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-handoff-unarchive-archived-child-parent"
	const childID = "task-handoff-unarchive-archived-child-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)

	if _, err := handoff.ArchiveTaskTree(ctx, parentID, false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after parent archive: %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatal("setup failure: child was not marked orphaned by parent archive")
	}

	// The child is archived independently — e.g. its own Done step's
	// auto_archive_after_hours fired — BEFORE the parent is restored.
	if err := svc.ArchiveTask(ctx, childID); err != nil {
		t.Fatalf("ArchiveTask(child): %v", err)
	}

	bus.ClearEvents()
	if _, err := handoff.UnarchiveTaskTree(ctx, parentID); err != nil {
		t.Fatalf("UnarchiveTaskTree: %v", err)
	}

	child, err = repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after parent unarchive: %v", err)
	}
	if child.ArchivedAt == nil {
		t.Fatal("child was unexpectedly unarchived by the parent's cascade unarchive")
	}
	workspace, _ = child.Metadata["workspace"].(map[string]interface{})
	if workspace == nil {
		t.Fatal("child metadata.workspace missing after parent unarchive")
	}
	for _, key := range []string{"orphaned", "orphaned_reason", "orphaned_parent_id", "orphaned_at"} {
		if _, ok := workspace[key]; ok {
			t.Errorf("archived child metadata.workspace[%q] = %v, want cleared after parent unarchive", key, workspace[key])
		}
	}

	found := false
	for _, evt := range bus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["task_id"] == childID {
			found = true
		}
	}
	if !found {
		t.Fatal("no task.updated event published for un-orphaned archived child")
	}
}

// A child orphaned by an EARLIER, still-archived parent must not be
// disturbed by unarchiving an unrelated parent — clearing is scoped to
// children whose orphaned_parent_id matches the task actually restored.
func TestHandoffUnarchiveTaskTree_DoesNotClearOtherParentsOrphanMarker(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const parentAID = "task-handoff-unarchive-scope-parent-a"
	const parentBID = "task-handoff-unarchive-scope-parent-b"
	const childID = "task-handoff-unarchive-scope-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentAID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentAID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskEventPublisher(svc)

	if _, err := handoff.ArchiveTaskTree(ctx, parentAID, false); err != nil {
		t.Fatalf("ArchiveTaskTree: %v", err)
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if gotParentID, _ := workspace["orphaned_parent_id"].(string); gotParentID != parentAID {
		t.Fatalf("child metadata.workspace.orphaned_parent_id = %q, want %q", gotParentID, parentAID)
	}

	// Simulate clearing on behalf of a different (unrelated) parent ID —
	// this must be a no-op because the marker names parentAID, not
	// parentBID.
	handoff.clearOrphanedInheritParentChildren(ctx, parentBID)

	child, err = repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child) after unrelated clear: %v", err)
	}
	workspace, _ = child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatal("child orphan marker was cleared by an unrelated parent ID")
	}
}
