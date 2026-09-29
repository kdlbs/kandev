package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestAssignExactTaskProfileAcceptsGlobalProfileForWorkspaceTask(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	revision := time.Date(2026, 9, 8, 2, 30, 29, 739642361, time.UTC)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-1", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-1", WorkspaceID: "workspace-1", Name: "Workflow",
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		WorkflowStepID: "step-1", State: v1.TaskStateInProgress, Title: "Coordinator",
	}))
	manager := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "profile-astra", WorkspaceID: "", Enabled: true,
		Revision: revision, Model: "gpt-6-astra",
	}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	decision, err := service.AssignExactTaskProfile(ctx, ExactTaskProfileAssignmentRequest{
		TaskID: "task-1", AgentProfileID: "profile-astra", ExpectedModel: "gpt-6-astra", Generation: 1,
		ExpectedWorkflowID: "workflow-1", ExpectedWorkflowStepID: "step-1",
		ExpectedTaskState: v1.TaskStateInProgress,
	})
	require.NoError(t, err)
	require.Equal(t, &ExactProfileLaunchDecision{
		AgentProfileID: "profile-astra", Generation: 1, Revision: revision.UnixNano(),
		Model: "gpt-6-astra", Changed: true,
	}, decision)

	assignment, err := repo.GetExactProfileAssignment(ctx, "task-1")
	require.NoError(t, err)
	require.NotNil(t, assignment)
	require.True(t, assignment.Active)
	require.Equal(t, "workspace-1", assignment.WorkspaceID)
	require.Equal(t, "profile-astra", assignment.AgentProfileID)
	require.Equal(t, revision, assignment.ProfileRevision)
}

func TestAssignExactTaskProfileRejectsForeignWorkspaceProfile(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	revision := time.Date(2026, 9, 8, 2, 30, 29, 739642361, time.UTC)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-1", Name: "Workspace"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "workflow-1", WorkspaceID: "workspace-1", Name: "Workflow",
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-1", WorkspaceID: "workspace-1", WorkflowID: "workflow-1",
		WorkflowStepID: "step-1", State: v1.TaskStateInProgress, Title: "Coordinator",
	}))
	manager := &mockAgentManager{resolveProfileInfo: &executor.AgentProfileInfo{
		ProfileID: "profile-astra", WorkspaceID: "workspace-2", Enabled: true,
		Revision: revision, Model: "gpt-6-astra",
	}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	_, err := service.AssignExactTaskProfile(ctx, ExactTaskProfileAssignmentRequest{
		TaskID: "task-1", AgentProfileID: "profile-astra", ExpectedModel: "gpt-6-astra", Generation: 1,
		ExpectedWorkflowID: "workflow-1", ExpectedWorkflowStepID: "step-1",
		ExpectedTaskState: v1.TaskStateInProgress,
	})
	require.ErrorIs(t, err, ErrExactProfileAssignmentInvalid)
	assignment, readErr := repo.GetExactProfileAssignment(ctx, "task-1")
	require.NoError(t, readErr)
	require.Nil(t, assignment)
}

func TestExactProfileAssignerAcceptsGlobalProfileForWorkspaceTask(t *testing.T) {
	revision := time.Date(2026, 9, 8, 2, 30, 29, 739642361, time.UTC)
	repo := &exactAssignmentRepo{assignment: &models.ExactProfileAssignment{
		TaskID: "task-1", WorkspaceID: "workspace-1", AgentProfileID: "profile-astra",
		ProfileRevision: revision, Generation: 1, Active: true,
	}}
	assigner := ExactProfileAssigner{
		Assignments: repo,
		Profiles: exactProfileLookupFunc(func(context.Context, string) (*executor.AgentProfileInfo, error) {
			return &executor.AgentProfileInfo{
				ProfileID: "profile-astra", WorkspaceID: "", Enabled: true,
				Revision: revision, Model: "gpt-6-astra",
			}, nil
		}),
	}

	decision, err := assigner.Resolve(context.Background(), "task-1", "workspace-1")
	require.NoError(t, err)
	require.Equal(t, &ExactProfileLaunchDecision{
		AgentProfileID: "profile-astra", Generation: 1,
		Revision: revision.UnixNano(), Model: "gpt-6-astra",
	}, decision)
}

func TestExactProfileAssignerRejectsForeignWorkspaceProfile(t *testing.T) {
	revision := time.Date(2026, 9, 8, 2, 30, 29, 739642361, time.UTC)
	repo := &exactAssignmentRepo{assignment: &models.ExactProfileAssignment{
		TaskID: "task-1", WorkspaceID: "workspace-1", AgentProfileID: "profile-astra",
		ProfileRevision: revision, Generation: 1, Active: true,
	}}
	assigner := ExactProfileAssigner{
		Assignments: repo,
		Profiles: exactProfileLookupFunc(func(context.Context, string) (*executor.AgentProfileInfo, error) {
			return &executor.AgentProfileInfo{
				ProfileID: "profile-astra", WorkspaceID: "workspace-2", Enabled: true,
				Revision: revision, Model: "gpt-6-astra",
			}, nil
		}),
	}

	_, err := assigner.Resolve(context.Background(), "task-1", "workspace-1")
	require.ErrorIs(t, err, ErrExactProfileAssignmentInvalid)
}

func TestExactProfileAssignerRejectsChangedProfileRevision(t *testing.T) {
	repo := &exactAssignmentRepo{assignment: &models.ExactProfileAssignment{
		TaskID: "task-1", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: time.Date(2026, 9, 11, 20, 0, 0, 0, time.UTC), Generation: 1, Active: true,
	}}
	assigner := ExactProfileAssigner{
		Assignments: repo,
		Profiles: exactProfileLookupFunc(func(context.Context, string) (*executor.AgentProfileInfo, error) {
			return &executor.AgentProfileInfo{
				ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
				Revision: time.Date(2026, 9, 11, 20, 1, 0, 0, time.UTC),
			}, nil
		}),
	}

	_, err := assigner.Resolve(context.Background(), "task-1", "workspace-1")
	if !errors.Is(err, ErrExactProfileAssignmentInvalid) {
		t.Fatalf("Resolve error = %v, want invalid exact profile assignment", err)
	}
}

type exactAssignmentRepo struct {
	assignment *models.ExactProfileAssignment
}

func (r *exactAssignmentRepo) GetExactProfileAssignment(_ context.Context, _ string) (*models.ExactProfileAssignment, error) {
	return r.assignment, nil
}

type exactProfileLookupFunc func(context.Context, string) (*executor.AgentProfileInfo, error)

func (f exactProfileLookupFunc) ResolveAgentProfile(ctx context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return f(ctx, profileID)
}
