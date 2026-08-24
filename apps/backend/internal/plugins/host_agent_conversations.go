package plugins

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

// AgentConversationService is the narrow slice of the task service that
// manages hidden workflowless ephemeral agent conversations. It is satisfied
// by the adapter in internal/backendapp, avoiding an import cycle with
// internal/task/service.
type AgentConversationService interface {
	// Ensure creates or repairs one conversation per
	// (pluginID, workspaceID, conversationKey). Returns the existing
	// descriptor when one already exists. Returns status "configuration_required"
	// when the referenced agent profile is missing, disabled, or incompatible.
	Ensure(ctx context.Context, pluginID string, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error)

	// Dispatch sends text to an ensured conversation. occurrenceKey provides
	// stable idempotency. Returns status "duplicate_occurrence" when
	// occurrenceKey matches a previously dispatched occurrence, and
	// "skipped_busy" when the session is mid-turn.
	Dispatch(ctx context.Context, pluginID, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error)

	// Delete removes all conversations owned by pluginID matching workspaceID
	// and conversationKey.
	Delete(ctx context.Context, pluginID, workspaceID, conversationKey string) (int32, error)

	// DeleteAllForPlugin removes every managed conversation owned by pluginID
	// across every workspace and conversation key. Unlike Delete/Ensure/
	// Dispatch, this is not part of pluginsdk.AgentConversationManager — it is
	// never callable by the plugin's own gRPC requests, only by the host's
	// Uninstall lifecycle (Service.Uninstall), which calls it directly through
	// this interface. Returns the number of managed conversations removed.
	DeleteAllForPlugin(ctx context.Context, pluginID string) (int32, error)
}

// pluginHostAgentConversationManager implements pluginsdk.AgentConversationManager,
// wrapping the service layer with the plugin's identity for ownership checks.
// Every method re-checks the agent_conversation capability and the live
// service wiring, so an undeclared capability is denied with a typed
// PermissionDenied rather than crashing the host.
type pluginHostAgentConversationManager struct {
	host *pluginHost
}

// resolve returns the owning plugin id and the live conversation service, or
// a typed error when the plugin did not declare agent_conversation (denied)
// or the service is not wired yet (unavailable).
func (m *pluginHostAgentConversationManager) resolve() (string, AgentConversationService, error) {
	h := m.host
	if h == nil || !h.capabilities.AgentConversation {
		return "", nil, permissionDenied("agent_conversation")
	}
	if h.agentConversations == nil {
		return "", nil, errAgentConversationsUnavailable()
	}
	svc := h.agentConversations()
	if svc == nil {
		return "", nil, errAgentConversationsUnavailable()
	}
	return h.pluginID, svc, nil
}

func errAgentConversationsUnavailable() error {
	return status.Error(codes.Unavailable, "agent conversations are not available on this host")
}

func (m *pluginHostAgentConversationManager) Ensure(ctx context.Context, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	pluginID, svc, err := m.resolve()
	if err != nil {
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	if err := m.authorizeWorkspaceRun(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey); err != nil {
		if status.Code(err) == codes.FailedPrecondition {
			return pluginsdk.AgentConversationDescriptor{}, "configuration_required", nil
		}
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	descriptor, result, err := svc.Ensure(ctx, pluginID, spec)
	if err != nil || !m.requiresWorkspaceAgentPrincipal() {
		return descriptor, result, err
	}
	if err := m.bindWorkspaceRun(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey, descriptor.TaskID, descriptor.SessionID); err != nil {
		// A principal revocation may race task/session creation. Delete only a
		// brand-new run; an existing run may belong to a prior valid binding and
		// must merely become unusable, not be destroyed by a retry.
		if result == "created" {
			_, _ = svc.Delete(ctx, pluginID, spec.WorkspaceID, spec.ConversationKey)
		}
		return pluginsdk.AgentConversationDescriptor{}, "", err
	}
	return descriptor, result, nil
}

func (m *pluginHostAgentConversationManager) Dispatch(ctx context.Context, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	pluginID, svc, err := m.resolve()
	if err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	if err := m.authorizeWorkspaceRun(ctx, pluginID, workspaceID, conversationKey); err != nil {
		return pluginsdk.AgentConversationDispatch{}, err
	}
	return svc.Dispatch(ctx, pluginID, workspaceID, conversationKey, text, occurrenceKey)
}

func (m *pluginHostAgentConversationManager) Delete(ctx context.Context, workspaceID, conversationKey string) (int32, error) {
	pluginID, svc, err := m.resolve()
	if err != nil {
		return 0, err
	}
	return svc.Delete(ctx, pluginID, workspaceID, conversationKey)
}

// requiresWorkspaceAgentPrincipal opts a plugin into principal-bound managed
// runs by its dedicated read capability. Existing agent_conversation plugins
// that have not declared this new resource retain their source-compatible
// managed-conversation behaviour.
func (m *pluginHostAgentConversationManager) requiresWorkspaceAgentPrincipal() bool {
	return m != nil && m.host != nil && m.host.capabilities.CanRead(resourceWorkspaceAgentPrincipals)
}

func (m *pluginHostAgentConversationManager) authorizeWorkspaceRun(ctx context.Context, pluginID, workspaceID, logicalKey string) error {
	if !m.requiresWorkspaceAgentPrincipal() {
		return nil
	}
	if m.host.workspaceAgentPrincipals == nil {
		return status.Error(codes.Unavailable, "workspace agent principals are not available on this host")
	}
	source := m.host.workspaceAgentPrincipals()
	if source == nil {
		return status.Error(codes.Unavailable, "workspace agent principals are not available on this host")
	}
	return source.AuthorizePluginWorkspaceAgentRun(ctx, pluginID, workspaceID, logicalKey)
}

func (m *pluginHostAgentConversationManager) bindWorkspaceRun(ctx context.Context, pluginID, workspaceID, logicalKey, taskID, sessionID string) error {
	if !m.requiresWorkspaceAgentPrincipal() {
		return nil
	}
	if m.host.workspaceAgentPrincipals == nil {
		return status.Error(codes.Unavailable, "workspace agent principals are not available on this host")
	}
	source := m.host.workspaceAgentPrincipals()
	if source == nil {
		return status.Error(codes.Unavailable, "workspace agent principals are not available on this host")
	}
	return source.BindPluginWorkspaceAgentRun(ctx, pluginID, workspaceID, logicalKey, taskID, sessionID)
}
