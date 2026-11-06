package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func executorOwnershipError(err error) (int, string, string) {
	switch {
	case errors.Is(err, repoerrors.ErrExecutorProfileNotFound), errors.Is(err, models.ErrExecutorNotFound):
		return http.StatusNotFound, ws.ErrorCodeNotFound, "Executor profile not found"
	case errors.Is(err, service.ErrForbidden), errors.Is(err, service.ErrRemoteDockerAdminRequired):
		return http.StatusForbidden, ws.ErrorCodeForbidden, "Executor configuration access denied"
	case errors.Is(err, models.ErrExecutorOwnershipChanged):
		return http.StatusConflict, ws.ErrorCodeConflict, "Executor ownership changed. Reload before saving."
	case errors.Is(err, models.ErrExecutorOwnershipInvalid):
		return http.StatusBadRequest, ws.ErrorCodeValidation, "Invalid executor ownership assignment"
	}
	return 0, "", ""
}

func writeExecutorOwnershipError(c *gin.Context, err error) bool {
	status, _, message := executorOwnershipError(err)
	if status == 0 {
		return false
	}
	c.JSON(status, gin.H{"error": message})
	return true
}
