package routines

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

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
	DeleteRoutineTrigger(ctx context.Context, id string) error

	CreateRoutineRun(ctx context.Context, run *RoutineRun) error
	ListRoutineRuns(ctx context.Context, routineID string, limit, offset int) ([]*RoutineRun, error)
	ListAllRuns(ctx context.Context, workspaceID string, limit int) ([]*RoutineRun, error)
	GetActiveRunForFingerprint(ctx context.Context, routineID, fingerprint string) (*RoutineRun, error)
	UpdateRunStatus(ctx context.Context, runID string, status models.RoutineRunStatus, linkedTaskID string) error
	UpdateRunCoalesced(ctx context.Context, runID, coalescedIntoRunID string) error
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
// (empty task_template), coalesce_if_active, enqueue_missed_with_cap
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
		CatchUpPolicy:          models.CatchUpPolicyEnqueueMissedWithCap,
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
func (s *RoutineService) CreateRoutineTrigger(ctx context.Context, t *RoutineTrigger) error {
	if t.Kind == "cron" && t.CronExpression != "" {
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

// TickScheduledTriggers queries due cron triggers, claims each, and dispatches.
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
	return nil
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
	// Catch-up cap: count how many cron ticks elapsed between the missed
	// trigger.NextRunAt (inclusive) and now, capped at routine.CatchUpMax.
	// Mirror of the agent_heartbeat catch-up math, adapted for cron
	// expressions (no fixed interval — we walk NextCronTime).
	runCount, advanceTo, err := computeRoutineMissed(trigger, routine, now)
	if err != nil {
		s.logger.Warn("compute routine catch-up failed",
			zap.String("trigger_id", trigger.ID), zap.Error(err))
		// Fall through with sane defaults: fire once, advance one tick.
		runCount = 1
		advanceTo = now
	}
	if err := s.repo.UpdateTriggerNextRun(ctx, trigger.ID, &advanceTo); err != nil {
		s.logger.Warn("update trigger next_run_at failed",
			zap.String("trigger_id", trigger.ID), zap.Error(err))
	}
	missedForPayload := 0
	if routine.CatchUpPolicy == models.CatchUpPolicySkipMissed {
		// skip_missed: collapse all misses, fire once with no attribution.
		missedForPayload = 0
	} else if runCount > 0 {
		// enqueue_missed_with_cap (default and unknowns): fire once, surface
		// runCount-1 as missed_ticks ("you missed N since the last fire").
		missedForPayload = runCount - 1
	}
	_, err = s.DispatchRoutineRunWithMissed(ctx, routine, trigger, "cron", nil, missedForPayload)
	return err
}

const defaultCatchUpMax = 25

// computeRoutineMissed counts cron ticks between trigger.NextRunAt
// (inclusive — that's the tick "due now") and `now`, capped at
// routine.CatchUpMax (default 25). Returns the count plus the next
// future tick the trigger should re-arm to.
//
// For an "every minute" cron with a 10-minute backend outage, this
// returns 11 (10 missed + 1 due now) and the next-minute tick. With a
// cap of 5, it returns 5 and aligns the cursor to the next future tick
// from `now` so the trigger leaves the catch-up window cleanly.
func computeRoutineMissed(trigger *RoutineTrigger, routine *Routine, now time.Time) (int, time.Time, error) {
	cap := routine.CatchUpMax
	if cap <= 0 {
		cap = defaultCatchUpMax
	}
	cursor := *trigger.NextRunAt
	runCount := 0
	for runCount < cap && !cursor.After(now) {
		runCount++
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, cursor)
		if err != nil {
			return runCount, cursor, fmt.Errorf("next cron tick: %w", err)
		}
		cursor = next
	}
	if runCount == cap && !cursor.After(now) {
		// Hit the cap with more pending ticks. Advance cursor to the next
		// future tick from now to leave the catch-up window cleanly.
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, now)
		if err != nil {
			return runCount, cursor, fmt.Errorf("next cron tick from now: %w", err)
		}
		cursor = next
	}
	if runCount == 0 {
		// Defensive: ClaimTrigger should only succeed when NextRunAt <= now,
		// so the loop must have iterated at least once. If clock skew leaves
		// us here, fire once and advance by one tick.
		next, err := shared.NextCronTime(trigger.CronExpression, trigger.Timezone, *trigger.NextRunAt)
		if err != nil {
			return 1, *trigger.NextRunAt, fmt.Errorf("defensive next cron tick: %w", err)
		}
		return 1, next, nil
	}
	return runCount, cursor, nil
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
func (s *RoutineService) DispatchRoutineRun(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
) (*RoutineRun, error) {
	return s.dispatchRoutineRun(ctx, routine, trigger, source, provided, 0)
}

// DispatchRoutineRunWithMissed is the cron-tick entry point that
// surfaces the catch-up cap's missed-tick count to the wakeup payload.
// missedTicks is set to N-1 by the cron tick when N consecutive cron
// ticks fell within the catch-up window — the agent gets one fire and
// learns "you missed N-1 ticks" via wakeup.RoutinePayload.MissedTicks.
// Manual fires (UI / API) call DispatchRoutineRun directly with no
// missed-tick attribution.
func (s *RoutineService) DispatchRoutineRunWithMissed(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
	missedTicks int,
) (*RoutineRun, error) {
	return s.dispatchRoutineRun(ctx, routine, trigger, source, provided, missedTicks)
}

func (s *RoutineService) dispatchRoutineRun(
	ctx context.Context,
	routine *Routine,
	trigger *RoutineTrigger,
	source string,
	provided map[string]string,
	missedTicks int,
) (*RoutineRun, error) {
	now := time.Now().UTC()
	defaults := parseDeclaredDefaults(routine.Variables)
	vars := shared.ResolveVariables(now, defaults, provided)

	tmpl := parseTaskTemplate(routine.TaskTemplate)
	title := shared.InterpolateTemplate(tmpl.Title, vars)
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
	if err := s.repo.CreateRoutineRun(ctx, run); err != nil {
		return nil, fmt.Errorf("create run: %w", err)
	}

	status, err := s.applyConcurrencyPolicy(ctx, routine, run, fingerprint)
	if err != nil {
		return run, err
	}
	if status != "" {
		return run, nil
	}

	if err := s.materialiseRoutineRun(ctx, routine, run, tmpl, title, description, vars, missedTicks); err != nil {
		return run, err
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
// (taskless) or heavy (real task) path. Both paths transition the run
// to models.RoutineRunStatusTaskCreated; only the heavy path attaches a real task id.
// missedTicks is forwarded to the lightweight wakeup payload; the heavy
// path doesn't attribute it (the agent reads context from the task).
func (s *RoutineService) materialiseRoutineRun(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	tmpl taskTemplate,
	title, description string,
	vars map[string]string,
	missedTicks int,
) error {
	if tmpl.Title != "" && s.workflowEnsurer != nil && s.taskCreator != nil {
		return s.materialiseHeavyRoutineRun(ctx, routine, run, title, description)
	}
	return s.materialiseLightweightRoutineRun(ctx, routine, run, vars, missedTicks)
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
// routine assignee with source="routine". The wakeup dispatcher claims
// it, applies the per-routine concurrency policy, and creates a fresh
// taskless runs row. LinkedTaskID stays empty for lightweight.
//
// missedTicks > 0 surfaces in the wakeup payload when the cron tick
// collapsed N missed fires into one (catch-up cap policy
// enqueue_missed_with_cap). Zero on the happy path.
func (s *RoutineService) materialiseLightweightRoutineRun(
	ctx context.Context,
	routine *Routine,
	run *RoutineRun,
	vars map[string]string,
	missedTicks int,
) error {
	run.Status = models.RoutineRunStatusTaskCreated
	if err := s.repo.UpdateRunStatus(ctx, run.ID, run.Status, ""); err != nil {
		return fmt.Errorf("update run status: %w", err)
	}
	if s.wakeup == nil || routine.AssigneeAgentProfileID == "" {
		return nil
	}
	idemKey := buildRoutineIdempotencyKey(routine.ID, run.TriggerID, run.StartedAt)
	payloadStr, _ := marshalRoutinePayload(routine.ID, vars, missedTicks)
	req := &WakeupRequest{
		ID:             uuid.New().String(),
		AgentProfileID: routine.AssigneeAgentProfileID,
		Source:         "routine",
		Reason:         "routine_dispatch",
		Payload:        payloadStr,
		IdempotencyKey: idemKey,
		RequestedAt:    time.Now().UTC(),
	}
	if err := s.wakeup.CreateWakeupRequest(ctx, req); err != nil {
		// Idempotency conflicts are quietly absorbed by the office repo
		// returning a sentinel error; the caller treats it as a no-op.
		s.logger.Warn("create routine wakeup request",
			zap.String("routine", routine.Name), zap.Error(err))
		return nil
	}
	if err := s.wakeup.Dispatch(ctx, req.ID); err != nil {
		s.logger.Warn("dispatch routine wakeup request",
			zap.String("routine", routine.Name),
			zap.String("wakeup_id", req.ID),
			zap.Error(err))
	}
	return nil
}

// buildRoutineIdempotencyKey composes the source-level dedup key for a
// routine fire. Format: routine:<routineID>:<triggerID>:<unix_minute>;
// the trigger segment is dropped when there is no trigger (manual fire).
// Mirrors the heartbeat key shape (heartbeat:<agent>:<unix_minute>).
func buildRoutineIdempotencyKey(routineID, triggerID string, startedAt *time.Time) string {
	now := time.Now().UTC()
	if startedAt != nil {
		now = *startedAt
	}
	minute := now.Unix() / 60
	if triggerID == "" {
		return fmt.Sprintf("routine:%s:%d", routineID, minute)
	}
	return fmt.Sprintf("routine:%s:%s:%d", routineID, triggerID, minute)
}

// marshalRoutinePayload renders the wakeup-request payload for a
// routine fire as JSON. The shape mirrors wakeup.RoutinePayload:
// {routine_id, variables, missed_ticks}. missed_ticks is set by the
// cron tick when the catch-up cap collapsed N missed fires into one.
func marshalRoutinePayload(routineID string, vars map[string]string, missedTicks int) (string, error) {
	body := map[string]any{
		"routine_id": routineID,
	}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	if missedTicks > 0 {
		body["missed_ticks"] = missedTicks
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "{}", err
	}
	return string(b), nil
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
	active, err := s.repo.GetActiveRunForFingerprint(ctx, routine.ID, fingerprint)
	if err != nil {
		return "", fmt.Errorf("check active run: %w", err)
	}
	if active == nil {
		return "", nil
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
	return "", nil
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

// SyncRunStatus updates a linked run when its task reaches a terminal state.
func (s *RoutineService) SyncRunStatus(ctx context.Context, taskID, terminalStatus string) error {
	s.logger.Info("sync run status",
		zap.String("task_id", taskID),
		zap.String("status", terminalStatus))
	return nil
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

func computeFingerprint(title, description, assignee string) string {
	h := sha256.Sum256([]byte(title + "|" + description + "|" + assignee))
	return fmt.Sprintf("%x", h[:16])
}
