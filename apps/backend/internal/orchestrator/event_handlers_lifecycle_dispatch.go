package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

// queueAndDrainLifecyclePrompt queues a lifecycle prompt through the shared
// durable lifecycle queue and, for a session already idle/waiting for input,
// drains it immediately. Shared mechanical plumbing between the GitHub PR
// and GitLab MR lifecycle dispatchers — every provider-specific concern
// (canonical URL, prompt metadata, coalesce key, inactive-task sentinel
// error) is resolved by the caller before this runs.
func (s *Service) queueAndDrainLifecyclePrompt(
	ctx context.Context, session *models.TaskSession, taskID, prompt string,
	metadata map[string]interface{}, coalesceKey string, inactiveErr error,
) (string, error) {
	if s.messageQueue == nil {
		return "", fmt.Errorf("message queue is not configured")
	}
	if err := s.checkSessionRecoveryBlock(ctx, session.ID); err != nil {
		return "", err
	}
	identity, err := s.resolveQueueIdentityForSession(ctx, session)
	if err != nil || identity.TaskID != taskID {
		if err != nil {
			return "", err
		}
		return "", messagequeue.ErrSessionIdentityMismatch
	}
	if _, _, accepted, err := s.messageQueue.QueueLifecycleMessageWithCoalesceKeyForSession(
		ctx, identity, prompt, "", messagequeue.QueuedByWorkflow,
		false, nil, metadata, coalesceKey, true,
	); err != nil {
		return "", err
	} else if !accepted {
		return "", inactiveErr
	}
	s.publishQueueStatusEventForIdentity(ctx, identity)
	// Always attempt the drain rather than gating it on this stale `session`
	// snapshot's state. The exact drain reloads promptability while preserving
	// the incarnation captured by the lifecycle admission.
	_, _ = s.drainQueuedMessageForPromptableSessionForIdentity(ctx, identity)
	return session.ID, nil
}
