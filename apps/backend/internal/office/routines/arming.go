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
	states := cronTriggerStates(cronTriggers)
	unarmed := unarmedCronTriggers(states, now)

	if state, ok := classifyCronTriggers(states, now); ok {
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

// cronTriggerState pairs a cron trigger with its schedulability, computed
// once per trigger and shared across classification and unarmed-list
// membership rather than re-parsed by each.
type cronTriggerState struct {
	trigger     *RoutineTrigger
	schedulable bool
}

func cronTriggerStates(cronTriggers []*RoutineTrigger) []cronTriggerState {
	states := make([]cronTriggerState, len(cronTriggers))
	for i, t := range cronTriggers {
		states[i] = cronTriggerState{trigger: t, schedulable: isSchedulable(t)}
	}
	return states
}

// classifyCronTriggers evaluates rules 1 through 5 (the ones decided by
// cron trigger state alone) and reports whether one of them matched.
func classifyCronTriggers(states []cronTriggerState, now time.Time) (ScheduleState, bool) {
	for _, s := range states {
		if isArmed(s.trigger, now) {
			return ScheduleStateArmed, true
		}
	}
	for _, s := range states {
		if s.trigger.Enabled && !s.schedulable {
			return ScheduleStateTriggerInvalid, true
		}
	}
	for _, s := range states {
		if s.trigger.Enabled && s.schedulable && s.trigger.NextRunAt == nil {
			return ScheduleStateTriggerUnscheduled, true
		}
	}
	if len(states) > 0 {
		return ScheduleStateTriggerDisabled, true
	}
	return "", false
}

// isArmed reports whether this single cron trigger can currently fire:
// either it has a next occurrence on the calendar (past-due or future), or
// it just fired and hasn't been recomputed yet (within dispatchGrace).
// Schedulability does not gate this: a trigger that already claimed a next
// occurrence still fires on it even if its expression or timezone can no
// longer be parsed (unarmedCronTriggers reports that separately).
func isArmed(t *RoutineTrigger, now time.Time) bool {
	if !t.Enabled {
		return false
	}
	if t.NextRunAt != nil {
		return true
	}
	return withinDispatchGrace(t, now)
}

func isSchedulable(t *RoutineTrigger) bool {
	return shared.ValidateCronSchedule(t.CronExpression, t.Timezone) == nil
}

func withinDispatchGrace(t *RoutineTrigger, now time.Time) bool {
	if t.LastFiredAt == nil {
		return false
	}
	elapsed := now.Sub(*t.LastFiredAt)
	return elapsed >= 0 && elapsed <= dispatchGrace
}

// unarmedCronTriggers reports every cron trigger, in trigger order, that
// matches AC-OFFICE-ROUTINE-ARMING-001.8's unarmed predicate — independent
// of whether the same trigger is also armed, so an armed-but-broken trigger
// appears here too.
func unarmedCronTriggers(states []cronTriggerState, now time.Time) []UnarmedCronTrigger {
	var out []UnarmedCronTrigger
	for _, s := range states {
		if !isUnarmedCron(s, now) {
			continue
		}
		out = append(out, UnarmedCronTrigger{TriggerID: s.trigger.ID, Reasons: unarmedReasons(s, now)})
	}
	return out
}

// isUnarmedCron implements AC-OFFICE-ROUTINE-ARMING-001.8's three-way OR:
// disabled, or not schedulable, or (enabled, schedulable, no next
// occurrence, and past the dispatch grace).
func isUnarmedCron(s cronTriggerState, now time.Time) bool {
	if !s.trigger.Enabled {
		return true
	}
	if !s.schedulable {
		return true
	}
	return s.trigger.NextRunAt == nil && !withinDispatchGrace(s.trigger, now)
}

func unarmedReasons(s cronTriggerState, now time.Time) []UnarmedReason {
	var reasons []UnarmedReason
	if !s.trigger.Enabled {
		reasons = append(reasons, UnarmedReasonDisabled)
	}
	if !s.schedulable {
		reasons = append(reasons, UnarmedReasonNotSchedulable)
	}
	if s.trigger.Enabled && s.schedulable && s.trigger.NextRunAt == nil && !withinDispatchGrace(s.trigger, now) {
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
