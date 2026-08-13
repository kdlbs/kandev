package service

import "expvar"

// subagentContextTotal exposes writer-health counters for
// RecordSubagentContext, under the existing expvar convention used by
// routing_* (internal/office/scheduler) and subproc_*. Counters only — see
// AC-26 in docs/specs/subagent-context-persistence/spec.md.
//
// Keys: attempted, persisted, skipped_no_identity, anomalous_value, failed.
var subagentContextTotal = expvar.NewMap("subagent_context_total")
