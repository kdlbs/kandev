package claudecode

import (
	"encoding/json"
	"testing"
)

func TestCLIMessage_GetResultData(t *testing.T) {
	tests := []struct {
		name     string
		result   json.RawMessage
		wantNil  bool
		wantText string
	}{
		{
			name:    "empty result",
			result:  nil,
			wantNil: true,
		},
		{
			name:    "string result (error)",
			result:  json.RawMessage(`"error message"`),
			wantNil: true, // GetResultData returns nil for strings
		},
		{
			name:     "object result with text",
			result:   json.RawMessage(`{"text":"success message","session_id":"abc123"}`),
			wantNil:  false,
			wantText: "success message",
		},
		{
			name:    "invalid JSON",
			result:  json.RawMessage(`{invalid`),
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &CLIMessage{Result: tt.result}
			got := msg.GetResultData()
			switch {
			case tt.wantNil:
				if got != nil {
					t.Errorf("GetResultData() = %v, want nil", got)
				}
			case got == nil:
				t.Fatalf("GetResultData() = nil, want non-nil")
			case got.Text != tt.wantText:
				t.Errorf("GetResultData().Text = %q, want %q", got.Text, tt.wantText)
			}
		})
	}
}

func TestCLIMessage_GetResultString(t *testing.T) {
	tests := []struct {
		name   string
		result json.RawMessage
		want   string
	}{
		{
			name:   "empty result",
			result: nil,
			want:   "",
		},
		{
			name:   "string result",
			result: json.RawMessage(`"error message"`),
			want:   "error message",
		},
		{
			name:   "object result",
			result: json.RawMessage(`{"text":"success"}`),
			want:   "", // GetResultString returns empty for objects
		},
		{
			name:   "invalid JSON",
			result: json.RawMessage(`{invalid`),
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &CLIMessage{Result: tt.result}
			got := msg.GetResultString()
			if got != tt.want {
				t.Errorf("GetResultString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCLIMessage_JSONParsing(t *testing.T) {
	// Test parsing a system message
	systemJSON := `{"type":"system","session_id":"abc123","session_status":"active"}`
	var systemMsg CLIMessage
	if err := json.Unmarshal([]byte(systemJSON), &systemMsg); err != nil {
		t.Fatalf("failed to parse system message: %v", err)
	}
	if systemMsg.Type != MessageTypeSystem {
		t.Errorf("Type = %q, want %q", systemMsg.Type, MessageTypeSystem)
	}
	if systemMsg.SessionID != "abc123" {
		t.Errorf("SessionID = %q, want %q", systemMsg.SessionID, "abc123")
	}

	// Test parsing an assistant message
	assistantJSON := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}],"model":"claude-3"}}`
	var assistantMsg CLIMessage
	if err := json.Unmarshal([]byte(assistantJSON), &assistantMsg); err != nil {
		t.Fatalf("failed to parse assistant message: %v", err)
	}
	if assistantMsg.Type != MessageTypeAssistant {
		t.Errorf("Type = %q, want %q", assistantMsg.Type, MessageTypeAssistant)
	}
	if assistantMsg.Message == nil {
		t.Fatal("Message is nil")
	}
	if assistantMsg.Message.Model != "claude-3" {
		t.Errorf("Message.Model = %q, want %q", assistantMsg.Message.Model, "claude-3")
	}
}

func TestControlRequest_JSONParsing(t *testing.T) {
	// Test can_use_tool request
	jsonStr := `{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls -la"},"tool_use_id":"tool123"}`
	var req ControlRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to parse control request: %v", err)
	}
	if req.Subtype != SubtypeCanUseTool {
		t.Errorf("Subtype = %q, want %q", req.Subtype, SubtypeCanUseTool)
	}
	if req.ToolName != ToolBash {
		t.Errorf("ToolName = %q, want %q", req.ToolName, ToolBash)
	}
	if req.Input["command"] != "ls -la" {
		t.Errorf("Input[command] = %v, want %q", req.Input["command"], "ls -la")
	}
}

func TestControlResponseMessage_JSONMarshal(t *testing.T) {
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

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if parsed["type"] != MessageTypeControlResponse {
		t.Errorf("type = %v, want %q", parsed["type"], MessageTypeControlResponse)
	}
	if parsed["request_id"] != "req123" {
		t.Errorf("request_id = %v, want %q", parsed["request_id"], "req123")
	}
}

func TestUserMessage_JSONMarshal(t *testing.T) {
	msg := &UserMessage{
		Type: MessageTypeUser,
		Message: UserMessageBody{
			Role:    "user",
			Content: "Hello, Claude!",
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	expected := `{"type":"user","message":{"role":"user","content":"Hello, Claude!"}}`
	if string(data) != expected {
		t.Errorf("Marshal() = %s, want %s", string(data), expected)
	}
}

func TestContentBlock_Types(t *testing.T) {
	tests := []struct {
		name  string
		json  string
		check func(t *testing.T, block ContentBlock)
	}{
		{
			name: "text block",
			json: `{"type":"text","text":"Hello world"}`,
			check: func(t *testing.T, block ContentBlock) {
				if block.Type != "text" {
					t.Errorf("Type = %q, want %q", block.Type, "text")
				}
				if block.Text != "Hello world" {
					t.Errorf("Text = %q, want %q", block.Text, "Hello world")
				}
			},
		},
		{
			name: "thinking block",
			json: `{"type":"thinking","thinking":"Let me analyze..."}`,
			check: func(t *testing.T, block ContentBlock) {
				if block.Type != "thinking" {
					t.Errorf("Type = %q, want %q", block.Type, "thinking")
				}
				if block.Thinking != "Let me analyze..." {
					t.Errorf("Thinking = %q, want %q", block.Thinking, "Let me analyze...")
				}
			},
		},
		{
			name: "tool_use block",
			json: `{"type":"tool_use","id":"tool123","name":"Bash","input":{"command":"ls"}}`,
			check: func(t *testing.T, block ContentBlock) {
				if block.Type != "tool_use" {
					t.Errorf("Type = %q, want %q", block.Type, "tool_use")
				}
				if block.ID != "tool123" {
					t.Errorf("ID = %q, want %q", block.ID, "tool123")
				}
				if block.Name != "Bash" {
					t.Errorf("Name = %q, want %q", block.Name, "Bash")
				}
			},
		},
		{
			name: "tool_result block",
			json: `{"type":"tool_result","tool_use_id":"tool123","content":"output","is_error":false}`,
			check: func(t *testing.T, block ContentBlock) {
				if block.Type != "tool_result" {
					t.Errorf("Type = %q, want %q", block.Type, "tool_result")
				}
				if block.ToolUseID != "tool123" {
					t.Errorf("ToolUseID = %q, want %q", block.ToolUseID, "tool123")
				}
				if block.Content != "output" {
					t.Errorf("Content = %q, want %q", block.Content, "output")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var block ContentBlock
			if err := json.Unmarshal([]byte(tt.json), &block); err != nil {
				t.Fatalf("failed to parse: %v", err)
			}
			tt.check(t, block)
		})
	}
}

func TestModelUsageStats_ContextWindow(t *testing.T) {
	jsonStr := `{"context_window": 200000}`
	var stats ModelUsageStats
	if err := json.Unmarshal([]byte(jsonStr), &stats); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if stats.ContextWindow == nil {
		t.Fatal("ContextWindow is nil")
	}
	if *stats.ContextWindow != 200000 {
		t.Errorf("ContextWindow = %d, want %d", *stats.ContextWindow, 200000)
	}

	// Test nil context window
	jsonStr2 := `{}`
	var stats2 ModelUsageStats
	if err := json.Unmarshal([]byte(jsonStr2), &stats2); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if stats2.ContextWindow != nil {
		t.Errorf("ContextWindow = %v, want nil", stats2.ContextWindow)
	}
}

func TestAssistantMessage_GetContentBlocks(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantCount int
		wantType  string
	}{
		{
			name:      "array of content blocks",
			content:   `[{"type":"text","text":"Hello"},{"type":"text","text":"World"}]`,
			wantCount: 2,
			wantType:  "text",
		},
		{
			name:      "single content block",
			content:   `[{"type":"thinking","thinking":"Let me think..."}]`,
			wantCount: 1,
			wantType:  "thinking",
		},
		{
			name:      "empty array",
			content:   `[]`,
			wantCount: 0,
			wantType:  "",
		},
		{
			name:      "string content (not blocks)",
			content:   `"This is a string"`,
			wantCount: 0,
			wantType:  "",
		},
		{
			name:      "empty content",
			content:   ``,
			wantCount: 0,
			wantType:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &AssistantMessage{
				Content: json.RawMessage(tt.content),
			}
			blocks := msg.GetContentBlocks()
			if len(blocks) != tt.wantCount {
				t.Errorf("GetContentBlocks() returned %d blocks, want %d", len(blocks), tt.wantCount)
			}
			if tt.wantCount > 0 && blocks[0].Type != tt.wantType {
				t.Errorf("GetContentBlocks()[0].Type = %q, want %q", blocks[0].Type, tt.wantType)
			}
		})
	}
}

func TestAssistantMessage_GetContentString(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "plain string content",
			content: `"Hello, World!"`,
			want:    "Hello, World!",
		},
		{
			name:    "string with local-command-stdout tags",
			content: `"<local-command-stdout>Command output here</local-command-stdout>"`,
			want:    "<local-command-stdout>Command output here</local-command-stdout>",
		},
		{
			name:    "empty string",
			content: `""`,
			want:    "",
		},
		{
			name:    "array content (not string)",
			content: `[{"type":"text","text":"Hello"}]`,
			want:    "",
		},
		{
			name:    "empty content",
			content: ``,
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := &AssistantMessage{
				Content: json.RawMessage(tt.content),
			}
			got := msg.GetContentString()
			if got != tt.want {
				t.Errorf("GetContentString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIncomingControlResponse_JSONParsing(t *testing.T) {
	// Test successful initialize response
	jsonStr := `{
		"subtype": "success",
		"request_id": "req-123",
		"response": {
			"commands": [
				{"name": "cost", "description": "Show cost"},
				{"name": "context", "description": "Show context"}
			],
			"agents": ["Bash", "Explore"]
		}
	}`
	var resp IncomingControlResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if resp.Subtype != "success" {
		t.Errorf("Subtype = %q, want %q", resp.Subtype, "success")
	}
	if resp.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want %q", resp.RequestID, "req-123")
	}
	if resp.Response == nil {
		t.Fatal("Response is nil")
	}
	if len(resp.Response.Commands) != 2 {
		t.Errorf("Commands count = %d, want %d", len(resp.Response.Commands), 2)
	}
	if resp.Response.Commands[0].Name != "cost" {
		t.Errorf("Commands[0].Name = %q, want %q", resp.Response.Commands[0].Name, "cost")
	}
	if len(resp.Response.Agents) != 2 {
		t.Errorf("Agents count = %d, want %d", len(resp.Response.Agents), 2)
	}

	// Test error response
	errorJSON := `{"subtype": "error", "request_id": "req-456", "error": "Something went wrong"}`
	var errorResp IncomingControlResponse
	if err := json.Unmarshal([]byte(errorJSON), &errorResp); err != nil {
		t.Fatalf("failed to parse error response: %v", err)
	}
	if errorResp.Subtype != "error" {
		t.Errorf("Subtype = %q, want %q", errorResp.Subtype, "error")
	}
	if errorResp.Error != "Something went wrong" {
		t.Errorf("Error = %q, want %q", errorResp.Error, "Something went wrong")
	}
}
