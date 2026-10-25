package models

import (
	"time"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// ExecutionSnapshot contains only public launch identities and safe summaries.
// Private argv, runtime paths, environment values, and callback URLs stay outside it.
type ExecutionSnapshot struct {
	SchemaVersion  int                          `json:"schema_version"`
	AttemptID      string                       `json:"attempt_id"`
	SessionID      string                       `json:"session_id"`
	TaskID         string                       `json:"task_id"`
	ExecutionID    string                       `json:"execution_id"`
	Generation     uint64                       `json:"generation"`
	Sequence       uint64                       `json:"sequence"`
	ExecutorID     string                       `json:"executor_id"`
	ExecutorType   string                       `json:"executor_type"`
	AgentProfileID string                       `json:"agent_profile_id"`
	Transport      string                       `json:"transport"`
	Wrapper        *launchwrapper.Summary       `json:"wrapper"`
	Agent          launchwrapper.CommandSummary `json:"agent"`
	Sandbox        *ExecutionSandboxSnapshot    `json:"sandbox"`
	CapturedAt     time.Time                    `json:"captured_at"`
}

type ExecutionSandboxSnapshot struct {
	AttemptID         string `json:"attempt_id"`
	Generation        uint64 `json:"generation"`
	InstallationID    string `json:"installation_id"`
	ProviderKey       string `json:"provider_key"`
	ProviderVersion   string `json:"provider_version"`
	PackageDigest     string `json:"package_digest"`
	RuntimeVersion    string `json:"runtime_version"`
	RuntimeDigest     string `json:"runtime_digest"`
	RevisionID        string `json:"revision_id"`
	RevisionNumber    int64  `json:"revision_number"`
	PolicyName        string `json:"policy_name"`
	PolicyDigest      string `json:"policy_digest"`
	SelectionSource   string `json:"selection_source"`
	CeilingRevisionID string `json:"ceiling_revision_id"`
	CeilingDigest     string `json:"ceiling_digest"`
}
