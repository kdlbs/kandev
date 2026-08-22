// permission_response.go owns the permission-response sequence: claim the
// durable row, then dispatch to the agent.
//
// Delivery used to be read-then-dispatch, which let two responders (two
// browser tabs, or a tab and a plugin holding the Host interaction API) both
// observe a pending request and both answer it. agentctl keeps its pending
// entry after a response and its response channel holds a single slot, so the
// loser either answered a request the agent never asked again for, or failed
// and drove the durable row to "expired" over the winner's real outcome.
// Claiming first makes the row the arbiter, the same shape the clarification
// bundle path already used (ADR 0052).
package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// RespondToPermission sends a response to a permission request for a session.
func (s *Service) RespondToPermission(ctx context.Context, sessionID, pendingID, optionID string, cancelled, rejected bool) error {
	if err := s.authorizeSession(ctx, sessionID); err != nil {
		return err
	}
	return s.respondToPermission(ctx, sessionID, pendingID, optionID, cancelled, rejected,
		func(dispatchCtx context.Context) error {
			return s.executor.RespondToPermission(dispatchCtx, sessionID, pendingID, optionID, cancelled)
		})
}

// respondToPermission owns the claim-then-dispatch sequence. The dispatch is a
// callback so the arbitration can be tested without standing up an executor
// and an agent manager.
func (s *Service) respondToPermission(
	ctx context.Context,
	sessionID, pendingID, optionID string,
	cancelled, rejected bool,
	dispatch func(context.Context) error,
) error {
	s.logger.Debug("responding to permission request",
		zap.String("session_id", sessionID),
		zap.String("pending_id", pendingID),
		zap.String("option_id", optionID),
		zap.Bool("cancelled", cancelled),
		zap.Bool("rejected", rejected))

	// Determine status based on response. cancelled=true means the user dismissed
	// the dialog; rejected=true means the user explicitly clicked Deny with a
	// reject option. Both map to "rejected" message status.
	status := models.PermissionStatusApproved
	if cancelled || rejected {
		status = models.PermissionStatusRejected
	}

	// Claim the durable row BEFORE dispatching. Delivery is otherwise
	// read-then-dispatch, so two responders (two tabs, or a tab and a plugin)
	// could both reach agentctl: its pending entry survives a response and its
	// response channel holds one slot, so the loser either answers a request
	// the agent never asked again for, or fails and drives the row to
	// "expired" over the winner's real outcome. Claiming first makes the row
	// the arbiter, exactly as the clarification bundle path does.
	if s.messageCreator != nil {
		claimed, current, claimErr := s.messageCreator.ClaimPermissionResponse(ctx, sessionID, pendingID, status)
		if claimErr != nil {
			s.logger.Warn("failed to claim permission response",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.Error(claimErr))
			return claimErr
		}
		if !claimed {
			// An identical outcome is a duplicate submit (a double-click, or a
			// retry of a response that already landed): report success so the
			// caller is not told its own decision failed. A different outcome
			// is a genuine conflict the caller must observe.
			if current == status {
				s.logger.Debug("permission response already recorded with the same outcome",
					zap.String("session_id", sessionID),
					zap.String("pending_id", pendingID),
					zap.String("status", string(status)))
				return nil
			}
			return &PermissionAlreadyResolvedError{PendingID: pendingID, Status: string(current)}
		}
	}

	// Respond to the permission via agentctl
	if err := dispatch(ctx); err != nil {
		// Permission likely expired — update message so frontend reflects this
		if s.messageCreator != nil {
			if updateErr := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, pendingID, models.PermissionStatusExpired); updateErr != nil {
				s.logger.Warn("failed to mark expired permission message",
					zap.String("session_id", sessionID),
					zap.String("pending_id", pendingID),
					zap.Error(updateErr))
			}
		}
		return err
	}

	// The claim above already recorded the status; this re-write keeps the
	// update path intact for a Host implementation without a claim.
	if s.messageCreator != nil {
		if err := s.messageCreator.UpdatePermissionMessage(ctx, sessionID, pendingID, status); err != nil {
			s.logger.Warn("failed to update permission message status",
				zap.String("session_id", sessionID),
				zap.String("pending_id", pendingID),
				zap.String("status", string(status)),
				zap.Error(err))
			// Don't fail the whole operation if message update fails
		}
	}

	if !cancelled {
		session, err := s.repo.GetTaskSession(ctx, sessionID)
		if err != nil {
			s.logger.Warn("failed to load task session after permission response",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return nil
		}
		s.setSessionRunning(ctx, session.TaskID, sessionID, session)
	}

	return nil
}

// PermissionAlreadyResolvedError reports that a permission request was already
// resolved with a DIFFERENT outcome than the one this caller submitted, so its
// response was not delivered. A duplicate submit of the same outcome is not an
// error (see RespondToPermission); this is the genuine race between two
// responders, and callers surface it as a conflict rather than a retryable
// failure.
type PermissionAlreadyResolvedError struct {
	PendingID string
	Status    string
}

func (e *PermissionAlreadyResolvedError) Error() string {
	return fmt.Sprintf("permission %s was already resolved as %s", e.PendingID, e.Status)
}
