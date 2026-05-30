package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

func handleNotFound(c *gin.Context, log *logger.Logger, err error, fallback string) {
	if isNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": fallback})
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

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, taskrepo.ErrTaskNotFound) ||
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
		strings.Contains(msg, "does not belong to target workflow")
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "pending approval") ||
		strings.Contains(msg, "validation") ||
		strings.Contains(msg, "required") ||
		strings.Contains(msg, "invalid")
}
