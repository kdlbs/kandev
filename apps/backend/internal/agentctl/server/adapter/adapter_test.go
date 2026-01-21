package adapter

import (
	"encoding/json"
	"testing"
)

func TestSessionUpdate_Fields(t *testing.T) {
	update := AgentEvent{
		Type:       "tool_call",
		SessionID:  "session-123",
		Text:       "test message",
		ToolCallID: "tool-call-456",
		ToolName:   "file_read",
		ToolTitle:  "Reading file",
		ToolStatus: "running",
		ToolArgs:   map[string]interface{}{"path": "/test/file.txt"},
		ToolResult: map[string]interface{}{"content": "file contents"},
		PlanEntries: []PlanEntry{
			{Description: "Step 1", Status: "completed", Priority: "high"},
		},
		Error: "",
		Data:  map[string]interface{}{"custom": "data"},
	}

	if update.Type != "tool_call" {
		t.Errorf("expected Type 'tool_call', got %q", update.Type)
	}
	if update.SessionID != "session-123" {
		t.Errorf("expected SessionID 'session-123', got %q", update.SessionID)
	}
	if update.Text != "test message" {
		t.Errorf("expected Text 'test message', got %q", update.Text)
	}
	if update.ToolCallID != "tool-call-456" {
		t.Errorf("expected ToolCallID 'tool-call-456', got %q", update.ToolCallID)
	}
	if update.ToolName != "file_read" {
		t.Errorf("expected ToolName 'file_read', got %q", update.ToolName)
	}
	if update.ToolTitle != "Reading file" {
		t.Errorf("expected ToolTitle 'Reading file', got %q", update.ToolTitle)
	}
	if update.ToolStatus != "running" {
		t.Errorf("expected ToolStatus 'running', got %q", update.ToolStatus)
	}
	if update.ToolArgs["path"] != "/test/file.txt" {
		t.Errorf("expected ToolArgs[path] '/test/file.txt', got %v", update.ToolArgs["path"])
	}
	if len(update.PlanEntries) != 1 {
		t.Errorf("expected 1 PlanEntry, got %d", len(update.PlanEntries))
	}
	if update.Data["custom"] != "data" {
		t.Errorf("expected Data[custom] 'data', got %v", update.Data["custom"])
	}
}

func TestSessionUpdate_JSONSerialization(t *testing.T) {
	update := AgentEvent{
		Type:       "message_chunk",
		SessionID:  "session-abc",
		Text:       "Hello, world!",
		ToolCallID: "",
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("failed to marshal SessionUpdate: %v", err)
	}

	var decoded AgentEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal SessionUpdate: %v", err)
	}

	if decoded.Type != update.Type {
		t.Errorf("expected Type %q, got %q", update.Type, decoded.Type)
	}
	if decoded.SessionID != update.SessionID {
		t.Errorf("expected SessionID %q, got %q", update.SessionID, decoded.SessionID)
	}
	if decoded.Text != update.Text {
		t.Errorf("expected Text %q, got %q", update.Text, decoded.Text)
	}
}

func TestSessionUpdate_JSONOmitEmpty(t *testing.T) {
	// Test that omitempty fields are not included when empty
	update := AgentEvent{
		Type: "error",
	}

	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("failed to marshal SessionUpdate: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}

	// Type should be present
	if _, ok := parsed["type"]; !ok {
		t.Error("expected 'type' field to be present")
	}

	// These should be omitted due to omitempty
	if _, ok := parsed["session_id"]; ok {
		t.Error("expected 'session_id' to be omitted when empty")
	}
	if _, ok := parsed["text"]; ok {
		t.Error("expected 'text' to be omitted when empty")
	}
	if _, ok := parsed["tool_call_id"]; ok {
		t.Error("expected 'tool_call_id' to be omitted when empty")
	}
}

func TestPlanEntry_Fields(t *testing.T) {
	entry := PlanEntry{
		Description: "Analyze the codebase",
		Status:      "in_progress",
		Priority:    "high",
	}

	if entry.Description != "Analyze the codebase" {
		t.Errorf("expected Description 'Analyze the codebase', got %q", entry.Description)
	}
	if entry.Status != "in_progress" {
		t.Errorf("expected Status 'in_progress', got %q", entry.Status)
	}
	if entry.Priority != "high" {
		t.Errorf("expected Priority 'high', got %q", entry.Priority)
	}
}

func TestPlanEntry_JSONSerialization(t *testing.T) {
	entries := []PlanEntry{
		{Description: "Task 1", Status: "pending", Priority: "low"},
		{Description: "Task 2", Status: "completed", Priority: "high"},
		{Description: "Task 3", Status: "failed", Priority: "medium"},
	}

	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("failed to marshal PlanEntry slice: %v", err)
	}

	var decoded []PlanEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal PlanEntry slice: %v", err)
	}

	if len(decoded) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(decoded))
	}

	for i, entry := range entries {
		if decoded[i].Description != entry.Description {
			t.Errorf("entry %d: expected Description %q, got %q", i, entry.Description, decoded[i].Description)
		}
		if decoded[i].Status != entry.Status {
			t.Errorf("entry %d: expected Status %q, got %q", i, entry.Status, decoded[i].Status)
		}
		if decoded[i].Priority != entry.Priority {
			t.Errorf("entry %d: expected Priority %q, got %q", i, entry.Priority, decoded[i].Priority)
		}
	}
}

func TestAgentInfo_Fields(t *testing.T) {
	info := AgentInfo{
		Name:    "test-agent",
		Version: "1.0.0",
	}

	if info.Name != "test-agent" {
		t.Errorf("expected Name 'test-agent', got %q", info.Name)
	}
	if info.Version != "1.0.0" {
		t.Errorf("expected Version '1.0.0', got %q", info.Version)
	}
}

func TestAgentInfo_JSONSerialization(t *testing.T) {
	info := AgentInfo{
		Name:    "claude-agent",
		Version: "2.5.0",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("failed to marshal AgentInfo: %v", err)
	}

	var decoded AgentInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal AgentInfo: %v", err)
	}

	if decoded.Name != info.Name {
		t.Errorf("expected Name %q, got %q", info.Name, decoded.Name)
	}
	if decoded.Version != info.Version {
		t.Errorf("expected Version %q, got %q", info.Version, decoded.Version)
	}
}

func TestConfig_Fields(t *testing.T) {
	cfg := Config{
		WorkDir:     "/workspace",
		AutoApprove: true,
		BaseURL:     "http://localhost:8080",
		AuthHeader:  "Authorization",
		AuthValue:   "Bearer token123",
		Headers:     map[string]string{"X-Custom": "value"},
		Extra:       map[string]string{"debug": "true"},
	}

	if cfg.WorkDir != "/workspace" {
		t.Errorf("expected WorkDir '/workspace', got %q", cfg.WorkDir)
	}
	if !cfg.AutoApprove {
		t.Error("expected AutoApprove true, got false")
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("expected BaseURL 'http://localhost:8080', got %q", cfg.BaseURL)
	}
	if cfg.AuthHeader != "Authorization" {
		t.Errorf("expected AuthHeader 'Authorization', got %q", cfg.AuthHeader)
	}
	if cfg.AuthValue != "Bearer token123" {
		t.Errorf("expected AuthValue 'Bearer token123', got %q", cfg.AuthValue)
	}
	if cfg.Headers["X-Custom"] != "value" {
		t.Errorf("expected Headers[X-Custom] 'value', got %v", cfg.Headers["X-Custom"])
	}
	if cfg.Extra["debug"] != "true" {
		t.Errorf("expected Extra[debug] 'true', got %v", cfg.Extra["debug"])
	}
}

func TestSessionUpdate_UpdateTypes(t *testing.T) {
	// Test various update type constants
	testCases := []struct {
		name       string
		updateType string
	}{
		{"message chunk", "message_chunk"},
		{"tool call", "tool_call"},
		{"tool update", "tool_update"},
		{"plan", "plan"},
		{"complete", "complete"},
		{"error", "error"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			update := AgentEvent{Type: tc.updateType}
			if update.Type != tc.updateType {
				t.Errorf("expected Type %q, got %q", tc.updateType, update.Type)
			}
		})
	}
}

func TestPlanEntry_StatusValues(t *testing.T) {
	// Test various status values
	testCases := []struct {
		status string
	}{
		{"pending"},
		{"in_progress"},
		{"completed"},
		{"failed"},
	}

	for _, tc := range testCases {
		t.Run(tc.status, func(t *testing.T) {
			entry := PlanEntry{Status: tc.status}
			if entry.Status != tc.status {
				t.Errorf("expected Status %q, got %q", tc.status, entry.Status)
			}
		})
	}
}

func TestConvertSSEToStreamableHTTP(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "standard SSE endpoint",
			input:    "http://localhost:9090/sse",
			expected: "http://localhost:9090/mcp",
		},
		{
			name:     "SSE endpoint with different port",
			input:    "http://example.com:8080/sse",
			expected: "http://example.com:8080/mcp",
		},
		{
			name:     "trailing slash not converted",
			input:    "http://localhost:9090/sse/",
			expected: "http://localhost:9090/sse/",
		},
		{
			name:     "non-SSE endpoint unchanged",
			input:    "http://localhost:9090/other",
			expected: "http://localhost:9090/other",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "already streamable HTTP endpoint",
			input:    "http://localhost:9090/mcp",
			expected: "http://localhost:9090/mcp",
		},
		{
			name:     "sse in path but not at end",
			input:    "http://localhost:9090/sse/something",
			expected: "http://localhost:9090/sse/something",
		},
		{
			name:     "https endpoint",
			input:    "https://secure.example.com/sse",
			expected: "https://secure.example.com/mcp",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := convertSSEToStreamableHTTP(tc.input)
			if result != tc.expected {
				t.Errorf("convertSSEToStreamableHTTP(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestSessionUpdate_WithComplexToolArgs(t *testing.T) {
	// Test with complex nested ToolArgs
	update := AgentEvent{
		Type:       "tool_call",
		ToolCallID: "call-123",
		ToolName:   "file_edit",
		ToolArgs: map[string]interface{}{
			"path": "/src/main.go",
			"locations": []map[string]interface{}{
				{"path": "/src/main.go", "line": 42},
				{"path": "/src/util.go", "line": 10},
			},
			"raw_input": map[string]interface{}{
				"old_text": "foo",
				"new_text": "bar",
			},
		},
	}

	// Marshal and unmarshal to verify complex structures work
	data, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AgentEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ToolArgs["path"] != "/src/main.go" {
		t.Errorf("expected path '/src/main.go', got %v", decoded.ToolArgs["path"])
	}

	locations, ok := decoded.ToolArgs["locations"].([]interface{})
	if !ok {
		t.Fatal("expected locations to be a slice")
	}
	if len(locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(locations))
	}
}
