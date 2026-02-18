package opencode

import (
	"encoding/json"
	"testing"
)

func TestSDKEventEnvelope_ParseSDKEvent(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantType  string
		wantError bool
	}{
		{
			name:     "message.updated event",
			input:    `{"type":"message.updated","properties":{"info":{"id":"123","sessionID":"sess-1","role":"assistant"}}}`,
			wantType: SDKEventMessageUpdated,
		},
		{
			name:     "message.part.updated event",
			input:    `{"type":"message.part.updated","properties":{"part":{"type":"text","text":"hello"}}}`,
			wantType: SDKEventMessagePartUpdated,
		},
		{
			name:     "permission.asked event",
			input:    `{"type":"permission.asked","properties":{"id":"perm-1","sessionID":"sess-1","permission":"edit"}}`,
			wantType: SDKEventPermissionAsked,
		},
		{
			name:     "session.idle event",
			input:    `{"type":"session.idle","properties":{"sessionID":"sess-1"}}`,
			wantType: SDKEventSessionIdle,
		},
		{
			name:     "session.error event",
			input:    `{"type":"session.error","properties":{"sessionID":"sess-1","error":{"message":"something went wrong"}}}`,
			wantType: SDKEventSessionError,
		},
		{
			name:      "invalid JSON",
			input:     `{invalid`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseSDKEvent([]byte(tt.input))
			if tt.wantError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if event.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, event.Type)
			}
		})
	}
}

func TestParseMessageUpdated(t *testing.T) {
	input := `{"info":{"id":"msg-123","sessionID":"sess-456","role":"assistant","model":{"providerID":"anthropic","modelID":"claude-3-sonnet"},"tokens":{"input":100,"output":50,"cache":{"read":20}}}}`

	props, err := ParseMessageUpdated(json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if props.Info.ID != "msg-123" {
		t.Errorf("expected ID 'msg-123', got %s", props.Info.ID)
	}
	if props.Info.SessionID != "sess-456" {
		t.Errorf("expected sessionID 'sess-456', got %s", props.Info.SessionID)
	}
	if props.Info.Role != "assistant" {
		t.Errorf("expected role 'assistant', got %s", props.Info.Role)
	}
	if props.Info.Model == nil {
		t.Error("expected model to be set")
	} else if props.Info.Model.ProviderID != "anthropic" {
		t.Errorf("expected providerID 'anthropic', got %s", props.Info.Model.ProviderID)
	}
	if props.Info.Tokens == nil {
		t.Error("expected tokens to be set")
	} else {
		if props.Info.Tokens.Input != 100 {
			t.Errorf("expected input tokens 100, got %d", props.Info.Tokens.Input)
		}
		if props.Info.Tokens.Output != 50 {
			t.Errorf("expected output tokens 50, got %d", props.Info.Tokens.Output)
		}
		if props.Info.Tokens.Cache == nil || props.Info.Tokens.Cache.Read != 20 {
			t.Error("expected cache read 20")
		}
	}
}

func TestParseMessagePartUpdated(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantType string
		wantText string
		wantID   string
	}{
		{
			name:     "text part with ID",
			input:    `{"part":{"id":"part-123","type":"text","messageID":"msg-1","sessionID":"sess-1","text":"Hello world"},"delta":"Hello"}`,
			wantType: PartTypeText,
			wantText: "Hello world",
			wantID:   "part-123",
		},
		{
			name:     "text part without ID (backwards compatibility)",
			input:    `{"part":{"type":"text","messageID":"msg-1","sessionID":"sess-1","text":"Hello world"},"delta":"Hello"}`,
			wantType: PartTypeText,
			wantText: "Hello world",
			wantID:   "",
		},
		{
			name:     "reasoning part",
			input:    `{"part":{"id":"reason-1","type":"reasoning","messageID":"msg-1","sessionID":"sess-1","text":"Let me think..."}}`,
			wantType: PartTypeReasoning,
			wantText: "Let me think...",
			wantID:   "reason-1",
		},
		{
			name:     "tool part",
			input:    `{"part":{"id":"tool-1","type":"tool","messageID":"msg-1","sessionID":"sess-1","callID":"call-1","tool":"bash","state":{"status":"running","title":"Running command"}}}`,
			wantType: PartTypeTool,
			wantID:   "tool-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props, err := ParseMessagePartUpdated(json.RawMessage(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if props.Part.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, props.Part.Type)
			}
			if tt.wantText != "" && props.Part.Text != tt.wantText {
				t.Errorf("expected text %q, got %q", tt.wantText, props.Part.Text)
			}
			if props.Part.ID != tt.wantID {
				t.Errorf("expected ID %q, got %q", tt.wantID, props.Part.ID)
			}
		})
	}
}

func TestParsePermissionAsked(t *testing.T) {
	input := `{"id":"perm-123","sessionID":"sess-456","permission":"bash","patterns":["npm run *"],"metadata":{"command":"npm run test"},"tool":{"callID":"call-789"}}`

	props, err := ParsePermissionAsked(json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if props.ID != "perm-123" {
		t.Errorf("expected ID 'perm-123', got %s", props.ID)
	}
	if props.SessionID != "sess-456" {
		t.Errorf("expected sessionID 'sess-456', got %s", props.SessionID)
	}
	if props.Permission != "bash" {
		t.Errorf("expected permission 'bash', got %s", props.Permission)
	}
	if len(props.Patterns) != 1 || props.Patterns[0] != "npm run *" {
		t.Errorf("expected patterns [npm run *], got %v", props.Patterns)
	}
	if props.Tool == nil || props.Tool.CallID != "call-789" {
		t.Error("expected tool with callID 'call-789'")
	}
	if cmd, ok := props.Metadata["command"].(string); !ok || cmd != "npm run test" {
		t.Errorf("expected metadata command 'npm run test', got %v", props.Metadata)
	}
}

func TestParseSessionIdle(t *testing.T) {
	input := `{"sessionID":"sess-123"}`

	props, err := ParseSessionIdle(json.RawMessage(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if props.SessionID != "sess-123" {
		t.Errorf("expected sessionID 'sess-123', got %s", props.SessionID)
	}
}

func TestParseSessionError(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantKind    string
		wantMessage string
	}{
		{
			name:        "error with name and data.message",
			input:       `{"sessionID":"sess-1","error":{"name":"ProviderAuthError","data":{"message":"API key invalid"}}}`,
			wantKind:    "ProviderAuthError",
			wantMessage: "API key invalid",
		},
		{
			name:        "error with type and message",
			input:       `{"sessionID":"sess-1","error":{"type":"RateLimitError","message":"Rate limit exceeded"}}`,
			wantKind:    "RateLimitError",
			wantMessage: "Rate limit exceeded",
		},
		{
			name:  "no error",
			input: `{"sessionID":"sess-1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			props, err := ParseSessionError(json.RawMessage(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantKind == "" {
				if props.Error != nil {
					t.Error("expected no error, but got one")
				}
				return
			}

			if props.Error == nil {
				t.Fatal("expected error, got nil")
			}

			if props.Error.GetKind() != tt.wantKind {
				t.Errorf("expected kind %s, got %s", tt.wantKind, props.Error.GetKind())
			}
			if props.Error.GetMessage() != tt.wantMessage {
				t.Errorf("expected message %s, got %s", tt.wantMessage, props.Error.GetMessage())
			}
		})
	}
}

func TestSDKError_GetKind(t *testing.T) {
	tests := []struct {
		name     string
		err      SDKError
		wantKind string
	}{
		{
			name:     "name takes precedence",
			err:      SDKError{Name: "AuthError", Type: "SomeType"},
			wantKind: "AuthError",
		},
		{
			name:     "falls back to type",
			err:      SDKError{Type: "SomeType"},
			wantKind: "SomeType",
		},
		{
			name:     "returns unknown",
			err:      SDKError{},
			wantKind: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.GetKind(); got != tt.wantKind {
				t.Errorf("expected kind %s, got %s", tt.wantKind, got)
			}
		})
	}
}

func TestSDKError_GetMessage(t *testing.T) {
	tests := []struct {
		name        string
		err         SDKError
		wantMessage string
	}{
		{
			name: "data.message takes precedence",
			err: SDKError{
				Message: "outer message",
				Data:    &struct{ Message string `json:"message,omitempty"` }{Message: "inner message"},
			},
			wantMessage: "inner message",
		},
		{
			name:        "falls back to message",
			err:         SDKError{Message: "outer message"},
			wantMessage: "outer message",
		},
		{
			name:        "returns empty",
			err:         SDKError{},
			wantMessage: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.GetMessage(); got != tt.wantMessage {
				t.Errorf("expected message %s, got %s", tt.wantMessage, got)
			}
		})
	}
}

func TestToolStateUpdate_Statuses(t *testing.T) {
	tests := []struct {
		status string
	}{
		{ToolStatusPending},
		{ToolStatusRunning},
		{ToolStatusCompleted},
		{ToolStatusError},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			state := ToolStateUpdate{Status: tt.status}
			if state.Status != tt.status {
				t.Errorf("expected status %s, got %s", tt.status, state.Status)
			}
		})
	}
}

func TestExecutorEvent_JSON(t *testing.T) {
	event := ExecutorEvent{
		Type:      EventTypeSessionStart,
		SessionID: "sess-123",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed ExecutorEvent
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Type != event.Type {
		t.Errorf("expected type %s, got %s", event.Type, parsed.Type)
	}
	if parsed.SessionID != event.SessionID {
		t.Errorf("expected sessionID %s, got %s", event.SessionID, parsed.SessionID)
	}
}
