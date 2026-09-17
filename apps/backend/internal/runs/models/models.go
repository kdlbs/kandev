package models

import "time"

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
	ContinuationScope string     `json:"continuation_scope,omitempty" db:"continuation_scope"`
	RequestedAt       time.Time  `json:"requested_at" db:"requested_at"`
	ClaimedAt         *time.Time `json:"claimed_at" db:"claimed_at"`
	FinishedAt        *time.Time `json:"finished_at" db:"finished_at"`

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
}

type RunEvent struct {
	RunID     string        `json:"run_id" db:"run_id"`
	Seq       int           `json:"seq" db:"seq"`
	EventType RunEventType  `json:"event_type" db:"event_type"`
	Level     RunEventLevel `json:"level" db:"level"`
	Payload   string        `json:"payload" db:"payload"`
	CreatedAt time.Time     `json:"created_at" db:"created_at"`
}
