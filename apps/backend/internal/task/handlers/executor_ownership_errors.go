package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/sshconnections"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func executorResourceError(err error) (int, string, string) {
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
	for _, response := range sshExecutorWriteErrors {
		if errors.Is(err, response.cause) {
			return response.status, response.code, response.cause.Error()
		}
	}
	return 0, "", ""
}

func writeExecutorResourceError(c *gin.Context, err error) bool {
	status, _, message := executorResourceError(err)
	if status == 0 {
		return false
	}
	c.JSON(status, gin.H{"error": message})
	return true
}

var sshExecutorWriteErrors = [...]struct {
	cause  error
	status int
	code   string
}{
	{sshconnections.ErrInvalidRoute, http.StatusBadRequest, ws.ErrorCodeValidation},
	{sshconnections.ErrForbidden, http.StatusForbidden, ws.ErrorCodeForbidden},
	{sshconnections.ErrConflict, http.StatusConflict, ws.ErrorCodeConflict},
	{sshconnections.ErrNotFound, http.StatusNotFound, ws.ErrorCodeNotFound},
	{sshconnections.ErrDisabled, http.StatusServiceUnavailable, ws.ErrorCodeUnavailable},
}
