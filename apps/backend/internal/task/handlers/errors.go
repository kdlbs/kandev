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
	"go.uber.org/zap"
)

// statusClientClosedRequest is nginx's 499. Go defines no constant for it and
// no standard 4xx says "the client hung up", which is what actually happened.
const statusClientClosedRequest = 499

func handleNotFound(c *gin.Context, log *logger.Logger, err error, fallback string) {
	if isClientDisconnect(err) {
		abortClientDisconnect(c)
		return
	}
	if isNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": fallback})
		return
	}
	if errors.Is(err, service.ErrWIPLimitExceeded) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if isValidationError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	log.Error("request failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "request failed"})
}

func handleSelectedMoveError(c *gin.Context, log *logger.Logger, err error) {
	switch {
	case isClientDisconnect(err):
		abortClientDisconnect(c)
	case isNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "task or workflow not found"})
	case isMoveConflict(err):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
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
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "active session") ||
		strings.Contains(msg, "archived tasks cannot be moved") ||
		strings.Contains(msg, "different workspace") ||
		strings.Contains(msg, "does not belong to target workflow") ||
		strings.Contains(msg, "wip limit exceeded")
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
