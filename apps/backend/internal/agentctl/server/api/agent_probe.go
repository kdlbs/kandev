package api

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
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
	result := probeProcessTreeRecovered(s.logger, func() string {
		return s.procMgr.ProbeProcessTree(ctx, req.SessionID)
	})
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{"result": result})
	return resp
}

// probeProcessTreeRecovered runs fn (the process-tree walk) with panic
// recovery, resolving to "unknown" on a panic instead of letting it escape.
// AC-46 condition 7 ("the port implementation panics" -> unknown) is already
// handled on the backend orchestrator's side of the WebSocket boundary
// (orchestrator.Service.runProbe's own recover) — but that recover cannot
// catch a panic that happens here, inside walkProcessTree
// (probe_linux.go/probe_darwin.go) on the agentctl side. Unrecovered, that
// panic would crash the whole agentctl process and kill the user's entire
// in-flight agent session, a worse outcome than the "unknown" result AC-46
// intends. Mirrors the recover pattern already used in port_proxy.go and
// vscode_proxy.go in this same package (Review round 2 MUST-FIX 2).
func probeProcessTreeRecovered(log *logger.Logger, fn func() string) (result string) {
	result = "unknown"
	defer func() {
		if r := recover(); r != nil {
			if log != nil {
				log.Warn("background probe process-tree walk panicked, treating as unknown",
					zap.Any("panic", r))
			}
			result = "unknown"
		}
	}()
	return fn()
}
