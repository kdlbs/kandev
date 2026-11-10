package pluginsdk

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// nilManagerHost implements AgentConversationHost but returns a nil manager,
// which is what any Host does when it decides the caller may not use agent
// conversations (an undeclared capability) or has not wired the service yet.
// The server adapter must answer with a typed error rather than dereferencing
// it: AgentConversationHost is a public interface, so the adapter cannot
// assume every implementation returns a usable manager.
type nilManagerHost struct {
	recordingHost
}

func (h *nilManagerHost) AgentConversations() AgentConversationManager { return nil }

func TestHost_AgentConversations_NilManagerIsTypedErrorNotPanic(t *testing.T) {
	host := dialHostOverBufconn(t, &nilManagerHost{})
	manager, ok := AgentConversations(host)
	if !ok {
		t.Fatal("expected the client to expose an agent conversation manager")
	}

	t.Run("ensure", func(t *testing.T) {
		_, _, err := manager.Ensure(context.Background(), AgentConversationSpec{
			WorkspaceID: "ws1", ConversationKey: "coordinator",
		})
		assertUnimplemented(t, err)
	})

	t.Run("dispatch", func(t *testing.T) {
		_, err := manager.Dispatch(context.Background(), "ws1", "coordinator", "WAKE:CYCLE", "occ-1")
		assertUnimplemented(t, err)
	})

	t.Run("delete", func(t *testing.T) {
		_, err := manager.Delete(context.Background(), "ws1", "coordinator")
		assertUnimplemented(t, err)
	})
}

func assertUnimplemented(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if got := status.Code(err); got != codes.Unimplemented {
		t.Fatalf("expected Unimplemented, got %s (%v)", got, err)
	}
}
