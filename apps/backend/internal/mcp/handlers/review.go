package handlers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// ReviewRunner launches native code-review passes. Implemented by
// *review.Runner; declared as an interface so these handlers stay testable and
// so a deployment without the runner wired simply reports the feature as
// unavailable rather than panicking.
type ReviewRunner interface {
	Launch(ctx context.Context, req review.RunRequest) (*models.TaskReviewRun, error)
	// Cancel stops a live pass as well as marking the row, so inference cannot
	// finish afterwards and overwrite the cancelled status.
	Cancel(ctx context.Context, runID string) (*models.TaskReviewRun, error)
}

// SetReviewService wires the code-review persistence service, enabling the
// review read/update actions and the publish_review_findings_kandev tool.
func (h *Handlers) SetReviewService(svc *service.ReviewService) {
	h.reviewService = svc
}

// SetReviewRunner wires the run orchestrator, enabling task.review.run.
func (h *Handlers) SetReviewRunner(runner ReviewRunner) {
	h.reviewRunner = runner
}

// registerReviewHandlers registers the native code-review actions. Both the
// agent-facing MCP publish action and the plain UI actions are gated on the
// review service being wired.
func (h *Handlers) registerReviewHandlers(d mcpActionRegistrar) int {
	if h.reviewService == nil {
		return 0
	}
	d.RegisterFunc(ws.ActionMCPPublishReviewFindings, h.handlePublishReviewFindings)
	d.RegisterFunc(ws.ActionTaskReviewGet, h.handleGetTaskReview)
	d.RegisterFunc(ws.ActionTaskReviewFindingUpdate, h.handleUpdateReviewFinding)
	d.RegisterFunc(ws.ActionTaskReviewClear, h.handleClearTaskReview)
	d.RegisterFunc(ws.ActionTaskReviewCancel, h.handleCancelTaskReview)
	registered := 5
	if h.reviewRunner != nil {
		d.RegisterFunc(ws.ActionTaskReviewRun, h.handleRunTaskReview)
		registered++
	}
	return registered
}

// reviewFindingPayload is the per-finding wire shape shared by the MCP tool and
// any future client-side publisher. Field names match the tool schema.
type reviewFindingPayload struct {
	Repo       string `json:"repo"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	LineEnd    int    `json:"line_end"`
	Side       string `json:"side"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion"`
}

// handlePublishReviewFindings stores agent-authored findings.
//
// Unlike the inference path — which drops one malformed entry and keeps going —
// a malformed entry here rejects the whole call, because an agent can read the
// error and retry, and a half-stored review is worse than none.
func (h *Handlers) handlePublishReviewFindings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID    string                 `json:"task_id"`
		SessionID string                 `json:"session_id"`
		Summary   string                 `json:"summary"`
		Findings  []reviewFindingPayload `json:"findings"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}
	if len(req.Findings) == 0 {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "at least one finding is required", nil)
	}

	inputs := make([]service.ReviewFindingInput, 0, len(req.Findings))
	for _, f := range req.Findings {
		normalized, err := review.NormalizeFindingInput(review.FindingInput{
			Repo: f.Repo, File: f.File, Line: f.Line, LineEnd: f.LineEnd, Side: f.Side,
			Severity: f.Severity, Category: f.Category, Title: f.Title,
			Body: f.Body, Suggestion: f.Suggestion,
		})
		if err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		inputs = append(inputs, service.ReviewFindingInput{
			RepositoryName: normalized.Repo,
			FilePath:       normalized.File,
			StartLine:      normalized.Line,
			EndLine:        normalized.LineEnd,
			Side:           normalized.Side,
			Severity:       normalized.Severity,
			Category:       normalized.Category,
			Title:          normalized.Title,
			Body:           normalized.Body,
			Suggestion:     normalized.Suggestion,
			// An agent-published finding carries no diff hash or anchor text: it
			// did not go through the change-set collector. It is therefore never
			// reported stale and never relocated — see the spec's Data model.
		})
	}

	run, findings, err := h.reviewService.PublishFindings(ctx, service.PublishFindingsRequest{
		TaskID:    req.TaskID,
		SessionID: req.SessionID,
		Trigger:   models.ReviewTriggerAgent,
		Summary:   req.Summary,
		Findings:  inputs,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidReviewFinding) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to publish review findings: "+err.Error(), nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"run_id":        run.ID,
		"finding_count": len(findings),
	})
}

// handleRunTaskReview starts a review pass and returns the pending run without
// waiting for inference to finish.
func (h *Handlers) handleRunTaskReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID         string `json:"task_id"`
		SessionID      string `json:"session_id"`
		RepositoryID   string `json:"repository_id"`
		AgentProfileID string `json:"agent_profile_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	run, err := h.reviewRunner.Launch(ctx, review.RunRequest{
		TaskID:         req.TaskID,
		SessionID:      req.SessionID,
		RepositoryID:   req.RepositoryID,
		AgentProfileID: req.AgentProfileID,
		Trigger:        models.ReviewTriggerManual,
	})
	if err != nil {
		return reviewLaunchError(msg, err)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"run": run})
}

// reviewLaunchError maps a launch failure onto a WS error that carries the
// machine-readable code the Review surface branches on, so the UI can show the
// "configure a utility agent" affordance instead of a generic failure.
func reviewLaunchError(msg *ws.Message, err error) (*ws.Message, error) {
	code := review.CodeFor(err)
	data := map[string]any{"code": code}
	switch code {
	case review.CodeNoChanges, review.CodeAgentUnavailable, review.CodeWorkspaceUnavailable:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), data)
	default:
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), data)
	}
}

// handleCancelTaskReview cancels a non-terminal run. Idempotent.
func (h *Handlers) handleCancelTaskReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.RunID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "run_id is required", nil)
	}
	// Prefer the runner: it cancels the live context too. The service alone only
	// marks the row, which a still-running pass would overwrite.
	cancel := h.reviewService.CancelRun
	if h.reviewRunner != nil {
		cancel = h.reviewRunner.Cancel
	}
	run, err := cancel(ctx, req.RunID)
	if err != nil {
		if errors.Is(err, service.ErrReviewRunNotFound) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Review run not found", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to cancel review: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"run": run})
}

// handleGetTaskReview returns a task's run history and findings, used to backfill
// the store on mount before live events arrive.
func (h *Handlers) handleGetTaskReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, errMsg, errErr := parseTaskIDPayload(msg)
	if errMsg != nil || errErr != nil {
		return errMsg, errErr
	}
	result, err := h.reviewService.GetTaskReview(ctx, taskID)
	if err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to get task review", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// handleUpdateReviewFinding records the human's disposition of a finding.
func (h *Handlers) handleUpdateReviewFinding(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		FindingID string `json:"finding_id"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.FindingID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "finding_id is required", nil)
	}
	finding, err := h.reviewService.UpdateFindingStatus(ctx, req.FindingID, models.ReviewFindingStatus(req.Status))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidReviewFinding):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		case errors.Is(err, service.ErrReviewFindingNotFound):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Review finding not found", nil)
		default:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update review finding: "+err.Error(), nil)
		}
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"finding": finding})
}

// handleClearTaskReview removes a task's runs and findings.
func (h *Handlers) handleClearTaskReview(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	taskID, errMsg, errErr := parseTaskIDPayload(msg)
	if errMsg != nil || errErr != nil {
		return errMsg, errErr
	}
	if err := h.reviewService.ClearTaskReview(ctx, taskID); err != nil {
		if errors.Is(err, service.ErrTaskIDRequired) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to clear task review: "+err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]any{"success": true})
}
