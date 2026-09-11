package runtime

import restoremetrics "github.com/kandev/kandev/internal/agent/runtime/lifecycle/metrics"

// RecordRestoreAttempt publishes bounded lifecycle restore telemetry through
// the runtime seam used by higher-level coordinators.
func RecordRestoreAttempt(outcome, reason, agentType string) {
	restoremetrics.RecordRestoreAttempt(outcome, reason, agentType)
}

// RecordRestoreContextTruncated publishes bounded snapshot truncation telemetry.
func RecordRestoreContextTruncated(agentType string) {
	restoremetrics.RecordRestoreContextTruncated(agentType)
}

// RecordRecoveryRequired publishes bounded recovery-block telemetry.
func RecordRecoveryRequired(consumer, reason string) {
	restoremetrics.RecordRecoveryRequired(consumer, reason)
}
