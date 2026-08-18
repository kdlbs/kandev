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
	return svc.Ensure(ctx, pluginID, spec)
}

func (m *pluginHostAgentConversationManager) Dispatch(ctx context.Context, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	pluginID, svc, err := m.resolve()
	if err != nil {
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
