package service

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	PlanErrorVersionRequired         = "plan_version_required"
	PlanErrorVersionConflict         = "plan_version_conflict"
	PlanErrorHeadUnavailable         = "plan_head_unavailable"
	PlanErrorTruncationRejected      = "plan_truncation_rejected"
	PlanErrorHistoryUnavailable      = "plan_history_unavailable"
	PlanErrorContentRequired         = "plan_content_required"
	PlanErrorEditTextRequired        = "plan_edit_text_required"
	PlanErrorEditNotFound            = "plan_edit_not_found"
	PlanErrorEditAmbiguous           = "plan_edit_ambiguous"
	PlanErrorRevisionChanged         = "plan_revision_changed"
	PlanErrorRevisionUnavailable     = "plan_revision_unavailable"
	PlanErrorRevisionVersionRequired = "plan_revision_version_required"
	PlanErrorAlreadyCurrent          = "plan_already_current"
)

var (
	ErrPlanVersionRequired         = errors.New("plan write version is required")
	ErrPlanVersionConflict         = errors.New("plan write version is stale")
	ErrPlanHeadUnavailable         = errors.New("current task plan could not be read")
	ErrPlanTruncationRejected      = errors.New("suspicious plan reduction was rejected")
	ErrPlanHistoryUnavailable      = errors.New("plan revision history could not be verified")
	ErrPlanEditTextRequired        = errors.New("old plan edit text is required")
	ErrPlanEditNotFound            = errors.New("plan edit text was not found")
	ErrPlanEditAmbiguous           = errors.New("plan edit text is ambiguous")
	ErrPlanRevisionChanged         = errors.New("plan revision changed")
	ErrPlanRevisionUnavailable     = errors.New("plan revision could not be read")
	ErrPlanRevisionVersionRequired = errors.New("plan revision version is required")
)

// PlanSafetyError is the structured, no-write failure returned by agent plan
// tools. The message and Code are stable; the details are safe to expose only
// after the service has authorized the target task.
type PlanSafetyError struct {
	Code                   string
	TaskID                 string
	CurrentVersion         string
	CurrentRevisionVersion string
	CurrentRevisionNumber  int
	ReplacedRunes          int
	NewRunes               int
	Message                string
	NextAction             string
	sentinel               error
}

func (e *PlanSafetyError) Error() string { return e.Message }

func (e *PlanSafetyError) Unwrap() error { return e.sentinel }

func newPlanSafetyError(code string, sentinel error, taskID, message, nextAction string) *PlanSafetyError {
	return &PlanSafetyError{
		Code: code, TaskID: taskID, Message: message, NextAction: nextAction,
		sentinel: sentinel,
	}
}

func (s *PlanService) readLatestRevisionDetailed(
	ctx context.Context, taskID string,
) (*models.TaskPlanRevision, planRevisionState, error) {
	rev, err := s.repo.GetLatestTaskPlanRevision(ctx, taskID)
	if err != nil {
		return nil, planRevisionUnknown, err
	}
	if rev == nil {
		return nil, planRevisionAbsent, nil
	}
	return rev, planRevisionFound, nil
}

// guardAgentPlanWrite validates a replacement against the HEAD snapshot that
// upsertPlan already read while holding the task lock.
func (s *PlanService) guardAgentPlanWrite(
	req CreatePlanRequest,
	head *models.TaskPlan,
	headState planHeadState,
	headErr error,
	latest *models.TaskPlanRevision,
	latestState planRevisionState,
	latestErr error,
) (CreatePlanRequest, error) {
	if !req.AgentWrite {
		return req, nil
	}
	if headState == planHeadUnknown {
		return req, s.headUnavailableError(req.TaskID, headErr)
	}
	if headState == planHeadAbsent {
		if req.ExpectedVersion != "" {
			return req, s.versionConflictError(req.TaskID, "")
		}
		return req, nil
	}
	if req.Mode == PlanWriteModeAppend {
		return req, s.guardAgentAppend(req, head)
	}
	if err := s.requireCurrentVersion(req, head); err != nil {
		return req, err
	}
	if !req.EvaluateTruncation || !planTruncationDetected(head.Content, req.Content) {
		return req, nil
	}
	if !req.AllowTruncation {
		return req, s.truncationError(req.TaskID, head, req.Content)
	}
	if err := verifyPlanHistory(head, latest, latestState, latestErr); err != nil {
		return req, err
	}
	// An acknowledged reduction must be a new revision, so it cannot merge
	// over the snapshot that provides recovery.
	req.ForceNewRevision = true
	req.EvaluateTruncation = false
	return req, nil
}

func (s *PlanService) guardAgentAppend(req CreatePlanRequest, head *models.TaskPlan) error {
	if req.AllowTruncation {
		return s.truncationError(req.TaskID, head, req.Content)
	}
	if req.ExpectedVersion == "" {
		return nil
	}
	return s.requireCurrentVersion(req, head)
}

func (s *PlanService) requireCurrentVersion(req CreatePlanRequest, head *models.TaskPlan) error {
	if req.ExpectedVersion == "" {
		return newPlanSafetyError(
			PlanErrorVersionRequired, ErrPlanVersionRequired, req.TaskID,
			"Plan was not changed because expected_version is required for an existing plan.",
			"Read the current plan with get_task_plan_kandev and retry with its version.",
		)
	}
	if req.ExpectedVersion != head.WriteVersion {
		return s.versionConflictError(req.TaskID, head.WriteVersion)
	}
	return nil
}

func (s *PlanService) emptyAgentPlanError(taskID string) error {
	return newPlanSafetyError(
		PlanErrorContentRequired, ErrContentRequired, taskID,
		"Plan was not changed because the operation would leave the document empty.",
		"Provide a non-empty replacement or use an exact edit that leaves plan content.",
	)
}

func (s *PlanService) versionConflictError(taskID, currentVersion string) error {
	err := newPlanSafetyError(
		PlanErrorVersionConflict, ErrPlanVersionConflict, taskID,
		"Plan was not changed because expected_version does not match the current plan.",
		"Read the current plan with get_task_plan_kandev, apply the intended change to that snapshot, and retry.",
	)
	err.CurrentVersion = currentVersion
	return err
}

func (s *PlanService) headUnavailableError(taskID string, cause error) error {
	message := "Plan was not changed because the current plan could not be read."
	if cause != nil {
		s.logger.Warn("agent plan safety read failed", zap.String("task_id", taskID), zap.Error(cause))
	}
	return newPlanSafetyError(
		PlanErrorHeadUnavailable, ErrPlanHeadUnavailable, taskID, message,
		"Read the plan again and retry after the read succeeds.",
	)
}

func (s *PlanService) truncationError(taskID string, head *models.TaskPlan, content string) error {
	err := newPlanSafetyError(
		PlanErrorTruncationRejected, ErrPlanTruncationRejected, taskID,
		"Plan was not changed because the replacement removes most of the existing document.",
		"Retry with the complete current document, use edit_task_plan_kandev for a local change, or retry this replacement with the matching expected_version and allow_truncation=true if the reduction is intentional.",
	)
	err.CurrentVersion = head.WriteVersion
	err.ReplacedRunes = models.PlanContentLength(head.Content)
	err.NewRunes = models.PlanContentLength(content)
	return err
}

func verifyPlanHistory(
	head *models.TaskPlan,
	latest *models.TaskPlanRevision,
	state planRevisionState,
	readErr error,
) error {
	if state != planRevisionFound || latest == nil || latest.Title != head.Title || latest.Content != head.Content {
		_ = readErr
		return newPlanSafetyError(
			PlanErrorHistoryUnavailable, ErrPlanHistoryUnavailable, head.TaskID,
			"Plan was not changed because its previous content could not be verified in revision history.",
			"Read the current plan and revision history, then retry only after the preserved snapshot is available.",
		)
	}
	return nil
}
