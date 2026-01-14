// Package websocket provides WebSocket message types and protocol definitions.
package websocket

import (
	"encoding/json"
	"time"
)

// MessageType represents the type of WebSocket message
type MessageType string

const (
	MessageTypeRequest      MessageType = "request"
	MessageTypeResponse     MessageType = "response"
	MessageTypeNotification MessageType = "notification"
	MessageTypeError        MessageType = "error"
)

// Message is the base envelope for all WebSocket messages
type Message struct {
	ID        string          `json:"id,omitempty"`
	Type      MessageType     `json:"type"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// Request represents a client request message
type Request struct {
	ID      string          `json:"id"`
	Type    MessageType     `json:"type"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// Response represents a server response message
type Response struct {
	ID        string          `json:"id"`
	Type      MessageType     `json:"type"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// Notification represents a server push notification
type Notification struct {
	Type      MessageType     `json:"type"`
	Action    string          `json:"action"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// ErrorPayload represents an error response payload
type ErrorPayload struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}



// NewRequest creates a new request message
func NewRequest(id, action string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:        id,
		Type:      MessageTypeRequest,
		Action:    action,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}

// NewResponse creates a new response message
func NewResponse(id, action string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:        id,
		Type:      MessageTypeResponse,
		Action:    action,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}

// NewNotification creates a new notification message
func NewNotification(action string, payload interface{}) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      MessageTypeNotification,
		Action:    action,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}

// NewError creates a new error response message
func NewError(id, action, code, message string, details map[string]interface{}) (*Message, error) {
	payload := ErrorPayload{
		Code:    code,
		Message: message,
		Details: details,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		ID:        id,
		Type:      MessageTypeError,
		Action:    action,
		Payload:   data,
		Timestamp: time.Now().UTC(),
	}, nil
}

// ParsePayload parses the payload into the given struct
func (m *Message) ParsePayload(v interface{}) error {
	if m.Payload == nil {
		return nil
	}
	return json.Unmarshal(m.Payload, v)
}
