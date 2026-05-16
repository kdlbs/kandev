// Package models defines data types for the office domain.
package models

import (
	"encoding/json"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// AgentRole and AgentStatus are aliases for the canonical types declared
// in internal/agent/settings/models. ADR 0005 Wave D moved the typed
// constants to the settings package because the agent_profiles row owns
// these attributes after the unification; the aliases here keep ~270
// office callsites compiling without an import-flag-day rename.
//
// New code should import settingsmodels directly. Office-only callers
// reading models.AgentRoleCEO etc. continue to work via the const
// re-exports below.
type (
	AgentRole   = settingsmodels.AgentRole
	AgentStatus = settingsmodels.AgentStatus
)

// Re-exported AgentRole values. These are package-level constant aliases —
// `models.AgentRoleCEO` evaluates to the same untyped string as
// `settingsmodels.AgentRoleCEO`, so callers can mix references across
// packages freely.
const (
	AgentRoleCEO        = settingsmodels.AgentRoleCEO
	AgentRoleWorker     = settingsmodels.AgentRoleWorker
	AgentRoleSpecialist = settingsmodels.AgentRoleSpecialist
	AgentRoleAssistant  = settingsmodels.AgentRoleAssistant
	AgentRoleSecurity   = settingsmodels.AgentRoleSecurity
	AgentRoleQA         = settingsmodels.AgentRoleQA
	AgentRoleDevOps     = settingsmodels.AgentRoleDevOps
)

// Re-exported AgentStatus values.
const (
	AgentStatusIdle            = settingsmodels.AgentStatusIdle
	AgentStatusWorking         = settingsmodels.AgentStatusWorking
	AgentStatusPaused          = settingsmodels.AgentStatusPaused
	AgentStatusStopped         = settingsmodels.AgentStatusStopped
	AgentStatusPendingApproval = settingsmodels.AgentStatusPendingApproval
)

// AgentInstance is an alias for settings.AgentProfile. ADR 0005 Wave G
// collapsed the two structs into one row representation: every office
// agent is an agent_profiles row with workspace_id != ”. Office code
// continues to import models.AgentInstance for ergonomic naming, but the
// underlying type is identical to settingsmodels.AgentProfile.
//
// The legacy AgentProfileID field is gone — under the unified model the
// row's ID is the profile id. Callsites that previously read .AgentProfileID
// have been migrated to .ID.
type AgentInstance = settingsmodels.AgentProfile

// Skill represents a reusable skill definition.
//
// System skills (is_system = true) are kandev-owned: they are
// bundled with the binary, upserted into office_skills at startup,
// and refreshed in place on every kandev release. Per-agent
// `desired_skills` references are preserved across updates because
// the row id stays stable. User skills (is_system = false) are
// imported by users (`source_type = git | inline | local_path |
// skills_sh | user_home`) and are never touched by the startup sync.
//
// `default_for_roles` is a JSON-encoded `[]string` of agent roles
// that should receive this system skill auto-attached on agent
// create (e.g. ["ceo"]). Empty array for "no auto-attach". Ignored
// for is_system = false.
type Skill struct {
	ID                      string    `json:"id" db:"id"`
	WorkspaceID             string    `json:"workspace_id" db:"workspace_id"`
	Name                    string    `json:"name" db:"name"`
	Slug                    string    `json:"slug" db:"slug"`
	Description             string    `json:"description" db:"description"`
	SourceType              string    `json:"source_type" db:"source_type"`
	SourceLocator           string    `json:"source_locator" db:"source_locator"`
	Content                 string    `json:"content" db:"content"`
	FileInventory           string    `json:"file_inventory" db:"file_inventory"`
	Version                 string    `json:"version" db:"version"`
	ContentHash             string    `json:"content_hash" db:"content_hash"`
	ApprovalState           string    `json:"approval_state" db:"approval_state"`
	CreatedByAgentProfileID string    `json:"created_by_agent_profile_id" db:"created_by_agent_profile_id"`
	IsSystem                bool      `json:"is_system" db:"is_system"`
	SystemVersion           string    `json:"system_version" db:"system_version"`
	DefaultForRoles         string    `json:"default_for_roles" db:"default_for_roles"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time `json:"updated_at" db:"updated_at"`
}

// RunSkillSnapshot records the exact skill package attached to a run.
type RunSkillSnapshot struct {
	RunID            string `json:"run_id" db:"run_id"`
	SkillID          string `json:"skill_id" db:"skill_id"`
	Version          string `json:"version" db:"version"`
	ContentHash      string `json:"content_hash" db:"content_hash"`
	MaterializedPath string `json:"materialized_path" db:"materialized_path"`
}

// ProjectStatus represents the status of a project.
type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusOnHold    ProjectStatus = "on_hold"
	ProjectStatusArchived  ProjectStatus = "archived"
)

// Project represents an office project.
//
// Repositories is stored in the DB and YAML as a JSON-encoded array
// string (e.g. `["github.com/foo/bar"]`). MarshalJSON/UnmarshalJSON
// convert it to/from a real `[]string` on the API wire so the
// frontend (and any other JSON consumer) sees an array.
type Project struct {
	ID                 string        `json:"id" db:"id"`
	WorkspaceID        string        `json:"workspace_id" db:"workspace_id"`
	Name               string        `json:"name" db:"name"`
	Description        string        `json:"description" db:"description"`
	Status             ProjectStatus `json:"status" db:"status"`
	LeadAgentProfileID string        `json:"lead_agent_profile_id" db:"lead_agent_profile_id"`
	Color              string        `json:"color" db:"color"`
	BudgetCents        int           `json:"budget_cents" db:"budget_cents"`
	Repositories       string        `json:"-" db:"repositories"`
	ExecutorConfig     string        `json:"executor_config" db:"executor_config"`
	CreatedAt          time.Time     `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at" db:"updated_at"`
}

// projectWire is the on-the-wire JSON representation of Project. It
// matches Project field-for-field except `repositories` is a real
// []string instead of a JSON-encoded string. Kept private so callers
// only ever construct Project directly.
type projectWire struct {
	ID                 string        `json:"id"`
	WorkspaceID        string        `json:"workspace_id"`
	Name               string        `json:"name"`
	Description        string        `json:"description"`
	Status             ProjectStatus `json:"status"`
	LeadAgentProfileID string        `json:"lead_agent_profile_id"`
	Color              string        `json:"color"`
	BudgetCents        int           `json:"budget_cents"`
	Repositories       []string      `json:"repositories"`
	ExecutorConfig     string        `json:"executor_config"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// MarshalJSON emits Project with `repositories` as a []string. The
// stored value is a JSON-encoded array string; empty values normalise
// to an empty array so the frontend never has to guard against null.
func (p Project) MarshalJSON() ([]byte, error) {
	repos, err := DecodeRepositories(p.Repositories)
	if err != nil {
		return nil, err
	}
	return json.Marshal(projectWire{
		ID:                 p.ID,
		WorkspaceID:        p.WorkspaceID,
		Name:               p.Name,
		Description:        p.Description,
		Status:             p.Status,
		LeadAgentProfileID: p.LeadAgentProfileID,
		Color:              p.Color,
		BudgetCents:        p.BudgetCents,
		Repositories:       repos,
		ExecutorConfig:     p.ExecutorConfig,
		CreatedAt:          p.CreatedAt,
		UpdatedAt:          p.UpdatedAt,
	})
}

// UnmarshalJSON accepts `repositories` as a []string and stores it
// back as the canonical JSON-encoded string. Other consumers
// (config sync, YAML) treat the field as a string so we preserve the
// internal shape.
func (p *Project) UnmarshalJSON(data []byte) error {
	var w projectWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	encoded, err := EncodeRepositories(w.Repositories)
	if err != nil {
		return err
	}
	p.ID = w.ID
	p.WorkspaceID = w.WorkspaceID
	p.Name = w.Name
	p.Description = w.Description
	p.Status = w.Status
	p.LeadAgentProfileID = w.LeadAgentProfileID
	p.Color = w.Color
	p.BudgetCents = w.BudgetCents
	p.Repositories = encoded
	p.ExecutorConfig = w.ExecutorConfig
	p.CreatedAt = w.CreatedAt
	p.UpdatedAt = w.UpdatedAt
	return nil
}

// DecodeRepositories parses the stored JSON-encoded array string into
// a []string. Empty input returns an empty (non-nil) slice so callers
// never serialise `null`.
func DecodeRepositories(raw string) ([]string, error) {
	if raw == "" || raw == "[]" {
		return []string{}, nil
	}
	var repos []string
	if err := json.Unmarshal([]byte(raw), &repos); err != nil {
		return nil, err
	}
	if repos == nil {
		return []string{}, nil
	}
	return repos, nil
}

// EncodeRepositories renders a []string as the canonical JSON-encoded
// array string used by the DB and YAML layers. A nil/empty slice
// becomes "[]" so the validator's empty-check stays consistent.
func EncodeRepositories(repos []string) (string, error) {
	if len(repos) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(repos)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// TaskCounts holds aggregated task status counts for a project.
type TaskCounts struct {
	Total      int `json:"total" db:"total"`
	InProgress int `json:"in_progress" db:"in_progress"`
	Done       int `json:"done" db:"done"`
	Blocked    int `json:"blocked" db:"blocked"`
}

// ProjectWithCounts is a project with aggregated task counts.
type ProjectWithCounts struct {
	Project
	TaskCounts TaskCounts `json:"task_counts"`
}

// MarshalJSON inlines the embedded Project alongside `task_counts`.
// Required because Project has a custom MarshalJSON and Go would
// otherwise use it for the whole struct, dropping TaskCounts.
func (p ProjectWithCounts) MarshalJSON() ([]byte, error) {
	repos, err := DecodeRepositories(p.Repositories)
	if err != nil {
		return nil, err
	}
	type wire struct {
		projectWire
		TaskCounts TaskCounts `json:"task_counts"`
	}
	return json.Marshal(wire{
		projectWire: projectWire{
			ID:                 p.ID,
			WorkspaceID:        p.WorkspaceID,
			Name:               p.Name,
			Description:        p.Description,
			Status:             p.Status,
			LeadAgentProfileID: p.LeadAgentProfileID,
			Color:              p.Color,
			BudgetCents:        p.BudgetCents,
			Repositories:       repos,
			ExecutorConfig:     p.ExecutorConfig,
			CreatedAt:          p.CreatedAt,
			UpdatedAt:          p.UpdatedAt,
		},
		TaskCounts: p.TaskCounts,
	})
}

// ValidProjectStatuses contains valid project status values.
var ValidProjectStatuses = map[ProjectStatus]bool{
	ProjectStatusActive:    true,
	ProjectStatusCompleted: true,
	ProjectStatusOnHold:    true,
	ProjectStatusArchived:  true,
}

// CostEvent represents a cost tracking event.
//
// CostSubcents is stored as hundredths of a cent (int64) to keep token-rate
// math integer-only. UI divides by 10000 when rendering dollars. Estimated
// is true when token counts were synthesised by the adapter (e.g.
// cumulative-delta inference for codex-acp) rather than reported directly
// by the agent; the row still counts toward budget totals at face value.
type CostEvent struct {
	ID             string    `json:"id" db:"id"`
	SessionID      string    `json:"session_id" db:"session_id"`
	TaskID         string    `json:"task_id" db:"task_id"`
	AgentProfileID string    `json:"agent_profile_id" db:"agent_profile_id"`
	ProjectID      string    `json:"project_id" db:"project_id"`
	Model          string    `json:"model" db:"model"`
	Provider       string    `json:"provider" db:"provider"`
	TokensIn       int64     `json:"tokens_in" db:"tokens_in"`
	TokensCachedIn int64     `json:"tokens_cached_in" db:"tokens_cached_in"`
	TokensOut      int64     `json:"tokens_out" db:"tokens_out"`
	CostSubcents   int64     `json:"cost_subcents" db:"cost_subcents"`
	Estimated      bool      `json:"estimated" db:"estimated"`
	OccurredAt     time.Time `json:"occurred_at" db:"occurred_at"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// BudgetPolicy represents a budget limit policy. LimitSubcents is
// hundredths of a cent (matches CostEvent.CostSubcents); the UI divides
// by 10000 to render dollars via apps/web/lib/utils.ts:formatDollars.
type BudgetPolicy struct {
	ID                string    `json:"id" db:"id"`
	WorkspaceID       string    `json:"workspace_id" db:"workspace_id"`
	ScopeType         string    `json:"scope_type" db:"scope_type"`
	ScopeID           string    `json:"scope_id" db:"scope_id"`
	LimitSubcents     int64     `json:"limit_subcents" db:"limit_subcents"`
	Period            string    `json:"period" db:"period"`
	AlertThresholdPct int       `json:"alert_threshold_pct" db:"alert_threshold_pct"`
	ActionOnExceed    string    `json:"action_on_exceed" db:"action_on_exceed"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

// Run represents a run queue entry.
type Run struct {
	ID               string     `json:"id" db:"id"`
	AgentProfileID   string     `json:"agent_profile_id" db:"agent_profile_id"`
	Reason           string     `json:"reason" db:"reason"`
	Payload          string     `json:"payload" db:"payload"`
	Status           string     `json:"status" db:"status"`
	CoalescedCount   int        `json:"coalesced_count" db:"coalesced_count"`
	IdempotencyKey   *string    `json:"idempotency_key" db:"idempotency_key"`
	ContextSnapshot  string     `json:"context_snapshot" db:"context_snapshot"`
	Capabilities     string     `json:"capabilities" db:"capabilities"`
	InputSnapshot    string     `json:"input_snapshot" db:"input_snapshot"`
	OutputSummary    string     `json:"output_summary" db:"output_summary"`
	FailureReason    string     `json:"failure_reason" db:"failure_reason"`
	SessionID        string     `json:"session_id" db:"session_id"`
	RetryCount       int        `json:"retry_count" db:"retry_count"`
	ScheduledRetryAt *time.Time `json:"scheduled_retry_at" db:"scheduled_retry_at"`
	CancelReason     *string    `json:"cancel_reason,omitempty" db:"cancel_reason"`
	// ErrorMessage is set when the run transitions to status=failed.
	// Stored verbatim from the agent error event for inbox + chat use.
	ErrorMessage string `json:"error_message,omitempty" db:"error_message"`
	// ResultJSON is the structured adapter output captured at run
	// completion. The continuation-summary builder reads this to
	// populate the "Recent decisions" / "Recent actions" sections.
	// Defaults to "{}".
	ResultJSON string `json:"result_json,omitempty" db:"result_json"`
	// AssembledPrompt is the final prompt string the agent received.
	// Persisted at dispatch so the run-detail UI can render exactly
	// what the agent saw (independent of session replay).
	AssembledPrompt string `json:"assembled_prompt,omitempty" db:"assembled_prompt"`
	// SummaryInjected is the continuation-summary content prepended
	// to the prompt at dispatch time, snapshot for inspection. Empty
	// when no summary was injected (today: every run, until PR 2).
	SummaryInjected string     `json:"summary_injected,omitempty" db:"summary_injected"`
	RequestedAt     time.Time  `json:"requested_at" db:"requested_at"`
	ClaimedAt       *time.Time `json:"claimed_at" db:"claimed_at"`
	FinishedAt      *time.Time `json:"finished_at" db:"finished_at"`

	// Provider-routing columns (office-provider-routing spec). All
	// optional and ignored when workspace routing is disabled. The TEXT
	// columns are pointer-typed so SELECT * StructScan handles the NULL
	// rows existing runs ship with after the ADD COLUMN migration.
	//
	// LogicalProviderOrder is a JSON snapshot of the effective provider
	// order at launch time; remains stable across post-start fallbacks
	// within the same run.
	LogicalProviderOrder *string `json:"logical_provider_order,omitempty" db:"logical_provider_order"`
	// RequestedTier is the tier the resolver consumed (override > workspace
	// default) when the run was first dispatched.
	RequestedTier *string `json:"requested_tier,omitempty" db:"requested_tier"`
	// ResolvedProviderID/Model is the candidate that actually launched;
	// empty until a launch succeeds.
	ResolvedProviderID *string `json:"resolved_provider_id,omitempty" db:"resolved_provider_id"`
	ResolvedModel      *string `json:"resolved_model,omitempty" db:"resolved_model"`
	// CurrentRouteAttemptSeq tracks the in-flight attempt so post-start
	// fallback can find the right row to update and exclude already-tried
	// providers when re-resolving.
	CurrentRouteAttemptSeq int `json:"current_route_attempt_seq" db:"current_route_attempt_seq"`
	// RouteCycleBaselineSeq marks the seq floor for the current retry
	// cycle: prior attempts with seq <= baseline are NOT counted toward
	// the dispatcher's exclude-set. Bumped to CurrentRouteAttemptSeq
	// whenever a parked run is lifted (auto wake-up or manual retry) so
	// the run can re-try every provider in its order. Post-start fallback
	// does NOT bump it — within a single cycle, providers that fail still
	// stay excluded.
	RouteCycleBaselineSeq int `json:"route_cycle_baseline_seq" db:"route_cycle_baseline_seq"`
	// RoutingBlockedStatus is set when every provider candidate is
	// unavailable; values: 'waiting_for_provider_capacity' |
	// 'blocked_provider_action_required'.
	RoutingBlockedStatus *string `json:"routing_blocked_status,omitempty" db:"routing_blocked_status"`
	// EarliestRetryAt is the earliest moment a parked run should be re-
	// resolved. Set only when at least one degraded route is auto-retryable.
	EarliestRetryAt *time.Time `json:"earliest_retry_at,omitempty" db:"earliest_retry_at"`
}

// RouteAttempt records one provider attempt inside a Run. Each fallback
// (resolver candidate tried) appends a new row keyed by (run_id, seq).
// Persisted by the routing scheduler dispatcher in
// internal/office/scheduler/dispatch_routing.go.
type RouteAttempt struct {
	RunID           string     `json:"run_id" db:"run_id"`
	Seq             int        `json:"seq" db:"seq"`
	ProviderID      string     `json:"provider_id" db:"provider_id"`
	Model           string     `json:"model" db:"model"`
	Tier            string     `json:"tier" db:"tier"`
	Outcome         string     `json:"outcome" db:"outcome"`
	ErrorCode       string     `json:"error_code,omitempty" db:"error_code"`
	ErrorConfidence string     `json:"error_confidence,omitempty" db:"error_confidence"`
	AdapterPhase    string     `json:"adapter_phase,omitempty" db:"adapter_phase"`
	ClassifierRule  string     `json:"classifier_rule,omitempty" db:"classifier_rule"`
	ExitCode        *int       `json:"exit_code,omitempty" db:"exit_code"`
	RawExcerpt      string     `json:"raw_excerpt,omitempty" db:"raw_excerpt"`
	ResetHint       *time.Time `json:"reset_hint,omitempty" db:"reset_hint"`
	StartedAt       time.Time  `json:"started_at" db:"started_at"`
	FinishedAt      *time.Time `json:"finished_at,omitempty" db:"finished_at"`
}

// ProviderHealth records the health state of one (workspace, provider,
// scope) tuple. Scope is one of 'provider' (whole provider), 'model'
// (a specific model on this provider), or 'tier' (a specific workspace
// tier mapping on this provider). The resolver checks scopes in order
// provider → tier → model before considering a candidate eligible.
type ProviderHealth struct {
	WorkspaceID string     `json:"workspace_id" db:"workspace_id"`
	ProviderID  string     `json:"provider_id" db:"provider_id"`
	Scope       string     `json:"scope" db:"scope"`
	ScopeValue  string     `json:"scope_value" db:"scope_value"`
	State       string     `json:"state" db:"state"`
	ErrorCode   string     `json:"error_code,omitempty" db:"error_code"`
	RetryAt     *time.Time `json:"retry_at,omitempty" db:"retry_at"`
	BackoffStep int        `json:"backoff_step" db:"backoff_step"`
	LastFailure *time.Time `json:"last_failure,omitempty" db:"last_failure"`
	LastSuccess *time.Time `json:"last_success,omitempty" db:"last_success"`
	RawExcerpt  string     `json:"raw_excerpt,omitempty" db:"raw_excerpt"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}

// RunEvent is one row in office_run_events. Each row is a discrete
// lifecycle event for an office run: init, adapter.invoke, step,
// complete, error. The frontend renders these in the run detail
// page's Events log.
type RunEvent struct {
	RunID     string    `json:"run_id" db:"run_id"`
	Seq       int       `json:"seq" db:"seq"`
	EventType string    `json:"event_type" db:"event_type"`
	Level     string    `json:"level" db:"level"`
	Payload   string    `json:"payload" db:"payload"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Routine represents a recurring task definition.
type Routine struct {
	ID                     string     `json:"id" db:"id"`
	WorkspaceID            string     `json:"workspace_id" db:"workspace_id"`
	Name                   string     `json:"name" db:"name"`
	Description            string     `json:"description" db:"description"`
	TaskTemplate           string     `json:"task_template" db:"task_template"`
	AssigneeAgentProfileID string     `json:"assignee_agent_profile_id" db:"assignee_agent_profile_id"`
	Status                 string     `json:"status" db:"status"`
	ConcurrencyPolicy      string     `json:"concurrency_policy" db:"concurrency_policy"`
	CatchUpPolicy          string     `json:"catch_up_policy" db:"catch_up_policy"`
	CatchUpMax             int        `json:"catch_up_max" db:"catch_up_max"`
	Variables              string     `json:"variables" db:"variables"`
	LastRunAt              *time.Time `json:"last_run_at" db:"last_run_at"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// RoutineTrigger represents a trigger for a routine.
type RoutineTrigger struct {
	ID             string     `json:"id" db:"id"`
	RoutineID      string     `json:"routine_id" db:"routine_id"`
	Kind           string     `json:"kind" db:"kind"`
	CronExpression string     `json:"cron_expression" db:"cron_expression"`
	Timezone       string     `json:"timezone" db:"timezone"`
	PublicID       string     `json:"public_id" db:"public_id"`
	SigningMode    string     `json:"signing_mode" db:"signing_mode"`
	Secret         string     `json:"secret" db:"secret"`
	NextRunAt      *time.Time `json:"next_run_at" db:"next_run_at"`
	LastFiredAt    *time.Time `json:"last_fired_at" db:"last_fired_at"`
	Enabled        bool       `json:"enabled" db:"enabled"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

// RoutineRun represents a single run of a routine.
type RoutineRun struct {
	ID                  string     `json:"id" db:"id"`
	RoutineID           string     `json:"routine_id" db:"routine_id"`
	TriggerID           string     `json:"trigger_id" db:"trigger_id"`
	Source              string     `json:"source" db:"source"`
	Status              string     `json:"status" db:"status"`
	TriggerPayload      string     `json:"trigger_payload" db:"trigger_payload"`
	LinkedTaskID        string     `json:"linked_task_id" db:"linked_task_id"`
	CoalescedIntoRunID  string     `json:"coalesced_into_run_id" db:"coalesced_into_run_id"`
	DispatchFingerprint string     `json:"dispatch_fingerprint" db:"dispatch_fingerprint"`
	StartedAt           *time.Time `json:"started_at" db:"started_at"`
	CompletedAt         *time.Time `json:"completed_at" db:"completed_at"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
}

// ApprovalType constants for approval request types.
const (
	ApprovalTypeHireAgent      = "hire_agent"
	ApprovalTypeBudgetIncrease = "budget_increase"
	ApprovalTypeBoardApproval  = "board_approval"
	ApprovalTypeTaskReview     = "task_review"
	ApprovalTypeSkillCreation  = "skill_creation"
)

// Approval represents a pending or resolved approval request.
type Approval struct {
	ID                        string     `json:"id" db:"id"`
	WorkspaceID               string     `json:"workspace_id" db:"workspace_id"`
	Type                      string     `json:"type" db:"type"`
	RequestedByAgentProfileID string     `json:"requested_by_agent_profile_id" db:"requested_by_agent_profile_id"`
	Status                    string     `json:"status" db:"status"`
	Payload                   string     `json:"payload" db:"payload"`
	DecisionNote              string     `json:"decision_note" db:"decision_note"`
	DecidedBy                 string     `json:"decided_by" db:"decided_by"`
	DecidedAt                 *time.Time `json:"decided_at" db:"decided_at"`
	CreatedAt                 time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt                 time.Time  `json:"updated_at" db:"updated_at"`
}

// ActivityEntry represents an entry in the activity log.
type ActivityEntry struct {
	ID          string `json:"id" db:"id"`
	WorkspaceID string `json:"workspace_id" db:"workspace_id"`
	ActorType   string `json:"actor_type" db:"actor_type"`
	ActorID     string `json:"actor_id" db:"actor_id"`
	Action      string `json:"action" db:"action"`
	TargetType  string `json:"target_type" db:"target_type"`
	TargetID    string `json:"target_id" db:"target_id"`
	Details     string `json:"details" db:"details"`
	// RunID + SessionID let the run detail page join activity rows
	// back to the originating run for the "Tasks Touched" surface.
	// Empty string for activity not produced under a run (manual
	// user actions, etc.).
	RunID     string    `json:"run_id,omitempty" db:"run_id"`
	SessionID string    `json:"session_id,omitempty" db:"session_id"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// AgentMemory represents a memory entry for an agent.
type AgentMemory struct {
	ID             string    `json:"id" db:"id"`
	AgentProfileID string    `json:"agent_profile_id" db:"agent_profile_id"`
	Layer          string    `json:"layer" db:"layer"`
	Key            string    `json:"key" db:"key"`
	Content        string    `json:"content" db:"content"`
	Metadata       string    `json:"metadata" db:"metadata"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// Channel represents a communication channel for an agent.
type Channel struct {
	ID             string    `json:"id" db:"id"`
	WorkspaceID    string    `json:"workspace_id" db:"workspace_id"`
	AgentProfileID string    `json:"agent_profile_id" db:"agent_profile_id"`
	Platform       string    `json:"platform" db:"platform"`
	Config         string    `json:"config" db:"config"`
	WebhookSecret  string    `json:"webhook_secret,omitempty" db:"webhook_secret"`
	Status         string    `json:"status" db:"status"`
	TaskID         string    `json:"task_id" db:"task_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}

// TaskBlocker represents a blocker relationship between tasks.
type TaskBlocker struct {
	TaskID        string    `json:"task_id" db:"task_id"`
	BlockerTaskID string    `json:"blocker_task_id" db:"blocker_task_id"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// Participant role values for the office_task_participants table.
// The DB has a CHECK constraint that pins these to two literals.
const (
	ParticipantRoleReviewer = "reviewer"
	ParticipantRoleApprover = "approver"
)

// Decision values recorded by the office approval flow. ADR 0005 Wave E
// moved storage from the legacy office_task_approval_decisions table to
// workflow_step_decisions; the literals below are still the canonical
// values office writes for `decision` and `decider_type`.
const (
	DecisionApproved         = "approved"
	DecisionChangesRequested = "changes_requested"

	DeciderTypeUser  = "user"
	DeciderTypeAgent = "agent"
)

// TaskComment represents an asynchronous comment on a task.
type TaskComment struct {
	ID             string    `json:"id" db:"id"`
	TaskID         string    `json:"task_id" db:"task_id"`
	AuthorType     string    `json:"author_type" db:"author_type"`
	AuthorID       string    `json:"author_id" db:"author_id"`
	Body           string    `json:"body" db:"body"`
	Source         string    `json:"source" db:"source"`
	ReplyChannelID string    `json:"reply_channel_id" db:"reply_channel_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

// InboxItem represents a computed inbox entry for the user.
type InboxItem struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Status      string                 `json:"status"`
	EntityID    string                 `json:"entity_id,omitempty"`
	EntityType  string                 `json:"entity_type,omitempty"`
	Payload     map[string]interface{} `json:"payload,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// RunActivityDay holds aggregated run outcome counts for a single calendar day.
type RunActivityDay struct {
	Date      string
	Succeeded int
	Failed    int
	Other     int
}

// TaskBreakdown holds task counts bucketed by status category.
type TaskBreakdown struct {
	Open       int
	InProgress int
	Blocked    int
	Done       int
}

// RecentTask holds minimal fields for a recently-updated task.
type RecentTask struct {
	ID                     string
	Identifier             string
	Title                  string
	Status                 string
	AssigneeAgentProfileID string
	UpdatedAt              string
}

// DashboardData represents aggregated dashboard information.
type DashboardData struct {
	AgentCount         int              `json:"agent_count"`
	RunningCount       int              `json:"running_count"`
	PausedCount        int              `json:"paused_count"`
	ErrorCount         int              `json:"error_count"`
	TasksInProgress    int              `json:"tasks_in_progress"`
	OpenTasks          int              `json:"open_tasks"`
	BlockedTasks       int              `json:"blocked_tasks"`
	MonthSpendSubcents int64            `json:"month_spend_subcents"`
	PendingApprovals   int              `json:"pending_approvals"`
	RecentActivity     []*ActivityEntry `json:"recent_activity"`
	RecentIssues       []interface{}    `json:"recent_issues,omitempty"`
	TaskCount          int              `json:"task_count"`
	SkillCount         int              `json:"skill_count"`
	RoutineCount       int              `json:"routine_count"`
	RunActivity        []RunActivityDay
	TaskBreakdown      TaskBreakdown
	RecentTasks        []RecentTask
}

// InstructionFile represents an instruction file for an agent instance.
type InstructionFile struct {
	ID             string `json:"id" db:"id"`
	AgentProfileID string `json:"agent_profile_id" db:"agent_profile_id"`
	Filename       string `json:"filename" db:"filename"`
	Content        string `json:"content" db:"content"`
	IsEntry        bool   `json:"is_entry" db:"is_entry"`
	CreatedAt      string `json:"created_at" db:"created_at"`
	UpdatedAt      string `json:"updated_at" db:"updated_at"`
}

// CostBreakdown represents an aggregated cost entry. TotalSubcents stores
// hundredths of a cent (UI divides by 10000 for dollars). GroupKey is the
// stable id used by the breakdown grouping (agent_profile_id, project_id,
// or model). GroupLabel is the human-readable label resolved by the query
// (e.g. agent profile name, project name); empty when no lookup applies
// or when the id has no row in the source table.
type CostBreakdown struct {
	GroupKey      string `json:"group_key" db:"group_key"`
	GroupLabel    string `json:"group_label" db:"group_label"`
	TotalSubcents int64  `json:"total_subcents" db:"total_subcents"`
	Count         int    `json:"count" db:"count"`
}
