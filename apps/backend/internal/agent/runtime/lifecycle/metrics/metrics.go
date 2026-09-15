// Package metrics publishes the bounded restore metrics owned by agent
// lifecycle. Callers use these functions instead of declaring metric names
// at individual restore entry points.
package metrics

import (
	"expvar"
	"strings"
)

var (
	restoreAttemptsTotal  = expvar.NewMap("agent_restore_attempts_total")
	restoreTruncatedTotal = expvar.NewMap("agent_restore_context_truncated_total")
	recoveryRequiredTotal = expvar.NewMap("agent_restore_recovery_required_total")
)

var restoreOutcomes = map[string]struct{}{
	"native_resumed":    {},
	"reattached":        {},
	"context_continued": {},
	"blocked":           {},
}

var restoreReasons = map[string]struct{}{
	"none":                      {},
	"native_state_missing":      {},
	"native_resume_unsupported": {},
	"workspace_incompatible":    {},
	"transport_failure":         {},
	"authentication_failure":    {},
	"configuration_failure":     {},
	"permission_failure":        {},
	"unknown_failure":           {},
	"branch_unrecoverable":      {},
}

var agentTypes = map[string]struct{}{
	"codex":    {},
	"claude":   {},
	"opencode": {},
	"pi":       {},
	"mock":     {},
}

var consumers = map[string]struct{}{
	"interactive": {},
	"office":      {},
	"automation":  {},
	"queue":       {},
}

// RecordRestoreAttempt records one completed restore decision. Unknown label
// values are collapsed to bounded "other" buckets.
func RecordRestoreAttempt(outcome, reason, agentType string) {
	if outcome == "" {
		outcome = "blocked"
	}
	if reason == "" {
		reason = "none"
	}
	restoreAttemptsTotal.Add(label(
		"outcome", bounded(outcome, restoreOutcomes),
		"reason", bounded(reason, restoreReasons),
		"agent_type", boundedAgentType(agentType),
	), 1)
}

// RecordRestoreContextTruncated records a persisted snapshot that omitted or
// shortened canonical history.
func RecordRestoreContextTruncated(agentType string) {
	restoreTruncatedTotal.Add("agent_type="+boundedAgentType(agentType), 1)
}

// RecordRecoveryRequired records a new recovery block exposed to a consumer.
func RecordRecoveryRequired(consumer, reason string) {
	recoveryRequiredTotal.Add(label(
		"consumer", bounded(consumer, consumers),
		"reason", bounded(reason, restoreReasons),
	), 1)
}

func bounded(value string, allowed map[string]struct{}) string {
	value = strings.TrimSpace(value)
	if _, ok := allowed[value]; ok {
		return value
	}
	return "other"
}

func boundedAgentType(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if _, ok := agentTypes[value]; ok {
		return value
	}
	return "other"
}

func label(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for index := 0; index+1 < len(pairs); index += 2 {
		parts = append(parts, pairs[index]+"="+pairs[index+1])
	}
	return strings.Join(parts, ";")
}
