package acpv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	acp "github.com/coder/acp-go-sdk"
	wire "github.com/coder/acp-go-sdk/experimental/v2"
	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type sessionAttachment struct {
	session  wire.SessionID
	replay   chan struct{}
	state    projection
	live     []wire.UpdateSessionNotification
	replayed bool
}

func (a *Adapter) GetSessionRestoreCapabilities() shared.SessionRestoreCapabilities {
	return shared.SessionRestoreCapabilities{
		SupportsNativeResume:  a.ACPProtocolVersion() == 2,
		SupportsDirectoryMove: true, RequiresNativeState: true,
	}
}

func (a *Adapter) SupportsSessionLoad() bool   { return false }
func (a *Adapter) SupportsSessionResume() bool { return a.ACPProtocolVersion() == 2 }

func (a *Adapter) NewSession(ctx context.Context, servers []types.McpServer) (string, error) {
	return a.openSession(ctx, "", servers, false)
}

func (a *Adapter) ResetSession(ctx context.Context, servers []types.McpServer) (string, error) {
	return a.openSession(ctx, "", servers, false)
}

func (a *Adapter) LoadSession(ctx context.Context, id string, servers []types.McpServer) error {
	return a.ResumeSession(ctx, id, servers)
}

func (a *Adapter) ResumeSession(ctx context.Context, id string, servers []types.McpServer) error {
	if id == "" {
		return errors.New("ACP v2 resume requires a native session ID")
	}
	_, err := a.openSession(ctx, wire.SessionID(id), servers, true)
	return err
}

func (a *Adapter) openSession(ctx context.Context, id wire.SessionID, servers []types.McpServer, resume bool) (string, error) {
	if err := a.acquire(ctx); err != nil {
		return "", err
	}
	defer a.release()
	attach, err := a.beginAttachment(id)
	if err != nil {
		return "", err
	}
	defer func() {
		a.mu.Lock()
		a.attach = nil
		a.mu.Unlock()
	}()
	delivery := a.prepareMCP(servers)
	a.publishMCPStart(ctx, delivery)
	if delivery.err != nil {
		return "", delivery.err
	}
	session, options, commands, err := a.requestSession(ctx, attach, delivery.servers, resume)
	a.publishMCPResult(ctx, delivery, err)
	if err != nil {
		return "", safeRPCError("session setup", err)
	}
	previous := a.commitAttachment(ctx, attach, session, options, commands, resume)
	if previous != "" && previous != session {
		closeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, _ = a.client.CloseSession(closeCtx, wire.CloseSessionRequest{SessionID: previous})
	}
	return string(session), nil
}

func (a *Adapter) beginAttachment(id wire.SessionID) (*sessionAttachment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.info == nil || a.blocked {
		return nil, errors.New("ACP v2 session needs a new connection")
	}
	if a.turn != nil {
		return nil, errors.New("ACP v2 foreground work is active")
	}
	attach := &sessionAttachment{session: id, replay: make(chan struct{})}
	a.attach = attach
	return attach, nil
}

func (a *Adapter) requestSession(
	ctx context.Context, attach *sessionAttachment, servers []wire.MCPServer, resume bool,
) (wire.SessionID, []wire.SessionConfigOption, []wire.AvailableCommand, error) {
	if !resume {
		ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.cfg.AgentID, "session.new")
		defer span.End()
		reply, err := a.client.NewSession(ctx, wire.NewSessionRequest{Cwd: a.cfg.WorkDir, MCPServers: servers})
		if err == nil && reply.SessionID == "" {
			err = errors.New("ACP v2 session/new response is missing sessionId")
		}
		return reply.SessionID, reply.ConfigOptions, reply.AvailableCommands, err
	}
	ctx, span := shared.TraceProtocolRequest(ctx, shared.ProtocolACP, a.cfg.AgentID, "session.resume")
	defer span.End()
	reply, err := a.client.ResumeSession(ctx, wire.ResumeSessionRequest{
		SessionID: attach.session, Cwd: a.cfg.WorkDir, MCPServers: servers, ReplayFrom: wire.ReplayFromStart(),
	})
	if err != nil {
		return "", nil, nil, err
	}
	select {
	case <-attach.replay:
		return attach.session, reply.ConfigOptions, reply.AvailableCommands, nil
	case <-ctx.Done():
		return "", nil, nil, ctx.Err()
	case <-a.ctx.Done():
		return "", nil, nil, a.ctx.Err()
	case <-a.client.Done():
		return "", nil, nil, errors.New("ACP v2 disconnected during replay")
	}
}

func (a *Adapter) commitAttachment(
	ctx context.Context, attach *sessionAttachment, session wire.SessionID,
	options []wire.SessionConfigOption, commands []wire.AvailableCommand, resume bool,
) wire.SessionID {
	var previous wire.SessionID
	a.project(func(batch *eventBatch) {
		previous = a.session
		a.session = session
		a.state = attach.state
		a.attach = nil
		status := streams.SessionStatusNew
		var policy streams.SessionSettingsPolicy
		if resume {
			status = streams.SessionStatusResumed
			policy = streams.SessionSettingsPolicyFromContext(ctx)
		}
		batch.add(streams.AgentEvent{Type: streams.EventTypeSessionStatus, SessionID: string(session), SessionStatus: status})
		a.projectSettings(options, policy, map[string]any{streams.ConfigOptionsSourceKey: streams.ConfigOptionsSourceSessionSetup}, batch)
		a.projectCommands(commands, batch)
		for _, update := range attach.live {
			a.projectUpdate(update, batch)
		}
	})
	return previous
}

func (a *Adapter) GetSessionOperationAuthMethods() []streams.AuthMethodInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.info == nil {
		return nil
	}
	methods := make([]streams.AuthMethodInfo, 0, len(a.info.AuthMethods))
	for _, method := range a.info.AuthMethods {
		methods = append(methods, streams.AuthMethodInfo{
			ID: string(method.MethodID), Name: method.Name, Description: valueOrZero(method.Description),
		})
	}
	return methods
}

func (a *Adapter) Authenticate(ctx context.Context, methodID string) error {
	if err := a.acquire(ctx); err != nil {
		return err
	}
	defer a.release()
	if a.ACPProtocolVersion() == 0 {
		return errors.New("ACP v2 adapter is uninitialized")
	}
	_, err := a.client.LoginAuth(ctx, wire.LoginAuthRequest{MethodID: wire.AuthMethodID(methodID)})
	return safeRPCError("authentication", err)
}

func valueOrZero[T any](value *T) T {
	if value != nil {
		return *value
	}
	var zero T
	return zero
}

func safeRPCError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var rpcErr *acp.RequestError
	if errors.As(err, &rpcErr) {
		return &acp.RequestError{Code: rpcErr.Code, Message: fmt.Sprintf("ACP v2 %s failed (code %d)", operation, rpcErr.Code)}
	}
	return fmt.Errorf("ACP v2 %s failed", operation)
}
