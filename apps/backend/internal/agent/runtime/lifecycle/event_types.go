// Package lifecycle provides event payload types for agent lifecycle events.
package lifecycle

import (
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// AgentEventPayload is the payload for agent lifecycle events (started, stopped, ready, completed, failed).
type AgentEventPayload struct {
	AgentExecutionID   string                 `json:"agent_execution_id"`
	RunID              string                 `json:"run_id,omitempty"`
	TaskID             string                 `json:"task_id"`
	SessionID          string                 `json:"session_id,omitempty"`
	TaskEnvironmentID  string                 `json:"task_environment_id,omitempty"`
	TurnID             string                 `json:"turn_id,omitempty"`
	AgentID            string                 `json:"agent_id,omitempty"`
	AgentProfileID     string                 `json:"agent_profile_id"`
	ExecutionProfileID string                 `json:"execution_profile_id,omitempty"`
	ContainerID        string                 `json:"container_id,omitempty"`
	Status             string                 `json:"status"`
	StartedAt          time.Time              `json:"started_at"`
	FinishedAt         *time.Time             `json:"finished_at,omitempty"`
	ErrorMessage       string                 `json:"error_message,omitempty"`
	FailureCode        string                 `json:"failure_code,omitempty"`
	FailureDetails     string                 `json:"failure_details,omitempty"`
	ProviderError      *streams.ProviderError `json:"provider_error,omitempty"`
	ExitCode           *int                   `json:"exit_code,omitempty"`
	PromptGeneration   uint64                 `json:"prompt_generation,omitempty"`
	// Prompt replay evidence is populated on terminal failure events. It is
	// captured by lifecycle before the terminal event is published so consumers
	// do not have to infer output or effects from independently subscribed
	// stream events.
	EvidenceKnown  bool `json:"evidence_known,omitempty"`
	OutputObserved bool `json:"output_observed,omitempty"`
	EffectObserved bool `json:"effect_observed,omitempty"`
}

// PromptAttemptEvidence is the immutable lifecycle snapshot attached to a
// terminal failure event. Lifecycle conservatively treats any genuine turn
// content as both output and effect evidence, which fails replay closed.
type PromptAttemptEvidence struct {
	EvidenceKnown  bool
	OutputObserved bool
	EffectObserved bool
}

// AgentStalledPayload describes a prompt that has stopped receiving agent events.
// The activity epoch lets the consumer reject a stale watchdog snapshot. A
// never-started prompt is terminal only when its snapshot remains current.
type AgentStalledPayload struct {
	AgentExecutionID string        `json:"agent_execution_id"`
	TaskID           string        `json:"task_id"`
	SessionID        string        `json:"session_id"`
	PromptGeneration uint64        `json:"prompt_generation"`
	ActivityEpoch    uint64        `json:"activity_epoch,omitempty"`
	LastActivityAt   time.Time     `json:"last_activity_at"`
	StalledFor       time.Duration `json:"stalled_for"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
	ToolName         string        `json:"tool_name,omitempty"`
	ToolTitle        string        `json:"tool_title,omitempty"`
	ToolStatus       string        `json:"tool_status,omitempty"`
	// NeverStarted is true when the agent has not emitted a single event since
	// this prompt was dispatched — a terminal failure, not a mid-work pause.
	NeverStarted bool `json:"never_started"`
}

// AgentctlEventPayload is the payload for agentctl lifecycle events (starting, ready, error).
type AgentctlEventPayload struct {
	TaskID            string `json:"task_id"`
	SessionID         string `json:"session_id"`
	TaskEnvironmentID string `json:"task_environment_id,omitempty"`
	AgentExecutionID  string `json:"agent_execution_id"`
	ErrorMessage      string `json:"error_message,omitempty"`
	FailureCode       string `json:"failure_code,omitempty"`
	FailureDetails    string `json:"failure_details,omitempty"`
	WorktreeID        string `json:"worktree_id,omitempty"`
	WorktreePath      string `json:"worktree_path,omitempty"`
	WorktreeBranch    string `json:"worktree_branch,omitempty"`
	// TaskWorkspacePath is the task root that contains every per-repo
	// worktree as a sibling subdir, populated when the event signals a
	// sibling worktree being added (multi-branch add_branch flow) rather
	// than the initial session ready. The frontend uses this to repoint the
	// file browser at the task root once the task becomes multi-branch.
	TaskWorkspacePath string `json:"task_workspace_path,omitempty"`
}

// ACPSessionCreatedPayload is the payload when an ACP session is created.
//
// AgentProfileID is the lifecycle execution ID, kept under its historical key
// for backward compatibility. AgentExecutionID is the same value under the
// canonical key the watcher / orchestrator uses, added so downstream code
// (resume-token CAS) can rely on a single field name across event types.
type ACPSessionCreatedPayload struct {
	TaskID           string `json:"task_id"`
	SessionID        string `json:"session_id"`
	AgentProfileID   string `json:"agent_profile_id"`
	AgentExecutionID string `json:"agent_execution_id"`
	ACPSessionID     string `json:"acp_session_id"`
}

// PrepareProgressEventPayload is the payload for environment preparation progress events.
type PrepareProgressEventPayload struct {
	TaskID        string     `json:"task_id"`
	SessionID     string     `json:"session_id"`
	ExecutionID   string     `json:"execution_id"`
	StepName      string     `json:"step_name"`
	StepCommand   string     `json:"step_command,omitempty"`
	StepIndex     int        `json:"step_index"`
	TotalSteps    int        `json:"total_steps"`
	Status        string     `json:"status"`
	Output        string     `json:"output,omitempty"`
	Error         string     `json:"error,omitempty"`
	Warning       string     `json:"warning,omitempty"`
	WarningDetail string     `json:"warning_detail,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	EndedAt       *time.Time `json:"ended_at,omitempty"`
	Timestamp     string     `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p PrepareProgressEventPayload) GetSessionID() string {
	return p.SessionID
}

// PrepareCompletedEventPayload is the payload when environment preparation finishes.
type PrepareCompletedEventPayload struct {
	TaskID        string        `json:"task_id"`
	SessionID     string        `json:"session_id"`
	ExecutionID   string        `json:"execution_id"`
	Success       bool          `json:"success"`
	ErrorMessage  string        `json:"error_message,omitempty"`
	DurationMs    int64         `json:"duration_ms"`
	WorkspacePath string        `json:"workspace_path,omitempty"`
	Steps         []PrepareStep `json:"steps,omitempty"`
	Timestamp     string        `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p PrepareCompletedEventPayload) GetSessionID() string {
	return p.SessionID
}

// AgentStreamEventData contains the nested event data within AgentStreamEventPayload.
type AgentStreamEventData struct {
	Type             string                 `json:"type"`
	ACPSessionID     string                 `json:"acp_session_id,omitempty"`
	Text             string                 `json:"text,omitempty"`
	ToolCallID       string                 `json:"tool_call_id,omitempty"`
	ToolName         string                 `json:"tool_name,omitempty"`
	ToolTitle        string                 `json:"tool_title,omitempty"`
	ToolStatus       string                 `json:"tool_status,omitempty"`
	Error            string                 `json:"error,omitempty"`
	ProviderError    *streams.ProviderError `json:"provider_error,omitempty"`
	SessionStatus    string                 `json:"session_status,omitempty"` // "resumed" or "new" for session_status events
	PromptGeneration uint64                 `json:"prompt_generation,omitempty"`
	TurnID           string                 `json:"turn_id,omitempty"`
	Data             interface{}            `json:"data,omitempty"`

	// ParentToolCallID identifies the parent Task tool call when this event
	// comes from a subagent. Used for visual nesting in the UI.
	ParentToolCallID string `json:"parent_tool_call_id,omitempty"`

	// PendingID identifies a permission request (for "permission_cancelled" events).
	PendingID string `json:"pending_id,omitempty"`

	// RequestID is the Kandev-generated identity for the exact permission
	// request this event concerns (for "permission_cancelled" events). A
	// provider may reuse PendingID for a later, unrelated request, so a
	// delayed cancellation must be matched against RequestID too before it
	// is allowed to expire a permission message.
	RequestID string `json:"request_id,omitempty"`

	// Normalized contains the typed tool payload data.
	// This is used to populate message metadata with structured tool information.
	Normalized *streams.NormalizedPayload `json:"normalized,omitempty"`

	// Streaming message fields (for "message_streaming" and "thinking_streaming" types)
	// MessageID is the ID of the message being streamed (empty for first chunk, set for appends)
	MessageID string `json:"message_id,omitempty"`
	// IsAppend indicates whether this is an append to an existing message (true) or a new message (false)
	IsAppend bool `json:"is_append,omitempty"`
	// MessageType distinguishes between "message" and "thinking" content types
	MessageType string `json:"message_type,omitempty"`

	// AvailableCommands contains the slash commands available from the agent.
	// Populated when Type is "available_commands".
	AvailableCommands []streams.AvailableCommand `json:"available_commands,omitempty"`

	// ToolCallContents contains rich content produced by a tool call (diffs, text, terminals).
	// Populated when Type is "tool_call" or "tool_update".
	ToolCallContents []streams.ToolCallContentItem `json:"tool_call_contents,omitempty"`

	// ContentBlocks contains multimodal content blocks (images, audio, resource links).
	// Populated when Type is "message_chunk" with non-text content.
	ContentBlocks []streams.ContentBlock `json:"content_blocks,omitempty"`

	// Role distinguishes user vs assistant messages (e.g., during session/load replay).
	// Populated when Type is "message_chunk" with role "user".
	Role string `json:"role,omitempty"`

	// CurrentModeID is the active session mode identifier.
	// Populated when Type is "session_mode".
	CurrentModeID string `json:"current_mode_id,omitempty"`

	// AvailableModes lists modes the agent supports.
	// Populated when Type is "session_mode".
	AvailableModes []streams.SessionModeInfo `json:"available_modes,omitempty"`

	// Agent capabilities (from "agent_capabilities" event)
	SupportsImage           bool                     `json:"supports_image"`
	SupportsAudio           bool                     `json:"supports_audio"`
	SupportsEmbeddedContext bool                     `json:"supports_embedded_context"`
	SupportsPromptQueueing  bool                     `json:"supports_prompt_queueing"`
	AuthMethods             []streams.AuthMethodInfo `json:"auth_methods,omitempty"`

	// Session models (from "session_models" event)
	CurrentModelID string                     `json:"current_model_id,omitempty"`
	SessionModels  []streams.SessionModelInfo `json:"session_models,omitempty"`
	ConfigOptions  []streams.ConfigOption     `json:"config_options,omitempty"`
	// ConfigBaselineCandidate is an authoritative startup response snapshot.
	// ConfigOptions remains the latest live provider state.
	ConfigBaselineCandidate []streams.ConfigOption `json:"config_baseline_candidate,omitempty"`
	// OriginalConfigCandidate is the profile-settled state captured before
	// runtime/workflow overrides are applied.
	OriginalConfigCandidate []streams.ConfigOption `json:"original_config_candidate,omitempty"`

	// Session model fallback (from "session_model_fallback" event): the
	// fallback model the session started on because the configured start
	// model was unavailable.
	FallbackModel string `json:"fallback_model,omitempty"`

	// ModelSelectionWarning explains an executor-authoritative default or
	// explicit fallback decision.
	ModelSelectionWarning *streams.ModelSelectionWarning `json:"model_selection_warning,omitempty"`

	// Session info (from "session_info" event)
	SessionTitle     string         `json:"session_title,omitempty"`
	SessionUpdatedAt string         `json:"session_updated_at,omitempty"`
	SessionMeta      map[string]any `json:"session_meta,omitempty"`

	// Usage (attached to "complete" event)
	Usage *streams.PromptUsage `json:"usage,omitempty"`

	// Plan entries (from "plan" event — ACP/Codex agent todos)
	PlanEntries []streams.PlanEntry `json:"plan_entries,omitempty"`

	// PlanContent contains rich markdown plan content (from "agent_plan" event).
	PlanContent string `json:"plan_content,omitempty"`

	// MCPAttachment is one safe MCP delivery or observation fact.
	MCPAttachment *streams.MCPAttachmentEvidence `json:"mcp_attachment,omitempty"`

	// MCPAttachmentAttempt starts a new backend-owned MCP evidence timeline.
	MCPAttachmentAttempt *streams.MCPAttachmentAttempt `json:"mcp_attachment_attempt,omitempty"`
}

// AgentStreamEventPayload is the payload for agent stream events (WebSocket streaming).
//
// AgentID historically was populated with execution.ID, not the agent-type slug.
// ExecutionID is the explicit, unambiguous version added so consumers can use it
// for execution-scoped logic (e.g., resume-token CAS that must reject writes from
// a defunct execution).
type AgentStreamEventPayload struct {
	Type        string                `json:"type"` // Always "agent/event"
	Timestamp   string                `json:"timestamp"`
	AgentID     string                `json:"agent_id"`     // Historical: execution.ID. Prefer ExecutionID.
	ExecutionID string                `json:"execution_id"` // Lifecycle execution ID; stable across the payload's lifetime.
	TaskID      string                `json:"task_id"`
	SessionID   string                `json:"session_id"` // Task session ID
	Data        *AgentStreamEventData `json:"data"`
}

// GitEventType discriminates the type of git event
type GitEventType string

const (
	GitEventTypeStatusUpdate    GitEventType = "status_update"
	GitEventTypeCommitCreated   GitEventType = "commit_created"
	GitEventTypeCommitsReset    GitEventType = "commits_reset"
	GitEventTypeBranchSwitched  GitEventType = "branch_switched"
	GitEventTypeSnapshotCreated GitEventType = "snapshot_created"
)

// GitEventPayload is a unified payload for all git-related WebSocket events.
// Uses discriminated union pattern with Type field.
type GitEventPayload struct {
	Type              GitEventType `json:"type"`
	TaskID            string       `json:"task_id,omitempty"`
	SessionID         string       `json:"session_id"`
	TaskEnvironmentID string       `json:"task_environment_id,omitempty"`
	AgentID           string       `json:"agent_id,omitempty"`
	Timestamp         string       `json:"timestamp"`

	// For status_update
	Status *GitStatusData `json:"status,omitempty"`

	// For commit_created
	Commit *GitCommitData `json:"commit,omitempty"`

	// For commits_reset
	Reset *GitResetData `json:"reset,omitempty"`

	// For branch_switched
	BranchSwitch *GitBranchSwitchData `json:"branch_switch,omitempty"`

	// For snapshot_created
	Snapshot *GitSnapshotData `json:"snapshot,omitempty"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p GitEventPayload) GetSessionID() string {
	return p.SessionID
}

type GitStatusData struct {
	Branch              string   `json:"branch"`
	RemoteBranch        string   `json:"remote_branch,omitempty"`
	HeadCommit          string   `json:"head_commit,omitempty"`
	BaseCommit          string   `json:"base_commit,omitempty"`
	ComparisonTarget    string   `json:"comparison_target,omitempty"`
	ComparisonStatus    string   `json:"comparison_status,omitempty"`
	ComparisonErrorCode string   `json:"comparison_error_code,omitempty"`
	Modified            []string `json:"modified"`
	Added               []string `json:"added"`
	Deleted             []string `json:"deleted"`
	Untracked           []string `json:"untracked"`
	Renamed             []string `json:"renamed"`
	Ahead               int      `json:"ahead"`
	Behind              int      `json:"behind"`
	// RemoteAhead/RemoteBehind compare against this branch's own upstream
	// (@{upstream}), unlike Ahead/Behind which are relative to the base
	// branch and never reach zero just because the branch was pushed. Push
	// detection (event_handlers_git.go) reads RemoteAhead, not Ahead.
	RemoteAhead      int         `json:"remote_ahead"`
	RemoteBehind     int         `json:"remote_behind"`
	RemoteHeadCommit string      `json:"remote_head_commit,omitempty"`
	Files            interface{} `json:"files,omitempty"`
	BranchAdditions  int         `json:"branch_additions,omitempty"`
	BranchDeletions  int         `json:"branch_deletions,omitempty"`
	// RepositoryName identifies which repository this status belongs to in
	// multi-repo task workspaces. Empty for single-repo. Carried through to
	// the frontend so the Changes panel can render per-repo group headers.
	RepositoryName string `json:"repository_name,omitempty"`
	// IsSubmodule identifies an initialized Git submodule repository so the
	// frontend can render its scope boundary without guessing from its name.
	IsSubmodule bool `json:"is_submodule,omitempty"`
}

type GitCommitData struct {
	ID           string `json:"id,omitempty"`
	CommitSHA    string `json:"commit_sha"`
	ParentSHA    string `json:"parent_sha"`
	Message      string `json:"commit_message"`
	AuthorName   string `json:"author_name"`
	AuthorEmail  string `json:"author_email"`
	FilesChanged int    `json:"files_changed"`
	Insertions   int    `json:"insertions"`
	Deletions    int    `json:"deletions"`
	CommittedAt  string `json:"committed_at"`
	CreatedAt    string `json:"created_at,omitempty"`
	// RepositoryName identifies which repo this commit belongs to in multi-repo
	// task workspaces. Empty for single-repo. Carried to the frontend so the
	// Commits panel can render per-repo group headers.
	RepositoryName string `json:"repository_name,omitempty"`
}

type GitResetData struct {
	PreviousHead string `json:"previous_head"`
	CurrentHead  string `json:"current_head"`
	DeletedCount int    `json:"deleted_count"`
	// RepositoryName identifies which repo this reset belongs to in multi-repo
	// task workspaces. Empty for single-repo. Carried to the frontend so the
	// Changes panel can scope its per-repo group state.
	RepositoryName string `json:"repository_name,omitempty"`
}

type GitBranchSwitchData struct {
	PreviousBranch string `json:"previous_branch"`
	CurrentBranch  string `json:"current_branch"`
	CurrentHead    string `json:"current_head"`
	BaseCommit     string `json:"base_commit"`
	// RepositoryName identifies which repo this branch switch belongs to in
	// multi-repo task workspaces. Empty for single-repo. Carried to the
	// frontend so the Changes panel can update the matching per-repo group.
	RepositoryName string `json:"repository_name,omitempty"`
}

type GitSnapshotData struct {
	ID           string      `json:"id"`
	SessionID    string      `json:"session_id"`
	SnapshotType string      `json:"snapshot_type"`
	Branch       string      `json:"branch"`
	RemoteBranch string      `json:"remote_branch"`
	HeadCommit   string      `json:"head_commit"`
	BaseCommit   string      `json:"base_commit"`
	Ahead        int         `json:"ahead"`
	Behind       int         `json:"behind"`
	Files        interface{} `json:"files,omitempty"`
	TriggeredBy  string      `json:"triggered_by"`
	CreatedAt    string      `json:"created_at"`
}

// FileChangeEventPayload is the payload for file change notifications.
type FileChangeEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Path      string `json:"path"`
	Operation string `json:"operation"`
	Timestamp string `json:"timestamp"`
	// RepositoryName identifies which repo emitted this file change in
	// multi-repo task workspaces. Empty for single-repo. Carried through to
	// the frontend so per-repo views can scope refresh signals correctly.
	RepositoryName string `json:"repository_name,omitempty"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p FileChangeEventPayload) GetSessionID() string {
	return p.SessionID
}

// PermissionOption represents a single permission option in a permission request.
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// PermissionRequestEventPayload is the payload when an agent requests permission.
type PermissionRequestEventPayload struct {
	Type          string                 `json:"type"` // Always "permission_request"
	Timestamp     string                 `json:"timestamp"`
	AgentID       string                 `json:"agent_id"`
	TaskID        string                 `json:"task_id"`
	SessionID     string                 `json:"session_id"`
	RequestID     string                 `json:"request_id"`
	PendingID     string                 `json:"pending_id"`
	ToolCallID    string                 `json:"tool_call_id"`
	Title         string                 `json:"title"`
	Options       []PermissionOption     `json:"options"`
	ActionType    string                 `json:"action_type"`
	ActionDetails map[string]interface{} `json:"action_details,omitempty"`
}

// ShellOutputEventPayload is the payload for shell output events.
type ShellOutputEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Type      string `json:"type"` // Always "output" for shell output events
	Data      string `json:"data"`
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ShellOutputEventPayload) GetSessionID() string {
	return p.SessionID
}

// ShellExitEventPayload is the payload for shell exit events.
type ShellExitEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	AgentID   string `json:"agent_id"`
	Type      string `json:"type"` // Always "exit" for shell exit events
	Code      int    `json:"code"` // Exit code
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ShellExitEventPayload) GetSessionID() string {
	return p.SessionID
}

// ProcessOutputEventPayload is the payload for process output events.
type ProcessOutputEventPayload struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
	ProcessID string `json:"process_id"`
	Kind      string `json:"kind"`
	Stream    string `json:"stream"` // stdout|stderr
	Data      string `json:"data"`
	Timestamp string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ProcessOutputEventPayload) GetSessionID() string {
	return p.SessionID
}

// ProcessStatusEventPayload is the payload for process status events.
type ProcessStatusEventPayload struct {
	SessionID  string `json:"session_id"`
	ProcessID  string `json:"process_id"`
	Kind       string `json:"kind"`
	ScriptName string `json:"script_name,omitempty"`
	Status     string `json:"status"`
	Command    string `json:"command,omitempty"`
	WorkingDir string `json:"working_dir,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
	Timestamp  string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ProcessStatusEventPayload) GetSessionID() string {
	return p.SessionID
}

// ContextWindowEventPayload is the payload for context window update events.
type ContextWindowEventPayload struct {
	TaskID                 string  `json:"task_id"`
	SessionID              string  `json:"session_id"`
	AgentID                string  `json:"agent_id"`
	ContextWindowSize      int64   `json:"context_window_size"`
	ContextWindowUsed      int64   `json:"context_window_used"`
	ContextWindowRemaining int64   `json:"context_window_remaining"`
	ContextEfficiency      float64 `json:"context_efficiency"`
	Timestamp              string  `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p ContextWindowEventPayload) GetSessionID() string {
	return p.SessionID
}

// AvailableCommandsEventPayload is the payload for available commands update events.
type AvailableCommandsEventPayload struct {
	TaskID            string                     `json:"task_id"`
	SessionID         string                     `json:"session_id"`
	AgentID           string                     `json:"agent_id"`
	AvailableCommands []streams.AvailableCommand `json:"available_commands"`
	Timestamp         string                     `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p AvailableCommandsEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionModeEventPayload is the payload for session mode change events.
type SessionModeEventPayload struct {
	TaskID         string                    `json:"task_id"`
	SessionID      string                    `json:"session_id"`
	AgentID        string                    `json:"agent_id"`
	CurrentModeID  string                    `json:"current_mode_id"`
	AvailableModes []streams.SessionModeInfo `json:"available_modes,omitempty"`
	Timestamp      string                    `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p SessionModeEventPayload) GetSessionID() string {
	return p.SessionID
}

// AgentCapabilitiesEventPayload is the payload for agent capabilities events.
type AgentCapabilitiesEventPayload struct {
	TaskID                  string                   `json:"task_id"`
	SessionID               string                   `json:"session_id"`
	AgentID                 string                   `json:"agent_id"`
	SupportsImage           bool                     `json:"supports_image"`
	SupportsAudio           bool                     `json:"supports_audio"`
	SupportsEmbeddedContext bool                     `json:"supports_embedded_context"`
	SupportsPromptQueueing  bool                     `json:"supports_prompt_queueing"`
	AuthMethods             []streams.AuthMethodInfo `json:"auth_methods"`
	Timestamp               string                   `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p AgentCapabilitiesEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionModelsEventPayload is the payload for session models events.
type SessionModelsEventPayload struct {
	TaskID         string                     `json:"task_id"`
	SessionID      string                     `json:"session_id"`
	AgentID        string                     `json:"agent_id"`
	CurrentModelID string                     `json:"current_model_id"`
	Models         []streams.SessionModelInfo `json:"models"`
	ConfigOptions  []streams.ConfigOption     `json:"config_options,omitempty"`
	// ConfigOptionsSettled distinguishes a complete empty provider snapshot
	// from the transient empty state sent before startup settles.
	ConfigOptionsSettled bool `json:"config_options_settled,omitempty"`
	// ConfigBaseline is the persisted ID/value projection used by clients to
	// compare the current ConfigOptions without duplicating provider metadata.
	ConfigBaseline map[string]string `json:"config_baseline,omitempty"`
	Timestamp      string            `json:"timestamp"`
}

// SessionModelFallbackEventPayload is the payload for session fallback-model
// events: the session started on the profile's fallback model because the
// configured start model was unavailable.
type SessionModelFallbackEventPayload struct {
	TaskID        string `json:"task_id"`
	SessionID     string `json:"session_id"`
	AgentID       string `json:"agent_id"`
	FallbackModel string `json:"fallback_model"`
	Timestamp     string `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
// Without it the gateway's extractSessionID cannot route the notification to
// the session's subscribers.
func (p SessionModelFallbackEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionModelSelectionWarningEventPayload is the provider-neutral live
// payload for an executor-authoritative model decision.
type SessionModelSelectionWarningEventPayload struct {
	TaskID    string                        `json:"task_id"`
	SessionID string                        `json:"session_id"`
	AgentID   string                        `json:"agent_id"`
	Warning   streams.ModelSelectionWarning `json:"warning"`
	Timestamp string                        `json:"timestamp"`
}

// GetSessionID returns the task-session ID used by the gateway for routing.
func (p SessionModelSelectionWarningEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionModelsSnapshot is the persisted provider-derived state needed to
// hydrate the task model selector before live session events reconnect.
type SessionModelsSnapshot struct {
	CurrentModelID       string                     `json:"current_model_id"`
	Models               []streams.SessionModelInfo `json:"models"`
	ConfigOptions        []streams.ConfigOption     `json:"config_options,omitempty"`
	ConfigOptionsSettled bool                       `json:"config_options_settled,omitempty"`
}

// LoadSessionModelsSnapshot decodes typed and JSON-rehydrated metadata values.
func LoadSessionModelsSnapshot(raw any) (SessionModelsSnapshot, bool) {
	if raw == nil {
		return SessionModelsSnapshot{}, false
	}
	if snapshot, ok := raw.(SessionModelsSnapshot); ok {
		return snapshot, snapshot.CurrentModelID != "" || len(snapshot.Models) > 0 || len(snapshot.ConfigOptions) > 0 || snapshot.ConfigOptionsSettled
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return SessionModelsSnapshot{}, false
	}
	var snapshot SessionModelsSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return SessionModelsSnapshot{}, false
	}
	return snapshot, snapshot.CurrentModelID != "" || len(snapshot.Models) > 0 || len(snapshot.ConfigOptions) > 0 || snapshot.ConfigOptionsSettled
}

// LoadMCPAttachmentHistory decodes typed and JSON-rehydrated MCP attachment
// metadata. It accepts only the current schema and a current attempt so a
// partial or malformed value cannot look like a healthy report after reload.
func LoadMCPAttachmentHistory(raw any) (streams.MCPAttachmentHistory, bool) {
	if raw == nil {
		return streams.MCPAttachmentHistory{}, false
	}
	if history, ok := raw.(streams.MCPAttachmentHistory); ok {
		return history, history.Valid()
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return streams.MCPAttachmentHistory{}, false
	}
	var history streams.MCPAttachmentHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return streams.MCPAttachmentHistory{}, false
	}
	return history, history.Valid()
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p SessionModelsEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionMCPStatusEventPayload is the persisted, safe MCP status history for
// one task session. It is deliberately session-scoped so parallel agents never
// overwrite each other's diagnostics.
type SessionMCPStatusEventPayload struct {
	TaskID    string                       `json:"task_id"`
	SessionID string                       `json:"session_id"`
	History   streams.MCPAttachmentHistory `json:"history"`
	Timestamp string                       `json:"timestamp"`
}

// GetSessionID returns the task-session routing key.
func (p SessionMCPStatusEventPayload) GetSessionID() string { return p.SessionID }

// SessionInfoEventPayload is the payload for ACP session info updates.
type SessionInfoEventPayload struct {
	TaskID           string         `json:"task_id"`
	SessionID        string         `json:"session_id"`
	AgentID          string         `json:"agent_id"`
	ACPSessionID     string         `json:"acp_session_id,omitempty"`
	SessionTitle     string         `json:"session_title,omitempty"`
	SessionUpdatedAt string         `json:"session_updated_at,omitempty"`
	SessionMeta      map[string]any `json:"session_meta,omitempty"`
	Timestamp        string         `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p SessionInfoEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionTodosEventPayload is the payload for session todos (ACP plan entries) events.
type SessionTodosEventPayload struct {
	TaskID    string              `json:"task_id"`
	SessionID string              `json:"session_id"`
	AgentID   string              `json:"agent_id"`
	Entries   []streams.PlanEntry `json:"entries"`
	Timestamp string              `json:"timestamp"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p SessionTodosEventPayload) GetSessionID() string {
	return p.SessionID
}

// SessionPromptUsageEventPayload is the payload for prompt usage events.
// AgentID is the lifecycle execution.ID (UUID); AgentType is the CLI engine
// slug (claude-acp, codex-acp, ...). The office cost subscriber derives the
// provider name from AgentType — AgentID is kept for legacy consumers.
// AgentProfileID is the stable profile recorded on the task session. It is
// copied at publish time so asynchronous cost consumers do not use a mutable
// workflow runner projection for attribution.
//
// TurnID is captured on the lifecycle completion frame before AgentReady can
// admit a successor prompt and is forwarded by the orchestrator. The
// orchestrator mints UsageEventID once at publish time — rather than per
// consumer — so it remains a stable idempotency key across event redelivery.
// Both are empty when unavailable (e.g. no active turn), never a synthesized
// placeholder.
type SessionPromptUsageEventPayload struct {
	TaskID         string               `json:"task_id"`
	SessionID      string               `json:"session_id"`
	AgentID        string               `json:"agent_id"`
	AgentProfileID string               `json:"agent_profile_id,omitempty"`
	AgentType      string               `json:"agent_type,omitempty"`
	Model          string               `json:"model,omitempty"`
	Usage          *streams.PromptUsage `json:"usage"`
	Timestamp      string               `json:"timestamp"`
	TurnID         string               `json:"turn_id,omitempty"`
	UsageEventID   string               `json:"usage_event_id,omitempty"`
}

// GetSessionID returns the session ID for this event (used by event routing).
func (p SessionPromptUsageEventPayload) GetSessionID() string {
	return p.SessionID
}
