package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// scheduledObservation is one automatic cron observation taken from a
// scheduler inventory snapshot. It carries the revision the scheduler saw so
// admission can reject a snapshot that an edit has made obsolete.
type scheduledObservation struct {
	AutomationID    string
	TriggerID       string
	TriggerRevision int64
	EvaluatedAt     time.Time
	TriggerData     json.RawMessage
	Dedup           DedupBinding
}

// admitScheduledObservation revalidates an automatic cron observation and
// admits at most one run for it. The reload, due check, dedup/capacity
// admission, and evaluation-cursor write all complete under the per-automation
// admission lock. Event publication happens after the lock is released so a
// subscriber can save an edit without deadlocking.
func (s *Service) admitScheduledObservation(ctx context.Context, obs scheduledObservation) (FireResult, error) {
	run, result, err := func() (*AutomationRun, FireResult, error) {
		unlock := s.automationRunLock(obs.AutomationID)
		defer unlock()
		return s.admitScheduledObservationLocked(ctx, obs)
	}()
	if err != nil || run == nil {
		return result, err
	}
	return s.completeAdmittedFire(
		ctx, run, obs.AutomationID, obs.TriggerID, TriggerTypeScheduled,
		obs.TriggerData, nil, obs.Dedup.Key(), obs.EvaluatedAt,
	)
}

func (s *Service) admitScheduledObservationLocked(
	ctx context.Context, obs scheduledObservation,
) (*AutomationRun, FireResult, error) {
	a, result, ok, err := s.loadScheduledObservation(ctx, obs)
	if err != nil || !ok {
		return nil, result, err
	}
	ctx, err = s.executorActorContext(ctx, a.CreatedByUserID, a.ExecutorProfileID, a.WorkspaceID)
	if err != nil {
		return nil, FireResult{}, err
	}
	run, capReason, duplicate, err := s.admitTriggerLocked(
		ctx, a, obs.TriggerID, TriggerTypeScheduled, obs.TriggerData, obs.Dedup,
		obs.EvaluatedAt, []string{obs.TriggerID},
	)
	if err != nil {
		return nil, FireResult{}, err
	}
	if duplicate {
		return nil, FireResult{Skipped: true, Duplicate: true, Reason: "this trigger has already fired"}, nil
	}
	if capReason != "" {
		return nil, FireResult{Skipped: true, Reason: capReason}, nil
	}
	return run, FireResult{RunID: run.ID}, nil
}

// loadScheduledObservation revalidates the automation and trigger state a
// scheduler snapshot was taken from. A false ok means the observation is
// obsolete or not due; the returned result explains the skip.
func (s *Service) loadScheduledObservation(
	ctx context.Context, obs scheduledObservation,
) (*Automation, FireResult, bool, error) {
	a, err := s.store.GetAutomation(ctx, obs.AutomationID)
	if err != nil {
		return nil, FireResult{}, false, fmt.Errorf("load automation: %w", err)
	}
	if a == nil {
		return nil, FireResult{Skipped: true, Reason: "automation no longer exists"}, false, nil
	}
	if !a.Enabled {
		return nil, FireResult{Skipped: true, Reason: "automation is disabled"}, false, nil
	}
	if automationHasPluginEventTrigger(a) {
		return nil, FireResult{Skipped: true, Reason: "automation uses a plugin event trigger"}, false, nil
	}
	t, err := s.store.GetTrigger(ctx, obs.TriggerID)
	if err != nil {
		return nil, FireResult{}, false, fmt.Errorf("load trigger: %w", err)
	}
	if !scheduledTriggerMatchesObservation(t, obs) {
		return nil, FireResult{Skipped: true, Reason: "trigger no longer matches the schedule observation"}, false, nil
	}
	due, dueErr := scheduledTriggerDue(t, obs.EvaluatedAt)
	if dueErr != nil {
		s.logger.Debug("cannot evaluate fresh scheduled trigger",
			zap.String("trigger_id", obs.TriggerID), zap.Error(dueErr))
		return nil, FireResult{Skipped: true, Reason: "trigger schedule is not evaluable"}, false, nil
	}
	if !due {
		return nil, FireResult{Skipped: true, Reason: "trigger is not due"}, false, nil
	}
	return a, FireResult{}, true, nil
}

func scheduledTriggerMatchesObservation(t *AutomationTrigger, obs scheduledObservation) bool {
	if t == nil {
		return false
	}
	return t.AutomationID == obs.AutomationID &&
		t.Type == TriggerTypeScheduled &&
		t.TriggerRevision == obs.TriggerRevision &&
		t.Enabled
}

func automationHasPluginEventTrigger(a *Automation) bool {
	if a == nil {
		return false
	}
	for _, condition := range a.Triggers {
		if condition.Type == TriggerTypePluginEvent {
			return true
		}
	}
	return false
}
