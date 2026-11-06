package routines

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrTriggerUpdateConflict leaves a claimed or concurrently changed trigger alone.
var ErrTriggerUpdateConflict = errors.New("routine trigger changed or is being dispatched")

type triggerUpdateRepository interface {
	GetRoutineTrigger(ctx context.Context, id string) (*RoutineTrigger, error)
	UpdateRoutineTriggerIfCurrent(ctx context.Context, before, after *RoutineTrigger) (bool, error)
}

// UpdateRoutineTrigger retains the existing trigger and fire history. A claimed
// cron slot is not re-armed here: its scheduler still owns the re-arm write.
func (s *RoutineService) UpdateRoutineTrigger(
	ctx context.Context, id string, request UpdateTriggerRequest,
) (*RoutineTrigger, error) {
	store, ok := s.repo.(triggerUpdateRepository)
	if !ok {
		return nil, errors.New("routine trigger updates are unavailable")
	}
	before, err := store.GetRoutineTrigger(ctx, id)
	if err != nil {
		return nil, err
	}
	if before == nil {
		return nil, sql.ErrNoRows
	}
	if request.CronExpression == nil && request.Timezone == nil && request.Enabled == nil {
		return before, nil
	}
	if before.Kind == triggerKindCron && before.Enabled && before.NextRunAt == nil {
		return nil, ErrTriggerUpdateConflict
	}
	after := *before
	if err := applyTriggerScheduleUpdate(&after, request, time.Now().UTC()); err != nil {
		return nil, err
	}
	if triggerScheduleUnchanged(before, &after) {
		// The request matches the stored schedule, so the row is left alone.
		// Rewriting it would recompute next_run_at and skip a fire that is
		// already due but not yet claimed by the scheduler.
		return before, nil
	}
	updated, err := store.UpdateRoutineTriggerIfCurrent(ctx, before, &after)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, ErrTriggerUpdateConflict
	}
	return &after, nil
}

func triggerScheduleUnchanged(before, after *RoutineTrigger) bool {
	return before.CronExpression == after.CronExpression &&
		before.Timezone == after.Timezone &&
		before.Enabled == after.Enabled
}

func applyTriggerScheduleUpdate(trigger *RoutineTrigger, request UpdateTriggerRequest, now time.Time) error {
	if trigger.Kind != triggerKindCron && (request.CronExpression != nil || request.Timezone != nil) {
		return fmt.Errorf("%w: schedule fields require a cron trigger", ErrInvalidTrigger)
	}
	if request.CronExpression != nil {
		trigger.CronExpression = *request.CronExpression
	}
	if request.Timezone != nil {
		trigger.Timezone = *request.Timezone
	}
	if request.Enabled != nil {
		trigger.Enabled = *request.Enabled
	}
	if trigger.Kind == triggerKindCron && (trigger.Enabled || request.CronExpression != nil || request.Timezone != nil) {
		if err := prepareTrigger(trigger, now); err != nil {
			return err
		}
	}
	if !trigger.Enabled {
		trigger.NextRunAt = nil
	}
	trigger.UpdatedAt = now
	return nil
}
