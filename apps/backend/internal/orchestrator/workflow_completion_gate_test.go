package orchestrator

import (
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestExecuteStepTransitionReportsCompletionGateRefusal(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step1")
	blockTaskCompletion(t, repo, "task")
	_, err := repo.DB().ExecContext(ctx, `
		INSERT INTO workflow_steps (id, workflow_id, name, position, complete_task_on_enter)
		VALUES ('done', 'wf1', 'Done', 1, 1)
	`)
	require.NoError(t, err)
	entry := signalEntryForSession(t, repo, "session")
	signal, claimed, err := repo.ClaimStepCompletionSignal(ctx, "task", "session", models.PendingStepCompletionSignal{
		WorkflowStepTransitionID: entry, StepID: "step1", Summary: "Review requested",
	})
	require.NoError(t, err)
	require.True(t, claimed)

	getter := newMockStepGetter()
	getter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Work", AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "done"},
		}}},
	}
	getter.steps["done"] = &wfmodels.WorkflowStep{ID: "done", WorkflowID: "wf1", Position: 1, Name: "Done", CompleteTaskOnEnter: true}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["task"] = &v1.Task{ID: "task", State: v1.TaskStateInProgress}
	svc := createTestService(repo, getter, taskRepo)
	task, err := repo.GetTask(ctx, "task")
	require.NoError(t, err)
	session := mustGetSession(t, repo, "session")
	transitioned := svc.processOnTurnComplete(ctx, task, session)

	_, stepID, afterEntry, err := repo.GetTaskWorkflowStepEntry(ctx, "task")
	require.NoError(t, err)
	require.Equal(t, "step1", stepID)
	require.Equal(t, *entry, afterEntry)

	session = mustGetSession(t, repo, "session")
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	receipt, found := models.LoadStepCompletionReceipt(session.Metadata)
	require.True(t, found)
	require.Equal(t, signal.SignalID, receipt.SignalID)
	require.Equal(t, models.StepCompletionOutcomeBlocked, receipt.Outcome)
	require.Equal(t, models.StepCompletionReasonTransitionRefused, receipt.Reason)
	require.Nil(t, receipt.Transition)
	require.False(t, transitioned)
}

func TestEngineCompletionGateRefusalSettlesDelivery(t *testing.T) {
	fixture := newDeliveryWorkflowOutcomeFixture(t, "completion-gate", "work")
	blockTaskCompletion(t, fixture.repo, fixture.taskID)
	require.NoError(t, fixture.repo.SetSessionMetadataKey(fixture.ctx, fixture.session.ID, "plan_mode", true))
	fixture.session = mustGetSession(t, fixture.repo, fixture.session.ID)
	stepTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.installStep(t, &wfmodels.WorkflowStep{
		ID: "work", WorkflowID: "wf1", Position: 0, UpdatedAt: stepTime,
		Events: wfmodels.StepEvents{
			OnExit: []wfmodels.OnExitAction{{Type: wfmodels.OnExitDisablePlanMode}},
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "done"},
			}},
		},
	})
	fixture.installStep(t, &wfmodels.WorkflowStep{
		ID: "done", WorkflowID: "wf1", Position: 1, UpdatedAt: stepTime, CompleteTaskOnEnter: true,
	})
	_, err := fixture.repo.DB().ExecContext(fixture.ctx, `UPDATE workflow_steps SET complete_task_on_enter = 1 WHERE id = 'done'`)
	require.NoError(t, err)

	fixture.run(t, models.DeliveryWorkflowOutcomeNoAutomaticTransition, turnCompletionCauseAgentTurn)

	task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, "work", task.WorkflowStepID)
	session := mustGetSession(t, fixture.repo, fixture.session.ID)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	require.Equal(t, true, session.Metadata["plan_mode"])
}

func TestEngineCompletesTaskWithVerifiedTaskRevision(t *testing.T) {
	fixture := newDeliveryWorkflowOutcomeFixture(t, "verified-revision", "work")
	task, err := fixture.repo.GetTask(fixture.ctx, fixture.taskID)
	require.NoError(t, err)
	blockTaskCompletion(t, fixture.repo, task.ID)
	gate, err := fixture.repo.GetTaskCompletionGate(fixture.ctx, task.ID)
	require.NoError(t, err)
	_, err = fixture.repo.VerifyTaskCompletionCriterion(fixture.ctx, models.TaskCompletionEvidenceChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: gate.Revision,
		CriterionID: "review", ActorKind: "human", ActorID: "reviewer",
		Evidence: models.TaskCompletionEvidence{Subject: models.TaskCompletionEvidenceSubject{
			Kind: models.TaskCompletionEvidenceTaskRevision, ID: task.ID, Revision: task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		}, Summary: "Reviewed"},
	})
	require.NoError(t, err)
	stepTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fixture.installStep(t, &wfmodels.WorkflowStep{ID: "work", WorkflowID: "wf1", UpdatedAt: stepTime,
		Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
			Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": "done"},
		}}},
	})
	fixture.installStep(t, &wfmodels.WorkflowStep{ID: "done", WorkflowID: "wf1", Position: 1, UpdatedAt: stepTime, CompleteTaskOnEnter: true})
	_, err = fixture.repo.DB().ExecContext(fixture.ctx, `UPDATE workflow_steps SET complete_task_on_enter = 1 WHERE id = 'done'`)
	require.NoError(t, err)
	require.True(t, fixture.service.processOnTurnCompleteViaEngineWithCause(withWorkflowEffect(fixture.ctx, fixture.effect), task.ID, fixture.session, turnCompletionCauseAgentTurn))
	task, err = fixture.repo.GetTask(fixture.ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, "done", task.WorkflowStepID)
	require.Equal(t, v1.TaskStateCompleted, task.State)
}

func blockTaskCompletion(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := t.Context()
	task, err := repo.GetTask(ctx, taskID)
	require.NoError(t, err)
	_, err = repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ActorKind: "human", ActorID: "reviewer",
		Criteria: []models.TaskCompletionCriterion{{
			ID: "review", Description: "Review the task",
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceTaskRevision, ID: task.ID},
		}},
	})
	require.NoError(t, err)
}
