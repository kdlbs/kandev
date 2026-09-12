package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

type guardedTTYAuditFinalizer interface {
	FinalizeGuardedTTYExecution(context.Context, models.GuardedTTYAuditFinalize) (*models.Message, bool, error)
}

// ClaimGuardedTTYExecution creates the attestation row before provider
// dispatch. The attestation UUID is also the message ID, so replayed claims
// cannot create a second durable identity.
func (s *Service) ClaimGuardedTTYExecution(ctx context.Context, claim models.GuardedTTYAuditClaim) error {
	if claim.AttestationID == "" || claim.Execution.ExecutionID == "" || claim.Execution.TaskID == "" ||
		claim.Execution.SessionID == "" || claim.WorkspaceID == "" || claim.AgentID != streams.GuardedTTYAgentID || claim.PrincipalSurface == "" ||
		len(claim.Argv) == 0 || claim.Outcome != models.GuardedTTYOutcomePending {
		return errors.New("invalid guarded TTY audit claim")
	}
	_, err := s.CreateMessageWithID(ctx, claim.AttestationID, &CreateMessageRequest{
		TaskSessionID: claim.Execution.SessionID,
		TaskID:        claim.Execution.TaskID,
		Content:       "guarded_tty_exec_kandev",
		AuthorType:    "agent",
		Type:          string(models.MessageTypeToolExecute),
		Metadata: map[string]interface{}{
			models.GuardedTTYAuditMetadataKey: claim,
		},
	})
	if err != nil {
		return fmt.Errorf("create guarded TTY audit claim: %w", err)
	}
	return nil
}

// FinalizeGuardedTTYExecution closes only the exact pending claim and publishes
// the committed update. Duplicate, stale, and mismatched finalizers fail
// closed and cannot overwrite the first terminal evidence.
func (s *Service) FinalizeGuardedTTYExecution(ctx context.Context, finalize models.GuardedTTYAuditFinalize) error {
	repo, ok := s.messages.(guardedTTYAuditFinalizer)
	if !ok {
		return errors.New("guarded TTY audit finalization is unavailable")
	}
	message, finalized, err := repo.FinalizeGuardedTTYExecution(ctx, finalize)
	if err != nil {
		return err
	}
	if !finalized {
		return errors.New("guarded TTY audit was already finalized or did not match")
	}
	if message == nil {
		return errors.New("guarded TTY audit finalization returned no message")
	}
	_ = s.publishMessageEvent(ctx, events.MessageUpdated, message)
	return nil
}
