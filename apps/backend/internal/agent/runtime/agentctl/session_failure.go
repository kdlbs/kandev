package client

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/kandev/kandev/internal/agentctl/types"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	agentSessionNewAction    = "agent.session.new"
	agentSessionLoadAction   = "agent.session.load"
	agentSessionResumeAction = "agent.session.resume"
	agentSessionResetAction  = "agent.session.reset"
)

// SessionOperationAuthenticationError retains the validated session failure
// projection while keeping provider diagnostics available only through Unwrap.
type SessionOperationAuthenticationError struct {
	failure types.SessionOperationFailure
	cause   error
}

func (e *SessionOperationAuthenticationError) Error() string {
	return "session operation requires authentication"
}

func (e *SessionOperationAuthenticationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// SessionOperationFailure returns a safe copy of the validated failure data.
func (e *SessionOperationAuthenticationError) SessionOperationFailure() types.SessionOperationFailure {
	if e == nil {
		return types.SessionOperationFailure{}
	}
	normalized := types.NormalizeSessionOperationFailure(&e.failure)
	if normalized == nil {
		return types.SessionOperationFailure{}
	}
	return *normalized
}

func sessionOperationAuthenticationError(action string, response *ws.Message, payload ws.ErrorPayload) error {
	if response == nil || response.Action != action || !isSessionOperationAction(action) || payload.Details == nil {
		return nil
	}
	raw, ok := payload.Details["session_failure"]
	if !ok {
		return nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var projected types.SessionOperationFailure
	if err := decoder.Decode(&projected); err != nil {
		return nil
	}
	normalized := types.NormalizeSessionOperationFailure(&projected)
	if normalized == nil {
		return nil
	}
	message := payload.Message
	if message == "" {
		message = "agent authentication is required"
	}
	return &SessionOperationAuthenticationError{
		failure: *normalized,
		cause:   errors.New(message),
	}
}

func isSessionOperationAction(action string) bool {
	switch action {
	case agentSessionNewAction, agentSessionLoadAction, agentSessionResumeAction, agentSessionResetAction:
		return true
	default:
		return false
	}
}
