package runtime

// Process/run lifecycle states. These intentionally mirror runs.status.
const (
	ProcessQueued    = "queued"
	ProcessClaimed   = "claimed"
	ProcessStarting  = "starting"
	ProcessRunning   = "running"
	ProcessFinished  = "finished"
	ProcessFailed    = "failed"
	ProcessCancelled = "cancelled"
)
