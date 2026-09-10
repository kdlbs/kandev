package secrets

import (
	"errors"
	"net/http"

	ws "github.com/kandev/kandev/pkg/websocket"
)

func classifyDeleteError(err error) (int, string, string, map[string]any) {
	var inUse *InUseError
	if errors.As(err, &inUse) {
		return http.StatusConflict, ws.ErrorCodeConflict, inUse.Error(), map[string]any{
			"code": "secret_in_use", "references": inUse.References,
		}
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrWorkspaceAccessDenied) {
		return http.StatusNotFound, ws.ErrorCodeNotFound, "secret not found", map[string]any{}
	}
	return http.StatusInternalServerError, ws.ErrorCodeInternalError, "failed to delete secret", map[string]any{}
}
