package models

// TaskEnvironmentAdmissionClass is the durable admission result for a task
// environment recovery claim.
type TaskEnvironmentAdmissionClass string

const (
	TaskEnvironmentAdmissionInactivePreserved TaskEnvironmentAdmissionClass = "inactive_preserved"
	TaskEnvironmentAdmissionLiveBlocker       TaskEnvironmentAdmissionClass = "live_blocker"
)

// TaskEnvironmentAdmissionSnapshot contains the durable consumer state read
// while the recovery-claim transaction holds the environment authority lock.
type TaskEnvironmentAdmissionSnapshot struct {
	MaterializationSessionID string
	Consumers                []TaskEnvironmentAdmissionConsumer
}

// TaskEnvironmentAdmissionConsumer is the bounded durable state used to
// decide whether one attached session still blocks recovery.
type TaskEnvironmentAdmissionConsumer struct {
	SessionID      string
	SessionState   TaskSessionState
	HasActiveTurn  bool
	HasExecutor    bool
	ExecutorStatus string
	IsRequester    bool
}
