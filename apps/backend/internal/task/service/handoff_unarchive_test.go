package service

import (
	"context"
	"testing"
)

// Manual/legacy archives (empty cascade id — WS handler, MCP tool, or rows
// predating the cascade infrastructure) must be unarchivable root-only.
func TestUnarchiveTaskTree_ManualArchiveRestoresRootOnly(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addArchivedTask("root", "", "ws-1", "")
	tasks.addArchivedTask("c1", "root", "ws-1", "")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)
	pub := &fakeEventPublisher{}
	svc.SetTaskEventPublisher(pub)

	out, err := svc.UnarchiveTaskTree(context.Background(), "root")
	if err != nil {
		t.Fatalf("unarchive: %v", err)
	}
	if out.CascadeID != "" {
		t.Errorf("cascade id = %q, want empty for manual unarchive", out.CascadeID)
	}
	if len(out.ArchivedTaskIDs) != 1 || out.ArchivedTaskIDs[0] != "root" {
		t.Fatalf("unarchived = %v, want [root]", out.ArchivedTaskIDs)
	}
	root, _ := tasks.GetTask(context.Background(), "root")
	if root.ArchivedAt != nil {
		t.Error("root should be unarchived")
	}
	c1, _ := tasks.GetTask(context.Background(), "c1")
	if c1.ArchivedAt == nil {
		t.Error("c1 should remain archived (independent manual archive)")
	}
	pub.mu.Lock()
	defer pub.mu.Unlock()
	if len(pub.updated) != 1 || pub.updated[0] != "root" {
		t.Errorf("PublishTaskUpdated calls = %v, want [root]", pub.updated)
	}
}

// A non-archived root is a caller error, not a silent no-op.
func TestUnarchiveTaskTree_NotArchivedErrors(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err == nil {
		t.Fatal("expected error when unarchiving a non-archived task")
	}
}

// Cascade unarchive must publish task.updated per restored task — the WS
// handler keys off archived_at=null to put the card back on the kanban.
// Regression for the review finding that only the manual-root path
// published, leaving the UI stale after a cascade unarchive.
func TestUnarchiveTaskTree_CascadePublishesTaskUpdatedPerTask(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("root", "", "ws-1")
	tasks.addTask("c1", "root", "ws-1")
	groups := newCascadeWSGroupRepo()
	svc := newCascadeService(t, tasks, groups)
	pub := &fakeEventPublisher{}
	svc.SetTaskEventPublisher(pub)

	if _, err := svc.ArchiveTaskTree(context.Background(), "root", true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	pub.mu.Lock()
	pub.updated = nil
	pub.archived = nil
	pub.mu.Unlock()

	if _, err := svc.UnarchiveTaskTree(context.Background(), "root"); err != nil {
		t.Fatalf("unarchive: %v", err)
	}

	pub.mu.Lock()
	defer pub.mu.Unlock()
	want := map[string]bool{"root": true, "c1": true}
	for i, id := range pub.updated {
		if pub.archived[i] {
			t.Errorf("task %s published while still archived — event must carry archived_at=null", id)
		}
		delete(want, id)
	}
	if len(want) > 0 {
		t.Errorf("missing PublishTaskUpdated for: %v", want)
	}
}
