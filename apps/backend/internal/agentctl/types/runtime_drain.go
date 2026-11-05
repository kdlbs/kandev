package types

import "time"

const (
	// RuntimeDrainScope and RuntimeDrainVersion identify what a drain receipt
	// proves: managed process groups of this agentctl are stopped.
	RuntimeDrainScope   = "managed_process_groups"
	RuntimeDrainVersion = "runtime-seal-drain.v1"
)

// RuntimeDrainRequest asks the exact agentctl process to seal new work and
// wait for its managed instances and process groups to stop. RequestID is the
// cleanup claim operation the drain runs under.
type RuntimeDrainRequest struct {
	RequestID              string `json:"request_id"`
	ExpectedServerIdentity string `json:"expected_server_identity"`
	DeadlineMS             int64  `json:"deadline_ms"`
}

// RuntimeDrainReceipt is positive evidence that managed teardown completed.
// It does not prove that the external resource hosting the runtime has
// stopped, and it carries no authority beyond the request that produced it.
type RuntimeDrainReceipt struct {
	RequestID      string    `json:"request_id"`
	ServerIdentity string    `json:"server_identity"`
	Sealed         bool      `json:"sealed"`
	Drained        bool      `json:"drained"`
	Scope          string    `json:"scope"`
	Version        string    `json:"version"`
	CompletedAt    time.Time `json:"completed_at"`
}
