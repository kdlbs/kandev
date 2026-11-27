package types

import (
	"github.com/kandev/kandev/internal/common/passthroughconfig"
	"time"
)

const PassthroughProtocolVersion = 1

type PassthroughPrepareRequest struct {
	Generation uint64                   `json:"generation"`
	Files      []passthroughconfig.File `json:"files"`
}

type PassthroughStartRequest struct {
	SessionID          string            `json:"session_id"`
	OperationID        string            `json:"operation_id"`
	ExpectedGeneration uint64            `json:"expected_generation"`
	Command            []string          `json:"command"`
	Env                map[string]string `json:"env,omitempty"`
	StripEnv           []string          `json:"strip_env,omitempty"`
	PromptPattern      string            `json:"prompt_pattern,omitempty"`
	IdleTimeout        time.Duration     `json:"idle_timeout,omitempty"`
	StatusDetector     string            `json:"status_detector,omitempty"`
	CheckInterval      time.Duration     `json:"check_interval,omitempty"`
	StabilityWindow    time.Duration     `json:"stability_window,omitempty"`
	Cols               int               `json:"cols,omitempty"`
	Rows               int               `json:"rows,omitempty"`
}

type PassthroughTarget struct {
	ProcessID  string `json:"process_id"`
	Generation uint64 `json:"generation"`
}

type PassthroughInputRequest struct {
	PassthroughTarget
	SubmissionID string `json:"submission_id,omitempty"`
	Data         string `json:"data"`
}

type PassthroughResizeRequest struct {
	PassthroughTarget
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

type PassthroughSnapshot struct {
	PassthroughTarget
	Version         int           `json:"version"`
	InstanceID      string        `json:"instance_id"`
	SessionID       string        `json:"session_id"`
	WorkspacePath   string        `json:"workspace_path"`
	ConfigPath      string        `json:"config_path"`
	MCPURL          string        `json:"mcp_url"`
	OperationID     string        `json:"operation_id,omitempty"`
	Status          ProcessStatus `json:"status,omitempty"`
	OSPID           int           `json:"os_pid,omitempty"`
	ExitCode        *int          `json:"exit_code,omitempty"`
	StartedAt       time.Time     `json:"started_at,omitempty"`
	FirstIdle       bool          `json:"first_idle"`
	WaitingForInput bool          `json:"waiting_for_input"`
	Sequence        uint64        `json:"sequence"`
	StartSequence   uint64        `json:"start_sequence"`
	HasTerminal     bool          `json:"has_terminal"`
}

type PassthroughEvent struct {
	PassthroughTarget
	InstanceID string        `json:"instance_id"`
	SessionID  string        `json:"session_id"`
	Sequence   uint64        `json:"sequence"`
	Kind       string        `json:"kind"`
	Status     ProcessStatus `json:"status,omitempty"`
	ExitCode   *int          `json:"exit_code,omitempty"`
	Timestamp  time.Time     `json:"timestamp"`
}

type PassthroughEventPage struct {
	Events   []PassthroughEvent `json:"events"`
	Sequence uint64             `json:"sequence"`
	Gap      bool               `json:"gap"`
}

type PassthroughInputResult struct {
	Outcome  string `json:"outcome"`
	Sequence uint64 `json:"sequence"`
}
