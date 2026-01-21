package streams

import "time"

// ProcessKind identifies which repository script kind is running.
type ProcessKind string

const (
	ProcessKindSetup   ProcessKind = "setup"
	ProcessKindCleanup ProcessKind = "cleanup"
	ProcessKindDev     ProcessKind = "dev"
	ProcessKindCustom  ProcessKind = "custom"
)

// ProcessStatus represents the lifecycle status of a process.
type ProcessStatus string

const (
	ProcessStatusStarting ProcessStatus = "starting"
	ProcessStatusRunning  ProcessStatus = "running"
	ProcessStatusExited   ProcessStatus = "exited"
	ProcessStatusFailed   ProcessStatus = "failed"
	ProcessStatusStopped  ProcessStatus = "stopped"
)

// ProcessOutput carries stdout/stderr output from a process.
type ProcessOutput struct {
	SessionID string      `json:"session_id"`
	ProcessID string      `json:"process_id"`
	Kind      ProcessKind `json:"kind"`
	Stream    string      `json:"stream"` // stdout|stderr
	Data      string      `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// ProcessStatusUpdate carries status updates for a process.
type ProcessStatusUpdate struct {
	SessionID  string        `json:"session_id"`
	ProcessID  string        `json:"process_id"`
	Kind       ProcessKind   `json:"kind"`
	ScriptName string        `json:"script_name,omitempty"`
	Command    string        `json:"command"`
	WorkingDir string        `json:"working_dir"`
	Status     ProcessStatus `json:"status"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}
