package protocol

import (
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
	SessionID string                 `json:"session_id"`
	Data      map[string]interface{} `json:"data"`
}
