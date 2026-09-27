package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

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
