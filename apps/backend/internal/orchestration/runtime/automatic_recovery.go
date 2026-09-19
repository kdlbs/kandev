package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

const conversationRetryLimit = 5

// HandleFailure runs under the execution owner's session guard. The raw bus
// failure must not revoke authority before this recovery decision is made.
func (s *Service) HandleFailure(ctx context.Context, data watcher.AgentEventData) (int, time.Time, error) {
	owner, _, err := s.Repo.ConversationOwner(ctx, data.TaskID)
	if err != nil || owner == "" {
		return 0, time.Time{}, nil
	}
	claimed, claimErr := s.Runs.GetClaimedRunByTaskID(ctx, data.TaskID)
	if claimErr != nil || claimed == nil || claimed.ID != data.RunID || claimed.SessionID != data.SessionID {
		return 0, time.Time{}, nil
	}
	event := bus.NewEvent(events.AgentFailed, "orchestration", data)
	payload, err := decode(event)
	if err != nil {
		return 0, time.Time{}, err
	}
	if err = s.finishTurn(ctx, event, payload, data.TaskID, owner); err != nil {
		return 0, time.Time{}, err
	}
	run, err := s.Runs.GetRunByID(ctx, data.RunID)
	if err != nil || run.Status != "queued" || run.SessionID != data.SessionID || run.ScheduledRetryAt == nil {
		return 0, time.Time{}, nil
	}
	return run.RetryCount, *run.ScheduledRetryAt, nil
}

func (s *Service) retryTurn(ctx context.Context, run *runmodels.Run, data map[string]any) (bool, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	var failure watcher.AgentEventData
	if err = json.Unmarshal(raw, &failure); err != nil {
		return false, err
	}
	if run.RetryCount >= conversationRetryLimit || !failure.EvidenceKnown || failure.OutputObserved || failure.EffectObserved ||
		failure.PromptGeneration == 0 || failure.AgentExecutionID == "" || failure.DynamicRouteAttempt {
		return false, nil
	}
	classified := routingerr.Classify(routingerr.Input{Phase: routingerr.PhasePromptSend, ProviderID: failure.AgentID, Stderr: failure.ErrorMessage})
	now := time.Now().UTC()
	if routingerr.Decide(routingerr.ContextKanban, classified, now) != routingerr.DecisionShortRetry {
		return false, nil
	}
	// Credential refresh contention commonly takes a minute to clear. Keep the
	// retry durable and paced, rebuilding execution and authority on each launch.
	delays := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, time.Minute, time.Minute}
	retryAt := now.Add(delays[run.RetryCount])
	_, err = s.Runs.RetryClaimedSession(ctx, run.ID, run.SessionID, run.RetryCount, retryAt)
	// A lost claim belongs to cancellation or a successor. Never finalize it.
	return true, err
}

func (s *Service) CancelRecovery(ctx context.Context, taskID, sessionID string) bool {
	owner, _, err := s.Repo.ConversationOwner(ctx, taskID)
	if err != nil || owner == "" {
		return false
	}
	changed, err := s.Runs.CancelScheduledSessionRetry(ctx, sessionID)
	if err != nil || !changed {
		return false
	}
	_ = s.Repo.SetRuntimeWorking(ctx, owner, false)
	return true
}
