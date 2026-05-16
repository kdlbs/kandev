// Package models — enum types.
//
// This file colocates the named string types used by office structs.
// Each type wraps the existing magic-string vocabulary so callsites can
// compare against typed constants instead of bare string literals. The
// underlying type is `string` in every case, so `database/sql` Scan /
// `driver.Value` bind, and JSON marshaling, all continue to work
// without custom hooks.
//
// Convention: typed-string + untyped const, with a `String()` method.
// Untyped consts let existing comparison sites keep working when the
// struct field gets re-typed.
package models

// ApprovalStatus is the lifecycle state of an Approval row.
type ApprovalStatus string

// Approval status values. See internal/office/approvals/service.go for
// the state machine (pending → approved | rejected).
const (
	ApprovalStatusPending  ApprovalStatus = "pending"
	ApprovalStatusApproved ApprovalStatus = "approved"
	ApprovalStatusRejected ApprovalStatus = "rejected"
)

// String implements fmt.Stringer.
func (s ApprovalStatus) String() string { return string(s) }

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

// RoutineRunStatus is the lifecycle state of a RoutineRun row.
// See internal/office/routines/service.go for the state machine.
type RoutineRunStatus string

// Routine run status values.
const (
	RoutineRunStatusReceived    RoutineRunStatus = "received"
	RoutineRunStatusTaskCreated RoutineRunStatus = "task_created"
	RoutineRunStatusSkipped     RoutineRunStatus = "skipped"
	RoutineRunStatusCoalesced   RoutineRunStatus = "coalesced"
	RoutineRunStatusFailed      RoutineRunStatus = "failed"
	RoutineRunStatusDone        RoutineRunStatus = "done"
	RoutineRunStatusCancelled   RoutineRunStatus = "cancelled"
)

// String implements fmt.Stringer.
func (s RoutineRunStatus) String() string { return string(s) }

// RoutineConcurrencyPolicy controls what happens when a routine fires
// while a previous run is still in flight.
type RoutineConcurrencyPolicy string

// Routine concurrency policy values.
const (
	ConcurrencyPolicySkipIfActive     RoutineConcurrencyPolicy = "skip_if_active"
	ConcurrencyPolicyCoalesceIfActive RoutineConcurrencyPolicy = "coalesce_if_active"
	ConcurrencyPolicyAlwaysCreate     RoutineConcurrencyPolicy = "always_create"
)

// String implements fmt.Stringer.
func (p RoutineConcurrencyPolicy) String() string { return string(p) }

// RoutineCatchUpPolicy controls what happens when a scheduled routine
// missed fires (e.g. backend was down).
type RoutineCatchUpPolicy string

// Routine catch-up policy values. The DB default is
// `enqueue_missed_with_cap`; the alternative skips missed fires entirely.
const (
	CatchUpPolicyEnqueueMissedWithCap RoutineCatchUpPolicy = "enqueue_missed_with_cap"
	CatchUpPolicySkipMissed           RoutineCatchUpPolicy = "skip_missed"
)

// String implements fmt.Stringer.
func (p RoutineCatchUpPolicy) String() string { return string(p) }

// BudgetScopeType selects what a BudgetPolicy applies to.
type BudgetScopeType string

// Budget scope-type values.
const (
	BudgetScopeAgent     BudgetScopeType = "agent"
	BudgetScopeProject   BudgetScopeType = "project"
	BudgetScopeWorkspace BudgetScopeType = "workspace"
)

// String implements fmt.Stringer.
func (s BudgetScopeType) String() string { return string(s) }

// Valid reports whether s is one of the declared BudgetScopeType values.
// Use at trust boundaries (HTTP handlers, config loaders) to reject
// malformed input before it reaches the cost service.
func (s BudgetScopeType) Valid() bool {
	switch s {
	case BudgetScopeAgent, BudgetScopeProject, BudgetScopeWorkspace:
		return true
	}
	return false
}

// BudgetPeriod is the time window a BudgetPolicy limit applies to.
type BudgetPeriod string

// Budget period values.
const (
	BudgetPeriodDaily   BudgetPeriod = "daily"
	BudgetPeriodMonthly BudgetPeriod = "monthly"
	BudgetPeriodYearly  BudgetPeriod = "yearly"
)

// String implements fmt.Stringer.
func (p BudgetPeriod) String() string { return string(p) }

// Valid reports whether p is one of the declared BudgetPeriod values.
func (p BudgetPeriod) Valid() bool {
	switch p {
	case BudgetPeriodDaily, BudgetPeriodMonthly, BudgetPeriodYearly:
		return true
	}
	return false
}

// BudgetActionOnExceed is the side-effect a BudgetPolicy triggers on
// limit breach.
type BudgetActionOnExceed string

// Budget action values. The set matches what the cost service writes;
// see internal/office/costs/budgets.go.
const (
	BudgetActionPauseAgent    BudgetActionOnExceed = "pause_agent"
	BudgetActionBlockNewTasks BudgetActionOnExceed = "block_new_tasks"
	BudgetActionNotifyOnly    BudgetActionOnExceed = "notify_only"
)

// String implements fmt.Stringer.
func (a BudgetActionOnExceed) String() string { return string(a) }

// Valid reports whether a is one of the declared BudgetActionOnExceed values.
func (a BudgetActionOnExceed) Valid() bool {
	switch a {
	case BudgetActionPauseAgent, BudgetActionBlockNewTasks, BudgetActionNotifyOnly:
		return true
	}
	return false
}

// ProviderHealthScope identifies what slice of a provider a
// ProviderHealth row covers.
type ProviderHealthScope string

// Provider-health scope values. The triple
// (workspace_id, provider_id, scope, scope_value) is the PK so a
// tier-specific failure does not take the whole provider down.
const (
	ProviderHealthScopeProvider ProviderHealthScope = "provider"
	ProviderHealthScopeTier     ProviderHealthScope = "tier"
	ProviderHealthScopeModel    ProviderHealthScope = "model"
)

// String implements fmt.Stringer.
func (s ProviderHealthScope) String() string { return string(s) }

// ProviderHealthState is the current eligibility state of a
// (workspace, provider, scope) tuple.
type ProviderHealthState string

// Provider-health state values. user_action_required is the
// "blocked, won't auto-recover" terminal state; degraded is auto-retryable.
const (
	ProviderHealthHealthy            ProviderHealthState = "healthy"
	ProviderHealthDegraded           ProviderHealthState = "degraded"
	ProviderHealthUserActionRequired ProviderHealthState = "user_action_required"
)

// String implements fmt.Stringer.
func (s ProviderHealthState) String() string { return string(s) }

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

// RouteAttemptOutcome is the result of one route attempt persisted to
// office_route_attempts. See internal/office/scheduler/dispatch_routing.go
// for the lifecycle.
type RouteAttemptOutcome string

// Route attempt outcome values.
const (
	RouteAttemptOutcomeLaunched              RouteAttemptOutcome = "launched"
	RouteAttemptOutcomeFailedProviderUnavail RouteAttemptOutcome = "failed_provider_unavailable"
	RouteAttemptOutcomeFailedOther           RouteAttemptOutcome = "failed_other"
	RouteAttemptOutcomeSkippedDegraded       RouteAttemptOutcome = "skipped_degraded"
	RouteAttemptOutcomeSkippedUserAction     RouteAttemptOutcome = "skipped_user_action"
	RouteAttemptOutcomeSkippedMissingMapping RouteAttemptOutcome = "skipped_missing_mapping"
	RouteAttemptOutcomeMaxAttempts           RouteAttemptOutcome = "skipped_max_attempts"
)

// String implements fmt.Stringer.
func (o RouteAttemptOutcome) String() string { return string(o) }

// AdapterPhase narrows the lifecycle stage at which a routing error
// surfaced. Mirrors routingerr.Phase; redeclared so office-tier types
// have no dependency on the runtime tier.
type AdapterPhase string

// Adapter phase values. Source of truth:
// internal/agent/runtime/routingerr/routingerr.go.
const (
	AdapterPhaseAuthCheck     AdapterPhase = "auth_check"
	AdapterPhaseProcessStart  AdapterPhase = "process_start"
	AdapterPhaseSessionInit   AdapterPhase = "session_init"
	AdapterPhasePromptSend    AdapterPhase = "prompt_send"
	AdapterPhaseStreaming     AdapterPhase = "streaming"
	AdapterPhaseToolExecution AdapterPhase = "tool_execution"
	AdapterPhaseShutdown      AdapterPhase = "shutdown"
)

// String implements fmt.Stringer.
func (p AdapterPhase) String() string { return string(p) }

// ErrorConfidence is the classifier's confidence in its verdict.
// Mirrors routingerr.Confidence.
type ErrorConfidence string

// Error confidence values.
const (
	ErrorConfidenceHigh   ErrorConfidence = "high"
	ErrorConfidenceMedium ErrorConfidence = "medium"
	ErrorConfidenceLow    ErrorConfidence = "low"
)

// String implements fmt.Stringer.
func (c ErrorConfidence) String() string { return string(c) }

// SkillSourceType identifies where a Skill row's content came from.
// Mirrors the TS union in apps/web/lib/state/slices/office/types.ts.
type SkillSourceType string

// Skill source-type values.
const (
	SkillSourceTypeInline    SkillSourceType = "inline"
	SkillSourceTypeLocalPath SkillSourceType = "local_path"
	SkillSourceTypeGit       SkillSourceType = "git"
	SkillSourceTypeSkillsSh  SkillSourceType = "skills_sh"
	SkillSourceTypeUserHome  SkillSourceType = "user_home"
	SkillSourceTypeSystem    SkillSourceType = "system"
)

// String implements fmt.Stringer.
func (s SkillSourceType) String() string { return string(s) }

// SkillApprovalState gates whether a Skill row is usable.
type SkillApprovalState string

// Skill approval-state values.
const (
	SkillApprovalStatePending  SkillApprovalState = "pending"
	SkillApprovalStateApproved SkillApprovalState = "approved"
	SkillApprovalStateRejected SkillApprovalState = "rejected"
)

// String implements fmt.Stringer.
func (s SkillApprovalState) String() string { return string(s) }

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

// ChannelPlatform is the external service a Channel row relays to.
// Open set — new adapters add new platforms.
type ChannelPlatform string

// Known channel platforms.
const (
	ChannelPlatformSlack   ChannelPlatform = "slack"
	ChannelPlatformWebhook ChannelPlatform = "webhook"
)

// String implements fmt.Stringer.
func (p ChannelPlatform) String() string { return string(p) }

// ChannelStatus is the operational state of a Channel row. Free-form;
// typed for documentation.
type ChannelStatus string

// Known channel statuses.
const (
	ChannelStatusActive       ChannelStatus = "active"
	ChannelStatusDisconnected ChannelStatus = "disconnected"
	ChannelStatusError        ChannelStatus = "error"
)

// String implements fmt.Stringer.
func (s ChannelStatus) String() string { return string(s) }

// ActivityActorType identifies who emitted an ActivityEntry. Open set
// (typed for documentation): user | agent | system.
type ActivityActorType string

// Known activity actor types.
const (
	ActivityActorUser   ActivityActorType = "user"
	ActivityActorAgent  ActivityActorType = "agent"
	ActivityActorSystem ActivityActorType = "system"
)

// String implements fmt.Stringer.
func (a ActivityActorType) String() string { return string(a) }

// ActivityTargetType identifies what an ActivityEntry happened to.
// Free-form; typed for documentation.
type ActivityTargetType string

// Known activity target types (subset; new domains add new values).
const (
	ActivityTargetAgent   ActivityTargetType = "agent"
	ActivityTargetTask    ActivityTargetType = "task"
	ActivityTargetProject ActivityTargetType = "project"
	ActivityTargetBudget  ActivityTargetType = "budget"
	ActivityTargetSkill   ActivityTargetType = "skill"
	ActivityTargetChannel ActivityTargetType = "channel"
	ActivityTargetRoutine ActivityTargetType = "routine"
)

// String implements fmt.Stringer.
func (t ActivityTargetType) String() string { return string(t) }

// ActivityAction is the verb describing an ActivityEntry (free-form,
// dotted convention e.g. "budget.alert", "agent.hired").
type ActivityAction string

// String implements fmt.Stringer.
func (a ActivityAction) String() string { return string(a) }
