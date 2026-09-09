package routines

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// ErrWakeupAlreadyRequested is the routines-package-local signal that a
// WakeupEnqueuer.CreateWakeupRequest call lost an idempotency race: a
// request for this fire already exists, enqueued by another caller (a
// cron tick and a manual fire landing in the same dedup bucket, for
// example). That is success by another route, not a failure — the
// concrete adapter translates the sqlite layer's
// ErrWakeupIdempotencyConflict into this sentinel so routines never
// imports office/repository/sqlite directly (see routineWakeupAdapter).
// It is not a materialisation failure (AC-OFFICE-ROUTINE-CATCHUP-001.10
// second sentence): a duplicate wakeup request within the same
// wall-clock minute is a successful dedup, so
// materialiseLightweightRoutineRun absorbs it as a terminal "done" run
// rather than failing.
var ErrWakeupAlreadyRequested = errors.New("wakeup request already requested")

// Repository is the persistence interface required by RoutineService.
type Repository interface {
	CreateRoutine(ctx context.Context, routine *Routine) error
	GetRoutine(ctx context.Context, id string) (*Routine, error)
	ListRoutines(ctx context.Context, workspaceID string) ([]*Routine, error)
	UpdateRoutine(ctx context.Context, routine *Routine) error
	DeleteRoutine(ctx context.Context, id string) error

	CreateRoutineTrigger(ctx context.Context, t *RoutineTrigger) error
	ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*RoutineTrigger, error)
	GetTriggerByPublicID(ctx context.Context, publicID string) (*RoutineTrigger, error)
	GetDueTriggers(ctx context.Context, now time.Time) ([]*RoutineTrigger, error)
	ClaimTrigger(ctx context.Context, triggerID string, oldNextRunAt time.Time) (bool, error)
	UpdateTriggerNextRun(ctx context.Context, triggerID string, nextRunAt *time.Time) error
	ListStrandedTriggers(ctx context.Context, olderThan time.Time) ([]*RoutineTrigger, error)
	ReconcileTriggerNextRun(ctx context.Context, triggerID string, nextRunAt time.Time) (bool, error)
	DeleteRoutineTrigger(ctx context.Context, id string) error

	CreateRoutineRun(ctx context.Context, run *RoutineRun) error
	ListRoutineRuns(ctx context.Context, routineID string, limit, offset int) ([]*RoutineRun, error)
	ListAllRuns(ctx context.Context, workspaceID string, limit int) ([]*RoutineRun, error)
	GetActiveRunForFingerprint(ctx context.Context, routineID, fingerprint string) (*RoutineRun, error)
	GetRoutineRunByLinkedTaskID(ctx context.Context, taskID string) (*RoutineRun, error)
	UpdateRunStatus(ctx context.Context, runID string, status models.RoutineRunStatus, linkedTaskID string) error
	// UpdateRunStatusIfTaskCreated closes out a run only while it is still
	// task_created, returning whether this call was the one that closed
	// it. Used by both the TaskMoved-driven SyncRunStatus and the
	// concurrency gate's own inline check (applyConcurrencyPolicy), which
	// can both observe the same terminal task; the conditional WHERE
	// clause is what lets exactly one of them win instead of a
	// read-then-write race between them.
	UpdateRunStatusIfTaskCreated(ctx context.Context, runID string, status models.RoutineRunStatus, linkedTaskID string) (bool, error)
	UpdateRunCoalesced(ctx context.Context, runID, coalescedIntoRunID string) error
	TouchRoutineLastRun(ctx context.Context, routineID string, at time.Time) error
	// GetTaskTerminalStatus reads a task's real lifecycle state directly
	// (not via the TaskMoved event — see applyConcurrencyPolicy) and
	// reports "" when it is active, "done", "failed", "cancelled", or
	// "missing" for the other outcomes.
	GetTaskTerminalStatus(ctx context.Context, taskID string) (string, error)
}

// WakeupEnqueuer is the slim surface routines need to enqueue + dispatch
// a wakeup-request for the lightweight (taskless) routine flow. Defined
// here (not in office/wakeup) so the routines package depends on
// wakeup only behind an interface — wakeup never imports routines.
//
// Both methods are required: Create persists the wakeup-request row;
// Dispatch hands it to the dispatcher for claim + run creation.
type WakeupEnqueuer interface {
	CreateWakeupRequest(ctx context.Context, req *WakeupRequest) error
	Dispatch(ctx context.Context, requestID string) error
	FailWakeupRequest(ctx context.Context, requestID, reason string) error
}

// WakeupRequest mirrors *office/repository/sqlite.WakeupRequest with the
// minimal field set the routines lightweight path needs to populate.
// Defined locally to keep the routines package's import graph clean —
// wiring in main.go converts to the concrete sqlite type.
type WakeupRequest struct {
	ID             string
	AgentProfileID string
	Source         string
	Reason         string
	Payload        string
	IdempotencyKey string
	RequestedAt    time.Time
}

// RoutineWorkflowEnsurer materialises (lazily) the routine system
// workflow for a workspace. Heavy routine fires create a fresh task in
// this workflow so auto_start_agent on the start step kicks off the
// agent. The task repo's EnsureRoutineWorkflow satisfies this directly.
type RoutineWorkflowEnsurer interface {
	EnsureRoutineWorkflow(ctx context.Context, workspaceID string) (string, error)
}

// RoutineTaskCreator creates a task pinned to a specific workflow id
// (the routine workflow). Mirrors the office adapter's
// CreateOfficeTaskInWorkflow signature so the binary's existing adapter
// satisfies it for free.
type RoutineTaskCreator interface {
	CreateOfficeTaskInWorkflow(
		ctx context.Context,
		workspaceID, projectID, assigneeAgentID, workflowID, title, description string,
	) (string, error)
}

// RoutineService provides routine CRUD, trigger management, and dispatch logic.
type RoutineService struct {
	repo            Repository
	logger          *logger.Logger
	activity        shared.ActivityLogger
	wakeup          WakeupEnqueuer
	workflowEnsurer RoutineWorkflowEnsurer
	taskCreator     RoutineTaskCreator
}

// NewRoutineService creates a new RoutineService.
func NewRoutineService(repo Repository, log *logger.Logger, activity shared.ActivityLogger) *RoutineService {
	return &RoutineService{
		repo:     repo,
		logger:   log.WithFields(zap.String("component", "routines-service")),
		activity: activity,
	}
}

// SetWakeupEnqueuer wires the wakeup-request dispatcher used by the
// lightweight routine flow. Optional — when nil the lightweight branch
// degrades to a no-op (the run row is created but no wakeup happens).
func (s *RoutineService) SetWakeupEnqueuer(w WakeupEnqueuer) { s.wakeup = w }

// SetWorkflowEnsurer wires the workspace-scoped routine-workflow ensurer
// used by the heavy routine flow. Optional — when nil the heavy branch
// falls back to the lightweight no-task behaviour.
func (s *RoutineService) SetWorkflowEnsurer(e RoutineWorkflowEnsurer) { s.workflowEnsurer = e }

// SetTaskCreator wires the per-workflow task creator used by the heavy
// routine flow. Optional — when nil the heavy branch falls back to
// lightweight behaviour (no task created).
func (s *RoutineService) SetTaskCreator(c RoutineTaskCreator) { s.taskCreator = c }

// CoordinatorRoutineName is the canonical name used for the
// pre-installed coordinator-heartbeat routine. The (workspace_id, name,
// cron_expression) triple is the idempotency key for
// CreateDefaultCoordinatorRoutine — re-running it on an agent that
// already owns this routine returns the existing row.
const CoordinatorRoutineName = "Coordinator heartbeat"

// CoordinatorRoutineCron is the cron expression that drives the
// pre-installed coordinator-heartbeat routine: every five minutes.
// Slower than the previous 60s agent-level heartbeat — that's
// deliberate per the spec's cadence change.
const CoordinatorRoutineCron = "*/5 * * * *"

// CreateDefaultCoordinatorRoutine installs the pre-baked
// "Coordinator heartbeat" routine for a coordinator-role agent. The
// shape is described in office-heartbeat-as-routine: lightweight
// (empty task_template), coalesce_if_active, summarize_missed
// with a max of 25, status=active, with a single cron trigger firing
// every five minutes (UTC).
//
// Idempotent: if the assignee agent already has a routine with the
// canonical name + cron expression the existing routine is returned
// instead of inserting a duplicate.
func (s *RoutineService) CreateDefaultCoordinatorRoutine(
	ctx context.Context, workspaceID, agentID string,
) (*Routine, error) {
	if workspaceID == "" || agentID == "" {
		return nil, fmt.Errorf("create default coordinator routine: workspace_id and agent_id are required")
	}
	if existing, ok := s.findCoordinatorRoutine(ctx, workspaceID, agentID); ok {
		return existing, nil
	}
	routine := &Routine{
		ID:                     uuid.New().String(),
		WorkspaceID:            workspaceID,
		Name:                   CoordinatorRoutineName,
		Description:            "Wakes the coordinator on a recurring schedule so it can monitor the workspace, surface blockers, and react to events while no human is driving the loop.",
		TaskTemplate:           "",
		AssigneeAgentProfileID: agentID,
		Status:                 "active",
		ConcurrencyPolicy:      models.ConcurrencyPolicyCoalesceIfActive,
		CatchUpPolicy:          models.CatchUpPolicySummarizeMissed,
		CatchUpMax:             25,
		Variables:              "{}",
	}
	if err := s.repo.CreateRoutine(ctx, routine); err != nil {
		return nil, fmt.Errorf("create coordinator routine: %w", err)
	}
	trigger := &RoutineTrigger{
		ID:             uuid.New().String(),
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: CoordinatorRoutineCron,
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := s.CreateRoutineTrigger(ctx, trigger); err != nil {
		// The routine row is in place — leave it; the user can wire a
		// trigger from the UI. Returning the routine + the error lets
		// callers log a warn without losing the install.
		return routine, fmt.Errorf("create coordinator trigger: %w", err)
	}
	s.logger.Info("coordinator-heartbeat routine installed",
		zap.String("workspace_id", workspaceID),
		zap.String("agent_id", agentID),
		zap.String("routine_id", routine.ID),
		zap.String("cron", CoordinatorRoutineCron))
	return routine, nil
}

// findCoordinatorRoutine returns the existing coordinator-heartbeat
// routine for the (workspace, agent) pair when one already exists with
// the canonical name and a cron trigger matching CoordinatorRoutineCron.
// The boolean is the "found" flag — errors fall through as not-found
// so the caller proceeds to create a fresh row.
func (s *RoutineService) findCoordinatorRoutine(
	ctx context.Context, workspaceID, agentID string,
) (*Routine, bool) {
	routines, err := s.repo.ListRoutines(ctx, workspaceID)
	if err != nil {
		return nil, false
	}
	for _, r := range routines {
		if r.AssigneeAgentProfileID != agentID {
			continue
		}
		if r.Name != CoordinatorRoutineName {
			continue
		}
		if s.hasCronTrigger(ctx, r.ID, CoordinatorRoutineCron) {
			return r, true
		}
	}
	return nil, false
}

// hasCronTrigger returns true when the routine owns a cron trigger
// with the given expression. Triggers are listed via the routines repo;
// errors degrade to false so the caller treats the routine as missing
// its expected trigger and fixes it up via a fresh install.
func (s *RoutineService) hasCronTrigger(
	ctx context.Context, routineID, cronExpr string,
) bool {
	triggers, err := s.repo.ListTriggersByRoutineID(ctx, routineID)
	if err != nil {
		return false
	}
	for _, t := range triggers {
		if t.Kind == "cron" && t.CronExpression == cronExpr {
			return true
		}
	}
	return false
}

// -- Routine CRUD --

// CreateRoutine creates a new routine in the DB.
func (s *RoutineService) CreateRoutine(ctx context.Context, routine *Routine) error {
	if err := s.repo.CreateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("create routine: %w", err)
	}
	return nil
}

// GetRoutine returns a routine by ID or name.
func (s *RoutineService) GetRoutine(ctx context.Context, id string) (*Routine, error) {
	return s.GetRoutineFromConfig(ctx, id)
}

// ListRoutines returns all routines for a workspace.
func (s *RoutineService) ListRoutines(ctx context.Context, wsID string) ([]*Routine, error) {
	return s.ListRoutinesFromConfig(ctx, wsID)
}

// UpdateRoutine updates a routine in the DB.
func (s *RoutineService) UpdateRoutine(ctx context.Context, routine *Routine) error {
	if err := s.repo.UpdateRoutine(ctx, routine); err != nil {
		return fmt.Errorf("update routine: %w", err)
	}
	return nil
}

// DeleteRoutine deletes a routine from the DB.
func (s *RoutineService) DeleteRoutine(ctx context.Context, id string) error {
	routine, err := s.GetRoutineFromConfig(ctx, id)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteRoutine(ctx, routine.ID); err != nil {
		return fmt.Errorf("delete routine: %w", err)
	}
	return nil
}

// -- Config read helpers --

// GetRoutineFromConfig looks up a routine by ID or name.
func (s *RoutineService) GetRoutineFromConfig(ctx context.Context, idOrName string) (*Routine, error) {
	if routine, err := s.repo.GetRoutine(ctx, idOrName); err == nil {
		return routine, nil
	}
	routines, err := s.repo.ListRoutines(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, r := range routines {
		if r.Name == idOrName {
			return r, nil
		}
	}
	return nil, fmt.Errorf("routine not found: %s", idOrName)
}

// ListRoutinesFromConfig returns all routines for a workspace.
// An empty workspaceID returns rows across all workspaces.
func (s *RoutineService) ListRoutinesFromConfig(ctx context.Context, workspaceID string) ([]*Routine, error) {
	return s.repo.ListRoutines(ctx, workspaceID)
}

// -- Trigger management --

// CreateRoutineTrigger creates a trigger and computes next_run_at for cron.
// A cron trigger is rejected outright when its expression is empty or when
// the expression or timezone cannot be parsed: a trigger of kind "cron"
// must never be persisted with a null next_run_at at creation
// (AC-OFFICE-ROUTINE-CATCHUP-001.8). This is the only place that may arm a
// cron trigger's next_run_at; the reconciliation pass in
// TickScheduledTriggers only re-arms an already-created one.
func (s *RoutineService) CreateRoutineTrigger(ctx context.Context, t *RoutineTrigger) error {
	if t.Kind == "cron" {
		if t.CronExpression == "" {
			return fmt.Errorf("cron trigger requires a non-empty cron expression")
		}
		next, err := shared.NextCronTime(t.CronExpression, t.Timezone, time.Now().UTC())
		if err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
		t.NextRunAt = &next
	}
	return s.repo.CreateRoutineTrigger(ctx, t)
}

// ListRoutineTriggers returns triggers for a routine.
func (s *RoutineService) ListRoutineTriggers(ctx context.Context, routineID string) ([]*RoutineTrigger, error) {
	return s.repo.ListTriggersByRoutineID(ctx, routineID)
}

// DeleteRoutineTrigger deletes a trigger.
func (s *RoutineService) DeleteRoutineTrigger(ctx context.Context, id string) error {
	return s.repo.DeleteRoutineTrigger(ctx, id)
}

// GetTriggerByPublicID returns a trigger by its public ID (for webhook lookup).
func (s *RoutineService) GetTriggerByPublicID(ctx context.Context, publicID string) (*RoutineTrigger, error) {
	return s.repo.GetTriggerByPublicID(ctx, publicID)
}

// -- Run queries --

// ListRoutineRuns returns paginated runs for a routine.
func (s *RoutineService) ListRoutineRuns(ctx context.Context, routineID string, limit, offset int) ([]*RoutineRun, error) {
	return s.repo.ListRoutineRuns(ctx, routineID, limit, offset)
}

// ListAllRoutineRuns returns recent runs across all routines in a workspace.
func (s *RoutineService) ListAllRoutineRuns(ctx context.Context, wsID string, limit int) ([]*RoutineRun, error) {
	return s.repo.ListAllRuns(ctx, wsID, limit)
}

// -- Dispatch --

// catchUpFallbackInterval is the re-arm interval computeCatchUp uses when
// the elapsed-tick walk fails (AC-OFFICE-ROUTINE-CATCHUP-001.11). The
// failure is deterministic in the trigger's stored (expression, timezone)
// pair, not in the time argument, so retrying at the 30-second scheduler
// interval cannot succeed; 24 hours caps the trigger at one dispatch a day
// until it is deleted and recreated, matching shared.findNextMatch's own
// give-up interval.
const catchUpFallbackInterval = 24 * time.Hour

// catchUpReclaimAfter is how stale a claimed-but-never-armed trigger's
// updated_at must be before the reconciliation pass in TickScheduledTriggers
// treats it as abandoned rather than a live claim still mid-tick — two
// scheduler intervals, so a claim taken this tick or the previous one is
// never reconciled (AC-OFFICE-ROUTINE-CATCHUP-001.9).
const catchUpReclaimAfter = 60 * time.Second

// TickScheduledTriggers queries due cron triggers, claims and dispatches
// each in ascending (next_run_at, id) order (AC-001.7), then reconciles
// any trigger left claimed-but-unarmed by a process that stopped mid-tick.
func (s *RoutineService) TickScheduledTriggers(ctx context.Context, now time.Time) error {
	triggers, err := s.repo.GetDueTriggers(ctx, now)
	if err != nil {
		return fmt.Errorf("get due triggers: %w", err)
	}
	for _, trigger := range triggers {
		if err := s.processCronTrigger(ctx, trigger, now); err != nil {
			s.logger.Error("process cron trigger",
				zap.String("trigger_id", trigger.ID), zap.Error(err))
		}
	}
	s.reconcileStrandedTriggers(ctx, now)
	return nil
}

// reconcileStrandedTriggers arms enabled cron triggers whose claim was
// abandoned before the re-arm write landed. It never dispatches a run by
// this path (AC-OFFICE-ROUTINE-CATCHUP-001.9): the row cannot distinguish
// "crashed after claiming" from "crashed after claiming and dispatching",
// and a spurious wake is the more expensive error. A parse failure leaves
// next_run_at null and only warns — no claim was taken by this path, so no
// run is owed and AC-001.11's fallback does not apply.
func (s *RoutineService) reconcileStrandedTriggers(ctx context.Context, now time.Time) {
	stale, err := s.repo.ListStrandedTriggers(ctx, now.Add(-catchUpReclaimAfter))
	if err != nil {
		s.logger.Warn("list stranded triggers", zap.Error(err))
		return
	}
	for _, trigger := range stale {
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, now)
		if err != nil {
			s.logger.Warn("reconcile stranded trigger: compute next run",
				zap.String("trigger_id", trigger.ID), zap.Error(err))
			continue
		}
		if _, err := s.repo.ReconcileTriggerNextRun(ctx, trigger.ID, next); err != nil {
			s.logger.Warn("reconcile stranded trigger: arm",
				zap.String("trigger_id", trigger.ID), zap.Error(err))
		}
	}
}

func (s *RoutineService) processCronTrigger(ctx context.Context, trigger *RoutineTrigger, now time.Time) error {
	if trigger.NextRunAt == nil {
		return nil
	}
	claimed, err := s.repo.ClaimTrigger(ctx, trigger.ID, *trigger.NextRunAt)
	if err != nil || !claimed {
		return err
	}
	routine, err := s.GetRoutineFromConfig(ctx, trigger.RoutineID)
	if err != nil {
		return fmt.Errorf("get routine: %w", err)
	}
	result := computeCatchUp(trigger, routine, now)
	if result.Unknown {
		s.logger.Warn("compute routine catch-up failed",
			zap.String("trigger_id", trigger.ID),
			zap.String("cron_expression", trigger.CronExpression),
			zap.Error(result.Err))
	}
	if err := s.repo.UpdateTriggerNextRun(ctx, trigger.ID, &result.NextRunAt); err != nil {
		// AC-001.2: an arming-write failure means this claim dispatches
		// nothing at all; next_run_at is left null for the reconciliation
		// pass (AC-001.9) to arm on a later tick. That tick is lost.
		s.logger.Warn("update trigger next_run_at failed",
			zap.String("trigger_id", trigger.ID), zap.Error(err))
		return fmt.Errorf("arm trigger: %w", err)
	}
	gap := buildGapSummary(routine, result)
	run, err := s.dispatchRoutineRun(ctx, routine, trigger, shared.RoutineSourceCron, nil, "", gap)
	if err != nil && run != nil {
		// The status flip has one home: whichever branch of dispatch
		// failed, the run created for this claim is marked failed exactly
		// once, here (AC-001.10).
		if updateErr := s.repo.UpdateRunStatus(ctx, run.ID, models.RoutineRunStatusFailed, ""); updateErr != nil {
			s.logger.Warn("mark routine run failed",
				zap.String("run_id", run.ID), zap.Error(updateErr))
		}
	}
	return err
}

// catchUpResult is what computeCatchUp returns instead of a bare
// (int, time.Time, error): a single value object so no call site can
// discard a computed re-arm time on the error path.
type catchUpResult struct {
	// ElapsedTicks is >= 1 on success (Unknown false); includes the tick
	// due now. Not meaningful when Unknown is true.
	ElapsedTicks int
	// FirstMissedAt is the armed next_run_at at claim time, read directly
	// rather than derived from the walk so it stays exact under
	// truncation. Zero when ElapsedTicks <= 1.
	FirstMissedAt time.Time
	// Truncated is true only when the walk stopped at the cap with the
	// cursor still at or before the processing instant.
	Truncated bool
	// NextRunAt is strictly after the processing instant on every path,
	// including Unknown — it is never `now`.
	NextRunAt time.Time
	// Unknown is true when the walk failed; ElapsedTicks, FirstMissedAt
	// and Truncated are not meaningful in that case.
	Unknown bool
	// Err is the underlying cron computation error when Unknown is true.
	// Nil otherwise.
	Err error
}

// computeCatchUp walks cron ticks between trigger.NextRunAt (inclusive —
// that's the tick "due now") and now, capped at
// models.NormaliseCatchUpMax(routine.CatchUpMax). It replaces
// computeRoutineMissed: catch_up_max bounds only how many elapsed ticks are
// counted, never how many runs are dispatched
// (AC-OFFICE-ROUTINE-CATCHUP-001.3).
func computeCatchUp(trigger *RoutineTrigger, routine *Routine, now time.Time) catchUpResult {
	cap := models.NormaliseCatchUpMax(routine.CatchUpMax)
	firstMissedAt := *trigger.NextRunAt
	cursor := firstMissedAt
	elapsed := 0
	for elapsed < cap && !cursor.After(now) {
		elapsed++
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, cursor)
		if err != nil {
			return catchUpResult{NextRunAt: now.Add(catchUpFallbackInterval), Unknown: true, Err: err}
		}
		cursor = next
	}
	truncated := false
	if elapsed == cap && !cursor.After(now) {
		// Hit the cap with more pending ticks. Advance the cursor to the
		// next future tick from now so the trigger leaves the catch-up
		// window cleanly, and record the count as a lower bound.
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, now)
		if err != nil {
			return catchUpResult{NextRunAt: now.Add(catchUpFallbackInterval), Unknown: true, Err: err}
		}
		cursor = next
		truncated = true
	}
	result := catchUpResult{ElapsedTicks: elapsed, NextRunAt: cursor, Truncated: truncated}
	if elapsed > 1 {
		result.FirstMissedAt = firstMissedAt
	}
	return result
}

// gapSummary is the durable measurement of one claim's gap, threaded from
// processCronTrigger through the dispatch chain into the created run's
// three catch_up_* columns and, for a lightweight routine under the
// summarizing policy, the wakeup payload. A nil *gapSummary means no gap
// summary applies for this claim.
type gapSummary struct {
	MissedTicks int
	FirstMissed time.Time
	Truncated   bool
}

// buildGapSummary decides whether a claim's computed catch-up result
// produces a gap summary. A gap summary exists only when the walk
// succeeded, the policy is summarize_missed, and at least one tick was
// missed — collapsing what would otherwise be several near-identical states
// into one rule (AC-002.1, AC-002.2, AC-002.7, AC-002.11, AC-002.12: manual
// and webhook fires never reach this function, since only processCronTrigger
// calls it).
func buildGapSummary(routine *Routine, result catchUpResult) *gapSummary {
	if result.Unknown {
		return nil // AC-001.6: never report a gap that was not measured.
	}
	if routine.CatchUpPolicy == models.CatchUpPolicySkipMissed {
		return nil // AC-002.7
	}
	missed := result.ElapsedTicks - 1
	if missed < 1 {
		return nil // AC-002.2 (zero missed); AC-002.11 (catch_up_max == 1).
	}
	return &gapSummary{MissedTicks: missed, FirstMissed: result.FirstMissedAt, Truncated: result.Truncated}
}

// DispatchRoutineRun resolves variables, applies concurrency policy, creates run.
//
// PR 3 of office-heartbeat-rework: the legacy `LinkedTaskID = "task-<runID>"`
// placeholder is replaced with two real shapes governed by routine config:
//
//   - lightweight (task_template == "") — taskless. Insert an
//     agent_wakeup_requests row and dispatch via the wakeup dispatcher;
//     the dispatcher creates a fresh taskless run against the assignee
//     agent and runs the existing claim-time coalesce machinery.
//
//   - heavy (task_template != "") — create a real task in the routine
//     system workflow (auto_start_agent on the start step kicks off the
//     agent). LinkedTaskID is the new task's id.
//
// Manual and webhook fires call this directly and never carry a gap
// summary (AC-OFFICE-ROUTINE-CATCHUP-002.12) — gap measurement applies only
// to a cron trigger's armed next_run_at, computed by processCronTrigger.
func (s *RoutineService) DispatchRoutineRun(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
) (*RoutineRun, error) {
	return s.dispatchRoutineRun(ctx, routine, trigger, source, provided, "", nil)
}

// DispatchRoutineRunWithIdempotencyKey dispatches a fire with an explicit
// source request identity. Webhook callers can use this header to make
// retries idempotent without collapsing unrelated deliveries.
func (s *RoutineService) DispatchRoutineRunWithIdempotencyKey(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
	idempotencyKey string,
) (*RoutineRun, error) {
	return s.dispatchRoutineRun(ctx, routine, trigger, source, provided, idempotencyKey, nil)
}

func (s *RoutineService) dispatchRoutineRun(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
	idempotencyKey string,
	gap *gapSummary,
) (*RoutineRun, error) {
	now := time.Now().UTC()
	defaults := parseDeclaredDefaults(routine.Variables)
	vars := shared.ResolveVariables(now, defaults, provided)

	tmpl := parseTaskTemplate(routine.TaskTemplate)
	title := taskservice.TruncateTaskTitle(shared.InterpolateTemplate(tmpl.Title, vars))
	description := shared.InterpolateTemplate(tmpl.Description, vars)
	fingerprint := computeFingerprint(title, description, routine.AssigneeAgentProfileID)

	triggerID := ""
	if trigger != nil {
		triggerID = trigger.ID
	}
	payloadJSON, _ := json.Marshal(vars)

	run := &RoutineRun{
		RoutineID:           routine.ID,
		TriggerID:           triggerID,
		Source:              source,
		Status:              models.RoutineRunStatusReceived,
		TriggerPayload:      string(payloadJSON),
		DispatchFingerprint: fingerprint,
		StartedAt:           &now,
	}
	if gap != nil {
		missed := gap.MissedTicks
		firstMissed := gap.FirstMissed
		run.CatchUpMissedTicks = &missed
		run.CatchUpFirstMissedAt = &firstMissed
		run.CatchUpTruncated = gap.Truncated
	}
	// The gap summary is written once, here, before applyConcurrencyPolicy
	// runs — a run later marked skipped, coalesced or failed still carries
	// the gap measured for its tick (AC-002.10).
	if err := s.repo.CreateRoutineRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create run: %w", err) // AC-001.12: no run row exists.
	}

	status, err := s.applyConcurrencyPolicy(ctx, routine, run, fingerprint)
	if err != nil {
		return run, err
	}
	if status != "" {
		return run, nil
	}

	if err := s.materialiseRoutineRun(ctx, routine, run, tmpl, title, description, vars, source, idempotencyKey, gap); err != nil {
		return run, err
	}

	if err := s.repo.TouchRoutineLastRun(ctx, routine.ID, now); err != nil {
		s.logger.Warn("touch routine last_run_at",
			zap.String("routine", routine.Name), zap.Error(err))
	}
	routine.LastRunAt = &now
	s.logger.Info("routine run dispatched",
		zap.String("routine", routine.Name),
		zap.String("run_id", run.ID),
		zap.String("title", title),
		zap.Bool("heavy", tmpl.Title != ""))
	return run, nil
}

// materialiseRoutineRun branches on tmpl.Title to choose the lightweight
// (taskless) or heavy (real task) path. The heavy path transitions the
// run to models.RoutineRunStatusTaskCreated and attaches the new task's
// id — that status stays until the linked task reaches a terminal step
// (see SyncRunStatus). The lightweight path has no task to wait on, so
// it resolves to a terminal status (done/failed) immediately; see
// materialiseLightweightRoutineRun.
// gap is forwarded to the lightweight wakeup payload; the heavy path
// doesn't render it — mutating a user's template text with scheduler
// metadata would be a worse contract than leaving the gap on the routine
// run where the API exposes it (AC-002.6).
func (s *RoutineService) materialiseRoutineRun(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	tmpl taskTemplate,
	title, description string,
	vars map[string]string,
	source string,
	idempotencyKey string,
	gap *gapSummary,
) error {
	if tmpl.Title != "" && s.workflowEnsurer != nil && s.taskCreator != nil {
		return s.materialiseHeavyRoutineRun(ctx, routine, run, title, description)
	}
	return s.materialiseLightweightRoutineRun(ctx, routine, run, vars, source, idempotencyKey, gap)
}

// materialiseHeavyRoutineRun creates a real task in the routine system
// workflow and attaches its id to the routine run. The task lands in
// the workflow's start step where auto_start_agent fires the agent —
// no separate wakeup-request is needed.
func (s *RoutineService) materialiseHeavyRoutineRun(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	title, description string,
) error {
	workflowID, err := s.workflowEnsurer.EnsureRoutineWorkflow(ctx, routine.WorkspaceID)
	if err != nil {
		return fmt.Errorf("ensure routine workflow: %w", err)
	}
	taskID, err := s.taskCreator.CreateOfficeTaskInWorkflow(
		ctx, routine.WorkspaceID, "", routine.AssigneeAgentProfileID,
		workflowID, title, description,
	)
	if err != nil {
		return fmt.Errorf("create routine task: %w", err)
	}
	run.Status = models.RoutineRunStatusTaskCreated
	run.LinkedTaskID = taskID
	if err := s.repo.UpdateRunStatus(ctx, run.ID, run.Status, run.LinkedTaskID); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	return nil
}

// materialiseLightweightRoutineRun enqueues a wakeup-request for the
// routine assignee with source="routine" and then terminates this run:
// a lightweight routine has no task to wait on, so its own lifecycle
// ends the moment the request is enqueued and handed to the wakeup
// dispatcher. From there the dispatcher owns everything downstream,
// including its own concurrency gate over the same routine policy
// (wakeup/dispatcher.go resolvePolicy/resolveRoutinePolicy) — so this
// run must not sit in an "active" status waiting for that outcome, or
// it permanently blocks the routine's next fire under skip_if_active /
// coalesce_if_active (see the office-routine-runs triage).
//
// Terminal status: `done` once the request is dispatched; `failed` when
// enqueue or dispatch fails. A failed request is marked terminal in the
// wakeup queue because no background poller retries direct dispatch.
// LinkedTaskID stays empty for lightweight runs.
//
// gap, when non-nil, surfaces in the wakeup payload as the gap this claim
// measured (catch-up policy summarize_missed). CreateWakeupRequest and
// Dispatch errors are returned rather than absorbed — the only exception is
// the idempotency-key-conflict sentinel, which is a successful dedup, not a
// failed dispatch, and stays a no-op (AC-OFFICE-ROUTINE-CATCHUP-001.10).
func (s *RoutineService) materialiseLightweightRoutineRun(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	vars map[string]string,
	source string,
	idempotencyKey string,
	gap *gapSummary,
) error {
	if s.wakeup == nil || routine.AssigneeAgentProfileID == "" {
		return s.finalizeLightweightRun(ctx, run, models.RoutineRunStatusDone)
	}
	idemKey := buildRoutineIdempotencyKey(routine.ID, run.TriggerID, source, run.ID, idempotencyKey, run.StartedAt)
	payloadStr, _ := marshalRoutinePayload(routine.ID, vars, gap)
	req := &WakeupRequest{
		ID:             uuid.New().String(),
		AgentProfileID: routine.AssigneeAgentProfileID,
		Source:         "routine",
		Reason:         shared.RoutineDispatchReason(run.Source),
		Payload:        payloadStr,
		IdempotencyKey: idemKey,
		RequestedAt:    time.Now().UTC(),
	}
	if err := s.wakeup.CreateWakeupRequest(ctx, req); err != nil {
		if errors.Is(err, ErrWakeupAlreadyRequested) {
			// A request for this explicit fire identity already exists.
			// Another delivery completed the same request.
			return s.finalizeLightweightRun(ctx, run, models.RoutineRunStatusDone)
		}
		s.logger.Warn("create routine wakeup request",
			zap.String("routine", routine.Name), zap.Error(err))
		return s.finalizeLightweightRun(ctx, run, models.RoutineRunStatusFailed)
	}
	if err := s.wakeup.Dispatch(ctx, req.ID); err != nil {
		s.logger.Warn("dispatch routine wakeup request",
			zap.String("routine", routine.Name),
			zap.String("wakeup_id", req.ID),
			zap.Error(err))
		if failErr := s.wakeup.FailWakeupRequest(ctx, req.ID, "dispatch failed"); failErr != nil {
			s.logger.Warn("mark failed routine wakeup request",
				zap.String("routine", routine.Name),
				zap.String("wakeup_id", req.ID),
				zap.Error(failErr))
		}
		if finalizeErr := s.finalizeLightweightRun(ctx, run, models.RoutineRunStatusFailed); finalizeErr != nil {
			return finalizeErr
		}
		return fmt.Errorf("dispatch routine wakeup request: %w", err)
	}
	return s.finalizeLightweightRun(ctx, run, models.RoutineRunStatusDone)
}

// finalizeLightweightRun writes the lightweight run's terminal status.
// LinkedTaskID is always empty for lightweight runs.
func (s *RoutineService) finalizeLightweightRun(
	ctx context.Context, run *RoutineRun, status models.RoutineRunStatus,
) error {
	run.Status = status
	if err := s.repo.UpdateRunStatus(ctx, run.ID, status, ""); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	return nil
}

// buildRoutineIdempotencyKey composes the source-level dedup key for a
// routine fire. Cron fires use the trigger and minute bucket. Manual and
// webhook fires use a unique run identity unless the caller supplies an
// explicit request key, so distinct event deliveries never collide.
func buildRoutineIdempotencyKey(
	routineID, triggerID, source, runID, explicitKey string, startedAt *time.Time,
) string {
	if explicitKey != "" {
		return fmt.Sprintf("routine:%s:%s:%s", routineID, source, explicitKey)
	}
	if source != shared.RoutineSourceCron {
		return fmt.Sprintf("routine:%s:%s:%s", routineID, source, runID)
	}
	now := time.Now().UTC()
	if startedAt != nil {
		now = *startedAt
	}
	minute := now.Unix() / 60
	if triggerID == "" {
		return fmt.Sprintf("routine:%s:%s:%d", routineID, source, minute)
	}
	return fmt.Sprintf("routine:%s:%s:%d", routineID, triggerID, minute)
}

// marshalRoutinePayload renders the wakeup-request payload for a
// routine fire as JSON. The shape mirrors wakeup.RoutinePayload:
// {routine_id, variables, missed_ticks, missed_since, missed_truncated}.
// The three catch-up fields are set together, from gap, only when the
// claim measured at least one missed tick under the summarize_missed
// policy — they state the ticks counted and reported, never the ticks
// fired (only one run is ever dispatched per claim).
func marshalRoutinePayload(routineID string, vars map[string]string, gap *gapSummary) (string, error) {
	body := map[string]any{
		"routine_id": routineID,
	}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	if gap != nil {
		body["missed_ticks"] = gap.MissedTicks
		body["missed_since"] = gap.FirstMissed.UTC().Format(time.RFC3339)
		if gap.Truncated {
			body["missed_truncated"] = true
		}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
}

// selfHealIfTaskTerminal is the pull-based counterpart to SyncRunStatus:
// SyncRunStatus is driven by the TaskMoved event, but nothing in
// production populates the step names that event needs
// (office-routine-runs R1), so the gate cannot rely on that event alone
// ever clearing it. Reading the linked task's real state here means a
// heavy run's gate self-heals at the next fire even when that event
// never arrives. Returns active unchanged when it is still genuinely
// active, still lightweight (no linked task), or its state can't be
// determined. A lookup or close-out failure fails closed, leaving the task
// active until a later fire can retry reconciliation. Returns nil once the
// linked task is confirmed terminal, archived, or missing and the run closes.
func (s *RoutineService) selfHealIfTaskTerminal(
	ctx context.Context, routine *Routine, active *RoutineRun,
) *RoutineRun {
	if active.LinkedTaskID == "" {
		return active
	}
	terminalStatus, err := s.repo.GetTaskTerminalStatus(ctx, active.LinkedTaskID)
	if err != nil {
		s.logger.Warn("check linked task terminal state",
			zap.String("routine", routine.Name), zap.String("run_id", active.ID), zap.Error(err))
		return active
	}
	if terminalStatus == "" {
		return active
	}
	if _, err := s.closeOutRun(ctx, active, terminalStatus); err != nil {
		s.logger.Warn("close out stale active run",
			zap.String("routine", routine.Name), zap.String("run_id", active.ID), zap.Error(err))
		return active
	}
	return nil
}

func (s *RoutineService) applyConcurrencyPolicy(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	fingerprint string,
) (models.RoutineRunStatus, error) {
	if routine.ConcurrencyPolicy == models.ConcurrencyPolicyAlwaysCreate {
		return "", nil
	}
	for {
		active, err := s.repo.GetActiveRunForFingerprint(ctx, routine.ID, fingerprint)
		if err != nil {
			return "", fmt.Errorf("check active run: %w", err)
		}
		if active == nil {
			return "", nil
		}
		if repaired := s.selfHealIfTaskTerminal(ctx, routine, active); repaired == nil {
			continue
		} else {
			active = repaired
		}
		switch routine.ConcurrencyPolicy {
		case models.ConcurrencyPolicySkipIfActive:
			_ = s.repo.UpdateRunStatus(ctx, run.ID, models.RoutineRunStatusSkipped, "")
			run.Status = models.RoutineRunStatusSkipped
			return models.RoutineRunStatusSkipped, nil
		case models.ConcurrencyPolicyCoalesceIfActive:
			_ = s.repo.UpdateRunCoalesced(ctx, run.ID, active.ID)
			run.Status = models.RoutineRunStatusCoalesced
			run.CoalescedIntoRunID = active.ID
			return models.RoutineRunStatusCoalesced, nil
		}
	}
}

// FireManual dispatches a routine run from a manual trigger.
func (s *RoutineService) FireManual(
	ctx context.Context, routineID string, variableValues map[string]string,
) (*RoutineRun, error) {
	routine, err := s.GetRoutineFromConfig(ctx, routineID)
	if err != nil {
		return nil, fmt.Errorf("get routine: %w", err)
	}
	return s.DispatchRoutineRun(ctx, routine, nil, "manual", variableValues)
}

// SyncRunStatus closes out the heavy routine run linked to taskID when
// that task reaches a terminal step. This is the writer for the
// terminal side of D2 in the office-routine-runs triage: heavy runs
// stay in RoutineRunStatusTaskCreated (an active gate) until their task
// finishes, and this is what clears the gate. terminalStatus is
// "cancelled" for a task moved to the Cancelled step, "failed" for
// a failed task, and "done" for a completed task. A taskID with no
// linked run is not an error — most tasks aren't routine-created. An
// empty taskID is rejected outright: linked_task_id defaults to "" for
// every lightweight run, so an unguarded lookup would match (and
// rewrite) an arbitrary lightweight run instead of finding nothing.
func (s *RoutineService) SyncRunStatus(ctx context.Context, taskID, terminalStatus string) error {
	if taskID == "" {
		return nil
	}
	run, err := s.repo.GetRoutineRunByLinkedTaskID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("get routine run by linked task: %w", err)
	}
	if run == nil {
		return nil
	}
	_, err = s.closeOutRun(ctx, run, terminalStatus)
	return err
}

// closeOutRun writes a run's terminal status, but only while it is still
// task_created (see Repository.UpdateRunStatusIfTaskCreated) — a replayed
// or racing terminal signal for an already-closed run must not move
// completed_at or flip a terminal result. terminalStatus maps to the
// matching routine-run terminal state. Returns whether this call was the
// one that closed the run.
func (s *RoutineService) closeOutRun(ctx context.Context, run *RoutineRun, terminalStatus string) (bool, error) {
	status := models.RoutineRunStatusDone
	switch terminalStatus {
	case "cancelled":
		status = models.RoutineRunStatusCancelled
	case "failed", "missing":
		status = models.RoutineRunStatusFailed
	}
	closed, err := s.repo.UpdateRunStatusIfTaskCreated(ctx, run.ID, status, run.LinkedTaskID)
	if err != nil {
		return false, fmt.Errorf("update run status: %w", err)
	}
	return closed, nil
}

// -- helpers --

type taskTemplate struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func parseTaskTemplate(raw string) taskTemplate {
	var tmpl taskTemplate
	_ = json.Unmarshal([]byte(raw), &tmpl)
	return tmpl
}

func parseDeclaredDefaults(variablesJSON string) map[string]string {
	defaults := make(map[string]string)
	var vars map[string]struct {
		Default string `json:"default"`
	}
	if err := json.Unmarshal([]byte(variablesJSON), &vars); err != nil {
		return defaults
	}
	for k, v := range vars {
		if v.Default != "" {
			defaults[k] = v.Default
		}
	}
	return defaults
}

// computeFingerprint is the dedup key for "an identical unit of work is
// already in flight". Lightweight runs never sit in an active status
// (see materialiseLightweightRoutineRun), so the fingerprint only gates
// heavy runs in practice: it means "a task with this exact rendered
// title, description, and assignee is still open".
func computeFingerprint(title, description, assignee string) string {
	h := sha256.Sum256([]byte(title + "|" + description + "|" + assignee))
	return fmt.Sprintf("%x", h[:16])
}
