package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
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

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
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
