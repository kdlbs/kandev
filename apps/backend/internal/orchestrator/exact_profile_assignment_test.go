package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

func TestExactProfileAssignerRejectsChangedProfileRevision(t *testing.T) {
	repo := &exactAssignmentRepo{assignment: &models.ExactProfileAssignment{
		TaskID: "task-1", WorkspaceID: "workspace-1", AgentProfileID: "profile-1",
		ProfileRevision: 1, Generation: 1, Active: true,
	}}
	assigner := ExactProfileAssigner{
		Assignments: repo,
		Profiles: exactProfileLookupFunc(func(context.Context, string) (*executor.AgentProfileInfo, error) {
			return &executor.AgentProfileInfo{
				ProfileID: "profile-1", WorkspaceID: "workspace-1", Enabled: true,
				Revision: 2,
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
