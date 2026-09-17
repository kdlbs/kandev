package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
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
// the eight terminal call sites (the original six, plus the pause gate's
// early and final checks in scheduler_integration.go); NULL on the failed
// path and on every pre-activation row. RunOutcomeProcessed is the only
// value RunCountsByDayForAgent counts as succeeded. Every other value
// buckets into skipped.
const (
	RunOutcomeProcessed          = "processed"
	RunOutcomeBudgetBlocked      = "budget_blocked"
	RunOutcomeIdleSkipped        = "idle_skipped"
	RunOutcomeAgentInactive      = "agent_inactive"
	RunOutcomeTaskTreeHeld       = "task_tree_held"
	RunOutcomeBudgetUnmeasurable = "budget_unmeasurable"
	RunOutcomeWorkspacePaused    = "workspace_paused"
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
) (shared.QueueOutcome, error) {
	outcome, err := s.QueueRunWithActor(ctx, agentInstanceID, reason, payload, idempotencyKey, models.ActorKindSystem, "", "")
	return shared.QueueOutcome(outcome), err
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
) (runsservice.QueueOutcome, error) {
	agent, err := s.guardAgentStatus(ctx, agentInstanceID)
	if err != nil {
		return runsservice.QueueOutcomeNone, err
	}
	if err := s.checkPauseGateForAgent(ctx, agent, "queue_run"); err != nil {
		return runsservice.QueueOutcomeNone, err
	}

	if s.runsService != nil {
		return s.runsService.QueueRun(ctx, runsservice.QueueRunRequest{
			Reason:         reason,
			IdempotencyKey: idempotencyKey,
			Payload:        PayloadWithAgent(payload, agentInstanceID),
			ActorKind:      actorKind,
			ActorID:        actorID,
			CausingRunID:   causingRunID,
		})
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
	agent, err := s.guardAgentStatus(ctx, agentInstanceID)
	if err != nil {
		return err
	}
	if err := s.checkPauseGateForAgent(ctx, agent, "queue_run"); err != nil {
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
	_, err = s.queueRunInline(ctx, agentInstanceID, reason, payload, idempotencyKey)
	return err
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
//
// causationID is copied verbatim onto the created run's own CausationID
// column (AC-OFFICE-LOOP-LIVENESS-002.3) — the wakeup-request's own
// causation id, not this package's causation-chain resolution.
func (s *Service) QueueRunFromWakeup(
	ctx context.Context, agentProfileID, reason, routineID, contextSnapshot, causationID string,
) (string, error) {
	// An agent the dispatcher can no longer find must not block this
	// enqueue: the wakeup dispatcher's own pause-gate check
	// (checkPauseGate in office/wakeup) already treats a missing agent
	// as "proceed ungated" rather than a gate failure, and this status
	// guard must agree rather than re-introduce the block one layer
	// down. Any other guardAgentStatus error (paused/stopped/pending
	// approval, or a transient lookup fault) still fails closed.
	if _, err := s.guardAgentStatus(ctx, agentProfileID); err != nil && !errors.Is(err, sql.ErrNoRows) {
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
		CausationID:     causationID,
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
// by causingAgentProfileID (the agent executing this action's turn) is
// that hop: its own lineage is carried forward directly via
// carrierMetadataFromRun, the same way CreateOfficeSubtaskAsAgent resolves
// a live causing run. Scoping the claimed-run lookup to
// causingAgentProfileID (rather than taking whichever run was claimed
// most recently against the task) matters because more than one agent can
// hold a claimed run on the same task at once. When
// causingAgentProfileID is empty, or holds no claimed run on taskID, this
// falls back to forwarding taskID's own already-resolved carrier verbatim,
// which cannot advance depth on its own.
func (s *Service) TaskBoundaryCarrierMetadata(ctx context.Context, taskID, causingAgentProfileID string) map[string]interface{} {
	if causingAgentProfileID != "" {
		if run, err := s.repo.GetClaimedRunByTaskAndAgent(ctx, taskID, causingAgentProfileID); err == nil && run != nil {
			return carrierMetadataFromRun(run)
		}
	}
	return carrierMetadataFromCarrier(s.TaskBoundaryCarrier(ctx, taskID))
}

// TaskBoundaryCarrierForRunQueue resolves the causation carrier a run
// queued because of taskID should carry, applying the same live-run
// preference as TaskBoundaryCarrierMetadata: the run currently claimed
// against taskID by causingAgentProfileID (the turn actually queuing this
// run, e.g. the workflow engine's queue_run action) wins over taskID's
// own already-resolved carrier, so depth keeps advancing hop by hop
// across a chain instead of freezing at the task's original creating-run
// carrier. Scoping to causingAgentProfileID keeps this unambiguous when
// more than one agent holds a claimed run on the same task. When
// causingAgentProfileID is empty, or holds no claimed run on taskID, this
// falls back to taskID's own carrier.
func (s *Service) TaskBoundaryCarrierForRunQueue(ctx context.Context, taskID, causingAgentProfileID string) TaskBoundaryCarrier {
	if causingAgentProfileID != "" {
		if run, err := s.repo.GetClaimedRunByTaskAndAgent(ctx, taskID, causingAgentProfileID); err == nil && run != nil {
			return carrierFromRun(run)
		}
	}
	return s.TaskBoundaryCarrier(ctx, taskID)
}

// queueRunInline performs the legacy in-office insert path used when
// no runs service is wired (older tests, transitional deployments).
// Behaviour matches the pre-Phase-3 implementation.
func (s *Service) queueRunInline(
	ctx context.Context,
	agentInstanceID, reason, payload, idempotencyKey string,
) (runsservice.QueueOutcome, error) {
	if idempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKey(ctx, idempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return runsservice.QueueOutcomeNone, fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			return runsservice.ReportWindowedDedup(runsservice.QueueSourceRuns, reason, idempotencyKey), nil
		}
	}

	coalesced, err := s.repo.CoalesceRun(ctx, agentInstanceID, reason, CoalesceWindowSeconds, payload)
	if err != nil {
		return runsservice.QueueOutcomeNone, fmt.Errorf("coalesce check: %w", err)
	}
	if coalesced {
		s.logger.Debug("run coalesced",
			zap.String("agent", agentInstanceID),
			zap.String("reason", reason))
		return runsservice.QueueOutcomeCoalesced, nil
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
	insertErr := s.repo.CreateRun(ctx, req)
	outcome, err := runsservice.ReportInsertResult(runsservice.QueueSourceRuns, reason, idempotencyKey, agentInstanceID, insertErr)
	if err != nil {
		return runsservice.QueueOutcomeNone, fmt.Errorf("enqueue run: %w", err)
	}
	if outcome == runsservice.QueueOutcomeDeduped {
		return outcome, nil
	}

	s.logger.Info("run queued",
		zap.String("id", req.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", reason))

	s.publishRunQueued(ctx, req, idempotencyKey)
	return runsservice.QueueOutcomeQueued, nil
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

// guardAgentStatus returns an error if the agent is paused or stopped,
// and otherwise the resolved agent — callers that also need the pause
// gate's workspace scope (checkPauseGateForAgent) reuse this fetch instead
// of looking the agent up a second time.
func (s *Service) guardAgentStatus(ctx context.Context, agentInstanceID string) (*models.AgentInstance, error) {
	agent, err := s.GetAgentFromConfig(ctx, agentInstanceID)
	if err != nil {
		return nil, fmt.Errorf("get agent instance: %w", err)
	}
	switch agent.Status {
	case models.AgentStatusPaused:
		return nil, fmt.Errorf("agent %s is paused", agentInstanceID)
	case models.AgentStatusStopped:
		return nil, fmt.Errorf("agent %s is stopped", agentInstanceID)
	case models.AgentStatusPendingApproval:
		return nil, fmt.Errorf("agent %s is pending approval", agentInstanceID)
	}
	return agent, nil
}

// pauseGateState reads the workspace-pause gate directly (by workspace
// id, not agent id — used by scheduler_integration.go's run-processing
// gates, which already have the agent and its WorkspaceID in hand).
// The returned bool is true only for a confirmed pause; err is non-nil
// only on a gate-read failure. When s.pauseGate is nil (not wired) it
// always reports (false, nil) so dispatch proceeds ungated.
func (s *Service) pauseGateState(ctx context.Context, workspaceID string) (bool, error) {
	if s.pauseGate == nil {
		return false, nil
	}
	active, err := s.pauseGate.PauseState(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return active != nil, nil
}

// checkPauseGateForAgent blocks queuing when agent's workspace is paused
// (the operator kill switch). Takes the already-resolved agent — usually
// guardAgentStatus's return value — rather than re-resolving it, so the
// two checks can never see two different snapshots of the agent's
// workspace. Fails closed on a gate-read error
// (shared.ErrPauseGateUnavailable) — this write hasn't happened yet, so
// there is nothing to leave in a retryable state beyond simply not writing
// it; the caller's own retry (or the next event) tries again.
func (s *Service) checkPauseGateForAgent(ctx context.Context, agent *models.AgentInstance, gateName string) error {
	if s.pauseGate == nil {
		return nil
	}
	active, err := s.pauseGate.PauseState(ctx, agent.WorkspaceID)
	if err != nil {
		pause.RecordGateError(gateName)
		s.logger.Warn("queue run: pause gate read failed",
			zap.String("agent", agent.ID), zap.Error(err))
		return shared.ErrPauseGateUnavailable
	}
	if active != nil {
		pause.RecordBlocked(gateName)
		return shared.ErrWorkspacePaused
	}
	return nil
}
