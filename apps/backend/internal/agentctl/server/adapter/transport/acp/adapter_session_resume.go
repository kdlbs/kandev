package acp

import (
	"context"
	"errors"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

// restoreSessionState skips history replay for compatible dialects that advertise
// session/resume. Other dialects need session/load to preserve legacy model state,
// which the SDK's ResumeSessionResponse cannot represent.
func (a *Adapter) restoreSessionState(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	capabilities acp.AgentCapabilities,
	req acp.LoadSessionRequest,
) (acp.LoadSessionResponse, error) {
	if !a.dialect.resumeWithoutReplay || capabilities.SessionCapabilities.Resume == nil {
		return a.loadSessionWithReplay(ctx, conn, req)
	}
	a.logger.Info("resuming session without history replay", zap.String("session_id", string(req.SessionId)))
	resumeCtx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.resume")
	span.SetAttributes(attribute.String("session_id", string(req.SessionId)))
	resp, err := conn.ResumeSession(resumeCtx, acp.ResumeSessionRequest{
		SessionId:  req.SessionId,
		Cwd:        req.Cwd,
		McpServers: req.McpServers,
	})
	if err != nil {
		span.RecordError(err)
	}
	span.End()
	var requestErr *acp.RequestError
	if ctx.Err() == nil && capabilities.LoadSession && errors.As(err, &requestErr) && requestErr.Code == -32601 {
		a.logger.Warn("advertised session/resume unsupported, loading session history",
			zap.String("session_id", string(req.SessionId)))
		return a.loadSessionWithReplay(ctx, conn, req)
	}
	return acp.LoadSessionResponse{
		Meta:          resp.Meta,
		ConfigOptions: resp.ConfigOptions,
		Modes:         resp.Modes,
	}, err
}

// loadSessionWithReplay traces the history transfer separately from a preceding resume attempt.
func (a *Adapter) loadSessionWithReplay(
	ctx context.Context,
	conn *acp.ClientSideConnection,
	req acp.LoadSessionRequest,
) (acp.LoadSessionResponse, error) {
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.agentID, "session.load")
	defer span.End()
	span.SetAttributes(attribute.String("session_id", string(req.SessionId)))
	resp, err := conn.LoadSession(ctx, req)
	if err != nil {
		span.RecordError(err)
	}
	return resp, err
}
