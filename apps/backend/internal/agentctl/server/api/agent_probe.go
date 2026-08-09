package api

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// handleWSBackgroundProbe handles the agent.background.probe WS action.
// It reports whether background processes spawned during the given ACP session
// are still alive. This stub returns "unknown" for all calls; the real
// process-tree walk is implemented in a later task.
func (s *Server) handleWSBackgroundProbe(_ context.Context, msg *ws.Message) *ws.Message {
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": "unknown"})
	return resp
}
