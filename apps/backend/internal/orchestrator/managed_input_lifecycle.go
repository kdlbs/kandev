package orchestrator

import (
	"context"
	"errors"
	"fmt"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func managedInputIDFromQueueMessage(message *messagequeue.QueuedMessage) (string, bool) {
	if message == nil || message.Metadata == nil {
		return "", false
	}
	managed, _ := message.Metadata[messagequeue.MetadataManagedInput].(bool)
	inputID, _ := message.Metadata[messagequeue.MetadataManagedInputID].(string)
	return inputID, managed && inputID != ""
}

//nolint:cyclop // Keep durable acceptance and execution identity reconciliation in one state transition.
func (s *Service) recordManagedInputAcceptance(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	message *messagequeue.QueuedMessage,
	turnID string,
) (bool, error) {
	inputID, managed := managedInputIDFromQueueMessage(message)
	if !managed {
		return true, nil
	}
	if s.managedInputStorage == nil || s.agentManager == nil || turnID == "" {
		return s.markManagedInputAttemptUncertain(ctx, identity, inputID, "accepted_without_execution_identity")
	}

	acceptCtx := context.WithoutCancel(ctx)
	executionID, err := s.agentManager.GetExecutionIDForSession(acceptCtx, identity.SessionID)
	if errors.Is(err, agentruntime.ErrNoExecutionForSession) || (err == nil && executionID == "") {
		return s.markManagedInputAttemptUncertain(acceptCtx, identity, inputID, "accepted_without_execution_identity")
	}
	if err != nil {
		tracked, settleErr := s.markManagedInputAttemptUncertain(
			acceptCtx, identity, inputID, "accepted_execution_lookup_failed",
		)
		if settleErr != nil {
			return false, errors.Join(fmt.Errorf("resolve accepted managed input execution: %w", err), settleErr)
		}
		return tracked, nil
	}

	receipt, _, err := s.managedInputStorage.MarkManagedInputRunning(
		acceptCtx, identity, inputID, turnID, executionID,
	)
	if err == nil && receipt.State == messagequeue.ManagedInputStateRunning &&
		receipt.TurnID == turnID && receipt.ExecutionID == executionID {
		return true, nil
	}

	tracked, settleErr := s.markManagedInputAttemptUncertain(
		acceptCtx, identity, inputID, "accepted_receipt_link_failed",
	)
	if settleErr != nil {
		if err == nil {
			err = errors.New("managed input was not linked to the accepted execution")
		}
		return false, errors.Join(fmt.Errorf("record accepted managed input: %w", err), settleErr)
	}
	return tracked, nil
}

func (s *Service) markManagedInputAttemptUncertain(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	inputID, outcome string,
) (bool, error) {
	if s.managedInputStorage == nil {
		return false, errors.New("managed input storage is not configured")
	}
	receipt, err := s.managedInputStorage.GetManagedInput(ctx, identity, inputID)
	if err != nil {
		return false, err
	}
	switch receipt.State {
	case messagequeue.ManagedInputStateUncertain,
		messagequeue.ManagedInputStateCompleted,
		messagequeue.ManagedInputStateFailed,
		messagequeue.ManagedInputStateCancelled,
		messagequeue.ManagedInputStateSuperseded:
		return true, nil
	case messagequeue.ManagedInputStateAccepted:
		_, _, err = s.managedInputStorage.SettleManagedInput(
			ctx, identity, inputID, "", "", messagequeue.ManagedInputStateUncertain, outcome,
		)
	case messagequeue.ManagedInputStateRunning:
		_, _, err = s.managedInputStorage.SettleManagedInput(
			ctx, identity, inputID, receipt.TurnID, receipt.ExecutionID,
			messagequeue.ManagedInputStateUncertain, outcome,
		)
	default:
		return false, fmt.Errorf("managed input %q has unsupported state %q", inputID, receipt.State)
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// reconcileAttemptedManagedInput closes the crash window between the durable
// pre-dispatch marker and the accepted execution receipt. It never resends an
// entry whose external dispatch may already have happened.
//
//nolint:cyclop,nestif // Reconcile each durable queue and execution outcome before releasing the input.
func (s *Service) reconcileAttemptedManagedInput(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	message *messagequeue.QueuedMessage,
) (bool, error) {
	inputID, managed := managedInputIDFromQueueMessage(message)
	if !managed {
		return false, nil
	}
	if s.managedInputStorage == nil {
		return false, errors.New("managed input storage is not configured")
	}
	receipt, err := s.managedInputStorage.GetManagedInput(ctx, identity, inputID)
	if err != nil {
		return false, err
	}
	switch receipt.State {
	case messagequeue.ManagedInputStateAccepted:
		if _, _, err := s.managedInputStorage.SettleManagedInput(
			ctx, identity, inputID, "", "", messagequeue.ManagedInputStateUncertain,
			"dispatch_attempted_without_receipt",
		); err != nil {
			return false, err
		}
	case messagequeue.ManagedInputStateRunning:
		currentExecutionID := ""
		if s.agentManager == nil {
			return false, errors.New("agent execution resolver is not configured")
		}
		currentExecutionID, err = s.agentManager.GetExecutionIDForSession(ctx, identity.SessionID)
		if errors.Is(err, agentruntime.ErrNoExecutionForSession) {
			err = nil
		}
		if err != nil {
			return false, fmt.Errorf("resolve attempted managed input execution: %w", err)
		}
		if currentExecutionID == receipt.ExecutionID {
			if s.turnService != nil {
				active, activeErr := s.turnService.GetActiveTurn(ctx, identity.SessionID)
				if activeErr != nil {
					return false, fmt.Errorf("resolve attempted managed input turn: %w", activeErr)
				}
				if active != nil && active.ID == receipt.TurnID {
					return s.acknowledgeManagedInputQueueEntry(ctx, identity, message)
				}
			} else {
				return s.acknowledgeManagedInputQueueEntry(ctx, identity, message)
			}
		}
		if _, _, err := s.managedInputStorage.SettleManagedInput(
			ctx, identity, inputID, receipt.TurnID, receipt.ExecutionID,
			messagequeue.ManagedInputStateUncertain, "execution_no_longer_owns_turn",
		); err != nil {
			return false, err
		}
	case messagequeue.ManagedInputStateCompleted,
		messagequeue.ManagedInputStateFailed,
		messagequeue.ManagedInputStateCancelled,
		messagequeue.ManagedInputStateUncertain,
		messagequeue.ManagedInputStateSuperseded:
	default:
		return false, fmt.Errorf("managed input %q has unsupported state %q", inputID, receipt.State)
	}
	return s.acknowledgeManagedInputQueueEntry(ctx, identity, message)
}

func (s *Service) acknowledgeManagedInputQueueEntry(
	ctx context.Context,
	identity messagequeue.QueueSessionIdentity,
	message *messagequeue.QueuedMessage,
) (bool, error) {
	if s.messageQueue != nil && message != nil {
		if err := s.messageQueue.AcknowledgeQueuedForSession(ctx, identity, message); err != nil {
			return false, err
		}
		s.publishQueueStatusEventForIdentity(ctx, identity)
	}
	return true, nil
}

func (s *Service) settleManagedInputTurn(
	ctx context.Context,
	taskID, sessionID, turnID, executionID string,
	state messagequeue.ManagedInputState,
	outcome string,
) error {
	if s.managedInputStorage == nil || s.messageQueue == nil || turnID == "" || executionID == "" {
		return nil
	}
	identity, err := s.messageQueue.ResolveSessionIdentity(ctx, taskID, sessionID)
	if err != nil {
		return err
	}
	receipt, err := s.managedInputStorage.GetManagedInputByExecution(ctx, identity, turnID, executionID)
	if errors.Is(err, messagequeue.ErrEntryNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if receipt.State != messagequeue.ManagedInputStateRunning {
		return nil
	}
	_, _, err = s.managedInputStorage.SettleManagedInput(
		context.WithoutCancel(ctx), identity, receipt.ID, turnID, executionID, state, outcome,
	)
	return err
}
