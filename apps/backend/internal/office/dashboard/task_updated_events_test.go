package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// recordingTaskLifecyclePublisher is a fake dashboard.TaskLifecyclePublisher
// that records every PublishTaskUpdated call so tests can assert exactly
// one canonical task.updated reaches the bus per UpdateTaskStatus call.
type recordingTaskLifecyclePublisher struct {
	published []string
}

func (r *recordingTaskLifecyclePublisher) GetTask(_ context.Context, id string) (*taskmodels.Task, error) {
	return &taskmodels.Task{ID: id}, nil
}

func (r *recordingTaskLifecyclePublisher) PublishTaskUpdated(_ context.Context, task *taskmodels.Task, _ ...string) {
	r.published = append(r.published, task.ID)
}

// TestUpdateTaskStatus_PublishesCanonicalTaskUpdated pins AGENTS.md:228:
// any code path that mutates a task row must publish task.created /
// task.updated / task.deleted. UpdateTaskStatus writes the tasks row via
// s.repo.UpdateTaskState directly and, before this fix, published only the
// Office-native office.task.status_changed event — invisible to WS-driven
// UI like the All-Workflows kanban view.
func TestUpdateTaskStatus_PublishesCanonicalTaskUpdated(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "cu1", "ws-cu", "CU", "todo", 0)
	pub := &recordingTaskLifecyclePublisher{}
	deps.svc.SetTaskLifecyclePublisher(pub)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "cu1",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}

	if len(pub.published) != 1 || pub.published[0] != "cu1" {
		t.Fatalf("published = %v, want exactly one task.updated for cu1", pub.published)
	}
}

// TestUpdateTaskStatus_PublishesCanonicalTaskUpdatedWhenGateRedirects is the
// card's named DoD assertion: a gate-redirected status change (approver
// pending) still persists a state change (COMPLETED -> REVIEW redirect) and
// must still publish exactly one task.updated, even though the call itself
// returns a typed *ApprovalsPendingError.
func TestUpdateTaskStatus_PublishesCanonicalTaskUpdatedWhenGateRedirects(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "cu2", "ws-cu", "CU2", "in_progress", 0)
	mustAddParticipant(t, deps, "cu2", "agent-A", models.ParticipantRoleApprover)
	pub := &recordingTaskLifecyclePublisher{}
	deps.svc.SetTaskLifecyclePublisher(pub)

	err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "cu2",
		NewStatus: "done",
	})
	if err == nil {
		t.Fatal("expected *ApprovalsPendingError, got nil")
	}

	if len(pub.published) != 1 || pub.published[0] != "cu2" {
		t.Fatalf("published = %v, want exactly one task.updated for cu2 despite the gate redirect", pub.published)
	}
}

// TestUpdateTaskStatus_NoPublisherWiredIsSafe pins the nil-safe fallback:
// most existing DashboardService test harnesses (and any production
// deployment sequenced before main.go's wiring runs) never call
// SetTaskLifecyclePublisher, and UpdateTaskStatus must not panic.
func TestUpdateTaskStatus_NoPublisherWiredIsSafe(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "cu3", "ws-cu", "CU3", "todo", 0)

	if err := deps.svc.UpdateTaskStatus(context.Background(), dashboard.TaskStatusUpdateRequest{
		TaskID:    "cu3",
		NewStatus: "in_progress",
	}); err != nil {
		t.Fatalf("update task status: %v", err)
	}
}
