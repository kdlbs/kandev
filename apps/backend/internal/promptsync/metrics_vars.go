package promptsync

import "expvar"

// Closed-key counters: prompt_sync_runs_total keys success/failure, and
// prompt_sync_poll_skipped_total key suspended. No repository, workspace,
// or prompt identifier is ever a key.
var (
	runsTotal        = expvar.NewMap("prompt_sync_runs_total")
	pollSkippedTotal = expvar.NewMap("prompt_sync_poll_skipped_total")
)

func incRun(outcome string)        { runsTotal.Add(outcome, 1) }
func incPollSkipped(reason string) { pollSkippedTotal.Add(reason, 1) }
