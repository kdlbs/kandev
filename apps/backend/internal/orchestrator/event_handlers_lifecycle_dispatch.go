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
	if _, _, accepted, err := s.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, session.ID, taskID, prompt, "", messagequeue.QueuedByWorkflow,
		false, nil, metadata, coalesceKey, true,
	); err != nil {
		return "", err
	} else if !accepted {
		return "", inactiveErr
	}
	s.publishQueueStatusEvent(ctx, session.ID)
	// Always attempt the drain rather than gating it on this stale `session`
	// snapshot's state: drainQueuedMessageForPromptableSession reloads the
	// session and re-checks promptability itself, so it safely no-ops when
	// not ready. Gating here on the pre-queue snapshot left a real race — a
	// session that became promptable between snapshot and queueing would
	// have its prompt stuck until some unrelated later event triggered a
	// drain.
	s.drainQueuedMessageForPromptableSession(ctx, session.ID)
	return session.ID, nil
}
