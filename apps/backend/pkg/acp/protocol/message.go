package protocol

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of ACP message
type MessageType string

const (
	// Agent → Backend message types
	MessageTypeProgress      MessageType = "progress"
	MessageTypeLog           MessageType = "log"
	MessageTypeResult        MessageType = "result"
	MessageTypeError         MessageType = "error"
	MessageTypeStatus        MessageType = "status"
	MessageTypeHeartbeat     MessageType = "heartbeat"
	MessageTypeInputRequired MessageType = "input_required" // Agent requests input from user

	// Backend → Agent message types
	MessageTypeControl       MessageType = "control"        // Control commands (pause, resume, stop)
	MessageTypeInputResponse MessageType = "input_response" // Response to input_required
)

// Message represents an ACP protocol message
type Message struct {
	Type      MessageType            `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	AgentID   string                 `json:"agent_id"`
	TaskID    string                 `json:"task_id"`
	Data      map[string]interface{} `json:"data"`
}

// MarshalJSON implements custom JSON marshaling
func (m *Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
		Timestamp string `json:"timestamp"`
	}{
		Alias:     (*Alias)(m),
		Timestamp: m.Timestamp.Format(time.RFC3339Nano),
	})
}

// IsValid checks if the message has required fields
func (m *Message) IsValid() bool {
	if m.Type == "" {
		return false
	}
	if m.AgentID == "" {
		return false
	}
	if m.TaskID == "" {
		return false
	}
	return true
}
