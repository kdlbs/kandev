package workflowsync

import (
	"expvar"
	"strings"
)

var workflowSyncTransitionsTotal = expvar.NewMap("workflow_sync_recovery_transitions_total")

func workflowSyncMetricLabel(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		parts = append(parts, pairs[i]+"="+pairs[i+1])
	}
	return strings.Join(parts, ";")
}

func incWorkflowSyncTransition(transition, provider, failureClass, retrySource string) {
	workflowSyncTransitionsTotal.Add(workflowSyncMetricLabel(
		"transition", transition,
		"provider", provider,
		"failure_class", failureClass,
		"retry_source", retrySource,
	), 1)
}
