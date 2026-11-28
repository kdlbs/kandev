package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
)

func (s *Service) logQueuedDispatchFlow(ctx context.Context, queued *messagequeue.QueuedMessage, sessionID, outcome string) {
	_, submissionID, _ := queued.DeliverySubmission()
	s.logger.Flow(ctx, logger.ExecutionFlow{Stage: "queue_dispatch", Outcome: outcome, TaskID: queued.TaskID, SessionID: sessionID, QueueID: queued.ID, SubmissionID: submissionID})
}
