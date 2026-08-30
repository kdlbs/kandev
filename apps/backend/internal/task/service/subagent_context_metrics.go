package service

import "expvar"

// subagentContextTotal exposes writer-health counters for
// RecordSubagentContext, under the existing expvar convention used by
// routing_* (internal/office/scheduler) and subproc_*. Counters only — see
// AC-26 in docs/specs/agents/requirements/subagent-context-persistence.md.
//
// Keys: attempted, persisted, skipped_no_identity, anomalous_value, failed,
// unknown_execution (Amendment 1, AC-31).
var subagentContextTotal = expvar.NewMap("subagent_context_total")
