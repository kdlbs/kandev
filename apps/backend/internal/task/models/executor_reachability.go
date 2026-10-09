package models

import (
	"errors"
	"time"
)

// ErrExecutorReachabilityNotFound is returned when no reachability record
// exists for the requested executor.
var ErrExecutorReachabilityNotFound = errors.New("executor reachability record not found")

// ExecutorReachabilityState is the coarse status shown to operators.
type ExecutorReachabilityState string

const (
	ExecutorReachabilityStateUnknown     ExecutorReachabilityState = "unknown"
	ExecutorReachabilityStateReachable   ExecutorReachabilityState = "reachable"
	ExecutorReachabilityStateUnreachable ExecutorReachabilityState = "unreachable"
)

// ExecutorReachabilityReason mirrors lifecycle.SSHReachabilityReason's closed
// set as plain strings — this package sits below the agent runtime tier and
// must not import it. An empty reason means the observation was a success.
type ExecutorReachabilityReason string

const (
	ExecutorReachabilityReasonConfig  ExecutorReachabilityReason = "config"
	ExecutorReachabilityReasonTimeout ExecutorReachabilityReason = "timeout"
	ExecutorReachabilityReasonHostKey ExecutorReachabilityReason = "host_key"
	ExecutorReachabilityReasonAuth    ExecutorReachabilityReason = "auth"
	ExecutorReachabilityReasonNetwork ExecutorReachabilityReason = "network"
	ExecutorReachabilityReasonUnknown ExecutorReachabilityReason = "unknown"
)

// ExecutorReachability is the single stored row for one SSH executor's
// observed reachability. It lives in its own table, never on Executor
// itself — Executor.Config is user-authored input and Executor.Status is a
// user-controlled switch, and neither should share a row with observed
// state.
type ExecutorReachability struct {
	ExecutorID          string                     `json:"executor_id"`
	State               ExecutorReachabilityState  `json:"state"`
	Reason              ExecutorReachabilityReason `json:"reason"`
	Message             string                     `json:"message"`
	ConsecutiveFailures int                        `json:"consecutive_failures"`
	Host                string                     `json:"host"`
	CheckedAt           *time.Time                 `json:"checked_at,omitempty"`
	LastSuccessAt       *time.Time                 `json:"last_success_at,omitempty"`
	UpdatedAt           time.Time                  `json:"updated_at"`
}

// ExecutorReachabilityObservation is what a single probe (or a launch's own
// dial attempt) reports to UpsertExecutorReachability. The derived counter
// and state for an existing row live entirely in the upsert's own SQL — this
// struct carries only the observation and the parameters the SQL needs to
// derive from it. InitialState and InitialFailures are used only when no row
// exists yet for the executor; every later write derives both from the
// stored row instead.
type ExecutorReachabilityObservation struct {
	ExecutorID       string
	SeenUpdatedAt    time.Time
	InitialState     ExecutorReachabilityState
	InitialFailures  int
	Reason           ExecutorReachabilityReason
	Message          string
	Host             string
	CheckedAt        time.Time
	FailureThreshold int
}
