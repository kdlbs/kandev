package wakeup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
)

// Concurrency policy values stored on office_routines.concurrency_policy.
// The agent-level heartbeat columns that used to back these values were
// retired alongside the agent-heartbeat cron — every scheduled wake now
// flows through a routine, so the routine row is the policy authority
// for source="routine" wakeups, and the dispatcher defaults to
// coalesce_if_active for every other source.
const (
	PolicyCoalesceIfActive = "coalesce_if_active"
	PolicyAlwaysEnqueue    = "always_enqueue"
	PolicySkipIfActive     = "skip_if_active"
)

// AgentReader is retained for symmetry — the dispatcher no longer
// reads per-agent policy fields, but downstream callers may still want
// to look up the agent for context. The office sqlite repo's
// GetAgentInstance method satisfies this directly.
type AgentReader interface {
	GetAgentInstance(ctx context.Context, id string) (*models.AgentInstance, error)
}

// RoutineLookup is the slim interface the dispatcher uses to look up
// per-routine concurrency policy when processing a routine-sourced
// wakeup-request. Defined here (not in the routines package) to keep the
// dependency direction wakeup ← routines: the dispatcher must not import
// routines, but routines must satisfy this contract. The office sqlite
// repo's GetRoutine method satisfies this directly.
type RoutineLookup interface {
	GetRoutine(ctx context.Context, id string) (*models.Routine, error)
}

// Dispatcher processes one queued wakeup-request at a time, applying
// the three-layer coalesce model from the office-heartbeat-rework spec:
//
//  1. Source-level dedup happens on insert (UNIQUE idempotency_key).
//  2. Claim-time merge: when an in-flight run exists for the same
//     agent, mark this request "coalesced" + bump the run's
//     coalesced_count + JSON-merge this request's payload into the
//     existing run's context_snapshot.
//  3. Otherwise, create a fresh runs row (taskless: payload.task_id
//     empty, agent_profile_id set, reason = the request's reason or
//     source) and mark the request claimed.
//
// Concurrency policy applies BEFORE step 2:
//   - skip_if_active: any in-flight run → mark request skipped.
//   - coalesce_if_active: behave as step 2 (default).
//   - always_enqueue: skip step 2, always create a fresh run.
type Dispatcher struct {
	repo     *officesqlite.Repository
	agents   AgentReader
	routines RoutineLookup
	log      *logger.Logger
}

// NewDispatcher builds a Dispatcher. log MUST be non-nil; the agents
// reader and repo are required. Routine lookup is optional — when nil
// the dispatcher falls back to coalesce_if_active for routine-sourced
// wakeups (the safe default).
func NewDispatcher(
	repo *officesqlite.Repository,
	agents AgentReader,
	log *logger.Logger,
) *Dispatcher {
	return &Dispatcher{
		repo:     repo,
		agents:   agents,
		routines: nil,
		log:      log.WithFields(zap.String("component", "wakeup-dispatcher")),
	}
}

// SetRoutineLookup wires a RoutineLookup so the dispatcher can resolve
// per-routine concurrency policy for source="routine" wakeups. Optional
// — when not set the dispatcher uses coalesce_if_active for routines.
// Callers wire this after the routines service is constructed (the
// routines service depends on the office repo, which the dispatcher
// already holds — so the cycle is broken by setting this post-build).
func (d *Dispatcher) SetRoutineLookup(routines RoutineLookup) {
	d.routines = routines
}

// Dispatch processes one wakeup-request by id. The flow is:
//
//  1. Load the request; bail if it's not queued.
//  2. Resolve the agent's concurrency policy (heartbeat for source=
//     "heartbeat"; routine for source="routine" — defaulted to
//     coalesce_if_active until PR 3 wires routine-level policies; for
//     all other sources the agent's heartbeat_concurrency is used).
//  3. Look up an in-flight run for the agent.
//  4. Apply the policy:
//     - skip_if_active + in-flight → mark skipped, return.
//     - coalesce_if_active + in-flight → mark coalesced, return.
//     - always_enqueue OR no in-flight → create fresh run, mark claimed.
//
// Errors that prevent persistence are returned to the caller; bookkeeping
// failures (logging the run event, etc.) are logged at warn and swallowed.
func (d *Dispatcher) Dispatch(ctx context.Context, requestID string) error {
	req, err := d.repo.GetWakeupRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("get wakeup request %s: %w", requestID, err)
	}
	if req.Status != officesqlite.WakeupStatusQueued {
		// Already processed (or skipped) — idempotent no-op.
		return nil
	}

	policy, err := d.resolvePolicy(ctx, req)
	if err != nil {
		return err
	}

	if policy == PolicyAlwaysEnqueue {
		return d.createFreshRun(ctx, req)
	}

	inflight, err := d.repo.FindInflightRunForAgent(ctx, req.AgentProfileID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("find inflight run: %w", err)
	}
	if inflight == nil {
		return d.createFreshRun(ctx, req)
	}

	switch policy {
	case PolicySkipIfActive:
		return d.repo.MarkWakeupRequestSkipped(ctx, req.ID, "policy:skip_if_active")
	case PolicyCoalesceIfActive:
		return d.repo.MarkWakeupRequestCoalesced(ctx, req.ID, inflight.ID)
	}
	// Unknown policies fall back to coalesce — the safest "do something"
	// behaviour. Surface a warning so misconfigured rows are visible.
	d.log.Warn("unknown wakeup concurrency policy; coalescing",
		zap.String("policy", policy),
		zap.String("agent_id", req.AgentProfileID))
	return d.repo.MarkWakeupRequestCoalesced(ctx, req.ID, inflight.ID)
}

// resolvePolicy returns the concurrency policy for a wakeup-request.
//
// For source="routine" it reads office_routines.concurrency_policy via
// the RoutinePayload's RoutineID. Every other source defaults to
// coalesce_if_active — there is no per-agent policy column anymore now
// that the agent-level heartbeat path is gone. A future spec can wire
// per-agent overrides if a concrete need emerges.
func (d *Dispatcher) resolvePolicy(
	ctx context.Context, req *officesqlite.WakeupRequest,
) (string, error) {
	if req.Source == SourceRoutine {
		return d.resolveRoutinePolicy(ctx, req)
	}
	return PolicyCoalesceIfActive, nil
}

// resolveRoutinePolicy reads the per-routine concurrency policy from
// office_routines via the routine_id carried on the wakeup request's
// payload. Falls back to coalesce_if_active when the lookup is not
// wired or the payload is malformed — the safe default.
//
// The dispatcher translates the routines package's legacy
// "always_create" enum value to the wakeup-layer "always_enqueue" so
// the policy switch in Dispatch handles both naming conventions
// without divergence.
func (d *Dispatcher) resolveRoutinePolicy(
	ctx context.Context, req *officesqlite.WakeupRequest,
) (string, error) {
	if d.routines == nil {
		return PolicyCoalesceIfActive, nil
	}
	var p RoutinePayload
	if err := UnmarshalPayload(req.Payload, &p); err != nil || p.RoutineID == "" {
		return PolicyCoalesceIfActive, nil
	}
	routine, err := d.routines.GetRoutine(ctx, p.RoutineID)
	if err != nil || routine == nil {
		return PolicyCoalesceIfActive, nil
	}
	return normaliseRoutinePolicy(routine.ConcurrencyPolicy), nil
}

// normaliseRoutinePolicy maps the routines package's legacy enum values
// onto the wakeup-layer enum so a single switch in Dispatch handles
// both naming conventions. Empty / unknown values default to coalesce.
//
// Legacy mapping: routines.ConcurrencyAlwaysCreate → "always_create"
// translates to PolicyAlwaysEnqueue here.
func normaliseRoutinePolicy(p string) string {
	switch p {
	case PolicySkipIfActive:
		return PolicySkipIfActive
	case PolicyAlwaysEnqueue, "always_create":
		return PolicyAlwaysEnqueue
	case PolicyCoalesceIfActive:
		return PolicyCoalesceIfActive
	}
	return PolicyCoalesceIfActive
}

// createFreshRun inserts a new runs row for the wakeup-request and marks
// the request claimed against it. The run is taskless (payload.task_id
// is omitted) and carries agent_profile_id + reason directly.
//
// Reason: prefer req.Reason; fall back to req.Source so the run is
// always tagged with something useful (e.g. "heartbeat" / "comment").
// Payload: the wakeup-request's payload is copied verbatim onto the
// run's context_snapshot — the same JSON shape the agent sees when
// reading the run's bag of inputs.
func (d *Dispatcher) createFreshRun(
	ctx context.Context, req *officesqlite.WakeupRequest,
) error {
	reason := req.Reason
	if reason == "" {
		reason = req.Source
	}
	payload := req.Payload
	if payload == "" {
		payload = "{}"
	}
	run := &models.Run{
		ID:              uuid.New().String(),
		AgentProfileID:  req.AgentProfileID,
		Reason:          reason,
		Payload:         "{}",
		Status:          "queued",
		CoalescedCount:  1,
		ContextSnapshot: payload,
		RequestedAt:     time.Now().UTC(),
	}
	if err := d.repo.CreateRun(ctx, run); err != nil {
		return fmt.Errorf("create run for wakeup %s: %w", req.ID, err)
	}
	if err := d.repo.MarkWakeupRequestClaimed(ctx, req.ID, run.ID); err != nil {
		// Best-effort cleanup: the run already exists; the caller will
		// see a queued run without a corresponding wakeup-request claim,
		// which is harmless but logged for visibility.
		d.log.Warn("mark wakeup claimed failed (run already created)",
			zap.String("wakeup_id", req.ID),
			zap.String("run_id", run.ID),
			zap.Error(err))
		return err
	}
	d.log.Info("wakeup dispatched",
		zap.String("wakeup_id", req.ID),
		zap.String("run_id", run.ID),
		zap.String("agent_id", req.AgentProfileID),
		zap.String("source", req.Source),
		zap.String("reason", reason))
	return nil
}
