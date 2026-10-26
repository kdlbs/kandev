package types

import "time"

// RuntimeDrainRequest asks the exact agentctl process to seal new work and
// wait for its managed instance/process groups to stop. The request is
// idempotent only for an identical tuple while this server process lives.
type RuntimeDrainRequest struct {
	RequestID              string `json:"request_id"`
	ExpectedServerIdentity string `json:"expected_server_identity"`
	DeadlineMS             int64  `json:"deadline_ms"`
}

// RuntimeDrainReceipt is positive evidence that managed teardown completed;
// it does not by itself prove that an external Nomad allocation has stopped.
type RuntimeDrainReceipt struct {
	RequestID      string    `json:"request_id"`
	ServerIdentity string    `json:"server_identity"`
	Sealed         bool      `json:"sealed"`
	Drained        bool      `json:"drained"`
	Scope          string    `json:"scope"`
	Version        string    `json:"version"`
	CompletedAt    time.Time `json:"completed_at"`
}
