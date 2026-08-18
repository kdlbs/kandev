package plugins

import (
	"context"

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
type pluginHostAgentConversationManager struct {
	pluginID string
	svc      AgentConversationService
}

func (m *pluginHostAgentConversationManager) Ensure(ctx context.Context, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	return m.svc.Ensure(ctx, m.pluginID, spec)
}

func (m *pluginHostAgentConversationManager) Dispatch(ctx context.Context, workspaceID, conversationKey, text, occurrenceKey string) (pluginsdk.AgentConversationDispatch, error) {
	return m.svc.Dispatch(ctx, m.pluginID, workspaceID, conversationKey, text, occurrenceKey)
}

func (m *pluginHostAgentConversationManager) Delete(ctx context.Context, workspaceID, conversationKey string) (int32, error) {
	return m.svc.Delete(ctx, m.pluginID, workspaceID, conversationKey)
}
