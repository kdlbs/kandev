package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// Run reason constants. Aliases of shared's canonical declarations
// (AC-OFFICE-BACKPRESSURE-001.8) — see shared/runreasons.go.
const (
	RunReasonTaskAssigned          = shared.RunReasonTaskAssigned
	RunReasonTaskComment           = shared.RunReasonTaskComment
	RunReasonTaskBlockersResolved  = shared.RunReasonTaskBlockersResolved
	RunReasonTaskChildrenCompleted = shared.RunReasonTaskChildrenCompleted
	RunReasonApprovalResolved      = shared.RunReasonApprovalResolved
	RunReasonTaskReviewRequested   = shared.RunReasonTaskReviewRequested
	RunReasonTaskChangesRequested  = shared.RunReasonTaskChangesRequested
	RunReasonRoutineTrigger        = shared.RunReasonRoutineTrigger
	RunReasonHeartbeat             = shared.RunReasonHeartbeat
	RunReasonBudgetAlert           = shared.RunReasonBudgetAlert
	RunReasonAgentError            = shared.RunReasonAgentError
)

// These reasons were persisted by earlier workflow templates. Keep them
// readable so existing materialized workflows continue to wake the correct
// prompt after the built-in template changes.
const (
	legacyRunReasonBlockersResolved  = shared.RunReasonLegacyBlockersResolved
	legacyRunReasonChildrenCompleted = shared.RunReasonLegacyChildrenCompleted
	legacyRunReasonReviewStarted     = shared.RunReasonLegacyReviewStarted
	legacyRunReasonApprovalStarted   = shared.RunReasonLegacyApprovalStarted
)

// Run status constants.
const (
	RunStatusQueued    = "queued"
	RunStatusClaimed   = "claimed"
	RunStatusFinished  = "finished"
	RunStatusFailed    = "failed"
	RunStatusCancelled = "cancelled"
)

// Run outcome constants (docs/specs/task-delivery-ledger/spec.md, "Office run
// outcome"). Written into runs.outcome alongside status='finished' at each of
// the six terminal call sites; NULL on the failed path and on every
// pre-activation row. RunOutcomeProcessed is the only value
// RunCountsByDayForAgent counts as succeeded. Every other value buckets into
// skipped.
const (
	RunOutcomeProcessed     = "processed"
	RunOutcomeBudgetBlocked = "budget_blocked"
	RunOutcomeIdleSkipped   = "idle_skipped"
	RunOutcomeAgentInactive = "agent_inactive"
	RunOutcomeTaskTreeHeld  = "task_tree_held"
)

// CoalesceWindowSeconds is the default coalescing window.
const CoalesceWindowSeconds = 5

// IdempotencyWindowHours is the deduplication window.
const IdempotencyWindowHours = 24

// QueueRun enqueues a run request for an agent instance, attributed to
// the system actor and rooting a new causation chain. It exists for the
// RunQueuer/RunSpawner shared interfaces and their existing callers/mocks,
// which predate the actor contract (AC-OFFICE-RUN-CAUSATION-001.15) and are
// out of scope to widen here. New call sites that know their actor and/or
// causing run should call QueueRunWithActor directly instead.
func (s *Service) QueueRun(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	return s.QueueRunWithActor(ctx, agentInstanceID, reason, payload, idempotencyKey, models.ActorKindSystem, "", "")
}

// QueueRunWithActor enqueues a run request for an agent instance.
// It checks agent status, idempotency, and attempts coalescing before inserting.
//
// causingRunID is the run this enqueue happened inside, empty for a root
// cause (AC-OFFICE-RUN-CAUSATION-001.2/.3): when set, the resolved
// causation identifier, parent run identifier, and depth chain from that
// run instead of rooting a new one.
//
// When a runs service is wired (via SetRunsService) the insert +
// publish + scheduler signal are delegated to it so the engine and
// office paths share one queue implementation. The agent status guard
// stays here because it depends on office-specific tables.
func (s *Service) QueueRunWithActor(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
	actorKind models.ActorKind, actorID string,
	causingRunID string,
) error {
	if err := s.guardAgentStatus(ctx, agentInstanceID); err != nil {
		return err
	}

	if s.runsService != nil {
		_, err := s.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         reason,
			IdempotencyKey: idempotencyKey,
			Payload:        PayloadWithAgent(payload, agentInstanceID),
			ActorKind:      actorKind,
			ActorID:        actorID,
			CausingRunID:   causingRunID,
		})
		return err
	}
	return s.queueRunInline(ctx, agentInstanceID, reason, payload, idempotencyKey)
}

// QueueRunFromTaskBoundary enqueues a run attributed to the task-boundary
// carrier read off taskID's metadata (AC-OFFICE-RUN-CAUSATION-001.5/.18):
// the carrier's actor and lineage stand in for a live causing run,
// without requiring a read of the run that created the task. A taskID
// whose metadata carries no carrier at all (the common case: the task
// was never created by an Office trigger) resolves to the same
// system-actor root behaviour QueueRun already provides.
func (s *Service) QueueRunFromTaskBoundary(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey, taskID string,
) error {
	if err := s.guardAgentStatus(ctx, agentInstanceID); err != nil {
		return err
	}

	if s.runsService != nil {
		carrier := s.TaskBoundaryCarrier(ctx, taskID)
		humanRooted := carrier.HumanRooted
		_, err := s.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:                reason,
			IdempotencyKey:        idempotencyKey,
			Payload:               PayloadWithAgent(payload, agentInstanceID),
			ActorKind:             carrier.ActorKind,
			ActorID:               carrier.ActorID,
			RoutineID:             carrier.RoutineID,
			CarrierHumanRooted:    &humanRooted,
			CarrierCreatingRunID:  carrier.CreatingRunID,
			CarrierCausationID:    carrier.CausationID,
			CarrierCausationDepth: carrier.CausationDepth,
		})
		return err
	}
	return s.queueRunInline(ctx, agentInstanceID, reason, payload, idempotencyKey)
}

// QueueRunFromWakeup enqueues a taskless run on behalf of the wakeup
// dispatcher (AC-OFFICE-ENQUEUE-CONSOLIDATION-001.2): the dispatcher's own
// three-layer coalesce model (source-level dedup, claim-time merge into an
// in-flight run, concurrency-policy gating) already decided a fresh run is
// needed before calling this, so QueueRun's own independent coalescing
// window is skipped (AC-OFFICE-ENQUEUE-CONSOLIDATION-001.5) — running both
// could pick a different in-flight run to merge into than the one the
// dispatcher already checked against. Idempotency, causation resolution
// (priority class, actor, workspace, depth/self-trigger gates), and the
// insert all still run through the one authoritative seam, unlike the
// direct repository insert this replaced.
//
// Every wakeup-sourced run is system-actuated (routine fire, heartbeat, or
// any other wakeup source) per the design's actor-source table, so
// ActorKind is always models.ActorKindSystem here.
//
// Returns the created run's id so the caller can mark its own
// wakeup-request row claimed against it. Requires a wired runs service —
// there is no inline fallback insert here (AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6):
// a caller with no runs service must fail its enqueue rather than bypass
// the seam it exists to protect.
func (s *Service) QueueRunFromWakeup(ctx context.Context, agentProfileID, reason, routineID, contextSnapshot string) (string, error) {
	if err := s.guardAgentStatus(ctx, agentProfileID); err != nil {
		return "", err
	}
	if s.runsService == nil {
		return "", fmt.Errorf("queue run from wakeup: runs service not configured")
	}
	_, row, err := s.runsService.QueueRunAndReturn(ctx, runsservice.QueueRunRequest{
		Reason:          reason,
		Payload:         PayloadWithAgent("{}", agentProfileID),
		ActorKind:       models.ActorKindSystem,
		RoutineID:       routineID,
		ContextSnapshot: contextSnapshot,
		SkipCoalesce:    true,
	})
	if err != nil {
		return "", err
	}
	if row == nil {
		return "", fmt.Errorf("queue run from wakeup: no row returned for agent %s", agentProfileID)
	}
	return row.ID, nil
}

// TaskBoundaryCarrier reads and validates the causation carrier off
// taskID's metadata. A metadata read failure (task not found, transient
// error) is treated the same as "no carrier": this lookup must never
// block the enqueue it's attached to.
func (s *Service) TaskBoundaryCarrier(ctx context.Context, taskID string) TaskBoundaryCarrier {
	metadata, err := s.repo.GetTaskMetadata(ctx, taskID)
	if err != nil {
		return TaskBoundaryCarrier{}
	}
	return carrierFromTaskMetadata(metadata)
}

// TaskBoundaryCarrierMetadata resolves the causation carrier a task
// created from taskID (e.g. the workflow engine's create_child_task
// action) should carry, in the map shape CreateTaskRequest/ChildTaskSpec's
// OfficeCarrierMetadata field expects.
//
// taskID is the action's own task, not a fixed "root" — create_child_task
// can chain, so the carrier must advance one hop deeper each time or the
// depth ceiling never engages. The run currently claimed against taskID
// (the run executing this action's turn) is that hop: its own lineage is
// carried forward directly via carrierMetadataFromRun, the same way
// CreateOfficeSubtaskAsAgent resolves a live causing run. Only when no run
// is claimed against taskID — a genuinely sessionless trigger — does this
// fall back to forwarding taskID's own already-resolved carrier verbatim,
// which cannot advance depth on its own.
func (s *Service) TaskBoundaryCarrierMetadata(ctx context.Context, taskID string) map[string]interface{} {
	if run, err := s.repo.GetClaimedRunByTaskID(ctx, taskID); err == nil && run != nil {
		return carrierMetadataFromRun(run)
	}
	return carrierMetadataFromCarrier(s.TaskBoundaryCarrier(ctx, taskID))
}

// TaskBoundaryCarrierForRunQueue resolves the causation carrier a run
// queued because of taskID should carry, applying the same live-run
// preference as TaskBoundaryCarrierMetadata: the run currently claimed
// against taskID (the turn actually queuing this run, e.g. the workflow
// engine's queue_run action) wins over taskID's own already-resolved
// carrier, so depth keeps advancing hop by hop across a chain instead of
// freezing at the task's original creating-run carrier. Only when no run
// is claimed against taskID does this fall back to taskID's own carrier.
func (s *Service) TaskBoundaryCarrierForRunQueue(ctx context.Context, taskID string) TaskBoundaryCarrier {
	if run, err := s.repo.GetClaimedRunByTaskID(ctx, taskID); err == nil && run != nil {
		return carrierFromRun(run)
	}
	return s.TaskBoundaryCarrier(ctx, taskID)
}

// queueRunInline performs the legacy in-office insert path used when
// no runs service is wired (older tests, transitional deployments).
// Behaviour matches the pre-Phase-3 implementation.
func (s *Service) queueRunInline(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) error {
	if idempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKey(ctx, idempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			s.logger.Debug("run skipped (idempotent)",
				zap.String("key", idempotencyKey))
			return nil
		}
	}

	coalesced, err := s.repo.CoalesceRun(ctx, agentInstanceID, reason, CoalesceWindowSeconds, payload)
	if err != nil {
		return fmt.Errorf("coalesce check: %w", err)
	}
	if coalesced {
		s.logger.Debug("run coalesced",
			zap.String("agent", agentInstanceID),
			zap.String("reason", reason))
		return nil
	}

	var idemKeyPtr *string
	if idempotencyKey != "" {
		idemKeyPtr = &idempotencyKey
	}
	req := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: agentInstanceID,
		Reason:         reason,
		Payload:        payload,
		Status:         RunStatusQueued,
		CoalescedCount: 1,
		IdempotencyKey: idemKeyPtr,
		RequestedAt:    time.Now().UTC(),
		// This fallback path (no runs service wired) bypasses
		// runs/service.resolveCausation, so PriorityClass must be stamped
		// here or it silently ships as models.PriorityClass's Go zero
		// value — PriorityClassHuman (0), not PriorityClassEvent (2) —
		// falsely promoting every such run to the highest claim-order
		// preference (AC-OFFICE-BACKPRESSURE-001.1/.3). See the matching
		// comment in office/scheduler.QueueRun, which has the same gap.
		PriorityClass: shared.ClassifyPriority(models.ActorKindSystem, reason, false),
	}
	if err := s.repo.CreateRun(ctx, req); err != nil {
		return fmt.Errorf("enqueue run: %w", err)
	}

	s.logger.Info("run queued",
		zap.String("id", req.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", reason))

	s.publishRunQueued(ctx, req, idempotencyKey)
	return nil
}

// PayloadWithAgent decodes the JSON payload string and adds the
// agent_profile_id field so the runs service can resolve the
// instance without a separate resolver. The runs queue's payload
// column is JSON, so re-injecting the field here keeps the row shape
// identical to the legacy office.QueueRun insert. Exported so
// office/scheduler.SchedulerService.QueueRun can delegate through the
// same seam without duplicating this decode-and-inject step.
func PayloadWithAgent(payload, agentInstanceID string) map[string]any {
	out := map[string]any{}
	if payload != "" {
		_ = json.Unmarshal([]byte(payload), &out)
	}
	out["agent_profile_id"] = agentInstanceID
	return out
}

// publishRunQueued emits an OfficeRunQueued bus event so the WS
// gateway can fan it out to subscribed clients. Defensive: skips when
// the bus is not configured. Publish errors are logged at debug and
// swallowed — the queue write already succeeded by the time we get
// here.
func (s *Service) publishRunQueued(ctx context.Context, req *models.Run, idempotencyKey string) {
	if s.eb == nil {
		return
	}
	taskID, commentID := commentkeys.IdentityFromPayload(req.Payload)
	data := map[string]interface{}{
		"run_id":           req.ID,
		"agent_profile_id": req.AgentProfileID,
		"reason":           req.Reason,
		"task_id":          taskID,
		"comment_id":       commentID,
		"idempotency_key":  idempotencyKey,
	}
	event := bus.NewEvent(events.OfficeRunQueued, "office-service", data)
	if err := s.eb.Publish(ctx, events.OfficeRunQueued, event); err != nil {
		s.logger.Debug("publish run queued event failed",
			zap.String("run_id", req.ID),
			zap.Error(err))
	}
}

// guardAgentStatus returns an error if the agent is paused or stopped.
func (s *Service) guardAgentStatus(ctx context.Context, agentInstanceID string) error {
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return nil
}
