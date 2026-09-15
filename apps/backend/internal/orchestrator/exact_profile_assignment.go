package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrExactProfileAssignmentInvalid is returned before launch selection can
// fall through to a workflow/default profile. Exact assignments are a strict
// contract: a changed, disabled, or cross-workspace profile is never replaced.
var ErrExactProfileAssignmentInvalid = errors.New("exact profile assignment is no longer valid")

type exactProfileAssignmentStore interface {
	GetExactProfileAssignment(context.Context, string) (*models.ExactProfileAssignment, error)
}

type exactProfileAssignmentWriter interface {
	exactProfileAssignmentStore
	UpsertExactProfileAssignment(context.Context, *models.ExactProfileAssignment) (bool, error)
	ActivateExactProfileAssignment(context.Context, string, int64) (bool, error)
}

type exactProfileLookup interface {
	ResolveAgentProfile(context.Context, string) (*executor.AgentProfileInfo, error)
}

// ExactProfileLaunchDecision is the immutable assignment consumed by every
// session-creation and agent-start chokepoint.
type ExactProfileLaunchDecision struct {
	AgentProfileID string
	Generation     int64
	Revision       int64
}

// ExactProfileAssigner validates durable assignments against a current,
// concrete profile snapshot.
type ExactProfileAssigner struct {
	Assignments exactProfileAssignmentStore
	Profiles    exactProfileLookup
}

func (s *Service) resolveExactProfileAssignment(
	ctx context.Context,
	taskID string,
) (*ExactProfileLaunchDecision, error) {
	assignments, ok := s.repo.(exactProfileAssignmentStore)
	if !ok {
		return nil, nil
	}

	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task for exact profile assignment: %w", err)
	}

	if task == nil {
		return nil, ErrExactProfileAssignmentInvalid
	}
	return (ExactProfileAssigner{
		Assignments: assignments,
		Profiles:    s.agentManager,
	}).Resolve(ctx, taskID, task.WorkspaceID)
}

// ExactTaskProfileGeneration returns the currently active exact-assignment
// generation. Zero preserves legacy routing for tasks without one.
func (s *Service) ExactTaskProfileGeneration(ctx context.Context, taskID string) (int64, error) {
	assignments, ok := s.repo.(exactProfileAssignmentStore)
	if !ok {
		return 0, nil
	}
	assignment, err := assignments.GetExactProfileAssignment(ctx, taskID)
	if err != nil {
		return 0, fmt.Errorf("load exact profile assignment: %w", err)
	}
	if assignment == nil || !assignment.Active {
		return 0, nil
	}
	return assignment.Generation, nil
}

// AssignExactTaskProfile records and activates the next immutable profile
// selection. The profile must be concrete, enabled, task-workspace scoped, and
// unchanged at assignment time; later launch validation repeats those checks.
func (s *Service) AssignExactTaskProfile(
	ctx context.Context,
	taskID, agentProfileID string,
	generation int64,
) (*ExactProfileLaunchDecision, error) {
	assignments, ok := s.repo.(exactProfileAssignmentWriter)
	if !ok || s.agentManager == nil {
		return nil, ErrExactProfileAssignmentInvalid
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return nil, fmt.Errorf("load task for exact profile assignment: %w", err)
	}
	profile, err := s.agentManager.ResolveAgentProfile(ctx, agentProfileID)
	if err != nil || profile == nil || !profile.Enabled || profile.WorkspaceID != task.WorkspaceID || profile.Revision.IsZero() {
		return nil, ErrExactProfileAssignmentInvalid
	}
	assignment := &models.ExactProfileAssignment{
		TaskID: taskID, WorkspaceID: task.WorkspaceID, AgentProfileID: agentProfileID,
		ProfileRevision: profile.Revision, Generation: generation,
		SourceWorkflowID: task.WorkflowID, SourceWorkflowStepID: task.WorkflowStepID,
		SourceTaskState: string(task.State),
	}
	if _, err := assignments.UpsertExactProfileAssignment(ctx, assignment); err != nil {
		return nil, err
	}
	if _, err := assignments.ActivateExactProfileAssignment(ctx, taskID, generation); err != nil {
		return nil, err
	}
	return &ExactProfileLaunchDecision{
		AgentProfileID: agentProfileID, Generation: generation, Revision: profile.Revision.UnixNano(),
	}, nil
}

// Resolve returns nil when this task has no active assignment, preserving
// legacy routing. An active assignment that cannot be verified fails closed.
func (a ExactProfileAssigner) Resolve(ctx context.Context, taskID, workspaceID string) (*ExactProfileLaunchDecision, error) {
	if a.Assignments == nil {
		return nil, nil
	}
	assignment, err := a.Assignments.GetExactProfileAssignment(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load exact profile assignment: %w", err)
	}
	if assignment == nil || !assignment.Active {
		return nil, nil
	}
	if assignment.WorkspaceID != workspaceID || a.Profiles == nil {
		return nil, ErrExactProfileAssignmentInvalid
	}
	profile, err := a.Profiles.ResolveAgentProfile(ctx, assignment.AgentProfileID)
	if err != nil || profile == nil || profile.ProfileID != assignment.AgentProfileID ||
		!profile.Enabled || profile.WorkspaceID != workspaceID || !profile.Revision.Equal(assignment.ProfileRevision) {
		return nil, ErrExactProfileAssignmentInvalid
	}
	return &ExactProfileLaunchDecision{
		AgentProfileID: assignment.AgentProfileID,
		Generation:     assignment.Generation,
		Revision:       assignment.ProfileRevision.UnixNano(),
	}, nil
}
