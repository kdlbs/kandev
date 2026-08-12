// Package signalmetrics publishes the ADR 0015 step-completion-signal expvar
// counters. It is a neutral package so both MCP and UI code can record the
// signal source without depending on each other.
package signalmetrics

import (
	"expvar"
	"strings"
)

// workflowStepSignalReceived counts accepted `step_complete_kandev` signals
// (ADR 0015), labelled by source and agent type. It is the ADR's
// `step_completion_signal_received_total` counter.
var workflowStepSignalReceived = expvar.NewMap("workflow_step_completion_signal_received_total")

// workflowStepSignalFallbackUsed counts manual completion fallbacks, labelled
// by agent type. It is separate from workflowStepSignalReceived so the ADR's
// fallback-used / received ratio has the correct denominator.
var workflowStepSignalFallbackUsed = expvar.NewMap("workflow_step_completion_signal_fallback_used_total")

// metricLabel builds a "k1=v1;k2=v2;..." label string for an expvar map key,
// matching the convention in internal/office/scheduler/metrics_vars.go so a
// downstream Prometheus translation layer can split on the same delimiters.
// Empty values are still emitted so a downstream parser sees consistent
// cardinality dimensions. Keys are intentionally NOT escaped — callers
// control the inputs (source constants, agent profile IDs).
func metricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

// RecordSignalReceived counts one accepted step-completion signal. `source`
// is one of models.StepCompletionSource*. The manual fallback affordance may
// call RecordSignalFallbackUsed separately when it is available.
// `agentType` is the bounded agent-type dimension the ADR's ratio threshold
// ("fallback-used / received <= 10% per agent type") is expressed against —
// e.g. AgentProfile.AgentName ("claude", "codex"), the registry-facing type
// (internal/agent/runtime/lifecycle/manager_profile.go keys the agent
// registry on it) — not AgentProfile.AgentID, which is the store's
// auto-generated UUID for the agent row
// (internal/agent/settings/store/sqlite.go CreateAgent) and is therefore
// unique per install and per agent re-creation, not a bounded type.
func RecordSignalReceived(source, agentType string) {
	workflowStepSignalReceived.Add(
		metricLabel("source", source, "agent_type", agentType), 1)
}

// RecordSignalFallbackUsed counts one manual completion fallback. The
// separate counter keeps fallback events out of the received-signal count.
func RecordSignalFallbackUsed(agentType string) {
	workflowStepSignalFallbackUsed.Add(metricLabel("agent_type", agentType), 1)
}
