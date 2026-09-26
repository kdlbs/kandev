// Package models defines the shared run-row and run-event data contracts.
package models

import "time"

// RunStatus is the scheduler queue state for a Run row.
// See internal/office/scheduler/run.go for the state machine.
type RunStatus string

// Run queue status values.
const (
	RunStatusQueued   RunStatus = "queued"
	RunStatusClaimed  RunStatus = "claimed"
	RunStatusFinished RunStatus = "finished"
	RunStatusFailed   RunStatus = "failed"
)

// String implements fmt.Stringer.
func (s RunStatus) String() string { return string(s) }

// RoutingBlockedStatus is the "park reason" written onto Run when no
// provider can be selected.
type RoutingBlockedStatus string

// Routing blocked-status values.
const (
	RoutingBlockedWaitingForCapacity RoutingBlockedStatus = "waiting_for_provider_capacity"
	RoutingBlockedActionRequired     RoutingBlockedStatus = "blocked_provider_action_required"
)

// String implements fmt.Stringer.
func (s RoutingBlockedStatus) String() string { return string(s) }

// RunEventLevel is the severity classification of a RunEvent row.
// Free-form by convention: info | warn | error.
type RunEventLevel string

// Run event level values.
const (
	RunEventLevelInfo  RunEventLevel = "info"
	RunEventLevelWarn  RunEventLevel = "warn"
	RunEventLevelError RunEventLevel = "error"
)

// String implements fmt.Stringer.
func (l RunEventLevel) String() string { return string(l) }

// RunEventType is the kind of a RunEvent. Open set (init, step,
// adapter.invoke, complete, error, runtime.denied, runtime.action, …).
// Typed for documentation, not for exhaustiveness.
type RunEventType string

// Well-known run event types. Adapters may emit additional values.
const (
	RunEventTypeInit          RunEventType = "init"
	RunEventTypeAdapterInvoke RunEventType = "adapter.invoke"
	RunEventTypeStep          RunEventType = "step"
	RunEventTypeComplete      RunEventType = "complete"
	RunEventTypeError         RunEventType = "error"
	RunEventTypeRuntimeDenied RunEventType = "runtime.denied"
	RunEventTypeRuntimeAction RunEventType = "runtime.action"
)

// String implements fmt.Stringer.
func (t RunEventType) String() string { return string(t) }

// ActorKind names who performed the action that caused an enqueue, per
// AC-OFFICE-RUN-CAUSATION-001.15: exactly one of a human user, an Office
// agent, or the system itself. It is a property of the causing action,
// not of the agent being woken.
type ActorKind string

// Actor kind values. There is deliberately no "unspecified" zero value
// that resolves silently — an absent or unrecognized kind is handled by
// ResolveActorKind, never by relying on ActorKind's Go zero value.
const (
	ActorKindUser   ActorKind = "user"
	ActorKindAgent  ActorKind = "agent"
	ActorKindSystem ActorKind = "system"
)

// String implements fmt.Stringer.
func (k ActorKind) String() string { return string(k) }

// Valid reports whether k is one of the three declared actor kinds.
func (k ActorKind) Valid() bool {
	switch k {
	case ActorKindUser, ActorKindAgent, ActorKindSystem:
		return true
	default:
		return false
	}
}

// PriorityClass is the coarse bucket that decides which queued run is
// claimed first when capacity is scarce (AC-OFFICE-BACKPRESSURE-001.1).
// Values are ordered most to least preferred and the integer values are
// persisted on runs.priority_class, so they must not be renumbered.
type PriorityClass int

const (
	// PriorityClassRecovery is assigned only by re-queue (retry, recovery
	// sweep, routing re-dispatch); no wake reason maps here directly.
	PriorityClassRecovery PriorityClass = 1
	// PriorityClassEvent is the default class for ordinary wake reasons.
	PriorityClassEvent PriorityClass = 2
	// PriorityClassPeriodic is for unattended, schedule-driven fires.
	PriorityClassPeriodic PriorityClass = 3
	// PriorityClassHuman is the highest-preference class, assigned when
	// the actor is a human user or the wake reason is human-only.
	PriorityClassHuman PriorityClass = 0
)

// String implements fmt.Stringer.
func (c PriorityClass) String() string {
	switch c {
	case PriorityClassHuman:
		return "human"
	case PriorityClassRecovery:
		return "recovery"
	case PriorityClassEvent:
		return "event"
	case PriorityClassPeriodic:
		return "periodic"
	default:
		return "unknown"
	}
}

// Run represents a run queue entry.
type Run struct {
	ID               string     `json:"id" db:"id"`
	AgentProfileID   string     `json:"agent_profile_id" db:"agent_profile_id"`
	Reason           string     `json:"reason" db:"reason"`
	Payload          string     `json:"payload" db:"payload"`
	Status           RunStatus  `json:"status" db:"status"`
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
	SummaryInjected string `json:"summary_injected,omitempty" db:"summary_injected"`
	// ContinuationScope is the continuation-summary scope key
	// (ContinuationScopeForRun's output) computed once at run creation
	// and persisted here so every later reader/writer of this run's
	// continuation summary uses the same value. Computing this at
	// creation time — before any wakeup can coalesce into this row —
	// closes a race where a routine wakeup patches context_snapshot
	// after claim but a re-derivation against the freshly re-fetched
	// row would disagree with the derivation the claiming scheduler is
	// still holding in memory.
	ContinuationScope string `json:"continuation_scope,omitempty" db:"continuation_scope"`
	// WakeWaveKey and WakeWaveString are the completion-wave identity
	// (parent-wake-wave-identity): both set together, only for
	// task_children_completed runs, from one derivation per queued run.
	// WakeWaveKey is the digest idx_run_wake_wave indexes; WakeWaveString
	// is the plain string the backstop's candidate query compares. Empty
	// for every other run reason and for every pre-upgrade row.
	WakeWaveKey    string     `json:"wake_wave_key,omitempty" db:"wake_wave_key"`
	WakeWaveString string     `json:"wake_wave_string,omitempty" db:"wake_wave_string"`
	RequestedAt    time.Time  `json:"requested_at" db:"requested_at"`
	ClaimedAt      *time.Time `json:"claimed_at" db:"claimed_at"`
	FinishedAt     *time.Time `json:"finished_at" db:"finished_at"`

	// Outcome records why a finished run ended (docs/specs/
	// task-delivery-ledger/spec.md, "Office run outcome"): one of eight
	// values on the finished path, NULL on failed and on every
	// pre-activation row. Pointer-typed so StructScan reads the NULL
	// every pre-activation row carries, same idiom as the provider-
	// routing columns below.
	Outcome *string `json:"outcome,omitempty" db:"outcome"`

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
	// Causation chain identity (REQ-OFFICE-RUN-CAUSATION-001).
	// ChainCausationID is the root run's own ID; empty means the legacy
	// pre-capability marker, read by AC-OFFICE-RUN-CAUSATION-001.6 as a
	// root at depth 0. Distinct from CausationID below
	// (REQ-OFFICE-LOOP-LIVENESS-002), an unrelated wakeup-request
	// correlation that reached for the same name independently.
	ChainCausationID string        `json:"chain_causation_id" db:"chain_causation_id"`
	ParentRunID      string        `json:"parent_run_id" db:"parent_run_id"`
	CausationDepth   int           `json:"causation_depth" db:"causation_depth"`
	PriorityClass    PriorityClass `json:"priority_class" db:"priority_class"`
	HumanRooted      bool          `json:"human_rooted" db:"human_rooted"`
	RoutineID        string        `json:"routine_id" db:"routine_id"`
	ActorKind        ActorKind     `json:"actor_kind" db:"actor_kind"`
	ActorID          string        `json:"actor_id" db:"actor_id"`
	WorkspaceID      string        `json:"workspace_id" db:"workspace_id"`

	// ResolvedExecutionProfileID/ProviderID/Model identify the candidate
	// that actually launched; empty until a launch succeeds. Provider and
	// model are audit snapshots derived from the concrete profile.
	ResolvedExecutionProfileID *string `json:"resolved_execution_profile_id,omitempty" db:"resolved_execution_profile_id"`
	ResolvedProviderID         *string `json:"resolved_provider_id,omitempty" db:"resolved_provider_id"`
	ResolvedModel              *string `json:"resolved_model,omitempty" db:"resolved_model"`
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
	RoutingBlockedStatus *RoutingBlockedStatus `json:"routing_blocked_status,omitempty" db:"routing_blocked_status"`
	// EarliestRetryAt is the earliest moment a parked run should be re-
	// resolved. Set only when at least one degraded route is auto-retryable.
	EarliestRetryAt *time.Time `json:"earliest_retry_at,omitempty" db:"earliest_retry_at"`

	// CausationID is copied from the agent_wakeup_requests row that
	// created this run (REQ-OFFICE-LOOP-LIVENESS-002). "" means
	// uncorrelated — either a legacy pre-migration row or a run created
	// off a wake that never carried an id. Never a join/group-by key
	// without excluding "" first.
	CausationID string `json:"causation_id,omitempty" db:"causation_id"`
}

// RunEvent is one row in office_run_events. Each row is a discrete
// lifecycle event for an office run: init, adapter.invoke, step,
// complete, error. The frontend renders these in the run detail
// page's Events log.
type RunEvent struct {
	RunID     string        `json:"run_id" db:"run_id"`
	Seq       int           `json:"seq" db:"seq"`
	EventType RunEventType  `json:"event_type" db:"event_type"`
	Level     RunEventLevel `json:"level" db:"level"`
	Payload   string        `json:"payload" db:"payload"`
	CreatedAt time.Time     `json:"created_at" db:"created_at"`
}
