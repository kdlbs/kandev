package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	return log
}

func TestClient_SendUserMessage(t *testing.T) {
	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(""), newTestLogger())

	err := client.SendUserMessage("Hello, Claude!")
	if err != nil {
		t.Fatalf("SendUserMessage() error = %v", err)
	}

	// Parse what was written
	var msg UserMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &msg); err != nil {
		t.Fatalf("failed to parse sent message: %v", err)
	}

	if msg.Type != MessageTypeUser {
		t.Errorf("Type = %q, want %q", msg.Type, MessageTypeUser)
	}
	if msg.Message.Role != "user" {
		t.Errorf("Message.Role = %q, want %q", msg.Message.Role, "user")
	}
	if msg.Message.Content != "Hello, Claude!" {
		t.Errorf("Message.Content = %q, want %q", msg.Message.Content, "Hello, Claude!")
	}
}

func TestClient_SendControlResponse(t *testing.T) {
	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(""), newTestLogger())

	resp := &ControlResponseMessage{
		Type:      MessageTypeControlResponse,
		RequestID: "req123",
		Response: &ControlResponse{
			Subtype: "success",
			Result: &PermissionResult{
				Behavior: BehaviorAllow,
			},
		},
	}

	err := client.SendControlResponse(resp)
	if err != nil {
		t.Fatalf("SendControlResponse() error = %v", err)
	}

	// Parse what was written
	var parsed ControlResponseMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &parsed); err != nil {
		t.Fatalf("failed to parse sent message: %v", err)
	}

	if parsed.RequestID != "req123" {
		t.Errorf("RequestID = %q, want %q", parsed.RequestID, "req123")
	}
}

func TestClient_SendControlRequest(t *testing.T) {
	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(""), newTestLogger())

	req := &SDKControlRequest{
		Type:      MessageTypeControlRequest,
		RequestID: "init123",
		Request: SDKControlRequestBody{
			Subtype: SubtypeInitialize,
		},
	}

	err := client.SendControlRequest(req)
	if err != nil {
		t.Fatalf("SendControlRequest() error = %v", err)
	}

	// Parse what was written
	var parsed SDKControlRequest
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &parsed); err != nil {
		t.Fatalf("failed to parse sent message: %v", err)
	}

	if parsed.Request.Subtype != SubtypeInitialize {
		t.Errorf("Request.Subtype = %q, want %q", parsed.Request.Subtype, SubtypeInitialize)
	}
}

func TestClient_HandleMessages(t *testing.T) {
	// Create input with multiple messages
	messages := []string{
		`{"type":"system","session_id":"sess123"}`,
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]}}`,
	}
	input := strings.Join(messages, "\n") + "\n"

	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(input), newTestLogger())

	var received []CLIMessage
	var mu sync.Mutex
	client.SetMessageHandler(func(msg *CLIMessage) {
		mu.Lock()
		received = append(received, *msg)
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)
	time.Sleep(50 * time.Millisecond) // Give time for processing

	mu.Lock()
	count := len(received)
	mu.Unlock()

	if count != 2 {
		t.Errorf("received %d messages, want 2", count)
	}
}

func TestClient_HandleControlRequest(t *testing.T) {
	// Create a control request message
	input := `{"type":"control_request","request_id":"req123","request":{"subtype":"can_use_tool","tool_name":"Bash"}}` + "\n"

	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(input), newTestLogger())

	var receivedReq *ControlRequest
	var receivedID string
	var mu sync.Mutex

	client.SetRequestHandler(func(requestID string, req *ControlRequest) {
		mu.Lock()
		receivedID = requestID
		receivedReq = req
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)
	time.Sleep(50 * time.Millisecond) // Give time for processing

	mu.Lock()
	defer mu.Unlock()

	if receivedID != "req123" {
		t.Errorf("requestID = %q, want %q", receivedID, "req123")
	}
	if receivedReq == nil {
		t.Fatal("receivedReq is nil")
	}
	if receivedReq.Subtype != SubtypeCanUseTool {
		t.Errorf("Subtype = %q, want %q", receivedReq.Subtype, SubtypeCanUseTool)
	}
}

func TestClient_Stop(t *testing.T) {
	// Use a pipe for continuous input
	pr, _ := io.Pipe()

	var buf bytes.Buffer
	client := NewClient(&buf, pr, newTestLogger())

	ctx := context.Background()
	client.Start(ctx)

	// Stop should not panic even if called multiple times
	client.Stop()
	client.Stop()
}

func TestClient_NoHandlerAutoReject(t *testing.T) {
	// Create a control request message
	input := `{"type":"control_request","request_id":"req123","request":{"subtype":"can_use_tool","tool_name":"Bash"}}` + "\n"

	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(input), newTestLogger())

	// No request handler set - should auto-reject

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)
	time.Sleep(50 * time.Millisecond) // Give time for processing

	// Should have sent an error response
	if buf.Len() == 0 {
		t.Error("expected error response to be sent")
	}

	var resp ControlResponseMessage
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Response == nil || resp.Response.Subtype != "error" {
		t.Error("expected error response")
	}
}

func TestClient_EmptyLines(t *testing.T) {
	// Test that empty lines are skipped
	input := "\n\n{\"type\":\"system\",\"session_id\":\"abc\"}\n\n"

	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(input), newTestLogger())

	var count int
	var mu sync.Mutex
	client.SetMessageHandler(func(msg *CLIMessage) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestClient_InvalidJSON(t *testing.T) {
	// Test that invalid JSON is handled gracefully
	input := "{invalid json}\n{\"type\":\"system\"}\n"

	var buf bytes.Buffer
	client := NewClient(&buf, strings.NewReader(input), newTestLogger())

	var count int
	var mu sync.Mutex
	client.SetMessageHandler(func(msg *CLIMessage) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Should still process the valid message
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
