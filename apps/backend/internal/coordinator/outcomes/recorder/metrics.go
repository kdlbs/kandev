// Package recorder grades decided coordinator proposals, captures manager
// overrides and reads the outcome measures.
package recorder

import "expvar"

// Reasons of coordinator_outcome_grade_failed_total; the set is closed.
const (
	GradeTaskRead     = "task_read"
	GradeProposalRead = "proposal_read"
	GradeUsageRead    = "usage_read"
	GradeQueueFull    = "queue_full"
)

// Reasons of coordinator_override_ignored_total; the set is closed.
const (
	IgnoredNotManager   = "not_manager"
	IgnoredPrincipal    = "principal"
	IgnoredSystem       = "system"
	IgnoredActorUnknown = "actor_unknown"
	IgnoredAuthzError   = "authz_error"
)

// Sites of coordinator_override_read_failed_total; the set is closed.
const (
	ReadFailedHistoryRows     = "history_rows"
	ReadFailedStepOrder       = "step_order"
	ReadFailedReferenceAction = "reference_action"
	ReadFailedReferenceSpec   = "reference_spec"
)

// Sites of coordinator_override_scan_dropped_total; the set is closed.
const (
	ScanDroppedChainCap  = "chain_cap"
	ScanDroppedQueueFull = "queue_full"
)

// Observers of coordinator_outcomes_observer_panic_total; the set is closed.
const (
	ObserverGrader   = "grader"
	ObserverOverride = "override"
)

var (
	gradeFailedTotal   = expvar.NewMap("coordinator_outcome_grade_failed_total")
	ignoredTotal       = expvar.NewMap("coordinator_override_ignored_total")
	readFailedTotal    = expvar.NewMap("coordinator_override_read_failed_total")
	scanDroppedTotal   = expvar.NewMap("coordinator_override_scan_dropped_total")
	observerPanicTotal = expvar.NewMap("coordinator_outcomes_observer_panic_total")
)

func bump(m *expvar.Map, label string) { m.Add(label, 1) }

func read(m *expvar.Map, label string) int64 {
	if v, ok := m.Get(label).(*expvar.Int); ok {
		return v.Value()
	}
	return 0
}

// GradeFailedCount reads one reason of coordinator_outcome_grade_failed_total.
func GradeFailedCount(reason string) int64 { return read(gradeFailedTotal, reason) }

// IgnoredCount reads one reason of coordinator_override_ignored_total.
func IgnoredCount(reason string) int64 { return read(ignoredTotal, reason) }

// ReadFailedCount reads one site of coordinator_override_read_failed_total.
func ReadFailedCount(site string) int64 { return read(readFailedTotal, site) }

// ScanDroppedCount reads one site of coordinator_override_scan_dropped_total.
func ScanDroppedCount(site string) int64 { return read(scanDroppedTotal, site) }

// ObserverPanicCount reads one observer of coordinator_outcomes_observer_panic_total.
func ObserverPanicCount(observer string) int64 { return read(observerPanicTotal, observer) }
