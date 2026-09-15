package toolretention

const (
	stateNone      = "none"
	statePending   = "pending"
	stateRunning   = "running"
	stateReady     = "ready"
	stateFailed    = "failed"
	statePartial   = "partial"
	stateSucceeded = "succeeded"
	stateCancelled = "cancelled"
	kindAnalysis   = "analysis"
	kindCleanup    = "cleanup"
	choiceBackup   = "backup"
	choiceSkip     = "skip"
)
