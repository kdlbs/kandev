package acp

import (
	"context"
	"errors"

	"github.com/coder/acp-go-sdk"
	"go.uber.org/zap"
)

// Kandev persists conversation history itself. Advertised session/resume
// restores the provider conversation without transferring that history again.
func (a *Adapter) restoreSessionState(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	capabilities acp.AgentCapabilities,
	req acp.LoadSessionRequest,
) (acp.LoadSessionResponse, error) {
	if capabilities.SessionCapabilities.Resume == nil {
		return conn.LoadSession(ctx, req)
	}
	a.logger.Info("resuming session without history replay", zap.String("session_id", string(req.SessionId)))
	resp, err := conn.ResumeSession(ctx, acp.ResumeSessionRequest{
		SessionId:  req.SessionId,
		Cwd:        req.Cwd,
		McpServers: req.McpServers,
	})
	var requestErr *acp.RequestError
	if ctx.Err() == nil && capabilities.LoadSession && errors.As(err, &requestErr) && requestErr.Code == -32601 {
		a.logger.Warn("advertised session/resume unsupported, loading session history",
			zap.String("session_id", string(req.SessionId)))
		return conn.LoadSession(ctx, req)
	}
	return acp.LoadSessionResponse{
		Meta:          resp.Meta,
		ConfigOptions: resp.ConfigOptions,
		Modes:         resp.Modes,
	}, err
}
