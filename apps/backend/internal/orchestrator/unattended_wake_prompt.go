package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ErrWakePromptNotDispatched marks a send that failed before its message was
// stored, so nothing can have reached the conversation.
var ErrWakePromptNotDispatched = errors.New("wake prompt not dispatched")

// UnattendedWakePrompt is one unattended wake send. OnReserved receives the
// reserved turn id once the message is stored on it; OnAccepted receives the
// accepted turn id at agentctl acceptance, the only proof of a send.
type UnattendedWakePrompt struct {
	TaskID     string
	SessionID  string
	Content    string
	WakeTurnID string
	OnReserved func(reservedTurnID string)
	OnAccepted func(turnID string)
}

// PromptUnattendedWake sends one wake message straight to the session, never
// through the message queue. The message is stored on the reserved turn
// inside the dispatch boundary; an error returned before that store wraps
// ErrWakePromptNotDispatched. It returns the stored message's id.
func (s *Service) PromptUnattendedWake(ctx context.Context, in UnattendedWakePrompt) (string, error) {
	if s.messageCreator == nil {
		return "", fmt.Errorf("%w: message creator is not configured", ErrWakePromptNotDispatched)
	}
	messageID := uuid.NewString()
	stored := false
	store := func() error {
		turnID := s.reservedPromptTurnID(in.SessionID)
		if turnID == "" {
			return errors.New("no reserved turn to store the wake message on")
		}
		metadata := map[string]interface{}{"coordinator_wake_turn_id": in.WakeTurnID}
		if err := s.messageCreator.CreateUserMessageIdempotent(
			ctx, messageID, in.TaskID, in.Content, in.SessionID, turnID, metadata,
		); err != nil {
			return fmt.Errorf("store wake message: %w", err)
		}
		stored = true
		if in.OnReserved != nil {
			in.OnReserved(turnID)
		}
		return nil
	}
	_, err := s.promptTask(ctx, in.TaskID, in.SessionID, in.Content, "", false, nil, true, launchOriginManual,
		promptTaskOptions{
			reserveTurnUntilDispatch: true,
			disableDispatchRetry:     true,
			afterDispatchAdmission:   store,
			onAccepted:               in.OnAccepted,
		})
	if err != nil {
		if !stored {
			return "", fmt.Errorf("%w: %w", ErrWakePromptNotDispatched, err)
		}
		return "", err
	}
	return messageID, nil
}
