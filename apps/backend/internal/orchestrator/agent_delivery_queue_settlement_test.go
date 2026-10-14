package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestDeliveryRetentionSettlesBeforeQueuedSuccessor(t *testing.T) {
	f := newDeliveryWorkflowOutcomeFixture(t, "queued-successor", "")
	seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "queued-execution")
	observed := make(chan error, 1)
	mgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo}
	mgr.promptAgentFunc = func(ctx context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool) (*executor.PromptResult, error) {
		submission, err := f.repo.GetAgentDeliverySubmissionMetadata(ctx, f.effect.SubmissionID)
		if err == nil && submission.FullySettledAt == nil {
			err = fmt.Errorf("predecessor remains unsettled at successor dispatch: %+v", submission)
		}
		if err == nil {
			receipt, readErr := f.repo.GetAgentDeliveryEffect(ctx, f.effect.EffectKey)
			if readErr != nil || receipt.Outcome != "not_required" {
				err = fmt.Errorf("queued predecessor workflow receipt = %+v, %v", receipt, readErr)
			}
		}
		observed <- err
		return &executor.PromptResult{}, nil
	}
	f.service.agentManager = mgr
	f.service.executor = executor.NewExecutor(mgr, f.repo, testLogger(), executor.ExecutorConfig{})
	if _, err := f.service.messageQueue.QueueMessage(f.ctx, f.session.ID, f.taskID, "queued successor", "", "user", false, nil); err != nil {
		t.Fatal(err)
	}
	f.service.handleAgentReady(f.ctx, watcher.AgentEventData{
		TaskID: f.taskID, SessionID: f.session.ID, AgentExecutionID: "queued-execution", TurnID: f.effect.TurnID,
		DeliveryStreamID: f.effect.StreamID, DeliveryIncarnationID: f.effect.IncarnationID,
		DeliverySubmissionID: f.effect.SubmissionID, DeliveryHarnessGeneration: uint64(f.effect.HarnessGeneration),
		DeliverySequence: uint64(f.effect.Sequence),
	})
	select {
	case err := <-observed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued successor did not reach provider boundary")
	}
}

type queueSettlementFailureRepository struct {
	*sqliterepo.Repository
}

func (r *queueSettlementFailureRepository) RecordAgentDeliveryWorkflowOutcome(context.Context, string, *models.AgentDeliveryEffect) (bool, error) {
	return false, errors.New("injected workflow receipt write failure")
}

func TestDeliveryRetentionQueuedSettlementFailureRetainsInput(t *testing.T) {
	f := newDeliveryWorkflowOutcomeFixture(t, "queued-settlement-failure", "")
	seedExecutorRunning(t, f.repo, f.session.ID, f.taskID, "queued-execution")
	mgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: f.repo, promptDone: make(chan struct{})}
	f.service.agentManager = mgr
	f.service.executor = executor.NewExecutor(mgr, f.repo, testLogger(), executor.ExecutorConfig{})
	queued, err := f.service.messageQueue.QueueMessage(f.ctx, f.session.ID, f.taskID, "retained successor", "", "user", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.service.repo = &queueSettlementFailureRepository{Repository: f.repo}
	ctx := withWorkflowEffect(f.ctx, f.effect)
	if f.service.drainQueuedBeforeWorkflowTransition(ctx, f.taskID, f.session.ID, f.session) {
		t.Fatal("successor dispatched after receipt write failed")
	}
	status := f.service.messageQueue.GetStatus(f.ctx, f.session.ID)
	if status.Count != 1 || status.Entries[0].ID != queued.ID || status.Entries[0].Content != queued.Content {
		t.Fatalf("queue input after failed settlement = %+v", status)
	}
	select {
	case <-mgr.promptDone:
		t.Fatal("failed settlement reached the provider")
	default:
	}
	f.service.repo = f.repo
	if !f.service.drainQueuedBeforeWorkflowTransition(ctx, f.taskID, f.session.ID, f.session) {
		t.Fatal("preserved successor could not dispatch after storage recovered")
	}
	select {
	case <-mgr.promptDone:
	case <-time.After(5 * time.Second):
		t.Fatal("recovered successor did not reach provider")
	}
}
