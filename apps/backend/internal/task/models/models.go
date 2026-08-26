package models

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/sysprompt"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ErrExecutorRunningNotFound is returned when no executor running record exists for a session.
var ErrExecutorRunningNotFound = errors.New("executor running not found")

// ErrTaskSessionNotFound is returned when no task session record exists.
var ErrTaskSessionNotFound = errors.New("task session not found")

// ErrTaskWalkthroughNotFound is returned when no walkthrough record exists.
var ErrTaskWalkthroughNotFound = errors.New("task walkthrough not found")

// ErrTaskReviewRunNotFound is returned when no code-review run record exists.
var ErrTaskReviewRunNotFound = errors.New("task review run not found")

// ErrTaskReviewFindingNotFound is returned when no code-review finding record exists.
var ErrTaskReviewFindingNotFound = errors.New("task review finding not found")

// ErrExecutorNotFound is returned by the executor repository when no
// executor row exists for the given ID. Callers should use errors.Is to
// distinguish "row doesn't exist" (404 semantically) from transport-level
// failures (storage outage, timeout, ctx cancel) that happen to also
// surface from the same lookup.
var ErrExecutorNotFound = errors.New("executor not found")

// ErrExecutionRotated is returned by CAS-style updates on executors_running when the
// row's agent_execution_id has rotated since the caller observed it. Indicates the
// caller's write target a now-defunct execution and should be discarded — typically
// a stale resume-token event from an execution that was replaced (model switch,
// context reset, fresh re-launch). Callers should not retry; the new execution
// will produce its own events.
var ErrExecutionRotated = errors.New("execution rotated; CAS write rejected")

// Status values for executors_running.status. The lifecycle manager mirrors
// active execution state into this column; the orchestrator flips a row to
// "prepared" when a prepare-only launch finishes with the agent process
// intentionally not started.
const (
	ExecutorRunningStatusStarting = "starting"
	ExecutorRunningStatusRunning  = "running"
	ExecutorRunningStatusReady    = "ready"
	ExecutorRunningStatusFailed   = "failed"
	ExecutorRunningStatusStopped  = "stopped"
	ExecutorRunningStatusComplete = "completed"
	ExecutorRunningStatusPrepared = "prepared"
	createdAtField                = "created_at"
)

// ListMessagesOptions defines pagination options for listing messages
type ListMessagesOptions struct {
	Limit  int
	Before string
	After  string
	Sort   string
}

// SearchMessagesOptions defines options for searching a session's messages.
type SearchMessagesOptions struct {
	Query string
	Limit int
}

// PluginMessageFilter defines the filters for the plugin Host data API's
// message reader (ADR 0047). SessionIDs and TaskIDs narrow by session/task
// (ORed within each, ANDed across the two — a message must match any
// requested session AND any requested task when both are set); empty means
// unconstrained on that axis. Since/Until bound created_at (Since inclusive,
// Until exclusive); nil means unbounded. Types narrows by message type; empty
// means all types. Results are ordered oldest-first by (created_at, id) with
// SQL Limit/Offset pagination.
type PluginMessageFilter struct {
	SessionIDs []string
	TaskIDs    []string
	Types      []string
	Since      *time.Time
	Until      *time.Time
	Limit      int
	Offset     int
}

// Task metadata keys used for deferred agent start (e.g., task.moved → handleTaskMovedNoSession).
const (
	MetaKeyAgentProfileID    = "agent_profile_id"
	MetaKeyExecutorID        = "executor_id"
	MetaKeyExecutorProfileID = "executor_profile_id"
	// Automation target metadata is written to continuation tasks so a
	// change from hidden to visible ownership or from repository-backed to
	// repository-free execution cannot silently reuse the old task.
	MetaKeyAutomationTaskMode       = "automation_task_mode"
	MetaKeyAutomationRepositoryMode = "automation_repository_mode"
	MetaKeyDeferredLaunch           = "deferred_launch"
	// MetaKeyQueuedMoveExitPending identifies a queued manual move whose
	// source-step on_exit side effect is not yet complete. Its value records the
	// source step so recovery can resume the work after a restart.
	MetaKeyQueuedMoveExitPending = "queued_move_exit_pending"
	// MetaKeyQueuedMoveExitCompleted is durable evidence that the source-step
	// on_exit side effect for a queued manual move finished. Promotion must not
	// enter the destination until this marker exists.
	MetaKeyQueuedMoveExitCompleted = "queued_move_exit_completed"
	// MetaKeyQueuePromotionPending is a one-shot token for destination entry
	// after queue promotion. It prevents duplicate task.queue_promoted events
	// from repeating on_enter or auto-start behavior.
	MetaKeyQueuePromotionPending = "queue_promotion_pending"
	// MetaKeyManualMoveLifecyclePending identifies an admitted manual move whose
	// task.moved lifecycle must finish before feeder reconciliation. Its value
	// records the source step so stale deliveries cannot run the wrong exit.
	MetaKeyManualMoveLifecyclePending = "manual_move_lifecycle_pending"
	// MetaKeyManualMoveLifecycleCompleted records that the admitted manual move
	// lifecycle finished. It remains as an idempotency marker until the next
	// step-changing move replaces it.
	MetaKeyManualMoveLifecycleCompleted = "manual_move_lifecycle_completed"
	// MetaKeyAppliedDeferredMoves stores deferred move IDs that have already
	// been applied, preventing a stale queue rollback from replaying one.
	MetaKeyAppliedDeferredMoves = "applied_deferred_moves"
	// DeferredLaunchStartWhenUnblockedKey marks a deferred launch intent as
	// belonging to a task dependency chain rather than to WIP overflow. The
	// record itself is identical; the flag lets dependency resolution recognise
	// its own intents and keeps a WIP-only intent from being read as a chain
	// step (and vice versa).
	DeferredLaunchStartWhenUnblockedKey = "start_when_unblocked"
	// MetaKeyWorkspacePath is the optional host folder for repo-less tasks
	// (set by CreateTask, read by the orchestrator when building a session).
	// Centralised here so the set/read sites can't drift apart.
	MetaKeyWorkspacePath = "workspace_path"
	// MetaKeyAutoStartGuard is a permanent marker set at create time on tasks
	// where both the watcher's synchronous Path B (autoStartReviewTask) and the
	// promotion's event-driven Path A (autoStartTaskForStep) could both try to
	// launch an agent. Its presence signals that the one-shot MetaKeyAutoStartClaimed
	// token governs who may launch. Never removed after task creation.
	MetaKeyAutoStartGuard = "auto_start_guard"
	// MetaKeyAutoStartClaimed is a one-shot token set alongside MetaKeyAutoStartGuard.
	// Both auto-start paths atomically remove this key; only the first removal
	// succeeds and that path proceeds to StartTask. The other path skips launch
	// because the winner will (or already did) handle it.
	// Absent on ordinary (non-watcher) auto-start tasks, which launch normally.
	MetaKeyAutoStartClaimed = "auto_start_claimed"
	// MetaKeyInterruptedAt is set by startup reconciliation when a task's
	// session was mid-turn (STARTING/RUNNING) when the backend died. Its
	// presence makes the task DTO report `interrupted: true` so task-list
	// surfaces show the red interruption icon; the orchestrator removes the
	// key when a session of the task next enters STARTING/RUNNING.
	MetaKeyInterruptedAt = "interrupted_at"
	// MetaKeyAutoStartFailed is set when a workflow step's auto_start_agent
	// on_enter action fails to launch a run (kanban StartTask error, or an
	// Office task queue-run error/unresolvable agent). Its presence makes the
	// task DTO report `auto_start_failed: true` so the failure surfaces on
	// the card instead of only in backend logs; the orchestrator removes the
	// key when a session of the task next enters STARTING/RUNNING (mirrors
	// MetaKeyInterruptedAt).
	MetaKeyAutoStartFailed = "auto_start_failed"
	// MetaKeyAgentTitlePending marks tasks created in prompt-first mode whose
	// provisional title still needs the first eligible agent session to replace it.
	MetaKeyAgentTitlePending = "agent_title_pending"
	// MetaKeyAgentTitleOwnerSessionID records the one session that atomically
	// claimed the first-turn title handoff for a pending task.
	MetaKeyAgentTitleOwnerSessionID = "agent_title_owner_session_id"
	// MetaKeyPortForwardingEnabled controls whether a task exposes its
	// session port-forwarding controls in the task UI.
	MetaKeyPortForwardingEnabled = "port_forwarding_enabled"
	// MetaKeyAutomationTargetTaskID binds a merged-PR automation run to the
	// task selected by its event. The archive handler enforces this value.
	MetaKeyAutomationTargetTaskID = "automation_target_task_id"
	// Parent-question message metadata is durable state for an autopilot child
	// waiting for a decision from its direct parent. The question ID is also
	// the child clarification message ID, so a parent reply can be idempotent
	// without a second persistence table.
	MetaKeyParentQuestionID       = "parent_question_id"
	MetaKeyParentQuestion         = "parent_question"
	MetaKeyParentQuestionParentID = "parent_task_id"
	MetaKeyParentQuestionChildID  = "child_task_id"
	MetaKeyParentQuestionStatus   = "parent_question_status"
	MetaKeyParentQuestionResponse = "parent_question_response"
	// MetaKeyInitialSessionRuntimeConfig is a launch-only seed for the first
	// session created for a task. PrepareSession consumes it into session
	// runtime overrides and never leaves it in session metadata.
	MetaKeyInitialSessionRuntimeConfig = "initial_session_runtime_config"
	// MetaKeyInitialSessionRuntimeConfigProfileID identifies the agent profile
	// that produced the launch-only runtime seed. A seed is valid only for this
	// profile, even if task profile selection changes before the first launch.
	MetaKeyInitialSessionRuntimeConfigProfileID = "initial_session_runtime_config_profile_id"
)

// IsAgentTitlePending reports whether task metadata contains the durable
// prompt-first title marker. JSON rehydration produces bool values, while a
// few in-process callers may provide typed metadata, so only an explicit true
// value enables the capability.
func IsAgentTitlePending(metadata map[string]interface{}) bool {
	pending, ok := metadata[MetaKeyAgentTitlePending].(bool)
	return ok && pending
}

// AgentTitleOwnerSessionID returns the session that owns the pending title
// handoff, if one has been claimed.
func AgentTitleOwnerSessionID(metadata map[string]interface{}) string {
	owner, _ := metadata[MetaKeyAgentTitleOwnerSessionID].(string)
	return owner
}

// IsAgentTitleOwner reports whether sessionID owns the pending title handoff.
func IsAgentTitleOwner(metadata map[string]interface{}, sessionID string) bool {
	return sessionID != "" && IsAgentTitlePending(metadata) && AgentTitleOwnerSessionID(metadata) == sessionID
}

// TaskSession.Metadata key that records how the session came into existence.
// workflow_switch means the session profile was selected by workflow routing
// rather than direct user selection.
const (
	SessionMetaKeyCreatedBy        = "created_by"
	SessionCreatedByWorkflowSwitch = "workflow_switch"
	// SessionMetaKeyOrigin identifies immutable task-session provenance. Unlike
	// IsPrimary, it never changes when the user selects another conversation tab.
	SessionMetaKeyOrigin                 = "origin"
	SessionOriginTaskInitial             = "task_initial"
	SessionMetaKeyContextWindow          = "context_window"
	SessionMetaKeyContextCompactionCount = "context_compaction_count"
	// SessionMetaKeyRecoveryResolvedAt stores the server timestamp of the
	// latest successful agent boot. Recovery cards compare this timestamp with
	// their own creation time, so the result survives transcript write failures.
	SessionMetaKeyRecoveryResolvedAt = "recovery_resolved_at"
)

// SessionMetaKeySessionMode records the agent's last-known session permission
// mode (auto / default / accept-edits, etc.) so it survives a backend restart or
// SSR reload, alongside the in-memory re-apply on context reset. See issue #1183.
const SessionMetaKeySessionMode = "session_mode"

// SessionMetaKeyRuntimeConfig records the provider's latest session runtime
// state (model, mode, and dynamic config options) separately from the profile
// snapshot that seeded the session.
const SessionMetaKeyRuntimeConfig = "runtime_config"

// SessionMetaKeyRuntimeConfigOverrides records explicit user selections
// separately from provider snapshots so delayed events cannot clobber resume
// intent. Overrides are applied after SessionMetaKeyRuntimeConfig.
const SessionMetaKeyRuntimeConfigOverrides = "runtime_config_overrides"

// SessionMetaKeyOriginalEffectiveConfig stores the write-once effective model
// and selectable ACP option values after profile settings settle. It is the
// restore source for workflow rules and is intentionally separate from the
// provider-default comparison baseline and mutable runtime state.
const SessionMetaKeyOriginalEffectiveConfig = "original_effective_config"

// SessionMetaKeyACPConfigBaseline records the write-once effective ACP select
// values with which a task session started. It is comparison metadata only;
// runtime restoration continues to use SessionMetaKeyRuntimeConfig.
const SessionMetaKeyACPConfigBaseline = "acp_config_baseline"

// SessionMetaKeyACPModelState records the provider's latest complete model
// selector state so task-detail boot hydration does not wait for WebSocket
// reconnection. It is display metadata and is not replayed to the provider.
const SessionMetaKeyACPModelState = "acp_model_state"

// SessionMetaKeyMCPAttachmentState records bounded, safe evidence about MCP
// attachment for this task session and its immediate prior attempts.
const SessionMetaKeyMCPAttachmentState = "mcp_attachment_state"

// SessionMetaKeyGitCredentialSnapshot records the non-secret Git credential
// routing contract that successfully launched or resumed a session.
const SessionMetaKeyGitCredentialSnapshot = "git_credential_snapshot"

// GitCredentialSnapshot is launch-time display metadata. It never contains a
// token, broker lease, helper command, credential file, or SSH key detail.
type GitCredentialSnapshot struct {
	Version         int       `json:"version"`
	Policy          string    `json:"policy"`
	Source          string    `json:"source"`
	WorkspaceMethod string    `json:"workspace_method,omitempty"`
	Actor           string    `json:"actor"`
	Transport       string    `json:"transport"`
	ExecutorType    string    `json:"executor_type,omitempty"`
	CapturedAt      time.Time `json:"captured_at"`
}

// TurnMetaKeyRuntimeConfigSnapshot stores the immutable effective runtime
// configuration attributed to one prompt/response turn.
const TurnMetaKeyRuntimeConfigSnapshot = "runtime_config_snapshot"

// TurnMetaKeyWorkflowStepIDAtStart records the workflow step the turn's task
// was in when the turn started. Absent when the task held no step.
const TurnMetaKeyWorkflowStepIDAtStart = "workflow_step_id_at_start"

// TurnMetaKeyPromptDispatchPending marks a successor created before agentctl
// acknowledges its prompt. Empty marked turns are not current-turn authority
// unless dispatch ambiguity was recorded; publication clears the marker, while
// a referencing message also proves ambiguous acceptance after a crash.
const TurnMetaKeyPromptDispatchPending = "prompt_dispatch_pending"

const (
	TurnMetaKeyPromptDispatchAttempted               = "prompt_dispatch_attempted"
	TurnMetaKeyPromptDispatchClarificationPendingID  = "prompt_dispatch_clarification_pending_id"
	TurnMetaKeyPromptDispatchClarificationTurnID     = "prompt_dispatch_clarification_turn_id"
	TurnMetaKeyPromptDispatchClarificationMessageIDs = "prompt_dispatch_clarification_message_ids"
	// TurnMetaKeyPromptDispatchStartEventPending is a durable outbox marker.
	// Startup recovery sets it after accepting an ambiguous reservation and
	// clears it only after the task service replays the public turn events.
	TurnMetaKeyPromptDispatchStartEventPending = "prompt_dispatch_start_event_pending"
)

var promptDispatchMetadataKeys = [...]string{
	TurnMetaKeyPromptDispatchPending,
	TurnMetaKeyPromptDispatchAttempted,
	TurnMetaKeyPromptDispatchClarificationPendingID,
	TurnMetaKeyPromptDispatchClarificationTurnID,
	TurnMetaKeyPromptDispatchClarificationMessageIDs,
	TurnMetaKeyPromptDispatchStartEventPending,
}

// ClearPromptDispatchMetadata removes every reservation-only field after live
// publication or successful startup event replay.
func ClearPromptDispatchMetadata(metadata map[string]interface{}) {
	for _, key := range promptDispatchMetadataKeys {
		delete(metadata, key)
	}
}

// PromptDispatchRecovery identifies the exact clarification claim that an
// unpublished successor reservation must restore after a backend restart.
type PromptDispatchRecovery struct {
	PendingID  string
	TurnID     string
	MessageIDs []string
}

// SessionRuntimeConfig is persisted as provider state or explicit overrides.
// On resume, explicit values take precedence over the latest provider snapshot
// so delayed provider events cannot replace user intent.
type SessionRuntimeConfig struct {
	Model         string            `json:"model,omitempty"`
	Mode          string            `json:"mode,omitempty"`
	ConfigOptions map[string]string `json:"config_options,omitempty"`
}

// LoadInitialSessionRuntimeConfig decodes the task launch seed from typed or
// JSON-rehydrated task metadata.
func LoadInitialSessionRuntimeConfig(metadata map[string]interface{}) (SessionRuntimeConfig, bool) {
	return loadSessionRuntimeConfig(metadata, MetaKeyInitialSessionRuntimeConfig)
}

// LoadInitialSessionRuntimeConfigProfileID returns the profile that owns the
// launch-only runtime seed. Older tasks did not store a separate owner, so
// their resolved task profile remains a compatibility fallback.
func LoadInitialSessionRuntimeConfigProfileID(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	if profileID := StringFromAny(metadata[MetaKeyInitialSessionRuntimeConfigProfileID]); profileID != "" {
		return profileID
	}
	return StringFromAny(metadata[MetaKeyAgentProfileID])
}

// LoadEffectiveSessionRuntimeConfig resolves the effective model, mode, and
// dynamic options for a task session. The profile snapshot is the base, the
// provider runtime state replaces it, the persisted session mode takes
// precedence over provider mode, and explicit runtime overrides win last.
func LoadEffectiveSessionRuntimeConfig(session *TaskSession) (SessionRuntimeConfig, bool) {
	if session == nil {
		return SessionRuntimeConfig{}, false
	}
	effective := runtimeConfigFromAgentProfileSnapshot(session.AgentProfileSnapshot)
	if runtime, ok := LoadSessionRuntimeConfig(session.Metadata); ok {
		mergeSessionRuntimeConfig(&effective, runtime)
		if runtime.ConfigOptions != nil {
			// Provider runtime options are a complete replacement for profile
			// options. Explicit overrides below are the only later merge.
			effective.ConfigOptions = maps.Clone(runtime.ConfigOptions)
		}
	}
	if mode := StringFromAny(session.Metadata[SessionMetaKeySessionMode]); mode != "" {
		effective.Mode = mode
	}
	if overrides, ok := LoadSessionRuntimeConfigOverrides(session.Metadata); ok {
		mergeSessionRuntimeConfig(&effective, overrides)
	}
	effective.ConfigOptions = cleanRuntimeConfigOptions(effective.ConfigOptions, session)
	return effective, !effective.IsZero()
}

func runtimeConfigFromAgentProfileSnapshot(snapshot map[string]interface{}) SessionRuntimeConfig {
	if snapshot == nil {
		return SessionRuntimeConfig{}
	}
	config := SessionRuntimeConfig{
		Model: StringFromAny(snapshot["model"]),
		Mode:  StringFromAny(snapshot["mode"]),
	}
	config.ConfigOptions = stringMapFromAny(snapshot["config_options"])
	if config.ConfigOptions == nil {
		config.ConfigOptions = stringMapFromAny(snapshot["configOptions"])
	}
	return config
}

func mergeSessionRuntimeConfig(target *SessionRuntimeConfig, source SessionRuntimeConfig) {
	if source.Model != "" {
		target.Model = source.Model
	}
	if source.Mode != "" {
		target.Mode = source.Mode
	}
	if len(source.ConfigOptions) == 0 {
		return
	}
	if target.ConfigOptions == nil {
		target.ConfigOptions = make(map[string]string, len(source.ConfigOptions))
	}
	for key, value := range source.ConfigOptions {
		target.ConfigOptions[key] = value
	}
}

func cleanRuntimeConfigOptions(options map[string]string, session *TaskSession) map[string]string {
	if len(options) == 0 {
		return nil
	}
	cleaned := maps.Clone(options)
	// Model and mode are carried as top-level fields. Other keys are provider-defined.
	delete(cleaned, "model")
	delete(cleaned, "mode")
	if isLegacyAgentIdentity(session, cleaned["agent"]) {
		delete(cleaned, "agent")
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func isLegacyAgentIdentity(session *TaskSession, value string) bool {
	if session == nil || value == "" || session.AgentProfileSnapshot == nil {
		return false
	}
	for _, key := range []string{"agent_id", "agent_name"} {
		if StringFromAny(session.AgentProfileSnapshot[key]) == value {
			return true
		}
	}
	return false
}

// SessionOriginalEffectiveConfiguration is the immutable configuration a task
// session had after provider defaults and its profile settings settled.
type SessionOriginalEffectiveConfiguration struct {
	Model         string            `json:"model,omitempty"`
	ConfigOptions map[string]string `json:"config_options,omitempty"`
}

// LoadOriginalSessionEffectiveConfiguration decodes the original configuration
// from typed or JSON-rehydrated session metadata.
func LoadOriginalSessionEffectiveConfiguration(metadata map[string]interface{}) (SessionOriginalEffectiveConfiguration, bool) {
	if metadata == nil {
		return SessionOriginalEffectiveConfiguration{}, false
	}
	raw, ok := metadata[SessionMetaKeyOriginalEffectiveConfig]
	if !ok || raw == nil {
		return SessionOriginalEffectiveConfiguration{}, false
	}
	if config, ok := raw.(SessionOriginalEffectiveConfiguration); ok {
		config.ConfigOptions = maps.Clone(config.ConfigOptions)
		return config, config.Model != "" || len(config.ConfigOptions) > 0
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return SessionOriginalEffectiveConfiguration{}, false
	}
	var config SessionOriginalEffectiveConfiguration
	if err := json.Unmarshal(data, &config); err != nil {
		return SessionOriginalEffectiveConfiguration{}, false
	}
	return config, config.Model != "" || len(config.ConfigOptions) > 0
}

// IsOriginalTaskSession reports whether immutable task-session provenance marks
// this row as the session created when the task first started.
func IsOriginalTaskSession(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	return StringFromAny(metadata[SessionMetaKeyOrigin]) == SessionOriginTaskInitial
}

// TurnRuntimeConfigSnapshot is the minimal display state captured when a turn
// starts. It intentionally excludes the provider's complete option catalog.
type TurnRuntimeConfigSnapshot struct {
	Model          string                    `json:"model,omitempty"`
	Mode           string                    `json:"mode,omitempty"`
	ConfigOptions  []TurnRuntimeConfigOption `json:"config_options,omitempty"`
	ConfigBaseline map[string]string         `json:"config_baseline,omitempty"`
}

// TurnRuntimeConfigOption preserves one selected value in provider order.
type TurnRuntimeConfigOption struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Value     string `json:"value"`
	ValueName string `json:"value_name,omitempty"`
}

// LoadTurnRuntimeConfigSnapshot decodes typed and JSON-rehydrated turn metadata.
func LoadTurnRuntimeConfigSnapshot(metadata map[string]interface{}) (TurnRuntimeConfigSnapshot, bool) {
	if metadata == nil {
		return TurnRuntimeConfigSnapshot{}, false
	}
	raw := metadata[TurnMetaKeyRuntimeConfigSnapshot]
	if snapshot, ok := raw.(TurnRuntimeConfigSnapshot); ok {
		return snapshot, snapshot.Model != "" || snapshot.Mode != "" || len(snapshot.ConfigOptions) > 0
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return TurnRuntimeConfigSnapshot{}, false
	}
	var snapshot TurnRuntimeConfigSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return TurnRuntimeConfigSnapshot{}, false
	}
	return snapshot, snapshot.Model != "" || snapshot.Mode != "" || len(snapshot.ConfigOptions) > 0
}

// SessionMetaKeyPendingStepCompletion stores the agent's (or manual fallback's)
// step-complete signal under TaskSession.Metadata. ADR 0015: the orchestrator
// reads this on turn-end for steps with AutoAdvanceRequiresSignal=true and
// only fires on_turn_complete transitions when a matching signal is present.
// Cleared on successful transition, on user reply before transition, or any
// other step change.
const SessionMetaKeyPendingStepCompletion = "pending_step_completion_signal"

// SessionMetaKeyLastAgentError stores the last recoverable agent runtime
// failure for UI surfaces that need to keep the error visible after auto-resume.
const SessionMetaKeyLastAgentError = "last_agent_error"

// LastAgentError is persisted under TaskSession.Metadata[SessionMetaKeyLastAgentError].
// RemediationURL is only ever set from an adapter-validated provider
// diagnostic; it is never reconstructed from the error message.
type LastAgentError struct {
	Message          string     `json:"message"`
	OccurredAt       time.Time  `json:"occurred_at"`
	AgentExecutionID string     `json:"agent_execution_id,omitempty"`
	RemediationURL   string     `json:"remediation_url,omitempty"`
	Code             string     `json:"code,omitempty"`
	Details          string     `json:"details,omitempty"`
	RecoveryActions  []string   `json:"recovery_actions,omitempty"`
	TaskRepositoryID string     `json:"task_repository_id,omitempty"`
	StampValue       string     `json:"stamp,omitempty"`
	DismissedAt      *time.Time `json:"dismissed_at,omitempty"`
}

func LoadLastAgentError(metadata map[string]interface{}) (LastAgentError, bool) {
	if metadata == nil {
		return LastAgentError{}, false
	}
	raw, ok := metadata[SessionMetaKeyLastAgentError]
	if !ok || raw == nil {
		return LastAgentError{}, false
	}
	var out LastAgentError
	if err := mapToLastAgentError(raw, &out); err != nil || out.Message == "" {
		return LastAgentError{}, false
	}
	return normalizeLastAgentError(out), true
}

func mapToLastAgentError(raw interface{}, out *LastAgentError) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func (e LastAgentError) Stamp() string {
	if stamp := boundedLaunchErrorStamp(e.StampValue); stamp != "" {
		return stamp
	}
	return e.OccurredAt.UTC().Format(time.RFC3339Nano) + ":" + e.Message
}

func (e LastAgentError) MatchesStamp(stamp string) bool {
	if stamp == e.Stamp() {
		return true
	}
	if boundedLaunchErrorStamp(e.StampValue) != "" {
		return false
	}
	suffix := ":" + e.Message
	if !strings.HasSuffix(stamp, suffix) {
		return false
	}
	rawOccurredAt := strings.TrimSuffix(stamp, suffix)
	if rawOccurredAt == "" {
		return e.OccurredAt.IsZero()
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, rawOccurredAt)
	if err != nil {
		return false
	}
	return occurredAt.Equal(e.OccurredAt)
}

func (e LastAgentError) IsDismissed() bool {
	return e.DismissedAt != nil && !e.DismissedAt.IsZero()
}

// PendingStepCompletionSignal source values.
const (
	StepCompletionSourceAgent          = "agent"
	StepCompletionSourceManualFallback = "manual_fallback"
)

// PendingStepCompletionSignal is the JSON shape persisted under
// TaskSession.Metadata[SessionMetaKeyPendingStepCompletion]. It records an
// agent-emitted (or user-emitted via the fallback button) completion signal
// that the orchestrator should consume to drive a workflow step transition.
// See ADR 0015 for the lifecycle (set → read → clear).
type PendingStepCompletionSignal struct {
	StepID     string    `json:"step_id"`
	Source     string    `json:"source"`
	Summary    string    `json:"summary"`
	Handoff    string    `json:"handoff,omitempty"`
	Blockers   string    `json:"blockers,omitempty"`
	SignaledAt time.Time `json:"signaled_at"`
}

// LoadSessionRuntimeConfig decodes the runtime-config bag entry from session
// metadata. It tolerates both typed values and JSON-rehydrated maps.
func LoadSessionRuntimeConfig(metadata map[string]interface{}) (SessionRuntimeConfig, bool) {
	return loadSessionRuntimeConfig(metadata, SessionMetaKeyRuntimeConfig)
}

// LoadSessionRuntimeConfigOverrides decodes explicit user runtime selections.
func LoadSessionRuntimeConfigOverrides(metadata map[string]interface{}) (SessionRuntimeConfig, bool) {
	return loadSessionRuntimeConfig(metadata, SessionMetaKeyRuntimeConfigOverrides)
}

func loadSessionRuntimeConfig(metadata map[string]interface{}, key string) (SessionRuntimeConfig, bool) {
	if metadata == nil {
		return SessionRuntimeConfig{}, false
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return SessionRuntimeConfig{}, false
	}
	switch v := raw.(type) {
	case SessionRuntimeConfig:
		v.ConfigOptions = maps.Clone(v.ConfigOptions)
		return v, !v.IsZero()
	case map[string]string:
		out := SessionRuntimeConfig{
			Model:         v["model"],
			Mode:          v["mode"],
			ConfigOptions: maps.Clone(v),
		}
		delete(out.ConfigOptions, "model")
		delete(out.ConfigOptions, "mode")
		if len(out.ConfigOptions) == 0 {
			out.ConfigOptions = nil
		}
		return out, !out.IsZero()
	case map[string]interface{}:
		out := SessionRuntimeConfig{
			Model: StringFromAny(v["model"]),
			Mode:  StringFromAny(v["mode"]),
		}
		if opts := stringMapFromAny(v["config_options"]); len(opts) > 0 {
			out.ConfigOptions = opts
		}
		return out, !out.IsZero()
	default:
		return SessionRuntimeConfig{}, false
	}
}

// LoadSessionACPConfigBaseline decodes the write-once ACP configuration
// baseline from session metadata. It tolerates typed and JSON-rehydrated maps.
func LoadSessionACPConfigBaseline(metadata map[string]interface{}) (map[string]string, bool) {
	if metadata == nil {
		return nil, false
	}
	values := stringMapFromAnyPreservingEmpty(metadata[SessionMetaKeyACPConfigBaseline])
	if len(values) == 0 {
		return nil, false
	}
	return values, true
}

func stringMapFromAnyPreservingEmpty(raw interface{}) map[string]string {
	switch values := raw.(type) {
	case map[string]string:
		return maps.Clone(values)
	case map[string]interface{}:
		out := make(map[string]string, len(values))
		for key, value := range values {
			if str, ok := value.(string); ok {
				out[key] = str
			}
		}
		return out
	default:
		return nil
	}
}

// IsZero reports whether the runtime config carries any selected value.
func (c SessionRuntimeConfig) IsZero() bool {
	return c.Model == "" && c.Mode == "" && len(c.ConfigOptions) == 0
}

func stringMapFromAny(raw interface{}) map[string]string {
	switch v := raw.(type) {
	case map[string]string:
		return maps.Clone(v)
	case map[string]interface{}:
		out := make(map[string]string, len(v))
		for key, value := range v {
			if str := StringFromAny(value); str != "" {
				out[key] = str
			}
		}
		return out
	default:
		return nil
	}
}

// LoadPendingStepSignal decodes the pending-completion bag entry from a
// session's metadata. Single source of truth shared by the MCP handler
// (write site, idempotency check) and the orchestrator (read site, gating).
// Survives both the in-process typed shape and the JSON-rehydrated
// `map[string]interface{}` shape produced when the row is loaded fresh from
// SQLite after a backend restart.
func LoadPendingStepSignal(metadata map[string]interface{}) (PendingStepCompletionSignal, bool) {
	if metadata == nil {
		return PendingStepCompletionSignal{}, false
	}
	raw, ok := metadata[SessionMetaKeyPendingStepCompletion]
	if !ok || raw == nil {
		return PendingStepCompletionSignal{}, false
	}
	switch v := raw.(type) {
	case PendingStepCompletionSignal:
		return v, true
	case map[string]interface{}:
		out := PendingStepCompletionSignal{
			StepID:   StringFromAny(v["step_id"]),
			Source:   StringFromAny(v["source"]),
			Summary:  StringFromAny(v["summary"]),
			Handoff:  StringFromAny(v["handoff"]),
			Blockers: StringFromAny(v["blockers"]),
		}
		if ts, ok := v["signaled_at"].(string); ok {
			if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
				out.SignaledAt = parsed
			}
		}
		return out, out.StepID != ""
	}
	return PendingStepCompletionSignal{}, false
}

// StringFromAny narrows an interface{} slot to a string, returning "" when
// the value is absent or of a different type. Used by both the
// PendingStepCompletionSignal map-shape decoder and the orchestrator's
// step-completion event-payload parser — single source of truth so the
// two decoders can't drift on what counts as a "missing string".
func StringFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// Task origin values for the Origin field.
const (
	TaskOriginManual        = "manual"
	TaskOriginAgentCreated  = "agent_created"
	TaskOriginRoutine       = "routine"
	TaskOriginOnboarding    = "onboarding"
	TaskOriginAutomationRun = "automation_run"
	// TaskOriginAutomationTask is a normal, user-visible task created by an
	// automation. Unlike automation_run, it remains in Kanban/sidebar flows.
	TaskOriginAutomationTask = "automation_task"
)

// Task represents a task in the database
type Task struct {
	ID             string       `json:"id"`
	WorkspaceID    string       `json:"workspace_id"`
	WorkflowID     string       `json:"workflow_id"`
	WorkflowStepID string       `json:"workflow_step_id"`
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	State          v1.TaskState `json:"state"`
	Priority       string       `json:"priority"`
	Position       int          `json:"position"` // Order within workflow step
	// WIPAdmitted indicates whether this task consumes an active slot in its
	// current workflow step. Queued tasks remain visible but do not consume the
	// destination step's WIP capacity.
	WIPAdmitted      bool                   `json:"wip_admitted"`
	QueuedForStepID  string                 `json:"queued_for_step_id,omitempty"`
	QueuedAt         *time.Time             `json:"queued_at,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
	Repositories     []*TaskRepository      `json:"repositories,omitempty"`
	WorkspaceFolders []*TaskWorkspaceFolder `json:"workspace_folders,omitempty"`
	IsEphemeral      bool                   `json:"is_ephemeral"`        // Ephemeral tasks are not shown in kanban, used for quick chat
	ParentID         string                 `json:"parent_id,omitempty"` // FK to parent task for subtasks
	Autopilot        bool                   `json:"autopilot"`           // Fixed at task creation
	ArchivedAt       *time.Time             `json:"archived_at,omitempty"`
	// ArchivedByCascadeID is set when the task was archived as part of an
	// office task-handoffs cascade (phase 6). UnarchiveTaskByCascade uses
	// it to scope restoration to exactly the descendants this cascade
	// owned, leaving manually-archived tasks alone. Empty for
	// manually-archived tasks and any task archived before phase 6.
	ArchivedByCascadeID string    `json:"archived_by_cascade_id,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
	// WorkflowStepTransitionID is the immutable ledger row identifier for the
	// latest workflow-step write. Repositories populate it after a transition;
	// it is transient and is not stored in the tasks table.
	WorkflowStepTransitionID int64 `json:"-"`

	// Office extensions.
	//
	// ADR 0005 Wave F: the task's assignee is no longer a column on the
	// tasks table. The canonical store is a `runner` row in
	// workflow_step_participants (resolve via
	// workflow.Repository.ResolveCurrentRunner). The struct field below
	// is a read-time projection populated by repo SELECT queries through
	// a correlated subquery and consumed unchanged by callers that
	// previously read tasks.assignee_agent_profile_id. Repo writes to
	// this field are routed to SetTaskRunner / ClearTaskRunner inside
	// the task repository.
	AssigneeAgentProfileID string `json:"assignee_agent_profile_id,omitempty"`
	Origin                 string `json:"origin,omitempty"`     // manual, agent_created, routine
	ProjectID              string `json:"project_id,omitempty"` // FK to office project
	Labels                 string `json:"labels,omitempty"`     // JSON array string, default "[]"
	Identifier             string `json:"identifier,omitempty"` // e.g. "KAN-42"

	// ExternalID is a caller-supplied identity used for create-idempotency
	// (docs/specs/tasks/requirements/external-id-idempotency.md). Empty when the task
	// holds none. Unique per (workspace_id, external_id) when non-empty.
	ExternalID string `json:"external_id,omitempty"`
	// ExternalIDSettledAt is non-nil once the create that claimed ExternalID
	// finished its required synchronous work. Nil means unsettled — the
	// claiming create may still be in progress. Both fields are cleared
	// together on release.
	ExternalIDSettledAt *time.Time `json:"external_id_settled_at,omitempty"`

	// IsFromOffice is a read-time projection set by the task repo's SELECT
	// (see isFromOfficeProjection in repository/sqlite/task.go). True when
	// the task is owned by office: either it has a non-empty ProjectID, or
	// its workflow matches the workspace's office_workflow_id. Kanban-board
	// tasks always come back false. UI callers gate office-only surfaces on
	// this (e.g. the "Open in office view" topbar link).
	IsFromOffice bool `json:"is_from_office,omitempty"`
}

// IsOfficeOwnedAndAssigned reports whether runtime behavior belongs to an
// Office-owned task with a designated runner. Kanban tasks may also project a
// runner from their workflow step, but retain normal per-session semantics.
func (t *Task) IsOfficeOwnedAndAssigned() bool {
	return t != nil && t.IsFromOffice && t.AssigneeAgentProfileID != ""
}

// ChildCompletionRow is the compact active-child projection used to decide
// whether a parent task's on_children_completed trigger is ready to fire.
type ChildCompletionRow struct {
	ID                   string       `json:"id" db:"id"`
	State                v1.TaskState `json:"state" db:"state"`
	Title                string       `json:"title" db:"title"`
	WorkflowStepID       string       `json:"workflow_step_id" db:"workflow_step_id"`
	TerminalWorkflowStep bool         `json:"terminal_workflow_step"` // computed by annotateTerminalChildSteps, not a DB column
	UpdatedAt            time.Time    `json:"updated_at" db:"updated_at"`
}

// HasStartWhenUnblockedIntent reports whether a task's deferred launch intent
// belongs to a dependency chain rather than to WIP overflow.
//
// The two share one record; DeferredLaunchStartWhenUnblockedKey is what tells
// them apart. Defined here, next to the key, so the DTO layer, the task service
// and the orchestrator cannot drift on what counts as a chain intent.
func HasStartWhenUnblockedIntent(task *Task) bool {
	if task == nil || task.Metadata == nil {
		return false
	}
	raw, ok := task.Metadata[MetaKeyDeferredLaunch]
	if !ok {
		return false
	}
	launch, ok := raw.(map[string]interface{})
	if !ok {
		return false
	}
	flag, ok := launch[DeferredLaunchStartWhenUnblockedKey].(bool)
	return ok && flag
}

// DropWIPDeferredLaunch removes a deferred launch record that only existed
// because the task was waiting on WIP capacity.
//
// Every WIP admission point calls this instead of deleting the key outright.
// An admitted task has nothing left to wait for as far as WIP is concerned, so
// its overflow record is stale. A start-when-unblocked intent is NOT stale in
// that situation: it is waiting on a predecessor, not on capacity, and the two
// share this one metadata record. Deleting it on admission silently unmakes
// every dependency chain, because admission happens at create time for any task
// entering a step with room.
func DropWIPDeferredLaunch(task *Task) {
	if task == nil || task.Metadata == nil || HasStartWhenUnblockedIntent(task) {
		return
	}
	delete(task.Metadata, MetaKeyDeferredLaunch)
}

// IsTerminalTaskState reports whether a task state means no further child work
// is expected from that task.
func IsTerminalTaskState(state v1.TaskState) bool {
	switch state {
	case v1.TaskStateCompleted, v1.TaskStateFailed, v1.TaskStateCancelled:
		return true
	default:
		return false
	}
}

// TaskTreeFilters provides filter options for the task tree query.
type TaskTreeFilters struct {
	ProjectID  string
	AssigneeID string
	WorkflowID string
	Origin     string
}

// WorkflowStyle values are persisted in workflows.style. See ADR-0004.
// They're a UX hint for the frontend; backend code MUST NOT branch on them.
const (
	WorkflowStyleKanban = "kanban"
	WorkflowStyleOffice = "office"
	WorkflowStyleCustom = "custom"
)

// WorkflowSource values are persisted in workflows.source and record where a
// workflow definition came from. Manual workflows are user-managed; GitHub
// workflows are owned by the workflow-sync poller and may be overwritten or
// removed on each sync.
const (
	WorkflowSourceManual = "manual"
	WorkflowSourceGitHub = "github"
)

// Workflow represents a task workflow
type Workflow struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Prompt is optional workflow-level agent instructions prepended at
	// every step entry before the step prompt. Empty means off.
	Prompt             string  `json:"prompt,omitempty"`
	AgentProfileID     string  `json:"agent_profile_id,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
	SortOrder          int     `json:"sort_order"`
	// Source records where the workflow definition came from ("manual" or
	// "github"). SourcePath is the repo-relative file path the definition
	// was synced from; empty for manual workflows.
	Source     string `json:"source,omitempty"`
	SourcePath string `json:"source_path,omitempty"`
	// Hidden workflows are excluded from management and picker UIs by default.
	// Used by system-only flows like Improve Kandev.
	Hidden bool `json:"hidden,omitempty"`
	// Style is a Phase 2 (ADR-0004) UX hint read by the frontend ONLY.
	// Allowed values: "kanban" | "office" | "custom". Empty / unknown
	// values fall back to "kanban" via the schema default. Backend code
	// MUST NOT branch on this field — it is a presentation hint, not a
	// behavioural invariant.
	Style     string    `json:"style,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkspaceNameImproveKandev is the exact name of the dedicated Improve Kandev
// workspace created by the improve-kandev bootstrap. The workspace is matched
// by this name everywhere (bootstrap, guards, frontend) and is
// configuration-immutable: its workflows and repositories are read-only.
const WorkspaceNameImproveKandev = "Improve Kandev"

// IsImproveKandev reports whether the workspace is the dedicated Improve
// Kandev workspace (matched by exact name).
func (w *Workspace) IsImproveKandev() bool {
	return w != nil && w.Name == WorkspaceNameImproveKandev
}

// Workspace represents a workspace
type Workspace struct {
	ID                          string    `json:"id"`
	Name                        string    `json:"name"`
	Description                 string    `json:"description"`
	OwnerID                     string    `json:"owner_id"`
	DefaultExecutorID           *string   `json:"default_executor_id,omitempty"`
	DefaultEnvironmentID        *string   `json:"default_environment_id,omitempty"`
	DefaultAgentProfileID       *string   `json:"default_agent_profile_id,omitempty"`
	DefaultConfigAgentProfileID *string   `json:"default_config_agent_profile_id,omitempty"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`

	// Office extensions
	TaskPrefix       string `json:"task_prefix,omitempty"`        // e.g. "KAN"
	TaskSequence     int    `json:"task_sequence,omitempty"`      // auto-incrementing counter
	OfficeWorkflowID string `json:"office_workflow_id,omitempty"` // FK to system office workflow
}

// TaskRepository represents a repository associated with a task
type TaskRepository struct {
	ID                            string                 `json:"id"`
	TaskID                        string                 `json:"task_id"`
	RepositoryID                  string                 `json:"repository_id"`
	BaseBranch                    string                 `json:"base_branch"`
	CheckoutBranch                string                 `json:"checkout_branch,omitempty"`
	BranchPolicyID                string                 `json:"branch_policy_id,omitempty"`
	BranchPolicyName              string                 `json:"branch_policy_name,omitempty"`
	BranchPolicyBaseBranch        string                 `json:"branch_policy_base_branch,omitempty"`
	BranchPolicyBranchTemplate    string                 `json:"branch_policy_branch_template,omitempty"`
	BranchPolicyPullRequestTarget string                 `json:"branch_policy_pull_request_target,omitempty"`
	Position                      int                    `json:"position"`
	Metadata                      map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt                     time.Time              `json:"created_at"`
	UpdatedAt                     time.Time              `json:"updated_at"`
}

// TaskWorkspaceFolder is a canonical host-folder attachment owned by a task.
// It stays separate from TaskRepository because folders are not Git sources.
type TaskWorkspaceFolder struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	LocalPath   string    `json:"local_path"`
	DisplayName string    `json:"display_name"`
	Position    int       `json:"position"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkspaceSourceBatch identifies exactly the durable rows created by one
// attachment operation, so a later materialization failure can compensate
// without touching pre-existing sources.
type WorkspaceSourceBatch struct {
	TaskID                    string                            `json:"task_id"`
	Sources                   []WorkspaceSource                 `json:"sources,omitempty"`
	RepositoryUpdates         []WorkspaceSourceRepositoryUpdate `json:"repository_updates,omitempty"`
	ExpectedParentID          string                            `json:"-"`
	ExpectedParentWorkspaceID string                            `json:"-"`
}

// WorkspaceSourceRepositoryUpdate records a legacy association branch derived
// during workspace-source validation. PreviousBaseBranch permits compensation
// if later runtime materialization fails.
type WorkspaceSourceRepositoryUpdate struct {
	TaskRepositoryID   string `json:"task_repository_id"`
	BaseBranch         string `json:"base_branch"`
	PreviousBaseBranch string `json:"previous_base_branch"`
}

// WorkspaceSource carries exactly one durable source kind. Sources preserve
// the caller's mixed order for global position allocation.
type WorkspaceSource struct {
	Repository *TaskRepository      `json:"repository,omitempty"`
	Folder     *TaskWorkspaceFolder `json:"folder,omitempty"`
}

// MessageAuthorType represents who authored a message
type MessageAuthorType string

const (
	// MessageAuthorUser indicates a message from a human user
	MessageAuthorUser MessageAuthorType = "user"
	// MessageAuthorAgent indicates a message from an AI agent
	MessageAuthorAgent MessageAuthorType = "agent"
)

// MessageType represents the type of message content
type MessageType string

const (
	// MessageTypeMessage is the default type for user/agent regular messages
	MessageTypeMessage MessageType = "message"
	// MessageTypeContent is for agent response content
	MessageTypeContent MessageType = "content"
	// MessageTypeToolCall is when agent uses a tool
	MessageTypeToolCall MessageType = "tool_call"
	// MessageTypeToolEdit is for file edit operations with diff visualization
	MessageTypeToolEdit MessageType = "tool_edit"
	// MessageTypeToolRead is for file read operations
	MessageTypeToolRead MessageType = "tool_read"
	// MessageTypeToolExecute is for command execution operations
	MessageTypeToolExecute MessageType = "tool_execute"
	// MessageTypeProgress is for progress updates
	MessageTypeProgress MessageType = "progress"
	// MessageTypeLog is for agent log messages (info, debug, warning, etc.)
	MessageTypeLog MessageType = "log"
	// MessageTypeError is for error messages
	MessageTypeError MessageType = "error"
	// MessageTypeStatus is for status changes: started, completed, failed
	MessageTypeStatus MessageType = "status"
	// MessageTypePermissionRequest is for agent permission requests
	MessageTypePermissionRequest MessageType = "permission_request"
	// MessageTypeClarificationRequest is for agent clarification questions
	MessageTypeClarificationRequest MessageType = "clarification_request"
	// MessageTypeScriptExecution is for setup/cleanup script execution messages
	MessageTypeScriptExecution MessageType = "script_execution"
	// MessageTypeThinking is for agent thinking/reasoning content
	MessageTypeThinking MessageType = "thinking"
	// MessageTypeAgentPlan is for agent native plan content (e.g. ExitPlanMode)
	MessageTypeAgentPlan MessageType = "agent_plan"
	// MessageTypeTodo is for agent todo/task list updates
	MessageTypeTodo MessageType = "todo"
)

// PermissionStatus is the resolution status of a permission request message,
// stored under metadata.status. Pending is the absence sentinel: a freshly
// created permission_request message does not set metadata.status, and any
// reader treats that as PermissionStatusPending.
type PermissionStatus string

const (
	// PermissionStatusPending is the empty-string sentinel for the unresolved
	// state. CreatePermissionRequestMessage does not write metadata.status at
	// all; readers compare against this constant instead of the bare "".
	PermissionStatusPending PermissionStatus = ""
	// PermissionStatusApproved means the user (or automation) accepted the request.
	PermissionStatusApproved PermissionStatus = "approved"
	// PermissionStatusRejected means the user did not accept the request.
	// Collapses two distinct ACP outcomes: explicit Deny (selected option with
	// kind reject_once/reject_always) AND dialog dismissal (ACP cancelled
	// outcome on the wire, triggered today only by the fallback path when the
	// prompt offers no reject_* option). Split into separate Rejected /
	// Dismissed statuses if the audit trail ever needs to distinguish them.
	PermissionStatusRejected PermissionStatus = "rejected"
	// PermissionStatusExpired means the request became unanswerable: the agent
	// withdrew the prompt before a response arrived (permission_cancelled
	// event), or agentctl rejected the response because the pending entry was
	// already gone. No ACP outcome ever reaches the wire in this state.
	PermissionStatusExpired PermissionStatus = "expired"
)

type PermissionResolutionActorKind string

const (
	PermissionActorBrowser             PermissionResolutionActorKind = "browser"
	PermissionActorPersonalAccessToken PermissionResolutionActorKind = "personal_access_token"
	PermissionActorAutomation          PermissionResolutionActorKind = "automation"
	PermissionActorSynthetic           PermissionResolutionActorKind = "synthetic"
)

type PermissionResolutionSource string

const (
	PermissionSourceWeb         PermissionResolutionSource = "web"
	PermissionSourceExternalMCP PermissionResolutionSource = "external_mcp"
	PermissionSourceAutomation  PermissionResolutionSource = "automation"
	// PermissionSourceAutomationMCP identifies a resolution made by the
	// fixed in-session coordinator surface. It is distinct from legacy
	// backend automation and from the authenticated external MCP bridge.
	PermissionSourceAutomationMCP PermissionResolutionSource = "automation_mcp"
)

type PermissionResolutionResult string

const (
	PermissionResolutionDispatching   PermissionResolutionResult = "dispatching"
	PermissionResolutionAccepted      PermissionResolutionResult = "accepted"
	PermissionResolutionStale         PermissionResolutionResult = "stale"
	PermissionResolutionExpired       PermissionResolutionResult = "expired"
	PermissionResolutionFailed        PermissionResolutionResult = "failed"
	PermissionResolutionIndeterminate PermissionResolutionResult = "indeterminate"
)

// PermissionResolutionAudit is the durable, presentation-free record of one
// exact resolution attempt. Credential-bearing action data is not part of this
// type by design.
type PermissionResolutionAudit struct {
	ClaimID     string                        `json:"claim_id"`
	ActorUserID string                        `json:"actor_user_id,omitempty"`
	ActorKind   PermissionResolutionActorKind `json:"actor_kind"`
	Source      PermissionResolutionSource    `json:"source"`
	RequestID   string                        `json:"request_id"`
	PendingID   string                        `json:"pending_id"`
	OptionID    string                        `json:"option_id"`
	OptionKind  string                        `json:"option_kind"`
	SelectedAt  time.Time                     `json:"selected_at"`
	FinalizedAt *time.Time                    `json:"finalized_at,omitempty"`
	Result      PermissionResolutionResult    `json:"result"`
}

type PermissionResolutionClaimOutcome string

const (
	PermissionClaimed           PermissionResolutionClaimOutcome = "claimed"
	PermissionClaimNotFound     PermissionResolutionClaimOutcome = "not_found"
	PermissionClaimInProgress   PermissionResolutionClaimOutcome = "in_progress"
	PermissionClaimAlreadyFinal PermissionResolutionClaimOutcome = "already_final"
)

type PermissionResolutionClaimRequest struct {
	TaskID    string
	SessionID string
	Audit     PermissionResolutionAudit
}

type PermissionResolutionClaimResult struct {
	Outcome PermissionResolutionClaimOutcome
	Message *Message
}

type PermissionResolutionFinalizeOutcome string

const (
	PermissionFinalized             PermissionResolutionFinalizeOutcome = "finalized"
	PermissionFinalizeNotFound      PermissionResolutionFinalizeOutcome = "not_found"
	PermissionFinalizeClaimMismatch PermissionResolutionFinalizeOutcome = "claim_mismatch"
	PermissionFinalizeAlreadyFinal  PermissionResolutionFinalizeOutcome = "already_final"
)

type PermissionResolutionFinalizeRequest struct {
	TaskID      string
	SessionID   string
	RequestID   string
	PendingID   string
	ClaimID     string
	Result      PermissionResolutionResult
	Status      PermissionStatus
	FinalizedAt time.Time
}

type PermissionResolutionFinalizeResult struct {
	Outcome PermissionResolutionFinalizeOutcome
	Message *Message
}

// TaskPendingAction is the compact task-list projection for a session blocked
// on user input.
type TaskPendingAction string

// PendingActionRevision is a logical clock shared by REST and WebSocket
// pending-action projections. Epoch is a durable, monotonically allocated
// backend generation; Sequence orders snapshots within that generation.
type PendingActionRevision struct {
	Epoch    string `json:"epoch"`
	Sequence uint64 `json:"sequence"`
}

const (
	TaskPendingActionClarification TaskPendingAction = "clarification"
	TaskPendingActionPermission    TaskPendingAction = "permission"
)

// Message represents a message in a task session
type Message struct {
	ID            string                 `json:"id"`
	TaskSessionID string                 `json:"session_id"`
	TaskID        string                 `json:"task_id,omitempty"`
	TurnID        string                 `json:"turn_id"` // FK to task_session_turns
	AuthorType    MessageAuthorType      `json:"author_type"`
	AuthorID      string                 `json:"author_id,omitempty"` // User ID or Agent Execution ID
	Content       string                 `json:"content"`
	Type          MessageType            `json:"type,omitempty"` // Defaults to "message"
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	RequestsInput bool                   `json:"requests_input"` // True if agent is requesting user input
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"` // Authoritative per-message change signal
	// PromptIndex is the 1-based ordinal of the message among ALL user
	// messages of its session, ordered by normalized-microsecond created_at
	// ascending with ties broken by id ascending. Read-time derived: zero for
	// non-user messages and for legacy 12-column reads, and serialized only
	// when > 0 (json omitempty).
	PromptIndex int `json:"prompt_index,omitempty"`
}

// ToAPI converts internal Message to API type.
// Only true system-injected content (wrapped in <kandev-system> tags) is stripped
// from the visible content sent to the UI.
func (m *Message) ToAPI() *v1.Message {
	messageType := string(m.Type)
	if messageType == "" {
		messageType = string(MessageTypeMessage)
	}
	hasHidden := sysprompt.HasSystemContent(m.Content)
	meta := ProjectMessageMetadata(m.Metadata)
	if hasHidden {
		if meta == nil {
			meta = make(map[string]interface{})
		} else {
			// Copy to avoid mutating original
			meta = copyMetadata(meta)
		}
		meta["has_hidden_prompts"] = true
	}
	result := &v1.Message{
		ID:            m.ID,
		TaskSessionID: m.TaskSessionID,
		TaskID:        m.TaskID,
		TurnID:        m.TurnID,
		AuthorType:    string(m.AuthorType),
		AuthorID:      m.AuthorID,
		Content:       sysprompt.StripSystemContent(m.Content),
		Type:          messageType,
		Metadata:      meta,
		RequestsInput: m.RequestsInput,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
		PromptIndex:   m.PromptIndex,
	}
	if hasHidden {
		result.RawContent = m.Content
	}
	return result
}

// copyMetadata returns a shallow copy of a metadata map.
func copyMetadata(m map[string]any) map[string]any {
	cp := make(map[string]any, len(m))
	maps.Copy(cp, m)
	return cp
}

// Turn represents a single prompt/response cycle within a task session.
// A turn starts when a user sends a prompt and ends when the agent completes,
// cancels, or errors.
type Turn struct {
	ID            string `json:"id"`
	TaskSessionID string `json:"session_id"`
	TaskID        string `json:"task_id"`
	// ExecutionProfileID and RouteGeneration are immutable attribution for the
	// concrete route that started this turn. They are kept alongside the
	// existing metadata during the compatibility migration so old rows remain
	// readable.
	ExecutionProfileID string                 `json:"execution_profile_id,omitempty"`
	RouteGeneration    int64                  `json:"route_generation,omitempty"`
	StartedAt          time.Time              `json:"started_at"`
	CompletedAt        *time.Time             `json:"completed_at,omitempty"`
	Metadata           map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

// ReviewStatus represents the review state of a TaskSession. The zero value
// (ReviewStatusNone, the empty string) means "no review needed" and serializes
// out via omitempty, preserving the JSON shape of the legacy *string field.
// The persisted column (task_sessions.review_status TEXT DEFAULT ”) is
// compatible with these values without any migration.
type ReviewStatus string

const (
	// ReviewStatusNone is the zero value meaning no review is needed.
	ReviewStatusNone ReviewStatus = ""
	// ReviewStatusPending indicates the session is waiting on a human review.
	ReviewStatusPending ReviewStatus = "pending"
	// ReviewStatusApproved indicates the reviewer accepted the session output.
	ReviewStatusApproved ReviewStatus = "approved"
)

// String returns the underlying string value for the review status.
func (r ReviewStatus) String() string { return string(r) }

// TaskSessionState represents the state of an agent session
type TaskSessionState string

// PromptableTaskSessionClaimStatus describes the outcome of atomically
// reserving a session for a lifecycle prompt.
type PromptableTaskSessionClaimStatus string

const (
	PromptableTaskSessionClaimed  PromptableTaskSessionClaimStatus = "claimed"
	PromptableTaskSessionBusy     PromptableTaskSessionClaimStatus = "busy"
	PromptableTaskSessionInactive PromptableTaskSessionClaimStatus = "inactive"
)

// PromptableTaskSessionClaim is the result of a lifecycle prompt reservation.
// PreviousState is set only when Status is PromptableTaskSessionClaimed.
type PromptableTaskSessionClaim struct {
	Status        PromptableTaskSessionClaimStatus
	PreviousState TaskSessionState
}

const (
	// TaskSessionStateCreated - session created but agent not started
	TaskSessionStateCreated TaskSessionState = "CREATED"
	// TaskSessionStateStarting - agent is starting up
	TaskSessionStateStarting TaskSessionState = "STARTING"
	// TaskSessionStateRunning - agent is actively running
	TaskSessionStateRunning TaskSessionState = "RUNNING"
	// TaskSessionStateIdle - office sessions only: agent process and executor torn down
	// between turns; conversation (acp_session_id) preserved for the next run.
	TaskSessionStateIdle TaskSessionState = "IDLE"
	// TaskSessionStateWaitingForInput - agent waiting for user input
	TaskSessionStateWaitingForInput TaskSessionState = "WAITING_FOR_INPUT"
	// TaskSessionStateCompleted - agent finished successfully
	TaskSessionStateCompleted TaskSessionState = "COMPLETED"
	// TaskSessionStateFailed - agent failed with error
	TaskSessionStateFailed TaskSessionState = "FAILED"
	// TaskSessionStateCancelled - agent was manually stopped
	TaskSessionStateCancelled TaskSessionState = "CANCELLED"
)

// SessionBranchInfo is a lightweight projection of a session with its worktree branch.
// Used by the PR watch reconciler to find sessions that may need PR watches.
type SessionBranchInfo struct {
	SessionID   string
	TaskID      string
	WorkspaceID string
	Branch      string
}

// TaskSession represents a persistent agent execution session for a task.
// This replaces the in-memory TaskExecution tracking and survives backend restarts.
type TaskSession struct {
	ID                     string                 `json:"id"`
	TaskID                 string                 `json:"task_id"`
	Name                   string                 `json:"name,omitempty"`       // Optional user-supplied label shown on the session tab
	AgentExecutionID       string                 `json:"agent_execution_id"`   // Docker container/agent execution
	ContainerID            string                 `json:"container_id"`         // Docker container ID for cleanup
	AgentProfileID         string                 `json:"agent_profile_id"`     // ID of the agent profile used
	ExecutionProfileID     string                 `json:"execution_profile_id"` // Concrete profile used for this execution
	RouteGeneration        int64                  `json:"route_generation,omitempty"`
	RouteState             string                 `json:"route_state,omitempty"`
	RouteReason            string                 `json:"route_reason,omitempty"`
	DownstreamACPSessionID string                 `json:"downstream_acp_session_id,omitempty"`
	RouteErrorCode         string                 `json:"route_error_code,omitempty"`
	RouteErrorClass        string                 `json:"route_error_class,omitempty"`
	RouteCatalogueVersion  string                 `json:"route_catalogue_version,omitempty"`
	RouteRetryOrdinal      int64                  `json:"route_retry_ordinal,omitempty"`
	RouteDeadline          *time.Time             `json:"route_deadline,omitempty"`
	RoutePendingOutcome    string                 `json:"route_pending_outcome,omitempty"`
	ExecutorID             string                 `json:"executor_id"`
	ExecutorProfileID      string                 `json:"executor_profile_id"`
	EnvironmentID          string                 `json:"environment_id"`
	RepositoryID           string                 `json:"repository_id"`   // Primary repository (for backward compatibility)
	BaseBranch             string                 `json:"base_branch"`     // Primary base branch (for backward compatibility)
	BaseCommitSHA          string                 `json:"base_commit_sha"` // Git commit SHA at session start (for cumulative diff)
	WorkspacePath          string                 `json:"workspace_path"`  // Effective task workspace root; legacy repo-less sessions may use the picked host folder
	Worktrees              []*TaskEnvironmentRepo `json:"-"`               // Environment repository rows for this session's workspace
	AgentProfileSnapshot   map[string]interface{} `json:"agent_profile_snapshot,omitempty"`
	ExecutorSnapshot       map[string]interface{} `json:"executor_snapshot,omitempty"`
	EnvironmentSnapshot    map[string]interface{} `json:"environment_snapshot,omitempty"`
	RepositorySnapshot     map[string]interface{} `json:"repository_snapshot,omitempty"`
	State                  TaskSessionState       `json:"state"`
	ErrorMessage           string                 `json:"error_message,omitempty"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	StartedAt              time.Time              `json:"started_at"`
	CompletedAt            *time.Time             `json:"completed_at,omitempty"`
	UpdatedAt              time.Time              `json:"updated_at"`

	// Environment reference
	TaskEnvironmentID string `json:"task_environment_id,omitempty"` // FK to task_environments for shared env

	// Workflow-related fields
	IsPrimary     bool         `json:"is_primary"`              // Whether this is the primary session for the task
	IsPassthrough bool         `json:"is_passthrough"`          // Whether this session uses passthrough (PTY) mode
	ReviewStatus  ReviewStatus `json:"review_status,omitempty"` // zero value = no review needed
	// LastReadMessageID is the id of the newest message the frontend has
	// marked as read for this session — the Slack-style "read cursor" used
	// to position the unread ("New") divider in the transcript. Advanced to
	// the latest message id whenever the session becomes the visible chat
	// panel; the frontend snapshots the PRIOR value at that moment to draw
	// the divider before overwriting it.
	LastReadMessageID string `json:"last_read_message_id,omitempty"`

	// Usage/cost rollup columns (docs/specs/task-cost-ledger/spec.md AC-28,
	// AC-29). task_usage_events is the source of truth; these are the
	// running totals internal/task/usage's writer maintains transactionally
	// alongside each ledger insert via IncrementTaskSessionUsageTx.
	CostSubcents   int64 `json:"cost_subcents"`
	TokensIn       int64 `json:"tokens_in"`
	TokensCachedIn int64 `json:"tokens_cached_in"`
	TokensOut      int64 `json:"tokens_out"`
}

// ToAPI converts internal TaskSession to API type
// TODO: Add v1.TaskSession type to pkg/api/v1/
func (s *TaskSession) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                   s.ID,
		"task_id":              s.TaskID,
		"agent_execution_id":   s.AgentExecutionID,
		"container_id":         s.ContainerID,
		"agent_profile_id":     s.AgentProfileID,
		"execution_profile_id": s.ExecutionProfileID,
		"executor_id":          s.ExecutorID,
		"executor_profile_id":  s.ExecutorProfileID,
		"environment_id":       s.EnvironmentID,
		"repository_id":        s.RepositoryID,
		"base_branch":          s.BaseBranch,
		"base_commit_sha":      s.BaseCommitSHA,
		"state":                string(s.State),
		"started_at":           s.StartedAt,
		"updated_at":           s.UpdatedAt,
	}
	if s.RouteGeneration > 0 {
		result["route_generation"] = s.RouteGeneration
	}
	if s.RouteState != "" {
		result["route_state"] = s.RouteState
	}
	if s.RouteReason != "" {
		result["route_reason"] = s.RouteReason
	}
	if s.DownstreamACPSessionID != "" {
		result["downstream_acp_session_id"] = s.DownstreamACPSessionID
	}
	if s.RouteErrorCode != "" {
		result["route_error_code"] = s.RouteErrorCode
	}
	if s.RouteErrorClass != "" {
		result["route_error_class"] = s.RouteErrorClass
	}
	if s.RouteCatalogueVersion != "" {
		result["route_catalogue_version"] = s.RouteCatalogueVersion
	}
	if s.RouteRetryOrdinal > 0 {
		result["route_retry_ordinal"] = s.RouteRetryOrdinal
	}
	if s.RouteDeadline != nil {
		result["route_deadline"] = s.RouteDeadline
	}
	if s.RoutePendingOutcome != "" {
		result["route_pending_outcome"] = s.RoutePendingOutcome
	}
	if worktrees := s.WorktreesAPI(); len(worktrees) > 0 {
		result["worktrees"] = worktrees
	}
	if s.Name != "" {
		result["name"] = s.Name
	}
	if s.ErrorMessage != "" {
		result["error_message"] = s.ErrorMessage
	}
	if s.CompletedAt != nil {
		result["completed_at"] = s.CompletedAt
	}
	if s.Metadata != nil {
		result["metadata"] = s.Metadata
	}
	if s.AgentProfileSnapshot != nil {
		result["agent_profile_snapshot"] = s.AgentProfileSnapshot
	}
	if s.ExecutorSnapshot != nil {
		result["executor_snapshot"] = s.ExecutorSnapshot
	}
	if s.EnvironmentSnapshot != nil {
		result["environment_snapshot"] = s.EnvironmentSnapshot
	}
	if s.RepositorySnapshot != nil {
		result["repository_snapshot"] = s.RepositorySnapshot
	}
	result["is_passthrough"] = s.IsPassthrough
	if s.TaskEnvironmentID != "" {
		result["task_environment_id"] = s.TaskEnvironmentID
	}
	// Office sessions carry an agent_profile_id; the frontend uses it
	// to distinguish per-(task, agent) office sessions from per-launch
	// kanban/quick-chat sessions, which drives "live" detection in the
	// chat. Omit when blank so kanban responses look unchanged.
	if s.AgentProfileID != "" {
		result["agent_profile_id"] = s.AgentProfileID
	}
	if s.LastReadMessageID != "" {
		result["last_read_message_id"] = s.LastReadMessageID
	}
	return result
}

// WorktreesAPI maps the session's environment repository rows to the legacy
// session-worktree API shape. The wire contract is stable: the frontend reads
// id, worktree_id, repository_id, branch_slug, position, worktree_path, and
// worktree_branch from each entry, and worktree_path/worktree_branch are
// mirrored onto the session for backward compatibility.
func (s *TaskSession) WorktreesAPI() []map[string]interface{} {
	if len(s.Worktrees) == 0 {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(s.Worktrees))
	for _, repo := range s.Worktrees {
		if repo == nil {
			continue
		}
		entry := map[string]interface{}{
			"id":            repo.ID,
			"session_id":    s.ID,
			"worktree_id":   repo.WorktreeID,
			"repository_id": repo.RepositoryID,
			"position":      repo.Position,
			createdAtField:  repo.CreatedAt,
		}
		if repo.BranchSlug != "" {
			entry["branch_slug"] = repo.BranchSlug
		}
		if repo.WorktreePath != "" {
			entry["worktree_path"] = repo.WorktreePath
		}
		if repo.WorktreeBranch != "" {
			entry["worktree_branch"] = repo.WorktreeBranch
		}
		out = append(out, entry)
	}
	return out
}

// WorktreePath returns the workspace path of the session's first repository
// row, used for backward-compatible projections.
func (s *TaskSession) WorktreePath() string {
	if len(s.Worktrees) > 0 && s.Worktrees[0] != nil {
		return s.Worktrees[0].WorktreePath
	}
	return ""
}

// WorktreeBranch returns the branch of the session's first repository row,
// used for backward-compatible projections.
func (s *TaskSession) WorktreeBranch() string {
	if len(s.Worktrees) > 0 && s.Worktrees[0] != nil {
		return s.Worktrees[0].WorktreeBranch
	}
	return ""
}

// Repository represents a workspace repository
type Repository struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	SourceType  string `json:"source_type"`
	// LocalPath is the path to a local checkout; for provider-backed repos, this is
	// populated after the repo is cloned/synced on the agent host.
	LocalPath string `json:"local_path"`
	// Provider fields describe the upstream source (e.g. github/gitlab) for future syncing.
	Provider               string                    `json:"provider"`
	ProviderRepoID         string                    `json:"provider_repo_id"`
	ProviderHost           string                    `json:"provider_host"`
	ProviderScope          string                    `json:"provider_scope"`
	ProviderOwner          string                    `json:"provider_owner"`
	ProviderName           string                    `json:"provider_name"`
	RemoteURL              string                    `json:"remote_url"`
	DefaultBranch          string                    `json:"default_branch"`
	WorktreeBranchPrefix   string                    `json:"worktree_branch_prefix"`
	WorktreeBranchTemplate string                    `json:"worktree_branch_template"`
	PullBeforeWorktree     bool                      `json:"pull_before_worktree"`
	SetupScript            string                    `json:"setup_script"`
	CleanupScript          string                    `json:"cleanup_script"`
	DevScript              string                    `json:"dev_script"`
	CopyFiles              string                    `json:"copy_files"`
	SecretBindings         []RepositorySecretBinding `json:"secret_bindings,omitempty"`
	CreatedAt              time.Time                 `json:"created_at"`
	UpdatedAt              time.Time                 `json:"updated_at"`
	DeletedAt              *time.Time                `json:"deleted_at,omitempty"`
}

// RepositoryBranchPolicy is a reusable branch workflow owned by one repository.
// It is configuration, not task history: task repositories copy these fields
// into their snapshot columns when a policy is selected.
type RepositoryBranchPolicy struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description,omitempty"`
	BaseBranch        string    `json:"base_branch"`
	BranchTemplate    string    `json:"branch_template"`
	PullRequestTarget string    `json:"pull_request_target"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// RepositorySet is a named, reusable group of workspace repositories that fills
// the task-creation repository picker in one action.
//
// A set deliberately stores no branch. Branch choice belongs to a task and is
// already modelled on TaskRepository; a branch cached here would go stale
// against the repository's real refs.
type RepositorySet struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Items is the ordered membership. Slice order is authoritative on write:
	// positions are assigned from it, and reads return items sorted by position.
	Items     []RepositorySetItem `json:"repositories"`
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// RepositorySetItem is one repository's membership in a set.
type RepositorySetItem struct {
	ID              string    `json:"id"`
	RepositorySetID string    `json:"repository_set_id"`
	RepositoryID    string    `json:"repository_id"`
	Position        int       `json:"position"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// RepositoryIDs returns the set's membership in position order.
func (s *RepositorySet) RepositoryIDs() []string {
	ids := make([]string, 0, len(s.Items))
	for _, item := range s.Items {
		ids = append(ids, item.RepositoryID)
	}
	return ids
}

// ProviderRepositoryIdentity is the durable provider lookup key. New plugin
// providers supply Scope + RepositoryID; legacy built-ins use Host/Owner/Name.
type ProviderRepositoryIdentity struct {
	WorkspaceID  string
	Provider     string
	Scope        string
	RepositoryID string
	Host         string
	Owner        string
	Name         string
}

// RepositorySecretBinding maps an environment key to a secret reference. The
// value is never persisted or returned; a missing secret intentionally leaves
// this row dangling so launches can report a broken binding.
type RepositorySecretBinding struct {
	RepositoryID string    `json:"repository_id,omitempty"`
	Key          string    `json:"key"`
	SecretID     string    `json:"secret_id"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// RepositoryScript represents a custom script for a repository
type RepositoryScript struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Name         string    `json:"name"`
	Command      string    `json:"command"`
	Position     int       `json:"position"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ExecutorType represents the executor runtime type.
type ExecutorType string

const (
	ExecutorTypeLocal        ExecutorType = "local"
	ExecutorTypeWorktree     ExecutorType = "worktree"
	ExecutorTypeLocalDocker  ExecutorType = "local_docker"
	ExecutorTypeRemoteDocker ExecutorType = "remote_docker"
	ExecutorTypeSprites      ExecutorType = "sprites"
	ExecutorTypeSSH          ExecutorType = "ssh"
	ExecutorTypeMockRemote   ExecutorType = "mock_remote"
)

// IsRemoteExecutorType reports whether the given executor type represents
// a remote execution environment (including containerized environments like Docker).
// These environments run shells inside the container/VM, not on the host.
func IsRemoteExecutorType(t ExecutorType) bool {
	switch t {
	case ExecutorTypeSprites, ExecutorTypeRemoteDocker, ExecutorTypeLocalDocker, ExecutorTypeSSH, ExecutorTypeMockRemote:
		return true
	default:
		return false
	}
}

// Runtime maps an ExecutorType to the runtime backend that hosts the
// agent subprocess. Mirrors executor.ExecutorTypeToBackend so callers
// outside the executor package can ask the property without importing
// it. The two are kept in lock-step intentionally — keep them in sync
// when adding a new ExecutorType.
func (t ExecutorType) Runtime() agentruntime.Runtime {
	switch t {
	case ExecutorTypeLocal, ExecutorTypeWorktree, ExecutorTypeMockRemote:
		return agentruntime.RuntimeStandalone
	case ExecutorTypeLocalDocker:
		return agentruntime.RuntimeDocker
	case ExecutorTypeRemoteDocker:
		return agentruntime.RuntimeRemoteDocker
	case ExecutorTypeSprites:
		return agentruntime.RuntimeSprites
	case ExecutorTypeSSH:
		return agentruntime.RuntimeSSH
	default:
		return agentruntime.RuntimeStandalone
	}
}

// IsContainerizedExecutorType reports whether the given executor type runs
// in a container/sandbox where shells must be executed inside the container
// via agentctl, not on the host. Single source of truth is
// Runtime().IsContainerized() so a new ExecutorType only has to declare its
// runtime once.
func IsContainerizedExecutorType(t ExecutorType) bool {
	return t.Runtime().IsContainerized()
}

// IsAlwaysResumableRuntime reports whether the given runtime represents
// an executor that can always be resumed even without an explicit resume token.
func IsAlwaysResumableRuntime(runtime agentruntime.Runtime) bool {
	return runtime == agentruntime.RuntimeSprites || runtime == agentruntime.RuntimeSSH
}

const (
	ExecutorIDLocal       = "exec-local"
	ExecutorIDWorktree    = "exec-worktree"
	ExecutorIDLocalDocker = "exec-local-docker"
	ExecutorIDSprites     = "exec-sprites"
)

// ExecutorStatus represents executor availability.
type ExecutorStatus string

const (
	ExecutorStatusActive   ExecutorStatus = "active"
	ExecutorStatusDisabled ExecutorStatus = "disabled"
)

// Executor represents an execution target.
type Executor struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      ExecutorType      `json:"type"`
	Status    ExecutorStatus    `json:"status"`
	IsSystem  bool              `json:"is_system"`
	Resumable bool              `json:"resumable"`
	Config    map[string]string `json:"config,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	DeletedAt *time.Time        `json:"deleted_at,omitempty"`
}

// ExecutorRunning tracks an active executor instance for a session.
type ExecutorRunning struct {
	ID                 string               `json:"id"`
	SessionID          string               `json:"session_id"`
	TaskID             string               `json:"task_id"`
	ExecutionProfileID string               `json:"execution_profile_id"`
	ExecutorID         string               `json:"executor_id"`
	Runtime            agentruntime.Runtime `json:"runtime,omitempty"`
	Status             string               `json:"status"`
	Resumable          bool                 `json:"resumable"`
	ResumeToken        string               `json:"resume_token,omitempty"`
	LastMessageUUID    string               `json:"last_message_uuid,omitempty"`
	AgentExecutionID   string               `json:"agent_execution_id,omitempty"`
	ContainerID        string               `json:"container_id,omitempty"`
	AgentctlURL        string               `json:"agentctl_url,omitempty"`
	AgentctlPort       int                  `json:"agentctl_port,omitempty"`
	// PID is SSH-only: the agentctl PID on the *remote* host, used by the SSH
	// executor's remote-pid stop path. It is 0 for local/standalone rows.
	PID int `json:"pid,omitempty"`
	// LocalPID is a host-local liveness handle: the PID of the standalone
	// agentctl control-server process Kandev spawned on this host. Populated for
	// local/standalone runtimes so a dead row is distinguishable from a live one
	// without external context. 0 for SSH/remote rows (their process lives on
	// another host — see PID). Never probe LocalPID for SSH rows.
	LocalPID       int                    `json:"local_pid,omitempty"`
	WorktreeID     string                 `json:"worktree_id,omitempty"`
	WorktreePath   string                 `json:"worktree_path,omitempty"`
	WorktreeBranch string                 `json:"worktree_branch,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	LastSeenAt     *time.Time             `json:"last_seen_at,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// ProfileEnvVar represents an environment variable for an executor profile.
// Either Value (plain text) or SecretID (reference to a secret) should be set, not both.
type ProfileEnvVar struct {
	Key      string `json:"key"`
	Value    string `json:"value,omitempty"`
	SecretID string `json:"secret_id,omitempty"`
}

// ExecutorProfile represents a named configuration preset for an executor.
type ExecutorProfile struct {
	ID            string            `json:"id"`
	ExecutorID    string            `json:"executor_id"`
	Name          string            `json:"name"`
	McpPolicy     string            `json:"mcp_policy,omitempty"`
	Config        map[string]string `json:"config,omitempty"`
	PrepareScript string            `json:"prepare_script,omitempty"`
	CleanupScript string            `json:"cleanup_script,omitempty"`
	EnvVars       []ProfileEnvVar   `json:"env_vars,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// EnvironmentKind represents the runtime type for environments.
type EnvironmentKind string

const (
	EnvironmentKindLocalPC     EnvironmentKind = "local_pc"
	EnvironmentKindDockerImage EnvironmentKind = "docker_image"
)

const (
	EnvironmentIDLocal = "env-local"
)

// Environment represents a runtime environment configuration.
type Environment struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Kind         EnvironmentKind   `json:"kind"`
	IsSystem     bool              `json:"is_system"`
	WorktreeRoot string            `json:"worktree_root,omitempty"`
	ImageTag     string            `json:"image_tag,omitempty"`
	Dockerfile   string            `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	DeletedAt    *time.Time        `json:"deleted_at,omitempty"`
}

// TaskEnvironmentStatus represents the lifecycle state of a task execution environment.
type TaskEnvironmentStatus string

const (
	TaskEnvironmentStatusCreating TaskEnvironmentStatus = "creating"
	TaskEnvironmentStatusReady    TaskEnvironmentStatus = "ready"
	TaskEnvironmentStatusStopped  TaskEnvironmentStatus = "stopped"
	TaskEnvironmentStatusFailed   TaskEnvironmentStatus = "failed"
)

// TaskEnvironment represents a per-task execution environment instance.
// It owns the workspace (worktree/container/sandbox) and the agentctl control server.
// Multiple sessions can share the same TaskEnvironment.
type TaskEnvironment struct {
	ID                string `json:"id"`
	TaskID            string `json:"task_id"`
	ExecutorType      string `json:"executor_type"`
	ExecutorID        string `json:"executor_id"`
	ExecutorProfileID string `json:"executor_profile_id"`
	// AgentExecutionID was removed: executors_running owns the execution<->session
	// mapping now. Read it via repo.GetExecutorRunningBySessionID(sessionID) when
	// needed (the orchestrator does this in service_turns.go for WorkspaceInfo).
	ControlPort int                   `json:"control_port"` // agentctl control port
	Status      TaskEnvironmentStatus `json:"status"`
	// MaterializationSessionID durably identifies the one session allowed to
	// turn a creating environment into a physical workspace. It is empty once
	// the environment is ready; sibling sessions must attach only.
	MaterializationSessionID string `json:"-"`

	// WorkspacePath points at the agent workspace root (the task root when
	// TaskDirName is set, otherwise the single repo's worktree path).
	// Physical worktree identity lives on Repos, never on the environment row.
	WorkspacePath string `json:"workspace_path,omitempty"`
	ContainerID   string `json:"container_id,omitempty"`
	// ContainerBootstrapNonceSecretID is an environment-scoped encrypted secret
	// reference used only to establish a new agentctl control connection to an
	// already-owned Docker container. It is deliberately not exposed in API
	// responses and is distinct from session runtime/auth metadata.
	ContainerBootstrapNonceSecretID string `json:"-"`
	// ContainerControlAuthTokenSecretID is the environment-scoped encrypted
	// agentctl control-token reference for a running Docker container. It is
	// deliberately separate from a session's agent runtime/auth metadata.
	ContainerControlAuthTokenSecretID string `json:"-"`
	SandboxID                         string `json:"sandbox_id,omitempty"`

	// TaskDirName is the semantic directory name for the task (e.g. "fix-bug_ab12").
	// Set when the task uses the multi-repo task-directory layout
	// (~/.kandev/tasks/{TaskDirName}/{RepoName}/).
	TaskDirName string `json:"task_dir_name,omitempty"`

	// Repos contains one entry per repository associated with this environment.
	// Each row is the single source of physical-worktree truth for that
	// repository slot. Populated by repository getters.
	Repos []*TaskEnvironmentRepo `json:"repos,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RepoFor returns the per-repo environment row for repositoryID, or nil if not present.
func (te *TaskEnvironment) RepoFor(repositoryID string) *TaskEnvironmentRepo {
	for _, r := range te.Repos {
		if r.RepositoryID == repositoryID {
			return r
		}
	}
	return nil
}

// TaskEnvironmentRepo represents the per-repository state of a task environment.
// One row per repository associated with the task: each carries its own worktree
// reference and any per-repo preparation error. The row is the single source of
// physical-worktree truth — identity, path, branch, status, and lifecycle
// timestamps.
type TaskEnvironmentRepo struct {
	ID                string     `json:"id"`
	TaskEnvironmentID string     `json:"task_environment_id"`
	RepositoryID      string     `json:"repository_id"`
	BranchSlug        string     `json:"branch_slug,omitempty"`
	WorktreeID        string     `json:"worktree_id,omitempty"`
	WorktreePath      string     `json:"worktree_path,omitempty"`
	WorktreeBranch    string     `json:"worktree_branch,omitempty"`
	Position          int        `json:"position"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	Status            string     `json:"status,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	MergedAt          *time.Time `json:"merged_at,omitempty"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
}

// ToAPI converts internal TaskEnvironment to API map.
func (te *TaskEnvironment) ToAPI() map[string]interface{} {
	result := map[string]interface{}{
		"id":                  te.ID,
		"task_id":             te.TaskID,
		"executor_type":       te.ExecutorType,
		"executor_id":         te.ExecutorID,
		"executor_profile_id": te.ExecutorProfileID,
		"status":              string(te.Status),
		"workspace_path":      te.WorkspacePath,
		createdAtField:        te.CreatedAt,
		"updated_at":          te.UpdatedAt,
	}
	// agent_execution_id is no longer carried on TaskEnvironment — see executors_running.
	if te.ControlPort != 0 {
		result["control_port"] = te.ControlPort
	}
	if te.ContainerID != "" {
		result["container_id"] = te.ContainerID
	}
	if te.SandboxID != "" {
		result["sandbox_id"] = te.SandboxID
	}
	if te.TaskDirName != "" {
		result["task_dir_name"] = te.TaskDirName
	}
	if len(te.Repos) > 0 {
		repos := make([]map[string]interface{}, 0, len(te.Repos))
		for _, repo := range te.Repos {
			repos = append(repos, repo.ToAPI())
		}
		result["repos"] = repos
	}
	return result
}

// ToAPI converts internal TaskEnvironmentRepo to API map.
func (r *TaskEnvironmentRepo) ToAPI() map[string]interface{} {
	out := map[string]interface{}{
		"id":                  r.ID,
		"task_environment_id": r.TaskEnvironmentID,
		"repository_id":       r.RepositoryID,
		"position":            r.Position,
		createdAtField:        r.CreatedAt,
		"updated_at":          r.UpdatedAt,
	}
	if r.WorktreeID != "" {
		out["worktree_id"] = r.WorktreeID
	}
	if r.BranchSlug != "" {
		out["branch_slug"] = r.BranchSlug
	}
	if r.WorktreePath != "" {
		out["worktree_path"] = r.WorktreePath
	}
	if r.WorktreeBranch != "" {
		out["worktree_branch"] = r.WorktreeBranch
	}
	if r.ErrorMessage != "" {
		out["error_message"] = r.ErrorMessage
	}
	return out
}

// TaskPlan represents a plan associated with a task
type TaskPlan struct {
	ID                             string     `json:"id"`
	TaskID                         string     `json:"task_id"`
	Title                          string     `json:"title"`
	Content                        string     `json:"content"`
	CreatedBy                      string     `json:"created_by"` // "agent" or "user"
	CreatedAt                      time.Time  `json:"created_at"`
	UpdatedAt                      time.Time  `json:"updated_at"`
	ImplementationStartedAt        *time.Time `json:"implementation_started_at,omitempty"`
	ImplementationStartedSessionID *string    `json:"implementation_started_session_id,omitempty"`
	ImplementationStartedBy        *string    `json:"implementation_started_by,omitempty"`
}

// TaskPlanRevision is one immutable snapshot in the revision history of a task plan.
// Revisions are the source of truth for history; TaskPlan stores the latest revision's content as HEAD.
type TaskPlanRevision struct {
	ID                 string    `json:"id"`
	TaskID             string    `json:"task_id"`
	RevisionNumber     int       `json:"revision_number"`
	Title              string    `json:"title"`
	Content            string    `json:"content"`
	AuthorKind         string    `json:"author_kind"` // "agent" | "user"
	AuthorName         string    `json:"author_name"` // display snapshot (agent profile name or user identifier)
	RevertOfRevisionID *string   `json:"revert_of_revision_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"` // bumps on coalesce merge
}

// TaskWalkthrough is an agent-authored guided code tour attached to a task.
// It is the "what & where" of a review narration: an ordered list of Steps,
// each anchored to a concrete repo/file/line, rendered as popovers over the
// review diff. Mirrors the TaskPlan artifact pattern (one per task, agent-authored).
type TaskWalkthrough struct {
	ID        string            `json:"id"`
	TaskID    string            `json:"task_id"`
	Title     string            `json:"title"`
	Steps     []WalkthroughStep `json:"steps"`
	CreatedBy string            `json:"created_by"` // always "agent"
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// WalkthroughStep is a single anchored stop in a TaskWalkthrough. Text is
// markdown shown in the popover; File/Line locate the anchor inside the diff
// (Repo disambiguates in multi-repo reviews, LineEnd optionally spans a range).
type WalkthroughStep struct {
	Title   string `json:"title,omitempty"`
	Repo    string `json:"repo,omitempty"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	LineEnd int    `json:"line_end,omitempty"`
	Text    string `json:"text"`
}

// ReviewRunStatus is the lifecycle state of a native code-review pass.
type ReviewRunStatus string

// Review run statuses. A run only leaves pending/running once, and every
// terminal state is final — an interrupted run is cancelled, never resumed.
const (
	ReviewRunPending   ReviewRunStatus = "pending"
	ReviewRunRunning   ReviewRunStatus = "running"
	ReviewRunCompleted ReviewRunStatus = "completed"
	ReviewRunFailed    ReviewRunStatus = "failed"
	ReviewRunCancelled ReviewRunStatus = "cancelled"
)

// IsTerminal reports whether the run has finished and cannot change again.
func (s ReviewRunStatus) IsTerminal() bool {
	return s == ReviewRunCompleted || s == ReviewRunFailed || s == ReviewRunCancelled
}

// ReviewRunTrigger records what started a review pass.
type ReviewRunTrigger string

// Review run triggers.
const (
	ReviewTriggerManual       ReviewRunTrigger = "manual"
	ReviewTriggerWorkflowStep ReviewRunTrigger = "workflow_step"
	ReviewTriggerAgent        ReviewRunTrigger = "agent"
)

// ReviewSeverity ranks how much a finding should worry the reader.
type ReviewSeverity string

// Review severities, most severe first.
const (
	ReviewSeverityBlocker ReviewSeverity = "blocker"
	ReviewSeverityMajor   ReviewSeverity = "major"
	ReviewSeverityMinor   ReviewSeverity = "minor"
	ReviewSeverityNit     ReviewSeverity = "nit"
)

// ValidReviewSeverity reports whether s is a known severity.
func ValidReviewSeverity(s ReviewSeverity) bool {
	switch s {
	case ReviewSeverityBlocker, ReviewSeverityMajor, ReviewSeverityMinor, ReviewSeverityNit:
		return true
	default:
		return false
	}
}

// ReviewFindingStatus is the human disposition of a finding. Findings are
// advisory: the human resolves or dismisses, kandev never does it for them.
type ReviewFindingStatus string

// Review finding statuses.
const (
	ReviewFindingOpen      ReviewFindingStatus = "open"
	ReviewFindingResolved  ReviewFindingStatus = "resolved"
	ReviewFindingDismissed ReviewFindingStatus = "dismissed"
)

// ValidReviewFindingStatus reports whether s is a known finding status.
func ValidReviewFindingStatus(s ReviewFindingStatus) bool {
	switch s {
	case ReviewFindingOpen, ReviewFindingResolved, ReviewFindingDismissed:
		return true
	default:
		return false
	}
}

// ReviewAnnotationSide selects which side of the diff a finding anchors to,
// matching the frontend annotation sides.
const (
	ReviewSideAdditions = "additions"
	ReviewSideDeletions = "deletions"
)

// TaskReviewRun is one native code-review pass over a task's changed files.
// A task keeps a bounded history of runs; findings reference the run that
// produced them so the UI can attribute and supersede them.
type TaskReviewRun struct {
	ID              string           `json:"id"`
	TaskID          string           `json:"task_id"`
	SessionID       string           `json:"session_id"`
	Trigger         ReviewRunTrigger `json:"trigger"`
	WorkflowStepID  string           `json:"workflow_step_id"`
	AgentID         string           `json:"agent_id"`
	Model           string           `json:"model"`
	Status          ReviewRunStatus  `json:"status"`
	ErrorCode       string           `json:"error_code"`
	ErrorMessage    string           `json:"error_message"`
	Summary         string           `json:"summary"`
	FindingCount    int              `json:"finding_count"`
	FileCount       int              `json:"file_count"`
	RepositoryCount int              `json:"repository_count"`
	PromptTokens    int              `json:"prompt_tokens"`
	ResponseTokens  int              `json:"response_tokens"`
	DurationMs      int              `json:"duration_ms"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}

// TaskReviewFinding is one anchored, advisory review comment produced by a
// review run. It renders in the Changes/Review diff at File/StartLine..EndLine
// of Repository, and carries FileDiffHash so a client can tell whether the
// diff has moved under it (see ../../../../docs/specs/agents/requirements/native-code-review.md).
type TaskReviewFinding struct {
	ID             string              `json:"id"`
	RunID          string              `json:"run_id"`
	TaskID         string              `json:"task_id"`
	RepositoryID   string              `json:"repository_id"`
	RepositoryName string              `json:"repository_name"`
	FilePath       string              `json:"file_path"`
	StartLine      int                 `json:"start_line"`
	EndLine        int                 `json:"end_line"`
	Side           string              `json:"side"`
	Severity       ReviewSeverity      `json:"severity"`
	Category       string              `json:"category"`
	Title          string              `json:"title"`
	Body           string              `json:"body"`
	Suggestion     string              `json:"suggestion"`
	AnchorText     string              `json:"anchor_text"`
	FileDiffHash   string              `json:"file_diff_hash"`
	Status         ReviewFindingStatus `json:"status"`
	ResolvedAt     *time.Time          `json:"resolved_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

// ReviewFindingKey identifies a finding's anchor for supersede matching: a new
// run that reports the same issue at the same place replaces the older row
// instead of listing it twice.
type ReviewFindingKey struct {
	RepositoryName string
	FilePath       string
	StartLine      int
	EndLine        int
	Title          string
}

// SupersedeKey returns the finding's supersede identity.
func (f *TaskReviewFinding) SupersedeKey() ReviewFindingKey {
	return ReviewFindingKey{
		RepositoryName: f.RepositoryName,
		FilePath:       f.FilePath,
		StartLine:      f.StartLine,
		EndLine:        f.EndLine,
		Title:          f.Title,
	}
}

// TaskDocument represents a named document (plan, spec, notes, etc.) associated with a task.
// The key uniquely identifies the document within a task (e.g., "plan", "spec", "notes").
type TaskDocument struct {
	ID         string    `json:"id" db:"id"`
	TaskID     string    `json:"task_id" db:"task_id"`
	Key        string    `json:"key" db:"key"`
	Type       string    `json:"type" db:"type"`
	Title      string    `json:"title" db:"title"`
	Content    string    `json:"content" db:"content"`
	AuthorKind string    `json:"author_kind" db:"author_kind"`
	AuthorName string    `json:"author_name" db:"author_name"`
	Filename   string    `json:"filename,omitempty" db:"filename"`
	MimeType   string    `json:"mime_type,omitempty" db:"mime_type"`
	SizeBytes  int64     `json:"size_bytes,omitempty" db:"size_bytes"`
	DiskPath   string    `json:"-" db:"disk_path"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// TaskDocumentRevision is one immutable snapshot in the revision history of a task document.
// Revisions are the source of truth for history; TaskDocument stores the latest revision's content as HEAD.
type TaskDocumentRevision struct {
	ID                 string    `json:"id" db:"id"`
	TaskID             string    `json:"task_id" db:"task_id"`
	DocumentKey        string    `json:"document_key" db:"document_key"`
	RevisionNumber     int       `json:"revision_number" db:"revision_number"`
	Title              string    `json:"title" db:"title"`
	Content            string    `json:"content" db:"content"`
	AuthorKind         string    `json:"author_kind" db:"author_kind"`
	AuthorName         string    `json:"author_name" db:"author_name"`
	RevertOfRevisionID *string   `json:"revert_of_revision_id,omitempty" db:"revert_of_revision_id"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// TaskMessageAttachment is the durable registry row for a prompt attachment.
// Message and queue metadata carry only the public descriptor; the bytes stay
// in private backend storage addressed by StorageKey.
type TaskMessageAttachment struct {
	ID           string    `json:"id" db:"id"`
	OwnerID      string    `json:"owner_id" db:"owner_id"`
	WorkspaceID  string    `json:"workspace_id" db:"workspace_id"`
	TaskID       string    `json:"task_id,omitempty" db:"task_id"`
	SessionID    string    `json:"session_id,omitempty" db:"session_id"`
	MessageID    string    `json:"message_id,omitempty" db:"message_id"`
	QueueID      string    `json:"queue_id,omitempty" db:"queue_id"`
	Name         string    `json:"name" db:"name"`
	MimeType     string    `json:"mime_type" db:"mime_type"`
	Kind         string    `json:"kind" db:"kind"`
	DeliveryMode string    `json:"delivery_mode" db:"delivery_mode"`
	SizeBytes    int64     `json:"size_bytes" db:"size_bytes"`
	StorageKey   string    `json:"-" db:"storage_key"`
	State        string    `json:"state" db:"state"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

const (
	AttachmentStateStaged  = "staged"
	AttachmentStateClaimed = "claimed"
	AttachmentStateExpired = "expired"
	AttachmentStateDeleted = "deleted"
)

// SessionFileReview tracks per-file review state within a session
type SessionFileReview struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"session_id"`
	FilePath   string     `json:"file_path"`
	Reviewed   bool       `json:"reviewed"`
	DiffHash   string     `json:"diff_hash"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// ToAPI converts internal Task to API type
func (t *Task) ToAPI() *v1.Task {
	// Convert TaskRepository models to API types
	var repositories []v1.TaskRepository
	for _, repo := range t.Repositories {
		repositories = append(repositories, v1.TaskRepository{
			ID:           repo.ID,
			TaskID:       repo.TaskID,
			RepositoryID: repo.RepositoryID,
			BaseBranch:   repo.BaseBranch,
			Position:     repo.Position,
			Metadata:     repo.Metadata,
			CreatedAt:    repo.CreatedAt,
			UpdatedAt:    repo.UpdatedAt,
		})
	}

	result := &v1.Task{
		ID:              t.ID,
		WorkspaceID:     t.WorkspaceID,
		WorkflowID:      t.WorkflowID,
		Title:           t.Title,
		Description:     t.Description,
		State:           t.State,
		Priority:        t.Priority,
		Repositories:    repositories,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
		Metadata:        t.Metadata,
		Interrupted:     t.Metadata[MetaKeyInterruptedAt] != nil,
		AutoStartFailed: t.Metadata[MetaKeyAutoStartFailed] != nil,
		IsEphemeral:     t.IsEphemeral,
		ParentID:        t.ParentID,
		Autopilot:       t.Autopilot,
	}
	if t.Identifier != "" {
		result.Identifier = t.Identifier
	}
	return result
}
