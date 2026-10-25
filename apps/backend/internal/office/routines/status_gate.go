package routines

import (
	"fmt"
	"sync"
)

// RoutineNotFiringErrorCode is the stable machine-readable code the API
// returns when a manual or webhook fire is refused because the routine's
// status does not permit it. The frontend selects localized copy from this
// code rather than the human-readable error string.
const RoutineNotFiringErrorCode = "routine_not_firing"

// RoutineNotFiringError is returned by FireManual when the routine's
// status does not permit a fire. The handler recognizes it with
// errors.As before its catch-all and maps it to HTTP 409; every other
// FireManual error — including a missing routine — keeps its existing
// 500 mapping.
type RoutineNotFiringError struct {
	// Status is the observed status, reported verbatim in the response
	// body so the surface can name it without re-reading the routine.
	Status string
}

func (e *RoutineNotFiringError) Error() string {
	return fmt.Sprintf("routine cannot fire: status is %q", e.Status)
}

// stuckTriggerOutcome names the two outcomes that leave a due cron trigger
// un-advanced: the routine backing it could not be read, or its cursor
// could not be advanced. Both re-evaluate every tick for the life of the
// process, so their log entries are bounded to once per (trigger, outcome);
// the suppression-succeeded entry itself is not one of these and is never
// deduplicated.
type stuckTriggerOutcome string

const (
	outcomeUnreadableRoutine stuckTriggerOutcome = "unreadable_routine"
	outcomeCursorNotAdvanced stuckTriggerOutcome = "cursor_not_advanced"
)

// stuckTriggerLog bounds the two "trigger stayed due" log entries to at
// most once per (trigger id, outcome) per process. The comparison key is
// the trigger id and the outcome kind only — no observed status, no error
// value — and the check-and-insert is one atomic operation, so concurrent
// callers observing the same pair cannot both log.
type stuckTriggerLog struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// shouldLog reports whether this (trigger, outcome) pair has not yet been
// logged in this process, recording it before returning so a concurrent
// caller observing the same pair gets false.
func (l *stuckTriggerLog) shouldLog(triggerID string, outcome stuckTriggerOutcome) bool {
	key := triggerID + "|" + string(outcome)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.seen == nil {
		l.seen = make(map[string]struct{})
	}
	if _, logged := l.seen[key]; logged {
		return false
	}
	l.seen[key] = struct{}{}
	return true
}
