package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestDeliveryRetentionWorkflowOutcome(t *testing.T) {
	t.Run("successful workflow transition records the applied receipt", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "applied", "applied-step")
		stepTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		fixture.installStep(t, &wfmodels.WorkflowStep{
			ID: "applied-step", WorkflowID: "wf1", Position: 0, UpdatedAt: stepTime,
			Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type:   wfmodels.OnTurnCompleteMoveToStep,
				Config: map[string]interface{}{"step_id": "applied-next"},
			}}},
		})
		fixture.installStep(t, &wfmodels.WorkflowStep{
			ID: "applied-next", WorkflowID: "wf1", Position: 1, UpdatedAt: stepTime,
		})
		transitioned := fixture.service.processOnTurnCompleteViaEngineWithCause(
			withWorkflowEffect(fixture.ctx, fixture.effect), fixture.taskID, fixture.session, turnCompletionCauseAgentTurn,
		)
		if !transitioned {
			t.Fatal("eligible bound workflow transition did not run")
		}
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		if err != nil || task.WorkflowStepID != "applied-next" {
			t.Fatalf("workflow transition step = %q, err %v", task.WorkflowStepID, err)
		}
		stored, err := fixture.repo.GetAgentDeliveryEffect(fixture.ctx, fixture.effect.EffectKey)
		if err != nil || stored.Outcome != "applied" || stored.StreamID != fixture.effect.StreamID ||
			stored.SubmissionID != fixture.effect.SubmissionID || stored.TurnID != fixture.effect.TurnID {
			t.Fatalf("applied workflow receipt = %+v, err %v", stored, err)
		}
		if err := fixture.service.reconcileAgentDeliverySettlements(fixture.ctx, fixture.session.ID); err != nil {
			t.Fatalf("reconcile applied workflow receipt: %v", err)
		}
		submission, err := fixture.repo.GetAgentDeliverySubmissionMetadata(fixture.ctx, fixture.effect.SubmissionID)
		if err != nil || submission.FullySettledAt == nil {
			t.Fatalf("applied workflow terminal settlement = %+v, err %v", submission, err)
		}
	})

	t.Run("no workflow records not required", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "no-workflow", "")
		fixture.run(t, "not_required", turnCompletionCauseAgentTurn)
	})

	t.Run("quick chat records not required", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "quick-chat", "quick-chat-step")
		if _, err := fixture.repo.DB().ExecContext(fixture.ctx,
			`UPDATE tasks SET is_ephemeral = 1 WHERE id = ?`, fixture.taskID); err != nil {
			t.Fatalf("mark quick-chat task ephemeral: %v", err)
		}
		stepTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		fixture.installStep(t, &wfmodels.WorkflowStep{ID: "quick-chat-step", WorkflowID: "wf1", UpdatedAt: stepTime})
		fixture.run(t, "not_required", turnCompletionCauseAgentTurn)
	})

	t.Run("Office scheduler terminal without workflow records not required", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "office-scheduler", "office-step")
		if _, err := fixture.repo.DB().ExecContext(fixture.ctx,
			`UPDATE tasks SET project_id = ?, workflow_step_id = '' WHERE id = ?`, "project-office", fixture.taskID); err != nil {
			t.Fatalf("mark scheduler-owned Office task: %v", err)
		}
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		if err != nil || !task.IsFromOffice {
			t.Fatalf("Office task classification = %+v, err %v", task, err)
		}
		fixture.run(t, "not_required", turnCompletionCauseAgentTurn)
	})

	t.Run("no automatic action records a guarded no-op", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "no-action", "no-action-step")
		stepTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		fixture.installStep(t, &wfmodels.WorkflowStep{
			ID: "no-action-step", WorkflowID: "wf1", UpdatedAt: stepTime,
		})
		fixture.run(t, "no_automatic_transition", turnCompletionCauseAgentTurn)
	})

	t.Run("approval-skipped action records a guarded no-op without moving", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "approval-skipped", "approval-step")
		stepTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		fixture.installStep(t, &wfmodels.WorkflowStep{
			ID: "approval-step", WorkflowID: "wf1", Position: 0, UpdatedAt: stepTime,
			Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type:   wfmodels.OnTurnCompleteMoveToNext,
				Config: map[string]interface{}{"requires_approval": true},
			}}},
		})
		fixture.setStep(t, &wfmodels.WorkflowStep{ID: "approval-next", WorkflowID: "wf1", Position: 1})
		fixture.run(t, "no_automatic_transition", turnCompletionCauseAgentTurn)
		task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
		if err != nil || task.WorkflowStepID != "approval-step" {
			t.Fatalf("approval-skipped action moved task to %q, err %v", task.WorkflowStepID, err)
		}
	})

	t.Run("missing completion signal stays protected", func(t *testing.T) {
		fixture := newDeliveryWorkflowOutcomeFixture(t, "missing-signal", "signal-step")
		stepTime := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
		fixture.installStep(t, &wfmodels.WorkflowStep{
			ID: "signal-step", WorkflowID: "wf1", Position: 0, UpdatedAt: stepTime,
			AutoAdvanceRequiresSignal: true,
			Events:                    wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}}},
		})
		fixture.setStep(t, &wfmodels.WorkflowStep{ID: "signal-next", WorkflowID: "wf1", Position: 1})
		fixture.run(t, "", turnCompletionCauseAgentTurn)
		submission, err := fixture.repo.GetAgentDeliverySubmissionMetadata(fixture.ctx, fixture.effect.SubmissionID)
		if err != nil || submission.FullySettledAt != nil || submission.PayloadPrunedAt != nil {
			t.Fatalf("deferred terminal became retention-eligible: %+v, err %v", submission, err)
		}
	})
}

type deliveryWorkflowOutcomeFixture struct {
	ctx     context.Context
	repo    *sqliterepo.Repository
	service *Service
	taskID  string
	session *models.TaskSession
	effect  *models.AgentDeliveryEffect
	step    *mockStepGetter
}

func newDeliveryWorkflowOutcomeFixture(t *testing.T, suffix, stepID string) *deliveryWorkflowOutcomeFixture {
	t.Helper()
	ctx := context.Background()
	taskID, sessionID := "retention-workflow-task-"+suffix, "retention-workflow-session-"+suffix
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, stepID)
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load retention workflow session: %v", err)
	}
	incarnationID := session.QueueIncarnationID
	if incarnationID == "" {
		incarnationID = session.ID
	}
	submissionID := "prompt:" + suffix
	if _, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "dispatch-" + suffix,
		PayloadHash: "payload-" + suffix, Payload: []byte("retained prompt"),
		State: models.DeliverySubmissionDispatching,
	}); err != nil {
		t.Fatalf("prepare retained workflow submission: %v", err)
	}
	streamID := "retention-workflow-stream-" + suffix
	event := persistedTerminalEvent(sessionID, incarnationID, 1, submissionID, streamID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	effect := &models.AgentDeliveryEffect{
		EffectKey:  "workflow.on_turn_complete:" + eventTurnID(event),
		EffectType: "workflow.on_turn_complete", State: models.DeliveryEffectCompleted,
		TaskID: taskID, SessionID: sessionID, TurnID: eventTurnID(event),
		StreamID: streamID, Sequence: 1, IncarnationID: incarnationID,
		HarnessGeneration: 1, SubmissionID: submissionID,
	}
	stepGetter := newMockStepGetter()
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[taskID] = &v1.Task{ID: taskID, State: v1.TaskStateInProgress}
	service := createEngineServiceWithTaskRepo(t, repo, stepGetter, taskRepo, &mockAgentManager{})
	return &deliveryWorkflowOutcomeFixture{
		ctx: ctx, repo: repo, service: service, taskID: taskID, session: session,
		effect: effect, step: stepGetter,
	}
}

func (f *deliveryWorkflowOutcomeFixture) setStep(t *testing.T, step *wfmodels.WorkflowStep) {
	t.Helper()
	f.step.steps[step.ID] = step
}

func (f *deliveryWorkflowOutcomeFixture) installStep(t *testing.T, step *wfmodels.WorkflowStep) {
	t.Helper()
	if _, err := f.repo.DB().ExecContext(f.ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, step.ID, step.WorkflowID, step.ID, step.Position,
		step.UpdatedAt, step.UpdatedAt); err != nil {
		t.Fatalf("insert workflow policy step: %v", err)
	}
	f.setStep(t, step)
}

func (f *deliveryWorkflowOutcomeFixture) run(t *testing.T, expectedOutcome string, cause turnCompletionCause) {
	t.Helper()
	transitioned := f.service.processOnTurnCompleteViaEngineWithCause(
		withWorkflowEffect(f.ctx, f.effect), f.taskID, f.session, cause,
	)
	if transitioned {
		t.Fatal("workflow outcome evaluation unexpectedly transitioned")
	}
	stored, err := f.repo.GetAgentDeliveryEffect(f.ctx, f.effect.EffectKey)
	if expectedOutcome == "" {
		if !errors.Is(err, repoerrors.ErrAgentDeliveryEffectNotFound) {
			t.Fatalf("deferred workflow receipt lookup = %+v, err %v; want absent", stored, err)
		}
		return
	}
	if err != nil || stored.Outcome != expectedOutcome || stored.StreamID != f.effect.StreamID ||
		stored.SubmissionID != f.effect.SubmissionID || stored.TurnID != f.effect.TurnID {
		t.Fatalf("guarded workflow outcome = %+v, err %v; want %q for the exact terminal", stored, err, expectedOutcome)
	}
	if err := f.service.reconcileAgentDeliverySettlements(f.ctx, f.session.ID); err != nil {
		t.Fatalf("reconcile receipt-backed terminal: %v", err)
	}
	submission, err := f.repo.GetAgentDeliverySubmissionMetadata(f.ctx, f.effect.SubmissionID)
	if err != nil || submission.FullySettledAt == nil {
		t.Fatalf("receipt-backed terminal settlement = %+v, err %v", submission, err)
	}
}

func eventTurnID(event models.AgentDeliveryEvent) string {
	var payload streams.AgentEvent
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return ""
	}
	return payload.TurnID
}
