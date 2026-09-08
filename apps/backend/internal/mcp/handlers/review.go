package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

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
	d.RegisterFunc(ws.ActionMCPListReviewFindings, h.handleListReviewFindings)
	d.RegisterFunc(ws.ActionMCPResolveReviewFinding, h.handleResolveReviewFinding)
	d.RegisterFunc(ws.ActionTaskReviewGet, h.handleGetTaskReview)
	d.RegisterFunc(ws.ActionTaskReviewFindingUpdate, h.handleUpdateReviewFinding)
	d.RegisterFunc(ws.ActionTaskReviewClear, h.handleClearTaskReview)
	d.RegisterFunc(ws.ActionTaskReviewCancel, h.handleCancelTaskReview)
	registered := 7
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

// reviewFindingWire is the spec-defined shape shared by list_review_findings_kandev
// and resolve_review_finding_kandev (AC-TWS-003.4 / AC-TWS-004.11). It exists
// because models.TaskReviewFinding tags ResolvedAt `omitempty`, which drops the
// key entirely for an unresolved finding; ResolvedAt has no omitempty here so it
// renders as JSON null instead, giving every finding the same key set.
type reviewFindingWire struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id"`
	RepositoryName string     `json:"repository_name"`
	FilePath       string     `json:"file_path"`
	StartLine      int        `json:"start_line"`
	EndLine        int        `json:"end_line"`
	Side           string     `json:"side"`
	Severity       string     `json:"severity"`
	Category       string     `json:"category"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	Suggestion     string     `json:"suggestion"`
	Status         string     `json:"status"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

func toReviewFindingWire(f *models.TaskReviewFinding) reviewFindingWire {
	return reviewFindingWire{
		ID:             f.ID,
		RunID:          f.RunID,
		RepositoryName: f.RepositoryName,
		FilePath:       f.FilePath,
		StartLine:      f.StartLine,
		EndLine:        f.EndLine,
		Side:           f.Side,
		Severity:       string(f.Severity),
		Category:       f.Category,
		Title:          f.Title,
		Body:           f.Body,
		Suggestion:     f.Suggestion,
		Status:         string(f.Status),
		ResolvedAt:     f.ResolvedAt,
		CreatedAt:      f.CreatedAt,
	}
}

func toReviewFindingWireList(findings []*models.TaskReviewFinding) []reviewFindingWire {
	out := make([]reviewFindingWire, 0, len(findings))
	for _, f := range findings {
		out = append(out, toReviewFindingWire(f))
	}
	return out
}

// handleListReviewFindings returns a task's review findings, filtered by
// status (default open) and optionally severity (REQ-TWS-003).
func (h *Handlers) handleListReviewFindings(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		TaskID   string `json:"task_id"`
		Status   string `json:"status"`
		Severity string `json:"severity"`
	}
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.TaskID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
	}

	result, err := h.reviewService.ListFindings(ctx, service.ListFindingsRequest{
		TaskID:   req.TaskID,
		Status:   req.Status,
		Severity: req.Severity,
	})
	if err != nil {
		var accessDenied *service.ErrReviewAccessDenied
		switch {
		case errors.As(err, &accessDenied):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
		case errors.Is(err, service.ErrInvalidReviewFindingFilter):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		case errors.Is(err, service.ErrTaskIDRequired):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		default:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list review findings: "+err.Error(), nil)
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"findings":      toReviewFindingWireList(result.Findings),
		"total_matched": result.TotalMatched,
		"truncated":     result.Truncated,
	})
}

// handleResolveReviewFinding closes out (or reopens) one finding, authorized
// against the finding's own owning task (REQ-TWS-004).
func (h *Handlers) handleResolveReviewFinding(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
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
	status := models.ReviewFindingStatus(strings.ToLower(strings.TrimSpace(req.Status)))
	if !models.ValidReviewFindingStatus(status) {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation,
			"status must be one of open, resolved, dismissed", nil)
	}

	finding, err := h.reviewService.ResolveFinding(ctx, service.ResolveFindingRequest{
		FindingID: req.FindingID,
		Status:    status,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrReviewFindingNotFound):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Review finding not found", nil)
		case errors.Is(err, service.ErrInvalidReviewFinding):
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		default:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to resolve review finding: "+err.Error(), nil)
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"finding": toReviewFindingWire(finding),
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
