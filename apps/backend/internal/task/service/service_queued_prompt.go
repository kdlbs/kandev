package service

import "context"

// QueuedPromptCounter supplies the pending prompt count per task (all
// sessions, pending semantics identical to message.queue.get). Satisfied at
// the composition root by an adapter over the messagequeue service; the task
// service keeps the interface narrow so it takes no hard orchestrator
// dependency and can be faked in tests.
type QueuedPromptCounter interface {
	CountPendingByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error)
}

// SetQueuedPromptCounter wires the per-task queued-prompt counter used to
// stamp status_summary.queued_prompt_count on task list/snapshot payloads.
// Optional; when unset the field is left absent.
func (s *Service) SetQueuedPromptCounter(counter QueuedPromptCounter) {
	if s != nil {
		s.queuedPromptCounter = counter
	}
}

// CountPendingQueuedByTaskIDs returns the pending prompt count for each
// requested task, keyed by task_id. Returns nil (no error) when no counter is
// wired or no tasks were requested, so callers can distinguish "provider
// unavailable — preserve the projected count" from a successful zero lookup.
func (s *Service) CountPendingQueuedByTaskIDs(ctx context.Context, taskIDs []string) (map[string]int, error) {
	if s == nil || s.queuedPromptCounter == nil || len(taskIDs) == 0 {
		return nil, nil
	}
	return s.queuedPromptCounter.CountPendingByTaskIDs(ctx, taskIDs)
}
