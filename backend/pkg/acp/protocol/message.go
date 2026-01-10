package protocol

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of ACP message
type MessageType string

const (
	MessageTypeProgress  MessageType = "progress"
	MessageTypeLog       MessageType = "log"
	MessageTypeResult    MessageType = "result"
	MessageTypeError     MessageType = "error"
	MessageTypeStatus    MessageType = "status"
	MessageTypeHeartbeat MessageType = "heartbeat"
	MessageTypeControl   MessageType = "control" // For commands sent to agents
)

// Message represents an ACP protocol message
type Message struct {
	Type      MessageType            `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	AgentID   string                 `json:"agent_id"`
	TaskID    string                 `json:"task_id"`
	Data      map[string]interface{} `json:"data"`
}

// NewMessage creates a new ACP message with the current timestamp
func NewMessage(msgType MessageType, agentID, taskID string, data map[string]interface{}) *Message {
	return &Message{
		Type:      msgType,
		Timestamp: time.Now().UTC(),
		AgentID:   agentID,
		TaskID:    taskID,
		Data:      data,
	}
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

// Parse parses a JSON string into an ACP message
func Parse(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
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

