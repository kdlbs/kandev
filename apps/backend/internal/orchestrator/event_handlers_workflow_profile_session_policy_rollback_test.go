package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestSwitchSessionForStep_ParkOnEndPreservesSourceWhenPromotionFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, fixture.current.ID, "t1", "queued promotion handoff", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue promotion handoff: %v", err)
	}
	fixture.svc.messageQueue.SetPendingMove(ctx, fixture.current.ID, &messagequeue.PendingMove{
		TaskID:         "t1",
		WorkflowID:     "wf1",
		WorkflowStepID: "step-b",
	})
	promotionErr := errors.New("destination promotion failed")
	fixture.svc.repo = failProfileSwitchPromotionRepo{repoStore: fixture.repo, err: promotionErr}

	_, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy,
	}, &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy})
	if !errors.Is(err, promotionErr) {
		t.Fatalf("parked profile switch error = %v, want promotion failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after promotion failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	if source.CompletedAt != nil {
		t.Fatal("source after promotion failure must not be completed")
	}
	if _, ok := source.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("promotion failure must not leave stop metadata")
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if status.Count != 1 || status.Entries[0].Content != "queued promotion handoff" {
		t.Fatalf("source queue after promotion failure = %+v, want queued promotion handoff preserved", status.Entries)
	}
	move, exists := fixture.svc.messageQueue.GetPendingMove(ctx, fixture.current.ID)
	if !exists || move == nil || move.WorkflowStepID != "step-b" {
		t.Fatalf("source pending move after promotion failure = %+v exists=%t, want step-b preserved", move, exists)
	}
}
