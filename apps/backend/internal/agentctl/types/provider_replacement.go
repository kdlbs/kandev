package types

import (
	p "github.com/kandev/kandev/internal/mcp/profile"
)

const ProviderMCPReplacementCapability = "provider-mcp-replacement-v1"
const ProviderMCPReplacementHeader = "X-Kandev-Provider-MCP-Replacement"

// ProviderReplacementSource is the complete compare-and-swap identity of a
// settled provider. Workspace identity is retained through the replacement.
type ProviderReplacementSource struct {
	ExecutionID           string `json:"execution_id"`
	TaskID                string `json:"task_id"`
	SessionID             string `json:"session_id"`
	HelperID              string `json:"helper_id"`
	ProcessGeneration     uint64 `json:"process_generation"`
	ConfigurationRevision uint64 `json:"configuration_revision"`
	StopGeneration        uint64 `json:"stop_generation"`
	BindingID             string `json:"binding_id"`
	ProfileDigest         string `json:"profile_digest"`
	HarnessGeneration     uint64 `json:"harness_generation"`
	StreamID              string `json:"stream_id"`
	IncarnationID         string `json:"incarnation_id"`
	NativeSessionID       string `json:"native_session_id"`
}

// ProviderReplacementRequest is accepted only on authenticated control routes.
// The backend resolves Profile and configuration from the saved human selection.
type ProviderReplacementRequest struct {
	PreviousOperationID string                    `json:"previous_operation_id,omitempty"`
	Action              string                    `json:"action"`
	OperationID         string                    `json:"operation_id"`
	ExpectedVersion     uint64                    `json:"expected_version,omitempty"`
	Source              ProviderReplacementSource `json:"source"`
	Scope               p.ProviderMCPScope        `json:"scope"`
	Profile             p.Context                 `json:"profile"`
	Args                []string                  `json:"args,omitempty"`
	Env                 map[string]string         `json:"env,omitempty"`
	ScopeCredential     string                    `json:"scope_credential,omitempty"`
	TargetStreamID      string                    `json:"target_stream_id,omitempty"`
	Instructions        string                    `json:"instructions,omitempty"`
}

type ProviderReplacementStatus struct {
	Ready                 bool                      `json:"ready"`
	Capability            string                    `json:"capability,omitempty"`
	OperationID           string                    `json:"operation_id"`
	Phase                 string                    `json:"phase"`
	Version               uint64                    `json:"version"`
	Source                ProviderReplacementSource `json:"source"`
	Scope                 *p.ProviderMCPScope       `json:"scope,omitempty"`
	Catalog               map[string][]string       `json:"catalog,omitempty"`
	FailureCode           string                    `json:"failure_code,omitempty"`
	SourceTerminated      bool                      `json:"source_terminated"`
	NativeResumeConfirmed bool                      `json:"native_resume_confirmed"`
}

const (
	ProviderReplacementActionAbandon  = "abandon"
	ProviderReplacementActionPrepare  = "prepare"
	ProviderReplacementActionRetire   = "retire"
	ProviderReplacementActionActivate = "activate"
	ProviderReplacementActionAccept   = "accept"
	ProviderReplacementActionRelease  = "release"
)

const (
	ProviderReplacementPhaseMissing  = "missing"
	ProviderReplacementPhasePrepared = "prepared"
	ProviderReplacementPhaseRetiring = "retiring"
	ProviderReplacementPhaseRetired  = "retired"
	ProviderReplacementPhaseStarting = "starting"
	ProviderReplacementPhaseStarted  = "started"
	ProviderReplacementPhaseAccepted = "accepted"
	ProviderReplacementPhaseFailed   = "failed"
	ProviderReplacementPhaseReleased = "released"
)
