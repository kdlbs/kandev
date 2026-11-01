package models

import "time"

// UnrecordedPluginExecutor is the durable handoff of a resource a plugin launch
// provisioned but could not record in executors_running, as when an archive
// holds the environment while the provider returns. Running carries the
// inventory row the environment cleanup adopts so it can destroy the resource
// under its own claim; the record is deleted only after the destroy settles.
type UnrecordedPluginExecutor struct {
	ExecutionID   string
	SessionID     string
	TaskID        string
	EnvironmentID string
	Running       *ExecutorRunning
	CreatedAt     time.Time
}
