package models

import (
	"errors"
	"time"
)

// ErrControlServerRecordNotFound is returned when no control-server record
// has been written yet, e.g. before the standalone control server is ever
// started or adopted on this installation.
var ErrControlServerRecordNotFound = errors.New("control server record not found")

// ControlServerRecord is the single installation-scoped durable record of the
// standalone agentctl control server that supervises every worktree and local
// agent instance on this host: where it is, how to prove ownership of it, and
// what it can do. Exactly one row exists per installation.
//
// It is deliberately separate from ExecutorRunning: that table is per-session
// and has no row when no session is live, which would leave a detached,
// instance-less server unlocatable. Written when a server is started or
// adopted, read before any control-server contact at startup.
type ControlServerRecord struct {
	// Endpoint is the control server's host:port. The launcher may relocate to
	// a free port when the configured one is busy, so this is the only durable
	// way a restarted backend can find a relocated survivor.
	Endpoint string `json:"endpoint"`
	// ServerIdentity is the opaque per-launch value the control server
	// echoes on its identity endpoint, compared by the adopting backend to
	// confirm it is talking to the server this record describes.
	ServerIdentity string `json:"server_identity"`
	// CredentialSecretID is a reference into the secret store for the single
	// rotating credential that authenticates both control-plane and
	// per-instance operations and streams (design 01 "Single driver",
	// AC-EXECUTORS-CONTROL-OWNERSHIP-002.6). The credential itself is never
	// stored here: this is a reference, not the bearer token.
	CredentialSecretID string `json:"credential_secret_id"`
	// Capabilities is the named capability set last observed from the
	// control server's identity endpoint.
	Capabilities []string `json:"capabilities"`
	// DiagnosticLogPath is the location of the control server's detached
	// diagnostic-output log sink, so both the next backend and an operator
	// can find it.
	DiagnosticLogPath string    `json:"diagnostic_log_path"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
