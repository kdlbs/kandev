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
	// occurrence_key matches a previously dispatched occurrence, and
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

// ManagedAgentConversationService is the lifecycle-aware service slice used
// by Host v2. Installation identity is supplied by the host, never by the
// plugin request.
type ManagedAgentConversationService interface {
	EnsureManaged(ctx context.Context, pluginID, installationID string, spec pluginsdk.ManagedAgentConversationSpec, operationID, payloadDigest string) (pluginsdk.ManagedAgentConversationDescriptor, string, error)
	GetManaged(ctx context.Context, installationID, workspaceID, instanceKey string) (pluginsdk.ManagedAgentConversationDescriptor, error)
	ListManaged(ctx context.Context, installationID, workspaceID string) ([]pluginsdk.ManagedAgentConversationDescriptor, error)
	SetManagedPaused(ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, paused bool, operationID, payloadDigest string) (pluginsdk.ManagedAgentConversationDescriptor, error)
	DeleteManaged(ctx context.Context, installationID, workspaceID, instanceKey string, expectedRevision uint64, operationID, payloadDigest string) error
	PauseManagedForInstallation(ctx context.Context, installationID string) error
	InvalidateManagedForInstallationWorkspace(ctx context.Context, installationID, workspaceID string) error
	DetachManagedForInstallation(ctx context.Context, installationID string) error
}

// ManagedAgentInputService is the optional durable-input slice of the task
// service used by Host v2. Keeping it separate preserves existing managed
// conversation service fakes and makes unsupported input operations explicit.
type ManagedAgentInputService interface {
	EnqueueManagedInput(ctx context.Context, installationID, hostInputID string, input pluginsdk.ManagedAgentInputEnqueue, operationID, payloadDigest string) (pluginsdk.ManagedAgentInputReceipt, bool, error)
	GetManagedInput(ctx context.Context, installationID string, query pluginsdk.ManagedAgentInputQuery) (pluginsdk.ManagedAgentInputReceipt, error)
	ListManagedInputs(ctx context.Context, installationID string, query pluginsdk.ManagedAgentInputListQuery) (pluginsdk.ManagedAgentInputPage, error)
	CancelManagedInput(ctx context.Context, installationID string, input pluginsdk.ManagedAgentInputCancel, operationID, payloadDigest string) (pluginsdk.ManagedAgentInputReceipt, bool, error)
	DispatchManagedInput(ctx context.Context, installationID string, input pluginsdk.ManagedAgentConversationDispatch, operationID, payloadDigest string) (pluginsdk.ManagedAgentDispatchStatus, pluginsdk.ManagedAgentConversationDescriptor, error)
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
