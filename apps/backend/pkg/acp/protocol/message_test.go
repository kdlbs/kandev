package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMessageType_Constants(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeProgress, "progress"},
		{MessageTypeLog, "log"},
		{MessageTypeResult, "result"},
		{MessageTypeError, "error"},
		{MessageTypeStatus, "status"},
		{MessageTypeHeartbeat, "heartbeat"},
		{MessageTypeControl, "control"},
	}

	for _, tt := range tests {
		if string(tt.msgType) != tt.expected {
			t.Errorf("Expected MessageType %s, got %s", tt.expected, tt.msgType)
		}
	}
}

func TestNewMessage(t *testing.T) {
	agentID := "agent-123"
	taskID := "task-456"
	data := map[string]interface{}{"key": "value"}

	before := time.Now().UTC()
	msg := NewMessage(MessageTypeProgress, agentID, taskID, data)
	after := time.Now().UTC()

	if msg.Type != MessageTypeProgress {
		t.Errorf("Expected type %s, got %s", MessageTypeProgress, msg.Type)
	}
	if msg.AgentID != agentID {
		t.Errorf("Expected agentID %s, got %s", agentID, msg.AgentID)
	}
	if msg.TaskID != taskID {
		t.Errorf("Expected taskID %s, got %s", taskID, msg.TaskID)
	}
	if msg.Data["key"] != "value" {
		t.Error("Expected data to contain key=value")
	}
	if msg.Timestamp.Before(before) || msg.Timestamp.After(after) {
		t.Error("Expected timestamp to be set to current time")
	}
}

func TestMessage_MarshalJSON(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	msg := &Message{
		Type:      MessageTypeLog,
		Timestamp: fixedTime,
		AgentID:   "agent-001",
		TaskID:    "task-001",
		Data:      map[string]interface{}{"level": "info"},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["type"] != "log" {
		t.Errorf("Expected type 'log', got %v", result["type"])
	}
	if result["agent_id"] != "agent-001" {
		t.Errorf("Expected agent_id 'agent-001', got %v", result["agent_id"])
	}
	if result["task_id"] != "task-001" {
		t.Errorf("Expected task_id 'task-001', got %v", result["task_id"])
	}

	// Check timestamp is in RFC3339Nano format
	ts, ok := result["timestamp"].(string)
	if !ok {
		t.Fatal("Expected timestamp to be a string")
	}
	_, err = time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		t.Errorf("Timestamp not in RFC3339Nano format: %v", err)
	}
}

func TestParse(t *testing.T) {
	jsonData := []byte(`{
		"type": "progress",
		"timestamp": "2024-01-15T10:30:00Z",
		"agent_id": "agent-test",
		"task_id": "task-test",
		"data": {"progress": 50, "message": "Processing..."}
	}`)

	msg, err := Parse(jsonData)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if msg.Type != MessageTypeProgress {
		t.Errorf("Expected type %s, got %s", MessageTypeProgress, msg.Type)
	}
	if msg.AgentID != "agent-test" {
		t.Errorf("Expected agentID 'agent-test', got %s", msg.AgentID)
	}
	if msg.TaskID != "task-test" {
		t.Errorf("Expected taskID 'task-test', got %s", msg.TaskID)
	}
	if msg.Data["progress"] != float64(50) {
		t.Errorf("Expected progress 50, got %v", msg.Data["progress"])
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	invalidJSON := []byte(`{invalid json}`)

	_, err := Parse(invalidJSON)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestMessage_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		msg      *Message
		expected bool
	}{
		{
			name: "valid message",
			msg: &Message{
				Type:    MessageTypeProgress,
				AgentID: "agent-1",
				TaskID:  "task-1",
			},
			expected: true,
		},
		{
			name: "missing type",
			msg: &Message{
				Type:    "",
				AgentID: "agent-1",
				TaskID:  "task-1",
			},
			expected: false,
		},
		{
			name: "missing agent_id",
			msg: &Message{
				Type:    MessageTypeProgress,
				AgentID: "",
				TaskID:  "task-1",
			},
			expected: false,
		},
		{
			name: "missing task_id",
			msg: &Message{
				Type:    MessageTypeProgress,
				AgentID: "agent-1",
				TaskID:  "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.msg.IsValid(); got != tt.expected {
				t.Errorf("IsValid() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

