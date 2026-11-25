package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestCompletingStepEntryAdmission(t *testing.T) {
	exerciseCompletingStepEntryAdmission(t, newRepoForBuiltinWorkflowTests(t))
}

func TestPostgresCompletingStepEntryAdmission(t *testing.T) {
	repo := openPostgresRepo(t)
	_, err := workflowrepo.NewWithDB(repo.db, repo.db, nil)
	require.NoError(t, err)
	exerciseCompletingStepEntryAdmission(t, repo)
}

func exerciseCompletingStepEntryAdmission(t *testing.T, repo *Repository) {
	t.Helper()
	t.Run("moves", func(t *testing.T) { exerciseCompletingStepMoves(t, repo) })
	t.Run("queued promotion", func(t *testing.T) { exerciseCompletingStepQueuePromotion(t, repo) })
}

func exerciseCompletingStepMoves(t *testing.T, repo *Repository) {
	t.Helper()
	for _, tc := range []struct {
		name          string
		criteria      bool
		verified      bool
		override      bool
		completeState bool
		wantErr       error
	}{
		{name: "step only refused", criteria: true, wantErr: repoerrors.ErrTaskCompletionGateBlocked},
		{name: "step and state refused", criteria: true, completeState: true, wantErr: repoerrors.ErrTaskCompletionGateBlocked},
		{name: "human override", criteria: true, override: true},
		{name: "no criteria"},
		{name: "verified criteria", criteria: true, verified: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			task, done := seedCompletingStepMove(t, repo)
			if tc.criteria {
				setStepEntryCriterion(t, repo, task, tc.verified)
			}
			if tc.override {
				ctx = stepEntryOverrideContext(t, repo, task, done)
			}
			candidate := *task
			if tc.completeState {
				candidate.State = v1.TaskStateCompleted
			}
			admitted, err := repo.UpdateTaskWithWorkflowStepAdmission(ctx, &candidate, task.WorkflowStepID, done, 0)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.False(t, admitted)
				assertCompletingMoveUnchanged(t, repo, task)
				return
			}
			require.NoError(t, err)
			require.True(t, admitted)
			stored, err := repo.GetTask(ctx, task.ID)
			require.NoError(t, err)
			require.Equal(t, done, stored.WorkflowStepID)
			require.Equal(t, v1.TaskStateInProgress, stored.State)
			if tc.override {
				history, err := repo.ListTaskCompletionGateHistory(ctx, task.ID)
				require.NoError(t, err)
				require.Len(t, history, 2)
				require.Equal(t, "completion_overridden", history[1].Action)
				require.Equal(t, "reviewer", history[1].ActorID)
				require.Equal(t, "Approved exception", history[1].Reason)
			}
		})
	}
}

func exerciseCompletingStepQueuePromotion(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := t.Context()
	task, done := seedCompletingStepMove(t, repo)
	setStepEntryCriterion(t, repo, task, false)

	occupant := &models.Task{ID: task.ID + "-occupant", Title: "Occupant", WorkspaceID: task.WorkspaceID,
		WorkflowID: task.WorkflowID, WorkflowStepID: done, State: v1.TaskStateInProgress, WIPAdmitted: true}
	require.NoError(t, repo.CreateTask(ctx, occupant))
	candidate := *task
	admitted, err := repo.UpdateTaskWithWorkflowStepAdmission(ctx, &candidate, task.WorkflowStepID, done, 1)
	require.NoError(t, err)
	require.False(t, admitted)
	queued, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.Equal(t, done, queued.WorkflowStepID)
	require.Equal(t, done, queued.QueuedForStepID)
	require.NoError(t, repo.RemoveTaskFromWorkflow(ctx, occupant.ID, occupant.WorkflowID))

	candidate = *queued
	candidate.WIPAdmitted = true
	candidate.QueuedForStepID = ""
	candidate.QueuedAt = nil
	for _, tc := range []struct {
		name    string
		promote func(*models.Task) error
	}{
		{name: "atomic", promote: func(candidate *models.Task) error {
			_, err := repo.PromoteQueuedTaskIfWorkflowStepHasCapacity(ctx, candidate, done, done, 1)
			return err
		}},
		{name: "capacity fallback", promote: func(candidate *models.Task) error {
			return repo.UpdateTaskIfWorkflowStepHasCapacity(ctx, candidate, done, task.ID, 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			promoted := candidate
			require.ErrorIs(t, tc.promote(&promoted), repoerrors.ErrTaskCompletionGateBlocked)
			assertCompletingMoveUnchanged(t, repo, queued)
		})
	}
}

func TestDirectTaskWritesCheckCompletingStepEntry(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	t.Run("plain update", func(t *testing.T) {
		task, done := seedCompletingStepMove(t, repo)
		setStepEntryCriterion(t, repo, task, false)
		candidate := *task
		candidate.WorkflowStepID = done
		require.ErrorIs(t, repo.UpdateTask(t.Context(), &candidate), repoerrors.ErrTaskCompletionGateBlocked)
		assertCompletingMoveUnchanged(t, repo, task)
	})
	t.Run("workflow attachment", func(t *testing.T) {
		task, done := seedCompletingStepMove(t, repo)
		setStepEntryCriterion(t, repo, task, false)
		err := repo.AddTaskToWorkflow(t.Context(), task.ID, task.WorkflowID, done, 0)
		require.ErrorIs(t, err, repoerrors.ErrTaskCompletionGateBlocked)
		assertCompletingMoveUnchanged(t, repo, task)
	})
}

func seedCompletingStepMove(t *testing.T, repo *Repository) (*models.Task, string) {
	t.Helper()
	ctx := t.Context()
	workspaceID := t.Name()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Office"}))
	workflowID, err := repo.EnsureOfficeWorkflow(ctx, workspaceID)
	require.NoError(t, err)
	var work, done string
	for _, step := range loadStepsForWorkflow(t, repo, workflowID) {
		switch step.Name {
		case "Work":
			work = step.ID
		case "Done":
			done = step.ID
		}
	}
	require.NotEmpty(t, work)
	require.NotEmpty(t, done)
	task := &models.Task{
		ID: workspaceID + "-task", WorkspaceID: workspaceID, Title: "Verify completion",
		WorkflowID: workflowID, WorkflowStepID: work, State: v1.TaskStateInProgress, WIPAdmitted: true,
	}
	require.NoError(t, repo.CreateTask(ctx, task))
	stored, err := repo.GetTask(ctx, task.ID)
	require.NoError(t, err)
	return stored, done
}

func setStepEntryCriterion(t *testing.T, repo *Repository, task *models.Task, verified bool) {
	t.Helper()
	ctx := t.Context()
	subject := models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceTaskRevision, ID: task.ID}
	const criterionID = "review"
	gate, err := repo.SetTaskCompletionCriteria(ctx, models.TaskCompletionCriteriaChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ActorKind: "human", ActorID: "reviewer",
		Criteria: []models.TaskCompletionCriterion{{
			ID: criterionID, Description: "Review the task", EvidenceSubject: subject,
		}},
	})
	require.NoError(t, err)
	require.True(t, gate.Blocked)
	if verified {
		subject.Revision = task.UpdatedAt.UTC().Format(time.RFC3339Nano)
		gate, err = repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
			TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: gate.Revision,
			CriterionID: criterionID, ActorKind: "human", ActorID: "reviewer",
			Evidence: models.TaskCompletionEvidence{Subject: subject, Summary: "Reviewed the task"},
		})
		require.NoError(t, err)
		require.False(t, gate.Blocked)
	}
}

func stepEntryOverrideContext(t *testing.T, repo *Repository, task *models.Task, done string) context.Context {
	t.Helper()
	ctx := t.Context()
	gate, err := repo.GetTaskCompletionGate(ctx, task.ID)
	require.NoError(t, err)
	require.True(t, gate.Blocked)
	return models.WithTaskCompletionMoveOverride(ctx, models.TaskCompletionMoveOverride{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: gate.Revision,
		SourceWorkflowID: task.WorkflowID, SourceStepID: task.WorkflowStepID,
		TargetWorkflowID: task.WorkflowID, TargetStepID: done,
		ActorID: "reviewer", Reason: "Approved exception",
	})
}

func assertCompletingMoveUnchanged(t *testing.T, repo *Repository, before *models.Task) {
	t.Helper()
	stored, err := repo.GetTask(t.Context(), before.ID)
	require.NoError(t, err)
	require.Equal(t, before.WorkflowStepID, stored.WorkflowStepID)
	require.Equal(t, before.State, stored.State)
	require.Equal(t, before.WIPAdmitted, stored.WIPAdmitted)
	require.Equal(t, before.QueuedForStepID, stored.QueuedForStepID)
	require.Equal(t, before.UpdatedAt, stored.UpdatedAt)
}
