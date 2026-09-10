package statussummary

import "expvar"

// Aggregate counters are intentionally unlabeled: a task or workspace label
// would create unbounded cardinality and expose user-controlled identifiers.
var (
	casRetriesTotal           = expvar.NewInt("task_status_summary_cas_retries_total")
	casExhaustionsTotal       = expvar.NewInt("task_status_summary_cas_exhaustions_total")
	eventHandlerFailuresTotal = expvar.NewInt("task_status_summary_event_handler_failures_total")
)

func recordCASRetry()       { casRetriesTotal.Add(1) }
func recordCASExhaustion()  { casExhaustionsTotal.Add(1) }
func recordHandlerFailure() { eventHandlerFailuresTotal.Add(1) }
