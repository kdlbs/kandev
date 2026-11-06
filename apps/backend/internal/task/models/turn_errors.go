package models

import (
	"encoding/json"
	"strings"
	"time"
)

// MetaKeyLastTurnError stores the task-owned attention pointer for the latest
// failed interactive turn. It is deliberately separate from
// MetaKeyLastLaunchError (pre-session launch failure) and from the session's
// last_agent_error: this record never blocks the composer or authorizes
// recovery, it only keeps a durable error signal on the task until an operator
// acknowledges it.
const MetaKeyLastTurnError = "last_turn_error"

const (
	maxTurnErrorMessageBytes = 4096
	maxTurnErrorStampBytes   = 64
	maxTurnErrorIDBytes      = 256
	maxTurnErrorAckReason    = 32

	// TurnErrorAcknowledgementDismiss is an explicit operator dismissal.
	TurnErrorAcknowledgementDismiss = "dismiss"
	// TurnErrorAcknowledgementPrompt is a newly accepted manual prompt that
	// carried the client-observed error stamp.
	TurnErrorAcknowledgementPrompt = "prompt"
)

// TurnErrorAcknowledgementResult reports the outcome of a conditional
// acknowledgement against one observed stamp.
type TurnErrorAcknowledgementResult uint8

const (
	// TurnErrorAcknowledgementStored means the observed record was acknowledged.
	TurnErrorAcknowledgementStored TurnErrorAcknowledgementResult = iota
	// TurnErrorAcknowledgementNoOp means it was already acknowledged.
	TurnErrorAcknowledgementNoOp
	// TurnErrorAcknowledgementStale means a newer record replaced the stamp.
	TurnErrorAcknowledgementStale
	// TurnErrorAcknowledgementMissing means no current record exists.
	TurnErrorAcknowledgementMissing
)

// TaskTurnError is persisted under Task.Metadata[MetaKeyLastTurnError].
type TaskTurnError struct {
	StampValue     string     `json:"stamp"`
	SessionID      string     `json:"session_id"`
	ExecutionID    string     `json:"execution_id,omitempty"`
	TurnID         string     `json:"turn_id,omitempty"`
	MessageID      string     `json:"message_id,omitempty"`
	OccurredAt     time.Time  `json:"occurred_at"`
	Message        string     `json:"message"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy string     `json:"acknowledged_by,omitempty"`
}

// NormalizeTaskTurnError bounds every persisted field before it crosses the
// task metadata or compact-summary boundary.
func NormalizeTaskTurnError(value TaskTurnError) TaskTurnError {
	value.StampValue = truncateUTF8Bytes(strings.TrimSpace(value.StampValue), maxTurnErrorStampBytes)
	value.SessionID = truncateUTF8Bytes(strings.TrimSpace(value.SessionID), maxTurnErrorIDBytes)
	value.ExecutionID = truncateUTF8Bytes(strings.TrimSpace(value.ExecutionID), maxTurnErrorIDBytes)
	value.TurnID = truncateUTF8Bytes(strings.TrimSpace(value.TurnID), maxTurnErrorIDBytes)
	value.MessageID = truncateUTF8Bytes(strings.TrimSpace(value.MessageID), maxTurnErrorIDBytes)
	value.Message = truncateUTF8Bytes(strings.TrimSpace(value.Message), maxTurnErrorMessageBytes)
	switch value.AcknowledgedBy {
	case TurnErrorAcknowledgementDismiss, TurnErrorAcknowledgementPrompt:
	default:
		value.AcknowledgedBy = ""
	}
	if value.AcknowledgedBy == "" {
		value.AcknowledgedAt = nil
	}
	value.OccurredAt = value.OccurredAt.UTC()
	return value
}

// Valid reports whether the record carries the identity and message needed for
// durable attention.
func (e TaskTurnError) Valid() bool {
	return strings.TrimSpace(e.StampValue) != "" && strings.TrimSpace(e.SessionID) != "" &&
		strings.TrimSpace(e.Message) != ""
}

// Acknowledged reports whether an operator or accepted prompt already retired
// this exact error.
func (e TaskTurnError) Acknowledged() bool {
	return e.AcknowledgedAt != nil
}

// LoadTaskTurnError reads and validates the current attention pointer.
func LoadTaskTurnError(metadata map[string]interface{}) (TaskTurnError, bool) {
	if metadata == nil {
		return TaskTurnError{}, false
	}
	raw, ok := metadata[MetaKeyLastTurnError]
	if !ok || raw == nil {
		return TaskTurnError{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return TaskTurnError{}, false
	}
	var value TaskTurnError
	if err := json.Unmarshal(data, &value); err != nil {
		return TaskTurnError{}, false
	}
	value = NormalizeTaskTurnError(value)
	if !value.Valid() {
		return TaskTurnError{}, false
	}
	return value, true
}

// SetTaskTurnError stores a new attention pointer. A duplicate stamp is a
// no-op, preserving the original occurrence time and any acknowledgement. An
// older failure can never replace a newer accepted one.
func SetTaskTurnError(metadata map[string]interface{}, value TaskTurnError) bool {
	if metadata == nil {
		return false
	}
	value = NormalizeTaskTurnError(value)
	if !value.Valid() {
		return false
	}
	if current, ok := LoadTaskTurnError(metadata); ok {
		if current.StampValue == value.StampValue || !value.OccurredAt.After(current.OccurredAt) {
			return false
		}
	}
	metadata[MetaKeyLastTurnError] = value
	return true
}

// AcknowledgeTaskTurnError retires the observed stamp only. A different
// current stamp is a newer failure and is left untouched.
func AcknowledgeTaskTurnError(
	metadata map[string]interface{},
	stamp string,
	acknowledgedAt time.Time,
	reason string,
) bool {
	if metadata == nil || strings.TrimSpace(stamp) == "" {
		return false
	}
	current, ok := LoadTaskTurnError(metadata)
	if !ok || current.StampValue != strings.TrimSpace(stamp) {
		return false
	}
	if current.Acknowledged() {
		return true
	}
	at := acknowledgedAt.UTC()
	current.AcknowledgedAt = &at
	current.AcknowledgedBy = reason
	metadata[MetaKeyLastTurnError] = current
	return true
}
