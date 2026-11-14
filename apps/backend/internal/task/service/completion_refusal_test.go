package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/completion"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type conversationCollaborationBoundary struct{}

func (conversationCollaborationBoundary) Enabled() bool                       { return true }
func (conversationCollaborationBoundary) IsConversation(context.Context) bool { return true }
func (conversationCollaborationBoundary) CallerTaskID(context.Context) string { return "caller-task" }
func (conversationCollaborationBoundary) Authorize(context.Context, string, models.WorkspaceCollaborationAction) error {
	return nil
}

type fakeApproverNameResolver struct{ names map[string]string }

func (r fakeApproverNameResolver) ResolvePendingApproverNames(_ context.Context, ids []string) []PendingApprover {
	pending := make([]PendingApprover, 0, len(ids))
	for _, id := range ids {
		name := r.names[id]
		if name == "" {
			name = id
		}
		pending = append(pending, PendingApprover{AgentProfileID: id, Name: name})
	}
	return pending
}

func TestTranslateCompletionRefusalResolvesSubjects(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetApproverNameResolver(fakeApproverNameResolver{names: map[string]string{"agent-ceo": "CEO"}})
	err := svc.translateCompletionError(context.Background(), &completion.RefusedError{
		Code: completion.CodeApprovalsPending, Subjects: []string{"agent-ceo", "agent-cto"},
	})
	pending, ok := err.(*ApprovalsPendingMoveError)
	if !ok {
		t.Fatalf("error = %T (%v), want ApprovalsPendingMoveError", err, err)
	}
	if len(pending.Pending) != 2 || pending.Pending[0].Name != "CEO" || pending.Pending[1].Name != "agent-cto" {
		t.Fatalf("pending = %+v, want resolved CEO and stable fallback ID", pending.Pending)
	}
}

func TestCompleteOfficeTaskAsHumanPublishesOnlyCommittedStateChanges(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "final-done-event-ws", Name: "Office"}); err != nil {
		t.Fatal(err)
	}
	workflowID, err := repo.EnsureOfficeWorkflow(ctx, "final-done-event-ws")
	if err != nil {
		t.Fatal(err)
	}
	var finalStepID string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT id FROM workflow_steps WHERE workflow_id = ? ORDER BY position DESC LIMIT 1
	`, workflowID).Scan(&finalStepID); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "final-done-event-task", WorkspaceID: "final-done-event-ws", WorkflowID: workflowID,
		WorkflowStepID: finalStepID, Title: "Final step completion", State: v1.TaskStateInProgress,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CompleteOfficeTaskAsHuman(ctx, "final-done-event-task", finalStepID); err != nil {
		t.Fatalf("CompleteOfficeTaskAsHuman: %v", err)
	}
	assertFinalStepStateEvents(t, eventBus.GetPublishedEvents(), 1)

	if _, err := svc.CompleteOfficeTaskAsHuman(ctx, "final-done-event-task", finalStepID); err != nil {
		t.Fatalf("no-op CompleteOfficeTaskAsHuman: %v", err)
	}
	assertFinalStepStateEvents(t, eventBus.GetPublishedEvents(), 1)
}

func assertFinalStepStateEvents(t *testing.T, published []*bus.Event, want int) {
	t.Helper()
	var stateEvents []*bus.Event
	for _, event := range published {
		if event.Type == events.TaskStateChanged {
			stateEvents = append(stateEvents, event)
		}
	}
	if len(stateEvents) != want {
		t.Fatalf("TaskStateChanged count = %d, want %d; events=%v", len(stateEvents), want, eventTypes(published))
	}
	if want > 0 {
		data, ok := stateEvents[0].Data.(map[string]interface{})
		if !ok || data["old_state"] != string(v1.TaskStateInProgress) {
			t.Fatalf("TaskStateChanged payload = %#v, want previous IN_PROGRESS state", stateEvents[0].Data)
		}
	}
}

func TestHumanCompletionRejectsOrphanWorkspaceDuringCollaboration(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.SetWorkspaceCollaboration(conversationCollaborationBoundary{}, nil)
	ctx := ctxAs("user-a")
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: "orphan-completion-task", WorkspaceID: "workspace-missing", Title: "Orphan task",
		State: v1.TaskStateTODO,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.officeHumanCompletionTaskSnapshot(ctx, "orphan-completion-task")
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("officeHumanCompletionTaskSnapshot error = %v, want task-not-found", err)
	}
	if updated != nil {
		t.Fatalf("officeHumanCompletionTaskSnapshot returned orphan task: %+v", updated)
	}
}
