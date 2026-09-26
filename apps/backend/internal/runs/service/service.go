// Package service implements the runs queue service. It owns the
// QueueRun call site (idempotency + coalescing + insert + publish)
// and exposes a RunQueueAdapter the workflow engine can use to
// enqueue runs without depending on the office package.
//
// Phase 3 of task-model-unification (see docs/specs/tasks/system-design/model-unification.md
// sections B3.2 and B3.5) lifted this logic out of internal/office/service
// and added an event-driven claim signal so engine-emitted runs reach
// the scheduler in a few ms instead of waiting up to one tick (5s).
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/runs/commentkeys"
	"github.com/kandev/kandev/internal/runs/models"
	runssqlite "github.com/kandev/kandev/internal/runs/repository/sqlite"
)

// errIdempotencyKeyConflict signals that insertRun's CreateRun failed
// because idx_run_idempotency rejected a duplicate idempotency_key —
// distinct from the earlier CheckIdempotencyKey miss, which only looks
// within IdempotencyWindowHours. Two independent producers deriving the
// same operation id for the same event can both pass that fast-path check
// before either commits (or the colliding row can simply be older than the
// window); the unique index is what actually stops the second insert.
// QueueRun treats this as a durable dedupe hit: QueueOutcomeDeduped, not an
// error, so the losing producer's caller does not abort or log a spurious
// failure for what is really a no-op.
var errIdempotencyKeyConflict = errors.New("idempotency key conflict")

// errWakeWaveConflict signals that insertRun's CreateRunTx failed because
// idx_run_wake_wave rejected a duplicate wave identity — the tx-scoped
// sibling of errIdempotencyKeyConflict, for the completion-wave dedup
// index instead of the idempotency index. Treated the same way: a durable
// dedupe hit, not an error.
var errWakeWaveConflict = errors.New("wake wave conflict")

// RunQueueAdapter is the interface the workflow engine uses to enqueue
// runs from queue_run actions. Phase 2 final's parallel agent
// declares the same shape inside internal/workflow/engine; both
// declarations MUST match. When Phase 2 final lands the duplicate is
// dropped and the engine's interface is imported here directly.
type RunQueueAdapter interface {
	QueueRun(ctx context.Context, req QueueRunRequest) (QueueOutcome, error)
}

// QueueOutcome is the shared queue contract used by office producers and the
// runs service. The workflow engine keeps its adapter-local declaration to
// avoid importing the office package; its values remain wire-compatible.
type QueueOutcome = shared.QueueOutcome

const (
	QueueOutcomeQueued      = shared.QueueOutcomeQueued
	QueueOutcomeDeduped     = shared.QueueOutcomeDeduped
	QueueOutcomeCoalesced   = shared.QueueOutcomeCoalesced
	QueueOutcomeRateLimited = shared.QueueOutcomeRateLimited
)

// QueueRunRequest carries everything the queue needs to insert a row.
// Fields:
//   - AgentProfileID: the agent's profile (model + tools). Stored on
//     the resolved agent_instance; the queue resolves agent_profile_id
//     from this in conjunction with TaskID.
//   - TaskID: the kanban / office task the run is about. Empty for
//     heartbeat / standalone agent runs.
//   - WorkflowStepID: the workflow step that emitted the queue_run
//     action. Empty when the queue_run did not originate from the
//     engine (legacy office paths).
//   - Reason: the run reason (task_assigned, task_comment, …).
//   - IdempotencyKey: when non-empty, the run is suppressed if the durable
//     queue identity already exists. QueueRun first checks recent rows, then
//     relies on the unique index to close races and preserve that identity.
//   - Payload: structured JSON-encoded payload. Must be a non-nil map
//     when set; serialised before insert. QueueRun adds the resolved
//     task / workflow / agent envelope before persisting.
type QueueRunRequest struct {
	AgentProfileID string
	TaskID         string
	WorkflowStepID string
	Reason         string
	IdempotencyKey string
	Payload        map[string]any

	// ActorKind and ActorID declare who caused this enqueue
	// (AC-OFFICE-RUN-CAUSATION-001.15). ActorKind is treated as required:
	// an empty, invalid, or unrecognized value — or ActorKindAgent with an
	// empty ActorID — resolves to ActorKindSystem and increments
	// office_launch_actor_missing_total, per AC-OFFICE-RUN-CAUSATION-001.16.
	// It never resolves to ActorKindUser, so a rule keyed on a human actor
	// fails toward the restrictive answer.
	ActorKind models.ActorKind
	ActorID   string

	// CausingRunID is the run this enqueue happened inside, empty for a
	// root cause. When set but unreadable, the enqueue is refused
	// (AC-OFFICE-RUN-CAUSATION-001.21) rather than rooted.
	CausingRunID string
	// RoutineID is the routine this enqueue is chargeable to, empty for
	// none (AC-OFFICE-RUN-CAUSATION-001.14). Must only be set by a
	// trusted internal caller (a routine fire, or the task-boundary
	// carrier) — never accepted verbatim from agent-supplied input.
	RoutineID string
	// CarrierHumanRooted carries the human-rooted flag across a task
	// boundary without requiring a read of the creating run
	// (AC-OFFICE-RUN-CAUSATION-001.13). Nil when the caller has no carrier
	// to report (a direct CausingRunID enqueue derives human-rooted from
	// that run instead).
	CarrierHumanRooted *bool
	// CarrierCreatingRunID and CarrierCausationDepth carry the
	// task-boundary carrier's lineage across a task boundary without
	// requiring a read of the creating run (AC-OFFICE-RUN-CAUSATION-001.18).
	// Both are empty/zero when the caller has no carrier lineage to
	// report. Per AC-OFFICE-RUN-CAUSATION-001.24, an empty
	// CarrierCreatingRunID means the resulting run is a root regardless of
	// what CarrierCausationID and CarrierCausationDepth say.
	CarrierCreatingRunID  string
	CarrierCausationID    string
	CarrierCausationDepth int

	// ContextSnapshot is stored on the run's context_snapshot column
	// verbatim, distinct from Payload (the routing envelope: task id,
	// workflow step id, agent profile id). Used by taskless
	// wakeup-originated runs to carry the wakeup's own JSON context
	// (including routine_id, which office/models.ContinuationScopeForRun reads
	// back out) without mixing it into the envelope. Empty for every
	// other caller, which leaves the column at its default.
	ContextSnapshot string

	// SkipCoalesce bypasses this call's own coalescing window
	// (shouldCoalesceRun/CoalesceRunTx) for a caller that already ran its
	// own, independent in-flight-merge decision before calling QueueRun
	// (AC-OFFICE-ENQUEUE-CONSOLIDATION-001.5): running both mechanisms on
	// the same request risks the two disagreeing about which existing row
	// (if any) this request should merge into. Idempotency, causation
	// resolution, and the insert still run normally.
	SkipCoalesce bool

	// WakeWaveKey and WakeWaveString are the completion-wave identity
	// (parent-wake-wave-identity). When WakeWaveKey is non-empty, QueueRun
	// never coalesces this request into an existing row, is never merged
	// into by a later request, and idx_run_wake_wave deduplicates a second
	// insert for the same (WakeWaveKey, AgentProfileID) into
	// QueueOutcomeDeduped instead of an error.
	WakeWaveKey    string
	WakeWaveString string

	// CausationID is stored verbatim on the created run's own CausationID
	// column (AC-OFFICE-LOOP-LIVENESS-002.3, unrelated to this package's
	// causation-chain resolution below and to the resulting row's
	// ChainCausationID) — copied from the wakeup request that produced
	// this enqueue, so a run created through this seam still carries the
	// loop-liveness correlation a direct-insert caller used to set by
	// hand. Empty for a caller with no wakeup request to correlate.
	CausationID string
}

// CoalesceWindowSeconds is the default coalescing window. When two
// queue_run requests for the same agent, reason, and task bucket land
// within this window, the second is merged into the first by bumping
// coalesced_count and replacing the payload. A task bucket is the
// nonempty task_id, or the taskless bucket when task_id is missing,
// null, or empty.
const CoalesceWindowSeconds = 5

// IdempotencyWindowHours is the lookback used by the fast duplicate query.
// The runs table's unique idempotency index remains durable for every
// persisted key, including rows older than this window.
const IdempotencyWindowHours = 24

// signalBuffer sizes the in-process channel used for event-driven
// claims (B3.5). The scheduler tick is the safety net so missed
// signals are tolerated; the channel only needs to carry "wake up,
// there's at least one new row" and a small buffer is plenty.
const signalBuffer = 64

// AgentResolver turns a queue request into the concrete agent instance
// id the row needs. Today's office paths pass agent_profile_id
// directly via the payload; the engine's queue_run will eventually
// pass an agent profile id and the resolver will look up the
// matching instance for the task. The interface keeps both call
// sites pluggable.
type AgentResolver interface {
	ResolveAgentInstance(ctx context.Context, req QueueRunRequest) (string, error)
}

// AgentResolverFunc adapts a function to the AgentResolver interface.
type AgentResolverFunc func(ctx context.Context, req QueueRunRequest) (string, error)

// ResolveAgentInstance implements AgentResolver.
func (f AgentResolverFunc) ResolveAgentInstance(ctx context.Context, req QueueRunRequest) (string, error) {
	return f(ctx, req)
}

// Service implements RunQueueAdapter against the runs SQLite
// repository. It also publishes an OfficeRunQueued bus event after
// each successful insert and signals the in-process scheduler so
// engine-emitted runs are claimed in milliseconds instead of waiting
// for the 5s tick.
type Service struct {
	repo     *runssqlite.Repository
	eb       bus.EventBus
	log      *logger.Logger
	resolver AgentResolver
	signalCh chan struct{}

	// maxCausationDepth, selfTriggerAllowance, and
	// selfTriggerTotalAllowance back REQ-OFFICE-LAUNCH-SAFETY-003/004.
	// Zero means "use the documented default", so a Service built via
	// New without SetLaunchSafetyLimits still enforces the spec defaults
	// rather than being unbounded.
	maxCausationDepth         int
	selfTriggerAllowance      int
	selfTriggerTotalAllowance int
}

// DefaultMaxCausationDepth is the documented default for
// AC-OFFICE-LAUNCH-SAFETY-003.1.
const DefaultMaxCausationDepth = 8

// DefaultSelfTriggerAllowance is the documented default for
// AC-OFFICE-LAUNCH-SAFETY-004.3.
const DefaultSelfTriggerAllowance = 3

// DefaultSelfTriggerTotalAllowance is the documented default for the
// reason-independent allowance of AC-OFFICE-LAUNCH-SAFETY-004.8.
const DefaultSelfTriggerTotalAllowance = 8

// SelfTriggerWindow is the rolling window AC-OFFICE-LAUNCH-SAFETY-004.3
// and AC-OFFICE-LAUNCH-SAFETY-004.8 count against. Unlike the
// allowances, the window itself is not configurable.
const SelfTriggerWindow = 60 * time.Minute

// SetLaunchSafetyLimits configures the causation-depth ceiling, the
// per-reason self-trigger allowance, and the reason-independent
// self-trigger allowance. A value less than 1 is replaced by the
// documented default, per AC-OFFICE-LAUNCH-SAFETY-003.1 / 004.3 / 004.8:
// a configured 0 must not mean "unlimited" or "no self-triggering ever
// allowed by accident of an unset override". A total below the
// per-reason value is a valid, honored configuration
// (AC-OFFICE-LAUNCH-SAFETY-004.8); clamping happens independently for
// each of the three values.
func (s *Service) SetLaunchSafetyLimits(maxCausationDepth, selfTriggerAllowance, selfTriggerTotalAllowance int) {
	s.maxCausationDepth = clampToDefault(maxCausationDepth, DefaultMaxCausationDepth)
	s.selfTriggerAllowance = clampToDefault(selfTriggerAllowance, DefaultSelfTriggerAllowance)
	s.selfTriggerTotalAllowance = clampToDefault(selfTriggerTotalAllowance, DefaultSelfTriggerTotalAllowance)
}

func clampToDefault(value, def int) int {
	if value < 1 {
		return def
	}
	return value
}

func (s *Service) effectiveMaxCausationDepth() int {
	return clampToDefault(s.maxCausationDepth, DefaultMaxCausationDepth)
}

func (s *Service) effectiveSelfTriggerAllowance() int {
	return clampToDefault(s.selfTriggerAllowance, DefaultSelfTriggerAllowance)
}

func (s *Service) effectiveSelfTriggerTotalAllowance() int {
	return clampToDefault(s.selfTriggerTotalAllowance, DefaultSelfTriggerTotalAllowance)
}

// New constructs a Service. The signal channel is created here so
// callers can call Signal() / SubscribeSignal() before a scheduler is
// attached without losing wake-ups.
func New(
	repo *runssqlite.Repository,
	eb bus.EventBus,
	log *logger.Logger,
	resolver AgentResolver,
) *Service {
	return &Service{
		repo:     repo,
		eb:       eb,
		log:      log.WithFields(zap.String("component", "runs-service")),
		resolver: resolver,
		signalCh: make(chan struct{}, signalBuffer),
	}
}

// SubscribeSignal returns the in-process channel the runs scheduler
// reads to claim newly-inserted rows without waiting for the tick.
// One reader is expected; the channel is buffered so quick bursts
// don't block QueueRun.
func (s *Service) SubscribeSignal() <-chan struct{} { return s.signalCh }

// QueueRun implements RunQueueAdapter. The flow is:
//  1. Resolve agent_profile_id (from the request field, payload fallback,
//     or a wired resolver).
//  2. Idempotency check, coalescing, causation resolution (depth and
//     self-trigger gates), and insert, all inside one transaction
//     serialized per agent profile — see enqueueLocked.
//  3. Publish OfficeRunQueued.
//  4. Signal the scheduler (B3.5 — event-driven claim).
//
// The returned QueueOutcome lets the caller distinguish a fresh insert from
// a deduplicated or coalesced request instead of treating any nil error as
// "a run was queued".
func (s *Service) QueueRun(ctx context.Context, req QueueRunRequest) (QueueOutcome, error) {
	outcome, _, err := s.QueueRunAndReturn(ctx, req)
	return outcome, err
}

// QueueRunAndReturn is QueueRun, additionally returning the inserted row
// on a QueueOutcomeQueued outcome (nil for QueueOutcomeDeduped/Coalesced,
// which insert nothing). Exposed for callers that need the new run's id
// — e.g. the wakeup dispatcher, which must mark its own wakeup-request
// row claimed against it.
func (s *Service) QueueRunAndReturn(ctx context.Context, req QueueRunRequest) (QueueOutcome, *models.Run, error) {
	agentInstanceID, err := s.resolveAgentInstance(ctx, req)
	if err != nil {
		return QueueOutcomeNone, nil, err
	}
	if agentInstanceID == "" {
		return QueueOutcomeNone, nil, fmt.Errorf("queue run: agent_profile_id is required")
	}

	payloadMap := runPayload(req, agentInstanceID)
	payload, err := encodePayload(payloadMap)
	if err != nil {
		return QueueOutcomeNone, nil, fmt.Errorf("encode payload: %w", err)
	}

	outcome, row, err := s.enqueueLocked(ctx, agentInstanceID, req, payload)
	if err != nil {
		return QueueOutcomeNone, nil, err
	}
	if outcome != QueueOutcomeQueued {
		return outcome, nil, nil
	}

	s.log.Info("run queued",
		zap.String("id", row.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", req.Reason))

	s.publishRunQueued(ctx, row, req.IdempotencyKey)
	s.signal()
	return QueueOutcomeQueued, row, nil
}

// enqueueLocked performs the idempotency check, the coalescing check,
// causation resolution, and the insert inside one transaction serialized
// per agent profile (AC-OFFICE-LAUNCH-SAFETY-003.8): the depth check,
// the self-trigger window count, the idempotency check, and the insert
// must be serialized against every other concurrent enqueue for the
// same agent profile, on every supported database engine, so that two
// concurrent enqueues can never both observe the same self-trigger
// count or the same idempotency-key absence before either commits.
// idx_run_wake_wave has no windowed pre-check the way IdempotencyKey does
// — it must stay unbounded — so a wave-carrying conflict is only ever
// caught by the insert itself, inside this same transaction.
func (s *Service) enqueueLocked(
	ctx context.Context, agentInstanceID string, req QueueRunRequest, payload string,
) (QueueOutcome, *models.Run, error) {
	tx, err := s.repo.BeginEnqueueTx(ctx, agentInstanceID)
	if err != nil {
		return "", nil, fmt.Errorf("begin enqueue transaction: %w", err)
	}
	rec := &gateOutcomeRecorder{}
	// Registered before the rollback defer below, so it runs after: Go
	// defers execute in LIFO order, and flushGateOutcomes must not write
	// until this transaction has actually committed or rolled back —
	// see gateOutcomeRecorder's doc comment.
	defer s.flushGateOutcomes(ctx, rec)
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if req.IdempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKeyTx(ctx, tx, req.IdempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return "", nil, fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			return ReportWindowedDedup(QueueSourceRuns, req.Reason, req.IdempotencyKey), nil, nil
		}
	}

	if shouldCoalesceRun(req) {
		coalesced, err := s.repo.CoalesceRunTx(ctx, tx, agentInstanceID, req.Reason, CoalesceWindowSeconds, payload)
		if err != nil {
			return "", nil, fmt.Errorf("coalesce check: %w", err)
		}
		if coalesced {
			if err := tx.Commit(); err != nil {
				return "", nil, fmt.Errorf("commit coalesce: %w", err)
			}
			committed = true
			s.log.Debug("run coalesced",
				zap.String("agent", agentInstanceID),
				zap.String("reason", req.Reason))
			// Coalesced rows are merged into an existing queued row, so
			// no new signal is needed — the scheduler already saw the
			// original insert.
			return QueueOutcomeCoalesced, nil, nil
		}
	}

	row, err := s.insertRun(ctx, tx, rec, agentInstanceID, req, payload)
	if err != nil {
		// idx_run_idempotency has no time bound, so a conflict here can
		// come from a row older than IdempotencyWindowHours, not just the
		// windowed race the idempotency check above guards against.
		// Either way the existing row is definitionally the same
		// operation this key identifies, so treat it as a no-op dedupe
		// rather than a hard error (see errIdempotencyKeyConflict's doc
		// comment).
		if errors.Is(err, errIdempotencyKeyConflict) {
			s.log.Debug("run skipped (idempotency index race)",
				zap.String("key", req.IdempotencyKey))
			return QueueOutcomeDeduped, nil, nil
		}
		// idx_run_wake_wave has no windowed pre-check the way
		// IdempotencyKey does, so a conflict here is the only place a
		// wave-carrying request's dedupe is caught.
		if errors.Is(err, errWakeWaveConflict) {
			s.log.Debug("run skipped (wake wave index race)",
				zap.String("wake_wave_key", req.WakeWaveKey))
			ParentWakeDedupedTotal.Add(1)
			return QueueOutcomeDeduped, nil, nil
		}
		return "", nil, err
	}

	if err := tx.Commit(); err != nil {
		return "", nil, fmt.Errorf("commit enqueue: %w", err)
	}
	committed = true
	return QueueOutcomeQueued, row, nil
}

// insertRun resolves causation (actor, workspace, depth, self-trigger,
// priority class) and creates the runs row, returning it. A refusal from
// resolveCausation is returned unchanged: no row is inserted and no
// idempotency key is consumed, per AC-OFFICE-LAUNCH-SAFETY-003.4.
func (s *Service) insertRun(
	ctx context.Context, tx *sqlx.Tx, rec *gateOutcomeRecorder, agentInstanceID string, req QueueRunRequest, payload string,
) (*models.Run, error) {
	causation, err := s.resolveCausation(ctx, tx, rec, agentInstanceID, req)
	if err != nil {
		return nil, err
	}

	var idemKeyPtr *string
	if req.IdempotencyKey != "" {
		k := req.IdempotencyKey
		idemKeyPtr = &k
	}
	row := &models.Run{
		ID:              uuid.New().String(),
		AgentProfileID:  agentInstanceID,
		Reason:          req.Reason,
		Payload:         payload,
		Status:          "queued",
		CoalescedCount:  1,
		IdempotencyKey:  idemKeyPtr,
		RequestedAt:     time.Now().UTC(),
		ContextSnapshot: req.ContextSnapshot,
		ParentRunID:     causation.ParentRunID,
		CausationDepth:  causation.CausationDepth,
		PriorityClass:   causation.PriorityClass,
		HumanRooted:     causation.HumanRooted,
		RoutineID:       causation.RoutineID,
		ActorKind:       causation.ActorKind,
		ActorID:         causation.ActorID,
		WorkspaceID:     causation.WorkspaceID,
		WakeWaveKey:     req.WakeWaveKey,
		WakeWaveString:  req.WakeWaveString,
		CausationID:     req.CausationID,
	}
	if causation.CausationID != "" {
		row.ChainCausationID = causation.CausationID
	} else {
		// AC-OFFICE-RUN-CAUSATION-001.2: a root's causation id is its own
		// run id. Known only now that the row's ID has been minted.
		row.ChainCausationID = row.ID
	}
	if err := s.repo.CreateRunTx(ctx, tx, row); err != nil {
		if runssqlite.IsIdempotencyKeyUniqueViolation(err) {
			return nil, errIdempotencyKeyConflict
		}
		// idx_run_wake_wave has no windowed pre-check the way
		// IdempotencyKey does — it must stay unbounded — so this insert is
		// the only place a wave-carrying conflict is caught.
		if runssqlite.IsWakeWaveUniqueViolation(err) {
			return nil, errWakeWaveConflict
		}
		return nil, fmt.Errorf("enqueue run: %w", err)
	}
	return row, nil
}

// resolveAgentInstance picks the agent_profile_id for a request.
// Engine queue_run sends AgentProfileID as a typed field. Legacy
// office QueueRun callers still carry agent_profile_id inside Payload,
// so the resolver-less path accepts both shapes.
func (s *Service) resolveAgentInstance(ctx context.Context, req QueueRunRequest) (string, error) {
	if s.resolver != nil {
		return s.resolver.ResolveAgentInstance(ctx, req)
	}
	if req.AgentProfileID != "" {
		return req.AgentProfileID, nil
	}
	if req.Payload != nil {
		if v, ok := req.Payload["agent_profile_id"].(string); ok && v != "" {
			return v, nil
		}
	}
	return "", nil
}

// runPayload starts from the caller-supplied payload, then overwrites the
// standard envelope keys from typed request fields. That overwrite is
// intentional: engine queue_run and legacy office QueueRun callers converge
// on the same persisted JSON shape even when their input payloads differ.
func runPayload(req QueueRunRequest, agentInstanceID string) map[string]any {
	out := make(map[string]any, len(req.Payload))
	for k, v := range req.Payload {
		out[k] = v
	}
	if req.TaskID != "" {
		out["task_id"] = req.TaskID
	}
	if req.WorkflowStepID != "" {
		out["workflow_step_id"] = req.WorkflowStepID
	}
	if agentInstanceID != "" {
		out["agent_profile_id"] = agentInstanceID
	}
	return out
}

// shouldCoalesceRun decides whether a request may be merged into an
// existing queued row. A wave-carrying request is never coalesced:
// coalescing replaces the target row's payload without moving its
// recorded identity, which would leave a run whose wave columns no
// longer describe the wake it delivers. idx_run_wake_wave, not this
// window, is what reconciles wave-carrying requests.
func shouldCoalesceRun(req QueueRunRequest) bool {
	return !req.SkipCoalesce && req.WakeWaveKey == "" && !commentkeys.HasTaskCommentPrefix(req.IdempotencyKey)
}

// publishRunQueued emits the OfficeRunQueued bus event so the WS
// gateway can fan it out to subscribed clients. Subject keeps the
// "office.run.queued" string for frontend stability — the schema
// rename to runs is server-side only.
func (s *Service) publishRunQueued(ctx context.Context, row *models.Run, idempotencyKey string) {
	if s.eb == nil {
		return
	}
	taskID, commentID := commentkeys.IdentityFromPayload(row.Payload)
	data := map[string]interface{}{
		"run_id":           row.ID,
		"agent_profile_id": row.AgentProfileID,
		"reason":           row.Reason,
		"task_id":          taskID,
		"comment_id":       commentID,
		"idempotency_key":  idempotencyKey,
	}
	event := bus.NewEvent(events.OfficeRunQueued, "runs-service", data)
	if err := s.eb.Publish(ctx, events.OfficeRunQueued, event); err != nil {
		s.log.Debug("publish run queued event failed",
			zap.String("run_id", row.ID),
			zap.Error(err))
	}
}

// signal pokes the in-process channel so the runs scheduler claims
// without waiting for the next tick. Non-blocking on a full buffer:
// the buffered channel already carries "there's work" — one more
// signal doesn't add information.
func (s *Service) signal() {
	select {
	case s.signalCh <- struct{}{}:
	default:
	}
}

// encodePayload renders the structured payload to a JSON string. An
// empty / nil payload becomes "{}" so DB inserts never store NULL.
func encodePayload(p map[string]any) (string, error) {
	if len(p) == 0 {
		return "{}", nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
