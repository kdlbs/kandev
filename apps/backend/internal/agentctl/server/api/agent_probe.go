package api

import (
	"context"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// handleWSBackgroundProbe handles the agent.background.probe WS action.
// It walks the agent's process tree and reports whether any descendant process
// born at or after the most recent turn start is still alive.
func (s *Server) handleWSBackgroundProbe(ctx context.Context, msg *ws.Message) *ws.Message {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": "unknown"})
		return resp
	}
	result := s.procMgr.ProbeProcessTree(ctx, req.SessionID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": result})
	return resp
}
