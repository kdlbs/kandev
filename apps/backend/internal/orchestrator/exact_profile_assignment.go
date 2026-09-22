package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

// ErrExactProfileAssignmentInvalid is returned before launch selection can
// fall through to a workflow/default profile. Exact assignments are a strict
// contract: a changed, disabled, or cross-workspace profile is never replaced.
var ErrExactProfileAssignmentInvalid = errors.New("exact profile assignment is no longer valid")

// ErrExactProfileAssignmentTargetChanged rejects a selector write when the
// task no longer matches the caller's expected workflow lane or state.
var ErrExactProfileAssignmentTargetChanged = errors.New("exact profile assignment target changed")

// ExactProfileModelMismatchError carries only bounded profile model IDs so an
// authorized caller can distinguish a stale expectation from infrastructure
// failure without receiving provider errors.
type ExactProfileModelMismatchError struct {
	Expected string
	Actual   string
}

func (e *ExactProfileModelMismatchError) Error() string {
	return "exact profile model does not match the expected model"
}

// ExactTaskProfileAssignmentRequest is the server-derived, fenced selector
// request. Generation is the next value to commit, not the current value.
type ExactTaskProfileAssignmentRequest struct {
	TaskID                 string
	AgentProfileID         string
	ExpectedModel          string
	Generation             int64
	ExpectedWorkflowID     string
	ExpectedWorkflowStepID string
	ExpectedTaskState      v1.TaskState
}

type exactProfileAssignmentStore interface {
	GetExactProfileAssignment(context.Context, string) (*models.ExactProfileAssignment, error)
}

type exactProfileAssignmentWriter interface {
	exactProfileAssignmentStore
	AssignExactProfileAssignment(context.Context, *models.ExactProfileAssignment) (bool, error)
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
	Model          string
	Changed        bool
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
// selection. The profile must be concrete, enabled, globally available or
// task-workspace scoped, and unchanged at assignment time; later launch
// validation repeats those checks.
//
//nolint:cyclop // Strict validation branches intentionally fail closed at each boundary.
func (s *Service) AssignExactTaskProfile(
	ctx context.Context, request ExactTaskProfileAssignmentRequest,
) (*ExactProfileLaunchDecision, error) {
	assignments, ok := s.repo.(exactProfileAssignmentWriter)
	if !ok || s.agentManager == nil {
		return nil, ErrExactProfileAssignmentInvalid
	}
	if err := s.authorizeTask(ctx, request.TaskID); err != nil {
		return nil, err
	}
	task, err := s.repo.GetTask(ctx, request.TaskID)
	if err != nil || task == nil {
		return nil, fmt.Errorf("load task for exact profile assignment: %w", err)
	}
	if task.WorkflowID != request.ExpectedWorkflowID ||
		task.WorkflowStepID != request.ExpectedWorkflowStepID || task.State != request.ExpectedTaskState ||
		task.ArchivedAt != nil {
		return nil, ErrExactProfileAssignmentTargetChanged
	}
	profile, err := s.agentManager.ResolveAgentProfile(ctx, request.AgentProfileID)
	if err != nil || profile == nil || !profile.Enabled ||
		(profile.WorkspaceID != "" && profile.WorkspaceID != task.WorkspaceID) || profile.Revision.IsZero() {
		return nil, ErrExactProfileAssignmentInvalid
	}
	if profile.Model != request.ExpectedModel {
		return nil, &ExactProfileModelMismatchError{Expected: request.ExpectedModel, Actual: profile.Model}
	}
	assignment := &models.ExactProfileAssignment{
		TaskID: request.TaskID, WorkspaceID: task.WorkspaceID, AgentProfileID: request.AgentProfileID,
		ProfileRevision: profile.Revision, Generation: request.Generation,
		SourceWorkflowID: request.ExpectedWorkflowID, SourceWorkflowStepID: request.ExpectedWorkflowStepID,
		SourceTaskState: string(request.ExpectedTaskState),
	}
	changed, err := assignments.AssignExactProfileAssignment(ctx, assignment)
	if err != nil {
		return nil, err
	}
	return &ExactProfileLaunchDecision{
		AgentProfileID: request.AgentProfileID, Generation: request.Generation,
		Revision: profile.Revision.UnixNano(), Model: profile.Model, Changed: changed,
	}, nil
}

// ExactProfileLaunchReceipt returns the bounded, task/session-keyed evidence
// used by the canonical Coordinator to verify the model that actually reached
// the launch chokepoint.
func (s *Service) ExactProfileLaunchReceipt(
	ctx context.Context, taskID, sessionID string,
) (*models.ExactProfileLaunchReceipt, error) {
	if err := s.authorizeTask(ctx, taskID); err != nil {
		return nil, err
	}
	receipts, ok := s.repo.(interface {
		GetExactProfileLaunchReceipt(context.Context, string, string) (*models.ExactProfileLaunchReceipt, error)
	})
	if !ok {
		return nil, nil
	}
	return receipts.GetExactProfileLaunchReceipt(ctx, taskID, sessionID)
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
		!profile.Enabled || (profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID) ||
		!profile.Revision.Equal(assignment.ProfileRevision) {
		return nil, ErrExactProfileAssignmentInvalid
	}
	return &ExactProfileLaunchDecision{
		AgentProfileID: assignment.AgentProfileID,
		Generation:     assignment.Generation,
		Revision:       assignment.ProfileRevision.UnixNano(),
		Model:          profile.Model,
	}, nil
}

func (s *Service) recordExactProfileLaunchReceipt(
	ctx context.Context, taskID, sessionID string, exact *ExactProfileLaunchDecision, model string, launchErr error,
) {
	if exact == nil {
		return
	}
	receipts, ok := s.repo.(interface {
		RecordExactProfileLaunchReceipt(context.Context, *models.ExactProfileLaunchReceipt) (bool, error)
	})
	if !ok {
		return
	}
	receipt := &models.ExactProfileLaunchReceipt{
		TaskID: taskID, SessionID: sessionID, AgentProfileID: exact.AgentProfileID,
		Generation: exact.Generation, ProfileRevision: time.Unix(0, exact.Revision).UTC(), Model: model,
		Outcome: models.ExactProfileLaunchOutcomeApplied, InferenceStarted: launchErr == nil,
	}
	if launchErr != nil {
		receipt.Outcome = models.ExactProfileLaunchOutcomeFailedClosed
		receipt.FailureReason = launchErr.Error()
	}
	if _, err := receipts.RecordExactProfileLaunchReceipt(ctx, receipt); err != nil {
		s.logger.Warn("failed to record exact-profile launch receipt", zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}

func exactProfileModel(exact *ExactProfileLaunchDecision) string {
	if exact == nil {
		return ""
	}
	return exact.Model
}

func exactAssignmentGeneration(exact *ExactProfileLaunchDecision) int64 {
	if exact == nil {
		return 0
	}
	return exact.Generation
}
func exactAssignmentRevision(exact *ExactProfileLaunchDecision) int64 {
	if exact == nil {
		return 0
	}
	return exact.Revision
}

func (s *Service) persistExactProfileSessionBinding(
	ctx context.Context,
	session *models.TaskSession,
	exact *ExactProfileLaunchDecision,
) error {
	if exact == nil ||
		(session.ExactProfileGeneration == exact.Generation && session.ExactProfileRevision == exact.Revision) {
		return nil
	}

	observedState := session.State
	session.ExactProfileGeneration = exact.Generation
	session.ExactProfileRevision = exact.Revision
	if err := s.persistFullTaskSessionIfCurrent(ctx, session, observedState); err != nil {
		return fmt.Errorf("persist exact profile session binding: %w", err)
	}
	return nil
}
