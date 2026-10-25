package sandbox

import (
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"time"
)

const (
	AttemptPrepared         = "prepared"
	AttemptStarting         = "starting"
	AttemptRunning          = "running"
	AttemptStopped          = "stopped"
	AttemptFailed           = "failed"
	AttemptRecoveryRequired = "recovery_required"
)

// AttemptProvider freezes the installed package and runtime used at launch.
type AttemptProvider struct {
	PluginID       string `json:"plugin_id"`
	InstallationID string `json:"installation_id"`
	Key            string `json:"key"`
	Version        string `json:"version"`
	PackageDigest  string `json:"package_digest"`
	RuntimeVersion string `json:"runtime_version"`
	RuntimeDigest  string `json:"runtime_digest"`
}

type CaptureRequest struct {
	RestartOf   string
	Selection   SelectionContext
	Resolved    *Resolved
	SessionID   string
	ExecutionID string
	AgentState  *contract.AgentState
	WorkDir     string
	Launch      *contract.Launch
	Provider    AttemptProvider
}

// AttemptSnapshot is private launch evidence. It is never a public API projection.
type AttemptSnapshot struct {
	RestartOf      string                       `json:"restart_of,omitempty"`
	RestartVersion int64                        `json:"restart_version,omitempty"`
	AgentProfileID string                       `json:"agent_profile_id"`
	AgentState     *contract.AgentStateSnapshot `json:"agent_state,omitempty"`
	SchemaVersion  int                          `json:"schema_version"`
	OrgID          string                       `json:"org_id"`
	WorkspaceID    string                       `json:"workspace_id"`
	TaskID         string                       `json:"task_id"`
	SessionID      string                       `json:"session_id"`
	ExecutionID    string                       `json:"execution_id"`
	WorkDir        string                       `json:"work_dir"`
	Resolved       *Resolved                    `json:"resolved"`
	Grants         contract.Grants              `json:"grants"`
	Ceiling        contract.Grants              `json:"ceiling"`
	Launch         *contract.Launch             `json:"launch"`
	Provider       AttemptProvider              `json:"provider"`
	CreatedAt      time.Time                    `json:"created_at"`
}

// AttemptOwner records core-verified process identities. It does not prove confinement.
type AttemptOwner struct {
	AgentctlIdentity  string `json:"agentctl_identity"`
	SupervisorPID     int    `json:"supervisor_pid"`
	SupervisorBirth   string `json:"supervisor_birth"`
	ChildPID          int    `json:"child_pid,omitempty"`
	ChildBirth        string `json:"child_birth,omitempty"`
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

type Attempt struct {
	ID        string          `json:"id"`
	Version   int64           `json:"version"`
	State     string          `json:"state"`
	Snapshot  AttemptSnapshot `json:"snapshot"`
	Owner     *AttemptOwner   `json:"owner,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}
