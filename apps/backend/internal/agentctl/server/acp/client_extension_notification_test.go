package acp

import (
	"encoding/json"
	"io"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

// TestClientAllowedExtensionNotifications proves the exact unprefixed Copilot
// method reaches the handler only when the connection opts in, using the real
// JSON-RPC reader rather than calling the callback directly.
func TestClientAllowedExtensionNotifications(t *testing.T) {
	const vendorMethod = "github.com/copilot/sessionEvent"

	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	got := make(chan string, 1)
	_ = NewClientSideConnectionWithLogger(
		NewClient(WithExtensionNotificationHandler(func(method string, _ json.RawMessage) {
			got <- method
		})),
		clientToAgentWriter,
		agentToClientReader,
		nil,
		acpsdk.WithAllowedExtensionNotifications(vendorMethod),
	)

	if err := writeJSONRPCLine(agentToClientWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  vendorMethod,
		"params":  map[string]any{"sessionId": "s-1", "type": "session.usage_info"},
	}); err != nil {
		t.Fatalf("write vendor notification: %v", err)
	}

	select {
	case method := <-got:
		if method != vendorMethod {
			t.Fatalf("handler method = %q, want %q", method, vendorMethod)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for allowed vendor notification")
	}
	_ = clientToAgentReader
}

// TestClientDefaultRejectsCopilotVendorNotification proves the same method is
// ignored without the explicit opt-in.
func TestClientDefaultRejectsCopilotVendorNotification(t *testing.T) {
	clientToAgentReader, clientToAgentWriter := io.Pipe()
	agentToClientReader, agentToClientWriter := io.Pipe()
	t.Cleanup(func() {
		_ = clientToAgentReader.Close()
		_ = clientToAgentWriter.Close()
		_ = agentToClientReader.Close()
		_ = agentToClientWriter.Close()
	})

	got := make(chan string, 1)
	_ = acpsdk.NewClientSideConnection(
		NewClient(WithExtensionNotificationHandler(func(method string, _ json.RawMessage) {
			got <- method
		})),
		clientToAgentWriter,
		agentToClientReader,
	)

	if err := writeJSONRPCLine(agentToClientWriter, map[string]any{
		"jsonrpc": "2.0",
		"method":  "github.com/copilot/sessionEvent",
		"params":  map[string]any{"sessionId": "s-1", "type": "session.usage_info"},
	}); err != nil {
		t.Fatalf("write vendor notification: %v", err)
	}

	// A standard request after the notification acts as an ordering barrier.
	if err := writeJSONRPCLine(agentToClientWriter, map[string]any{
		"jsonrpc": "2.0",
		"id":      9,
		"method":  "fs/read_text_file",
		"params":  map[string]any{"sessionId": "s-1", "path": "/tmp/test.txt"},
	}); err != nil {
		t.Fatalf("write barrier request: %v", err)
	}
	_ = readJSONRPCLine(t, clientToAgentReader)

	select {
	case method := <-got:
		t.Fatalf("unexpected handler dispatch for %q without opt-in", method)
	default:
	}
}
