package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// RefusalGate names the gate that refused an enqueue
// (AC-OFFICE-LAUNCH-SAFETY-001.7, -003.4, -004.4): a refusal is
// enqueue-time, creates no row, and never consumes an idempotency key —
// distinct from a claim-time deferral, which leaves an already-queued
// row untouched.
type RefusalGate string

const (
	// RefusalWorkspaceMissing means the agent profile's workspace could
	// not be resolved (AC-OFFICE-RUN-CAUSATION-001.20).
	RefusalWorkspaceMissing RefusalGate = "workspace_missing"
	// RefusalCausingRunUnreadable means req.CausingRunID was set but the
	// row could not be read (AC-OFFICE-RUN-CAUSATION-001.21).
	RefusalCausingRunUnreadable RefusalGate = "causing_run_unreadable"
	// RefusalCausationDepth means the resolved causation depth exceeds
	// the effective ceiling (AC-OFFICE-LAUNCH-SAFETY-003.4).
	RefusalCausationDepth RefusalGate = "causation_depth"
	// RefusalSelfTrigger means the agent has re-triggered itself with the
	// same reason more than the effective allowance within
	// SelfTriggerWindow (AC-OFFICE-LAUNCH-SAFETY-004.4).
	RefusalSelfTrigger RefusalGate = "self_trigger"
)

// RefusalError is returned by insertRun (via resolveCausation) instead of
// inserting a row. Callers check errors.As, not string matching.
type RefusalError struct {
	Gate   RefusalGate
	Reason string
}

func (e *RefusalError) Error() string {
	return fmt.Sprintf("run enqueue refused (%s): %s", e.Gate, e.Reason)
}

// gateOutcomeRecorder accumulates gate-evaluation outcomes for
// resolveCausation and its helpers to flush after the enqueue
// transaction concludes, rather than writing them while that
// transaction is still open. RecordGateOutcome opens its own
// transaction on the same writer pool the enqueue transaction holds;
// on SQLite (capped at one writer connection) calling it inline would
// deadlock waiting for a connection the open enqueue transaction is
// itself holding. Deferring the write also keeps the outcome durable
// independent of whether the enqueue transaction that observed it
// commits or rolls back, matching AC-OFFICE-BACKPRESSURE-003.4: this
// diagnostic bookkeeping must never affect, or be undone by, the
// admission decision.
type gateOutcomeRecorder struct {
	outcomes []gateOutcome
}

type gateOutcome struct {
	workspaceID string
	gate        RefusalGate
	success     bool
}

func (r *gateOutcomeRecorder) record(workspaceID string, gate RefusalGate, success bool) {
	r.outcomes = append(r.outcomes, gateOutcome{workspaceID: workspaceID, gate: gate, success: success})
}

// causationResolution is the resolved causation-chain identity for a
// new run row, computed by resolveCausation before insertRun mints the
// row's own id.
type causationResolution struct {
	// CausationID is empty for a root cause; insertRun sets it to the
	// new row's own id in that case (AC-OFFICE-RUN-CAUSATION-001.2).
	CausationID    string
	ParentRunID    string
	CausationDepth int
	HumanRooted    bool
	RoutineID      string
	ActorKind      models.ActorKind
	ActorID        string
	WorkspaceID    string
	PriorityClass  models.PriorityClass
}

// resolveCausation implements the enqueue control flow of
// docs/specs/office/system-design/unattended-launch-safety-02.md: actor
// normalization, workspace resolution, causing-run resolution, depth and
// self-trigger refusal gates, and priority-class stamping. Every gate
// fails closed: an unreadable input refuses the enqueue rather than
// silently rooting or defaulting it. tx is the caller's enqueue
// transaction (AC-OFFICE-LAUNCH-SAFETY-003.8): every read this function
// and its helpers make runs against it, not the repository's separate
// reader connection, so the depth check and the self-trigger window
// count are serialized against the insert and against every other
// concurrent enqueue for the same agent profile.
func (s *Service) resolveCausation(
	ctx context.Context, tx *sqlx.Tx, rec *gateOutcomeRecorder, agentInstanceID string, req QueueRunRequest,
) (causationResolution, error) {
	actorKind, actorID := normalizeActor(req)

	workspaceID, err := s.repo.ResolveAgentProfileWorkspaceIDTx(ctx, tx, agentInstanceID)
	if err != nil || workspaceID == "" {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalWorkspaceMissing)), 1)
		s.logRefusal(RefusalWorkspaceMissing, agentInstanceID, req, "")
		// No workspace was resolved, so there is no (workspace, gate) pair
		// to record this outcome against: AC-OFFICE-RUN-CAUSATION-001.20
		// is explicit that an empty workspace must never be used as a
		// countable scope value. RecordGateOutcome is skipped for this
		// gate only.
		return causationResolution{}, &RefusalError{
			Gate:   RefusalWorkspaceMissing,
			Reason: fmt.Sprintf("agent profile %s has no resolvable workspace: %v", agentInstanceID, err),
		}
	}

	res := causationResolution{
		ActorKind:   actorKind,
		ActorID:     actorID,
		WorkspaceID: workspaceID,
		RoutineID:   req.RoutineID,
	}

	if err := s.applyCausationLineage(ctx, tx, rec, agentInstanceID, req, actorKind, &res); err != nil {
		return causationResolution{}, err
	}

	if res.CausationDepth > s.effectiveMaxCausationDepth() {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalCausationDepth)), 1)
		s.logRefusal(RefusalCausationDepth, agentInstanceID, req, res.CausationID)
		// The depth compared above was already read without error inside
		// applyCausationLineage, so this gate can never fail closed on an
		// unreadable input; only a genuinely successful evaluation is
		// possible here, and there is nothing for RecordGateOutcome to
		// distinguish.
		return causationResolution{}, &RefusalError{
			Gate: RefusalCausationDepth,
			Reason: fmt.Sprintf("causation depth %d exceeds limit %d",
				res.CausationDepth, s.effectiveMaxCausationDepth()),
		}
	}

	if err := s.checkSelfTriggerAllowance(ctx, tx, rec, agentInstanceID, req, actorKind, actorID, res.WorkspaceID, res.CausationID); err != nil {
		return causationResolution{}, err
	}

	res.PriorityClass = shared.ClassifyPriority(actorKind, req.Reason, false)
	return res, nil
}

// applyCausationLineage fills in res's parent/depth/human-rooted/routine
// fields from either the causing run (refusing if it's unreadable) or,
// for a root cause, the task-boundary carrier. Split out of
// resolveCausation to keep that function's nesting under the repo's
// complexity limit.
func (s *Service) applyCausationLineage(
	ctx context.Context, tx *sqlx.Tx, rec *gateOutcomeRecorder, agentInstanceID string, req QueueRunRequest, actorKind models.ActorKind, res *causationResolution,
) error {
	if req.CausingRunID == "" {
		// AC-OFFICE-RUN-CAUSATION-001.9/.13: a human actor always roots a
		// new chain, discarding any carrier lineage or human-rooted flag
		// the task-boundary carrier reports.
		if actorKind == models.ActorKindUser {
			res.HumanRooted = true
			return nil
		}
		if req.CarrierHumanRooted != nil {
			res.HumanRooted = *req.CarrierHumanRooted
		}
		// AC-OFFICE-RUN-CAUSATION-001.18/.24: the task-boundary carrier's
		// creating run identifier stands in for a live causing run,
		// without requiring a read of that run. An empty value means the
		// resulting run is a root regardless of what the carried
		// causation identifier and depth say.
		if req.CarrierCreatingRunID != "" {
			res.ParentRunID = req.CarrierCreatingRunID
			res.CausationDepth = req.CarrierCausationDepth + 1
			res.CausationID = req.CarrierCausationID
		}
		return nil
	}

	causing, err := s.repo.GetRunByIDTx(ctx, tx, req.CausingRunID)
	if err != nil {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalCausingRunUnreadable)), 1)
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalCausingRunUnreadable)), 1)
		s.logRefusal(RefusalCausingRunUnreadable, agentInstanceID, req, "")
		// The causing run itself is the input this gate could not read,
		// which is exactly AC-OFFICE-BACKPRESSURE-003.3's "input cannot
		// be read" case, so it counts toward the durable escalation
		// record alongside the refusal.
		rec.record(res.WorkspaceID, RefusalCausingRunUnreadable, false)
		return &RefusalError{
			Gate:   RefusalCausingRunUnreadable,
			Reason: fmt.Sprintf("causing run %s unreadable: %v", req.CausingRunID, err),
		}
	}
	rec.record(res.WorkspaceID, RefusalCausingRunUnreadable, true)
	// AC-OFFICE-RUN-CAUSATION-001.9: an actor who is human always roots a
	// new causation chain, even when nested inside a human-rooted run's
	// own follow-on work.
	if actorKind == models.ActorKindUser {
		res.HumanRooted = true
		return nil
	}
	res.CausationID = causingCausationID(causing)
	res.ParentRunID = causing.ID
	res.CausationDepth = causing.CausationDepth + 1
	res.HumanRooted = causing.HumanRooted
	if res.RoutineID == "" {
		res.RoutineID = causing.RoutineID
	}
	return nil
}

// normalizeActor applies AC-OFFICE-RUN-CAUSATION-001.16: an absent,
// invalid, or (for ActorKindAgent) identity-less actor resolves to
// ActorKindSystem rather than a guessed identity, and is counted so a
// caller failing to declare its actor stays visible.
func normalizeActor(req QueueRunRequest) (models.ActorKind, string) {
	if !req.ActorKind.Valid() {
		shared.LaunchActorMissingTotal.Add(shared.LaunchSafetyLabel("reason", req.Reason), 1)
		return models.ActorKindSystem, ""
	}
	if req.ActorKind == models.ActorKindAgent && req.ActorID == "" {
		shared.LaunchActorMissingTotal.Add(shared.LaunchSafetyLabel("reason", req.Reason), 1)
		return models.ActorKindSystem, ""
	}
	return req.ActorKind, req.ActorID
}

// causingCausationID returns the causing run's causation id, adopting
// the causing run's own id when that run predates this column
// (AC-OFFICE-RUN-CAUSATION-001.10's most-restrictive-reading idiom
// applied to a legacy empty value).
func causingCausationID(causing *models.Run) string {
	if causing.CausationID != "" {
		return causing.CausationID
	}
	return causing.ID
}

// checkSelfTriggerAllowance refuses an enqueue whose agent has
// re-triggered itself with the same reason more than the effective
// allowance within SelfTriggerWindow (AC-OFFICE-LAUNCH-SAFETY-004).
// Only applies to an agent acting as itself: a system or human actor
// cannot self-trigger by definition. The count runs against the
// caller's enqueue transaction (AC-OFFICE-LAUNCH-SAFETY-003.8) so it is
// serialized against the insert and against every other concurrent
// enqueue for the same agent profile — a count over rows the insert
// alone does not lock.
func (s *Service) checkSelfTriggerAllowance(
	ctx context.Context, tx *sqlx.Tx, rec *gateOutcomeRecorder, agentInstanceID string, req QueueRunRequest, actorKind models.ActorKind, actorID string,
	workspaceID, causationID string,
) error {
	if actorKind != models.ActorKindAgent || actorID != agentInstanceID {
		// Not applicable to this actor: the gate is not evaluated, so
		// AC-OFFICE-BACKPRESSURE-003.10's "count left unchanged" rule
		// applies and RecordGateOutcome is not called.
		return nil
	}
	reason := req.Reason
	since := time.Now().UTC().Add(-SelfTriggerWindow)
	count, err := s.repo.CountSelfTriggeredRunsTx(ctx, tx, agentInstanceID, reason, since)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		shared.LaunchCheckFailedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalSelfTrigger)), 1)
		s.logRefusal(RefusalSelfTrigger, agentInstanceID, req, causationID)
		rec.record(workspaceID, RefusalSelfTrigger, false)
		return &RefusalError{
			Gate:   RefusalSelfTrigger,
			Reason: fmt.Sprintf("self-trigger count unreadable: %v", err),
		}
	}
	rec.record(workspaceID, RefusalSelfTrigger, true)
	if count >= s.effectiveSelfTriggerAllowance() {
		shared.LaunchRefusedTotal.Add(shared.LaunchSafetyLabel("gate", string(RefusalSelfTrigger)), 1)
		s.logRefusal(RefusalSelfTrigger, agentInstanceID, req, causationID)
		return &RefusalError{
			Gate: RefusalSelfTrigger,
			Reason: fmt.Sprintf("agent %s exceeded self-trigger allowance %d for reason %q within %s",
				agentInstanceID, s.effectiveSelfTriggerAllowance(), reason, SelfTriggerWindow),
		}
	}
	return nil
}

// logRefusal emits AC-OFFICE-BACKPRESSURE-003.1's structured log entry
// for an enqueue refused by gate: the gate, the agent profile the wake
// was for, the wake reason, and the causing run identifier the request
// supplied. A refusal creates no run row, so no run identifier is
// logged. causationID is logged only when the caller had already
// resolved it before this gate ran (empty otherwise, per
// AC-OFFICE-BACKPRESSURE-003.1's "not for the missing workspace" carve-out
// and applyCausationLineage's own refusal, neither of which has one yet).
func (s *Service) logRefusal(gate RefusalGate, agentInstanceID string, req QueueRunRequest, causationID string) {
	fields := []zap.Field{
		zap.String("gate", string(gate)),
		zap.String("agent_profile", agentInstanceID),
		zap.String("reason", req.Reason),
	}
	if req.CausingRunID != "" {
		fields = append(fields, zap.String("causing_run_id", req.CausingRunID))
	}
	if causationID != "" {
		fields = append(fields, zap.String("causation_id", causationID))
	}
	s.log.Info("run enqueue refused", fields...)
}

// flushGateOutcomes persists every gate-evaluation outcome rec
// accumulated, via the standalone RecordGateOutcome — each call opens
// its own transaction on the repository's writer pool, deliberately
// separate from (and run only after) the enqueue transaction that
// observed these outcomes has already committed or rolled back; see
// gateOutcomeRecorder's doc comment for why. Any persistence failure is
// counted rather than propagated, per AC-OFFICE-BACKPRESSURE-003.4:
// this diagnostic bookkeeping must never affect the admission decision.
func (s *Service) flushGateOutcomes(ctx context.Context, rec *gateOutcomeRecorder) {
	for _, o := range rec.outcomes {
		if err := s.repo.RecordGateOutcome(ctx, o.workspaceID, string(o.gate), o.success); err != nil {
			shared.GateOutcomeRecordFailedTotal.Add(shared.LaunchSafetyLabel("gate", string(o.gate)), 1)
		}
	}
}
