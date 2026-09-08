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
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/runs/commentkeys"
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

// RunQueueAdapter is the interface the workflow engine uses to enqueue
// runs from queue_run actions. Phase 2 final's parallel agent
// declares the same shape inside internal/workflow/engine; both
// declarations MUST match. When Phase 2 final lands the duplicate is
// dropped and the engine's interface is imported here directly.
type RunQueueAdapter interface {
	QueueRun(ctx context.Context, req QueueRunRequest) (QueueOutcome, error)
}

// QueueOutcome reports what QueueRun actually did with a request. A
// duplicated declaration lives in internal/workflow/engine (see that
// package's RunQueueAdapter doc); both MUST match.
type QueueOutcome string

const (
	// QueueOutcomeQueued means a new runs row was inserted.
	QueueOutcomeQueued QueueOutcome = "queued"
	// QueueOutcomeDeduped means an existing row with the same IdempotencyKey
	// already exists in the durable idempotency index, so nothing was inserted.
	QueueOutcomeDeduped QueueOutcome = "deduped"
	// QueueOutcomeCoalesced means the request was merged into an existing
	// queued row for the same agent + reason within the coalescing
	// window, so nothing new was inserted.
	QueueOutcomeCoalesced QueueOutcome = "coalesced"
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
}

// CoalesceWindowSeconds is the default coalescing window. When two
// queue_run requests for the same (agent, reason) land within this
// window, the second is merged into the first by bumping
// coalesced_count and replacing the payload.
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
//  2. Recent idempotency check on req.IdempotencyKey if set.
//  3. Coalescing (5s window for same agent + reason).
//  4. Insert into runs table.
//  5. Publish OfficeRunQueued.
//  6. Signal the scheduler (B3.5 — event-driven claim).
//
// The returned QueueOutcome lets the caller distinguish a fresh insert from
// a deduplicated or coalesced request instead of treating any nil error as
// "a run was queued".
func (s *Service) QueueRun(ctx context.Context, req QueueRunRequest) (QueueOutcome, error) {
	agentInstanceID, err := s.resolveAgentInstance(ctx, req)
	if err != nil {
		return "", err
	}
	if agentInstanceID == "" {
		return "", fmt.Errorf("queue run: agent_profile_id is required")
	}

	if req.IdempotencyKey != "" {
		dup, err := s.repo.CheckIdempotencyKey(ctx, req.IdempotencyKey, IdempotencyWindowHours)
		if err != nil {
			return "", fmt.Errorf("idempotency check: %w", err)
		}
		if dup {
			s.log.Debug("run skipped (idempotent)",
				zap.String("key", req.IdempotencyKey))
			return QueueOutcomeDeduped, nil
		}
	}

	payloadMap := runPayload(req, agentInstanceID)
	payload, err := encodePayload(payloadMap)
	if err != nil {
		return "", fmt.Errorf("encode payload: %w", err)
	}

	if shouldCoalesceRun(req) {
		coalesced, err := s.repo.CoalesceRun(ctx, agentInstanceID, req.Reason, CoalesceWindowSeconds, payload)
		if err != nil {
			return "", fmt.Errorf("coalesce check: %w", err)
		}
		if coalesced {
			s.log.Debug("run coalesced",
				zap.String("agent", agentInstanceID),
				zap.String("reason", req.Reason))
			// Coalesced rows are merged into an existing queued row, so
			// no new signal is needed — the scheduler already saw the
			// original insert.
			return QueueOutcomeCoalesced, nil
		}
	}

	row, err := s.insertRun(ctx, agentInstanceID, req, payload)
	if err != nil {
		// idx_run_idempotency has no time bound, so a conflict here can
		// come from a row older than IdempotencyWindowHours, not just the
		// windowed race CheckIdempotencyKey guards against above. Either
		// way the existing row is definitionally the same operation this
		// key identifies, so treat it as a no-op dedupe rather than a hard
		// error (see errIdempotencyKeyConflict's doc comment).
		if errors.Is(err, errIdempotencyKeyConflict) {
			s.log.Debug("run skipped (idempotency index race)",
				zap.String("key", req.IdempotencyKey))
			return QueueOutcomeDeduped, nil
		}
		return "", err
	}

	s.log.Info("run queued",
		zap.String("id", row.ID),
		zap.String("agent", agentInstanceID),
		zap.String("reason", req.Reason))

	s.publishRunQueued(ctx, row, req.IdempotencyKey)
	s.signal()
	return QueueOutcomeQueued, nil
}

// insertRun creates the runs row and returns it. Pulled out of
// QueueRun to keep the latter under the funlen budget.
func (s *Service) insertRun(
	ctx context.Context, agentInstanceID string, req QueueRunRequest, payload string,
) (*models.Run, error) {
	var idemKeyPtr *string
	if req.IdempotencyKey != "" {
		k := req.IdempotencyKey
		idemKeyPtr = &k
	}
	row := &models.Run{
		ID:             uuid.New().String(),
		AgentProfileID: agentInstanceID,
		Reason:         req.Reason,
		Payload:        payload,
		Status:         "queued",
		CoalescedCount: 1,
		IdempotencyKey: idemKeyPtr,
		RequestedAt:    time.Now().UTC(),
	}
	if err := s.repo.CreateRun(ctx, row); err != nil {
		if runssqlite.IsIdempotencyKeyUniqueViolation(err) {
			return nil, errIdempotencyKeyConflict
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

func shouldCoalesceRun(req QueueRunRequest) bool {
	return !commentkeys.HasTaskCommentPrefix(req.IdempotencyKey)
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
