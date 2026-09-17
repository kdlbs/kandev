package backendapp

import (
	"context"
	"github.com/jmoiron/sqlx"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestOrchestratedCompletionRespectsReviewAndReopen(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := context.Background()
	repo := adapter.taskRepo
	workflow, err := workflowrepo.NewWithDB(sqlx.NewDb(repo.DB(), "sqlite3"), sqlx.NewDb(repo.DB(), "sqlite3"), nil)
	require.NoError(t, err)
	repos := &Repositories{Workflow: workflow}
	task := &taskmodels.Task{ID: "delivery", WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "review", Title: "Delivery", State: "REVIEW"}
	require.NoError(t, repo.CreateTask(ctx, task))
	participant := &workflowmodels.WorkflowStepParticipant{StepID: "review", Role: workflowmodels.ParticipantRoleReviewer, AgentProfileID: "reviewer", DecisionRequired: true}
	require.NoError(t, workflow.UpsertStepParticipant(ctx, participant))
	require.ErrorContains(t, updateOrchestratedStatus(ctx, svc, repos, "other", task.ID, "done"), "workspace")
	require.ErrorContains(t, updateOrchestratedStatus(ctx, svc, repos, "ws-1", task.ID, "done"), "pending")
	require.NoError(t, workflow.RecordStepDecision(ctx, &workflowmodels.WorkflowStepDecision{TaskID: task.ID, StepID: "review", ParticipantID: participant.ID, Decision: "approved"}))
	require.NoError(t, updateOrchestratedStatus(ctx, svc, repos, "ws-1", task.ID, "done"))
	got, err := svc.GetTask(ctx, task.ID)
	require.NoError(t, err)
	require.EqualValues(t, "COMPLETED", got.State)
	require.NoError(t, updateOrchestratedStatus(ctx, svc, repos, "ws-1", task.ID, "todo"))
	decisions, err := workflow.ListActiveTaskDecisions(ctx, task.ID)
	require.NoError(t, err)
	require.Empty(t, decisions)
	require.ErrorContains(t, updateOrchestratedStatus(ctx, svc, repos, "ws-1", task.ID, "done"), "pending")
}
