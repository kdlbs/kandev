package dataretention

// Operation kinds and lifecycle states. These are persisted in the settings
// record, so the string values are a compatibility surface.
const (
	kindAnalysis = "analysis"
	kindCleanup  = "cleanup"
	choiceBackup = "backup"
	choiceSkip   = "skip"

	stateNone      = "none"
	statePending   = "pending"
	stateRunning   = "running"
	stateReady     = "ready"
	stateFailed    = "failed"
	stateSucceeded = "succeeded"
	statePartial   = "partial"
	stateCancelled = "cancelled"
)
