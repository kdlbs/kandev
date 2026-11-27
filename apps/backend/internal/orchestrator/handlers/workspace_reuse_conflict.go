package handlers

import (
	"errors"

	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func workspaceReuseUnsupportedConflictResponse(msg *ws.Message, err error) (*ws.Message, error) {
	if !errors.Is(err, models.ErrWorkspaceReuseUnsupported) {
		return nil, nil
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict, models.WorkspaceReuseUnsupportedMessage,
		map[string]interface{}{
			"reason":      "workspace_reuse_unsupported",
			"recoverable": true,
			"retry":       models.WorkspaceReuseUnsupportedRetry,
		})
}
