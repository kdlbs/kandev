package plugins

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// stubAgentConversationService records the plugin id every call is stamped
// with, so ownership can be asserted without a real task service.
type stubAgentConversationService struct {
	lastPluginID string
}

func (s *stubAgentConversationService) Ensure(_ context.Context, pluginID string, spec pluginsdk.AgentConversationSpec) (pluginsdk.AgentConversationDescriptor, string, error) {
	s.lastPluginID = pluginID
	return pluginsdk.AgentConversationDescriptor{TaskID: "task-1", WorkspaceID: spec.WorkspaceID}, "created", nil
}

func (s *stubAgentConversationService) Dispatch(_ context.Context, pluginID, _, _, _, _ string) (pluginsdk.AgentConversationDispatch, error) {
	s.lastPluginID = pluginID
	return pluginsdk.AgentConversationDispatch{Status: "sent"}, nil
}

func (s *stubAgentConversationService) Delete(_ context.Context, pluginID, _, _ string) (int32, error) {
	s.lastPluginID = pluginID
	return 1, nil
}

func (s *stubAgentConversationService) DeleteAllForPlugin(_ context.Context, pluginID string) (int32, error) {
	s.lastPluginID = pluginID
	return 1, nil
}

func agentConversationHost(capability bool, svc AgentConversationService) *pluginHost {
	return &pluginHost{
		pluginID:           "p1",
		capabilities:       manifest.Capabilities{AgentConversation: capability},
		agentConversations: func() AgentConversationService { return svc },
	}
}

func assertCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %s, got nil", want)
	}
	if got := status.Code(err); got != want {
		t.Fatalf("expected code %s, got %s (%v)", want, got, err)
	}
}

// A plugin that never declared agent_conversation must be denied on every
// method — and must not panic. AgentConversations() previously returned a nil
// interface here, which the gRPC host server dereferenced.
func TestAgentConversationsDeniedWithoutCapability(t *testing.T) {
	h := agentConversationHost(false, &stubAgentConversationService{})

	manager := h.AgentConversations()
	if manager == nil {
		t.Fatal("AgentConversations() must never return nil; the gRPC server calls methods on it directly")
	}

	_, _, err := manager.Ensure(context.Background(), pluginsdk.AgentConversationSpec{WorkspaceID: "ws1", ConversationKey: "coordinator"})
	assertCode(t, err, codes.PermissionDenied)

	_, err = manager.Dispatch(context.Background(), "ws1", "coordinator", "hello", "occ-1")
	assertCode(t, err, codes.PermissionDenied)

	_, err = manager.Delete(context.Background(), "ws1", "coordinator")
	assertCode(t, err, codes.PermissionDenied)
}

// The capability alone is not enough: an unwired service must report
// Unavailable rather than crashing.
func TestAgentConversationsUnavailableWhenNotWired(t *testing.T) {
	h := &pluginHost{pluginID: "p1", capabilities: manifest.Capabilities{AgentConversation: true}}

	_, _, err := h.AgentConversations().Ensure(context.Background(), pluginsdk.AgentConversationSpec{WorkspaceID: "ws1", ConversationKey: "coordinator"})
	assertCode(t, err, codes.Unavailable)

	nilSvc := agentConversationHost(true, nil)
	_, err = nilSvc.AgentConversations().Dispatch(context.Background(), "ws1", "coordinator", "hello", "occ-1")
	assertCode(t, err, codes.Unavailable)
}

// A declared capability reaches the service, stamped with the calling
// plugin's own id so one plugin can never act on another's conversation.
func TestAgentConversationsStampCallingPluginID(t *testing.T) {
	svc := &stubAgentConversationService{}
	h := agentConversationHost(true, svc)

	if _, _, err := h.AgentConversations().Ensure(context.Background(), pluginsdk.AgentConversationSpec{WorkspaceID: "ws1", ConversationKey: "coordinator"}); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if svc.lastPluginID != "p1" {
		t.Fatalf("expected plugin id %q, got %q", "p1", svc.lastPluginID)
	}

	svc.lastPluginID = ""
	if _, err := h.AgentConversations().Delete(context.Background(), "ws1", "coordinator"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if svc.lastPluginID != "p1" {
		t.Fatalf("expected plugin id %q, got %q", "p1", svc.lastPluginID)
	}
}
