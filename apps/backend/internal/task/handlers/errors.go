package handlers

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// statusClientClosedRequest is nginx's 499. Go defines no constant for it and
// no standard 4xx says "the client hung up", which is what actually happened.
const statusClientClosedRequest = 499

const (
	moveConflictCodeActiveSession      = "task_move_active_session"
	moveConflictCodeArchived           = "task_move_archived"
	moveConflictCodeDifferentWorkspace = "task_move_different_workspace"
	moveConflictCodeWorkflowStep       = "task_move_workflow_step"
	moveConflictCodeWIPLimit           = "task_move_wip_limit"
)

func handleNotFound(c *gin.Context, log *logger.Logger, err error, fallback string) {
	if isClientDisconnect(err) {
		abortClientDisconnect(c)
		return
	}
	if status, ok := repositorySelectionHTTPStatus(err); ok {
		c.JSON(status, taskErrorBody(err))
		return
	}
	if isNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": fallback})
		return
	}
	// A scope denial is 403, not 404: workspace.read was already granted, so
	// existence is known to the caller and there is nothing left to hide.
	if service.IsForbidden(err) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	// A rejected assignee is the caller naming someone who cannot reach the
	// workspace, not a server fault. Its message is written to be shown.
	if errors.Is(err, service.ErrAssigneeCannotReachWorkspace) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, service.ErrWIPLimitExceeded) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if isValidationError(err) {
		c.JSON(http.StatusBadRequest, taskErrorBody(err))
		return
	}
	log.Error("request failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
}

func taskErrorBody(err error) gin.H {
	body := gin.H{"error": err.Error()}
	for key, value := range taskErrorDetails(err) {
		body[key] = value
	}
	return body
}

func taskErrorDetails(err error) map[string]interface{} {
	var selectionErr *service.RepositorySelectionError
	if errors.As(err, &selectionErr) {
		return map[string]interface{}{"error_code": string(selectionErr.Code)}
	}
	if errors.Is(err, service.ErrRepositoryBranchPolicyStale) {
		return map[string]interface{}{"error_code": service.BranchPolicyStaleErrorCode}
	}
	return nil
}

func repositorySelectionHTTPStatus(err error) (int, bool) {
	var selectionErr *service.RepositorySelectionError
	if !errors.As(err, &selectionErr) {
		return 0, false
	}
	switch selectionErr.Code {
	case service.RepositorySelectionErrorInvalid:
		return http.StatusBadRequest, true
	case service.RepositorySelectionErrorNotFound:
		return http.StatusNotFound, true
	case service.RepositorySelectionErrorUnavailable:
		return http.StatusServiceUnavailable, true
	default:
		return http.StatusInternalServerError, true
	}
}

func repositorySelectionWSCode(err error) (string, bool) {
	var selectionErr *service.RepositorySelectionError
	if !errors.As(err, &selectionErr) {
		return "", false
	}
	switch selectionErr.Code {
	case service.RepositorySelectionErrorInvalid:
		return ws.ErrorCodeValidation, true
	case service.RepositorySelectionErrorNotFound:
		return ws.ErrorCodeNotFound, true
	case service.RepositorySelectionErrorUnavailable:
		return ws.ErrorCodeUnavailable, true
	default:
		return ws.ErrorCodeInternalError, true
	}
}

func handleSelectedMoveError(c *gin.Context, log *logger.Logger, err error) {
	switch {
	case isClientDisconnect(err):
		abortClientDisconnect(c)
	case isNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "task or workflow not found"})
	case isMoveConflict(err):
		body := gin.H{"error": err.Error()}
		if code := moveConflictCode(err); code != "" {
			body["code"] = code
		}
		c.JSON(http.StatusConflict, body)
	case isValidationError(err):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		log.Error("task move failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "task move failed"})
	}
}

// isClientDisconnect reports a request the caller abandoned. Handlers derive
// their context from c.Request.Context(), which the server cancels when the
// peer goes away, so context.Canceled here means the browser navigated,
// unmounted a component, or aborted an in-flight fetch. Nothing failed
// server-side and nobody is left to read a response.
//
// DeadlineExceeded is deliberately not included: that is our own timeout
// firing, and it stays a logged 500.
func isClientDisconnect(err error) bool {
	return err != nil && errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func abortClientDisconnect(c *gin.Context) {
	c.AbortWithStatus(statusClientClosedRequest)
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) ||
		errors.Is(err, taskrepo.ErrWorkspaceNotFound) ||
		errors.Is(err, taskrepo.ErrNoPrimarySession) ||
		errors.Is(err, service.ErrDocumentNotFound) ||
		errors.Is(err, service.ErrTaskPlanNotFound) ||
		errors.Is(err, service.ErrRevisionNotFound) {
		return true
	}
	// Legacy fallback for repository methods (sessions, environments, etc.)
	// that have not yet adopted a typed sentinel.
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func isMoveConflict(err error) bool {
	return moveConflictCode(err) != ""
}

func moveConflictCode(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "active session"):
		return moveConflictCodeActiveSession
	case strings.Contains(msg, "archived tasks cannot be moved"):
		return moveConflictCodeArchived
	case strings.Contains(msg, "different workspace"):
		return moveConflictCodeDifferentWorkspace
	case strings.Contains(msg, "does not belong to target workflow"):
		return moveConflictCodeWorkflowStep
	case strings.Contains(msg, "wip limit exceeded"):
		return moveConflictCodeWIPLimit
	default:
		return ""
	}
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrInvalidParent) || errors.Is(err, service.ErrAutoTitleUnsupportedForOffice) {
		return true
	}
	if errors.Is(err, service.ErrTaskTitleTooLong) {
		return true
	}
	if errors.Is(err, service.ErrExternalIDInvalid) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "pending approval") ||
		strings.Contains(msg, "validation") ||
		strings.Contains(msg, "required") ||
		strings.Contains(msg, "invalid") ||
		strings.Contains(msg, "not allowed in a git ref name")
}

func isTaskCreateValidationError(err error) bool {
	if err == nil {
		return false
	}
	return isValidationError(err) ||
		strings.Contains(strings.ToLower(err.Error()), "workflow not found")
}
