package statussummary

import "expvar"

// Aggregate counters are intentionally unlabeled: a task or workspace label
// would create unbounded cardinality and expose user-controlled identifiers.
var (
	casRetriesTotal           = expvar.NewInt("task_status_summary_cas_retries_total")
	casExhaustionsTotal       = expvar.NewInt("task_status_summary_cas_exhaustions_total")
	eventHandlerFailuresTotal = expvar.NewInt("task_status_summary_event_handler_failures_total")
	unreadFlipsTotal          = expvar.NewInt("task_status_summary_unread_flips_total")
	unreadLoadFailuresTotal   = expvar.NewInt("task_status_summary_unread_load_failures_total")
)

func recordCASRetry()       { casRetriesTotal.Add(1) }
func recordCASExhaustion()  { casExhaustionsTotal.Add(1) }
func recordHandlerFailure() { eventHandlerFailuresTotal.Add(1) }
func recordUnreadFlip()     { unreadFlipsTotal.Add(1) }

// RecordUnreadLoadFailure counts a failed unread-state read. Loaders outside
// this package call it when they degrade a snapshot to unobserved unread.
func RecordUnreadLoadFailure() { unreadLoadFailuresTotal.Add(1) }
