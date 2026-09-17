package routines

import (
	"context"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/office/shared"
)

// ScheduleState reports whether a routine's triggers can currently fire it,
// independent of the routine's own status (intent).
type ScheduleState string

const (
	ScheduleStateArmed                 ScheduleState = "armed"
	ScheduleStateTriggerInvalid        ScheduleState = "trigger_invalid"
	ScheduleStateTriggerUnscheduled    ScheduleState = "trigger_unscheduled"
	ScheduleStateTriggerDisabled       ScheduleState = "trigger_disabled"
	ScheduleStateEventOnly             ScheduleState = "event_only"
	ScheduleStateUnscheduledManualOnly ScheduleState = "unscheduled_manual_only"
	ScheduleStateUnscheduledNoTrigger  ScheduleState = "unscheduled_no_trigger"
	ScheduleStateUnknown               ScheduleState = "unknown"
)

// dispatchGrace is the single window, shared by classification and the
// startup scan, during which a cron trigger with a null next_run_at is
// still considered armed rather than stalled: the tick that fired it has
// not yet recomputed its next occurrence.
const dispatchGrace = 60 * time.Second

// UnarmedReason names why a cron trigger cannot currently fire. When more
// than one applies to the same trigger, they are reported in this order.
type UnarmedReason string

const (
	UnarmedReasonDisabled       UnarmedReason = "disabled"
	UnarmedReasonNotSchedulable UnarmedReason = "not_schedulable"
	UnarmedReasonStalled        UnarmedReason = "stalled"
)

// UnarmedCronTrigger is one cron trigger that cannot currently fire, and why.
type UnarmedCronTrigger struct {
	TriggerID string          `json:"trigger_id"`
	Reasons   []UnarmedReason `json:"reasons"`
}

const (
	triggerKindCron    = "cron"
	triggerKindWebhook = "webhook"
	triggerKindManual  = "manual"
)

// ClassifyRoutine implements the nine-rule schedule-state table over a
// routine's own trigger rows, in rule order, stopping at the first match.
// It takes no routine status: schedule state is independent of intent.
func ClassifyRoutine(triggers []*RoutineTrigger, now time.Time) (ScheduleState, []UnarmedCronTrigger) {
	ordered := sortTriggers(triggers)
	cronTriggers, hasEnabledWebhook, hasNonCronTrigger := partitionTriggers(ordered)
	unarmed := unarmedCronTriggers(cronTriggers, now)

	if state, ok := classifyCronTriggers(cronTriggers, now); ok {
		return state, unarmed
	}
	if hasEnabledWebhook {
		return ScheduleStateEventOnly, nil
	}
	if hasNonCronTrigger {
		return ScheduleStateUnscheduledManualOnly, nil
	}
	return ScheduleStateUnscheduledNoTrigger, nil
}

// partitionTriggers splits a routine's ordered triggers by kind: every cron
// trigger (in order), whether any enabled webhook trigger exists, and
// whether any non-cron trigger exists at all (rule 7 fires on existence
// alone, regardless of kind or enabled state).
func partitionTriggers(triggers []*RoutineTrigger) (cron []*RoutineTrigger, hasEnabledWebhook, hasNonCronTrigger bool) {
	for _, t := range triggers {
		switch t.Kind {
		case triggerKindCron:
			cron = append(cron, t)
		default:
			hasNonCronTrigger = true
			if t.Kind == triggerKindWebhook && t.Enabled {
				hasEnabledWebhook = true
			}
		}
	}
	return cron, hasEnabledWebhook, hasNonCronTrigger
}

// classifyCronTriggers evaluates rules 1 through 5 (the ones decided by
// cron trigger state alone) and reports whether one of them matched.
func classifyCronTriggers(cronTriggers []*RoutineTrigger, now time.Time) (ScheduleState, bool) {
	for _, t := range cronTriggers {
		if isArmed(t, now) {
			return ScheduleStateArmed, true
		}
	}
	for _, t := range cronTriggers {
		if t.Enabled && !isSchedulable(t) {
			return ScheduleStateTriggerInvalid, true
		}
	}
	for _, t := range cronTriggers {
		if t.Enabled && isSchedulable(t) && t.NextRunAt == nil {
			return ScheduleStateTriggerUnscheduled, true
		}
	}
	if len(cronTriggers) > 0 {
		return ScheduleStateTriggerDisabled, true
	}
	return "", false
}

// isArmed reports whether this single cron trigger can currently fire:
// either it has a next occurrence on the calendar (past-due or future), or
// it just fired and hasn't been recomputed yet (within dispatchGrace).
func isArmed(t *RoutineTrigger, now time.Time) bool {
	if !t.Enabled || !isSchedulable(t) {
		return false
	}
	if t.NextRunAt != nil {
		return true
	}
	return withinDispatchGrace(t, now)
}

func isSchedulable(t *RoutineTrigger) bool {
	_, err := shared.NextCronTime(t.CronExpression, t.Timezone, time.Time{})
	return err == nil
}

func withinDispatchGrace(t *RoutineTrigger, now time.Time) bool {
	if t.LastFiredAt == nil {
		return false
	}
	elapsed := now.Sub(*t.LastFiredAt)
	return elapsed >= 0 && elapsed <= dispatchGrace
}

// unarmedCronTriggers reports every cron trigger, in trigger order, that
// cannot currently fire, along with every applicable reason in fixed order.
func unarmedCronTriggers(cronTriggers []*RoutineTrigger, now time.Time) []UnarmedCronTrigger {
	var out []UnarmedCronTrigger
	for _, t := range cronTriggers {
		if isArmed(t, now) {
			continue
		}
		out = append(out, UnarmedCronTrigger{TriggerID: t.ID, Reasons: unarmedReasons(t)})
	}
	return out
}

func unarmedReasons(t *RoutineTrigger) []UnarmedReason {
	var reasons []UnarmedReason
	if !t.Enabled {
		reasons = append(reasons, UnarmedReasonDisabled)
	}
	if !isSchedulable(t) {
		reasons = append(reasons, UnarmedReasonNotSchedulable)
	}
	if len(reasons) == 0 {
		reasons = append(reasons, UnarmedReasonStalled)
	}
	return reasons
}

// sortTriggers returns a copy of triggers in trigger order: created_at
// ascending, ties broken by id ascending.
func sortTriggers(triggers []*RoutineTrigger) []*RoutineTrigger {
	ordered := make([]*RoutineTrigger, len(triggers))
	copy(ordered, triggers)
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].CreatedAt.Equal(ordered[j].CreatedAt) {
			return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

// RoutineClassification is one routine's schedule state plus its unarmed
// cron trigger list.
type RoutineClassification struct {
	State   ScheduleState
	Unarmed []UnarmedCronTrigger
}

// RoutineTriggerReader is the read surface ClassifyRoutines needs: a batch
// read across many routines, and a per-routine fallback read for when the
// batch read fails.
type RoutineTriggerReader interface {
	ListTriggersByRoutineIDs(ctx context.Context, routineIDs []string) (map[string][]*RoutineTrigger, error)
	ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*RoutineTrigger, error)
}

// ClassifyRoutines classifies every named routine against one batch trigger
// read and one captured instant. If the batch read fails, it falls back to
// re-reading each named routine's triggers individually, once each,
// reporting ScheduleStateUnknown only for a routine whose own re-read also
// fails.
func ClassifyRoutines(
	ctx context.Context, reader RoutineTriggerReader, routineIDs []string, now time.Time,
) (map[string]RoutineClassification, error) {
	results := make(map[string]RoutineClassification, len(routineIDs))

	byRoutine, err := reader.ListTriggersByRoutineIDs(ctx, routineIDs)
	if err == nil {
		for _, id := range routineIDs {
			state, unarmed := ClassifyRoutine(byRoutine[id], now)
			results[id] = RoutineClassification{State: state, Unarmed: unarmed}
		}
		return results, nil
	}

	for _, id := range routineIDs {
		triggers, rerr := reader.ListTriggersByRoutineID(ctx, id)
		if rerr != nil {
			results[id] = RoutineClassification{State: ScheduleStateUnknown}
			continue
		}
		state, unarmed := ClassifyRoutine(triggers, now)
		results[id] = RoutineClassification{State: state, Unarmed: unarmed}
	}
	return results, nil
}
