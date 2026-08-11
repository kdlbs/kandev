// Package signalmetrics publishes the ADR 0015 step-completion-signal expvar
// counter. It is a neutral package (not internal/mcp/handlers) so that a
// future manual-fallback affordance can increment through the same
// RecordSignalReceived call instead of forking a second call site.
package signalmetrics

import (
	"expvar"
	"strings"
)

// workflowStepSignalReceived counts accepted `step_complete_kandev` signals
// (ADR 0015), labelled by source and agent type. It is the ADR's
// `step_completion_signal_received_total` counter. The ADR also specifies a
// companion `step_completion_signal_fallback_used_total` counter, but that
// counter's only production increment site is the manual "Mark complete &
// advance" fallback button (ADR 0015 § Decision ¶6), which is unimplemented
// as of this package's introduction. Publishing that counter with no call
// site would pin it at zero and make the ADR's fallback-used/received ratio
// misreport "0% fallback" regardless of reality, so it is deliberately not
// published here. The `source` label on this map already carries the
// dimension the ratio needs: once the fallback path exists and calls
// RecordSignalReceived with source="manual_fallback", the same ratio is
// computable as received{source=manual_fallback} / received{all sources},
// with no second counter and no rework of this map.
var workflowStepSignalReceived = expvar.NewMap("workflow_step_completion_signal_received_total")

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
// is one of models.StepCompletionSource* — today only
// models.StepCompletionSourceAgent is reachable in production, since the
// ADR's manual fallback affordance (§ Decision ¶6) has no call site yet.
// `agentType` is the bounded agent-type dimension the ADR's ratio threshold
// ("fallback-used / received <= 10% per agent type") is expressed against —
// e.g. AgentProfile.AgentID ("claude", "codex") — not a per-user profile ID,
// which never shrinks in the expvar map and cannot be joined back to a type
// once the profile is deleted.
func RecordSignalReceived(source, agentType string) {
	workflowStepSignalReceived.Add(
		metricLabel("source", source, "agent_type", agentType), 1)
}
