// Package planws holds the WebSocket contract shared by Kandev's two
// task-plan surfaces: the browser-facing handlers in internal/task/handlers
// and the agent-facing MCP handlers in internal/mcp/handlers.
//
// Both surfaces drive the same *service.PlanService and are expected to report
// the same failure the same way, but neither package imports the other, so
// until now each carried its own copy of the sentinel mapping. Defining it once
// here is what keeps the two from drifting; the surfaces themselves stay
// separate on purpose, because their request shapes and success payloads
// legitimately differ.
package planws

import (
	"errors"

	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// TaskIDRequest is the payload for the plan actions keyed only by task, so the
// wire shape is declared once rather than re-spelled per surface.
type TaskIDRequest struct {
	TaskID string `json:"task_id"`
}

// mapping pairs a PlanService sentinel with the WebSocket code and message
// reported for it.
type mapping struct {
	sentinel error
	code     string
	message  string
}

// The task-plan error vocabulary. Messages are the ones the frontend and the
// MCP tools already receive, so editing one now moves both surfaces together.
var (
	taskIDRequired       = mapping{service.ErrTaskIDRequired, ws.ErrorCodeValidation, "task_id is required"}
	taskNotFound         = mapping{repository.ErrTaskNotFound, ws.ErrorCodeNotFound, "Task not found"}
	sessionIDRequired    = mapping{service.ErrSessionIDRequired, ws.ErrorCodeValidation, "session_id is required"}
	sessionTaskMismatch  = mapping{service.ErrSessionTaskMismatch, ws.ErrorCodeValidation, "Session does not belong to task"}
	planNotFound         = mapping{service.ErrTaskPlanNotFound, ws.ErrorCodeNotFound, "Task plan not found"}
	revisionIDRequired   = mapping{service.ErrRevisionIDRequired, ws.ErrorCodeValidation, "revision_id is required"}
	revisionNotFound     = mapping{service.ErrRevisionNotFound, ws.ErrorCodeNotFound, "Revision not found"}
	revisionTaskMismatch = mapping{service.ErrRevisionTaskMismatch, ws.ErrorCodeValidation, "Revision does not belong to task"}

	// allMappings is the vocabulary reachable by an action that can surface any
	// plan or revision failure.
	allMappings = []mapping{
		taskIDRequired,
		taskNotFound,
		sessionIDRequired,
		sessionTaskMismatch,
		planNotFound,
		revisionIDRequired,
		revisionNotFound,
		revisionTaskMismatch,
	}
)

// errorResponse returns the shared response when err matches one of recognized,
// and otherwise an internal error carrying fallback verbatim. Each action
// passes only the sentinels its PlanService method can actually return, so a
// sentinel that would mean something different for that action does not get
// silently reclassified.
func errorResponse(msg *ws.Message, err error, fallback string, recognized []mapping) (*ws.Message, error) {
	for _, m := range recognized {
		if errors.Is(err, m.sentinel) {
			return ws.NewError(msg.ID, msg.Action, m.code, m.message, nil)
		}
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, fallback, nil)
}

// contentTooLargeResponse reports service.PlanContentTooLargeError as a
// VALIDATION_ERROR. Unlike every other mapping, its message is computed from
// the request rather than fixed, so it is checked before the sentinel table
// walk instead of living in it. It is the only site that populates the
// structured details map: reason is a stable wire token the browser compares
// with ===, never localized or reworded, and limit/submitted let the browser
// render the rejection without re-deriving the ceiling.
func contentTooLargeResponse(msg *ws.Message, err error) (*ws.Message, bool, error) {
	var sizeErr *service.PlanContentTooLargeError
	if !errors.As(err, &sizeErr) {
		return nil, false, nil
	}
	out, mapErr := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, sizeErr.Error(),
		map[string]interface{}{
			"reason":    "plan_content_too_large",
			"limit":     sizeErr.Limit,
			"submitted": sizeErr.Submitted,
		})
	return out, true, mapErr
}

// CreateError maps a PlanService.CreatePlan failure.
//
// ErrTaskPlanNotFound is deliberately absent: CreatePlan only returns it when
// the plan vanishes between the write and the read-back, which is a server-side
// inconsistency rather than the caller naming a plan that does not exist. The
// repository's ErrTaskNotFound is different: it means the write target task
// did not exist and is safe to report as a not-found response.
func CreateError(msg *ws.Message, err error) (*ws.Message, error) {
	if out, matched, mapErr := contentTooLargeResponse(msg, err); matched {
		return out, mapErr
	}
	return errorResponse(msg, err, "Failed to create task plan: "+err.Error(), []mapping{taskIDRequired, taskNotFound})
}

// GetError maps a PlanService.GetPlan failure. The fallback omits err.Error()
// because a read that fails below the sentinels is a storage fault with nothing
// actionable for the caller.
func GetError(msg *ws.Message, err error) (*ws.Message, error) {
	return errorResponse(msg, err, "Failed to get task plan", []mapping{taskIDRequired})
}

// UpdateError maps a PlanService.UpdatePlan failure.
func UpdateError(msg *ws.Message, err error) (*ws.Message, error) {
	if out, matched, mapErr := contentTooLargeResponse(msg, err); matched {
		return out, mapErr
	}
	return errorResponse(msg, err, "Failed to update task plan: "+err.Error(), []mapping{taskIDRequired, taskNotFound, planNotFound})
}

// DeleteError maps a PlanService.DeletePlan failure.
func DeleteError(msg *ws.Message, err error) (*ws.Message, error) {
	return errorResponse(msg, err, "Failed to delete task plan: "+err.Error(), []mapping{taskIDRequired, planNotFound})
}

// Error maps a failure from a plan action that can reach the whole sentinel
// vocabulary — implementation-started, revert, and the revision reads. fallback
// is the action's own summary; err.Error() is appended to it.
func Error(msg *ws.Message, err error, fallback string) (*ws.Message, error) {
	return errorResponse(msg, err, fallback+": "+err.Error(), allMappings)
}
