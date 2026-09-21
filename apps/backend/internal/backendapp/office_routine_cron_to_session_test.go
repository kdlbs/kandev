package backendapp

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/agents"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officecosts "github.com/kandev/kandev/internal/office/costs"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	officeservice "github.com/kandev/kandev/internal/office/service"
	"github.com/kandev/kandev/internal/office/shared"
	officewakeup "github.com/kandev/kandev/internal/office/wakeup"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	sqlitetaskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// routineCronHarness wires the real routines service, the real task/workflow
// services, and a real orchestrator.Service on one shared in-memory event
// bus. newBootStateTestHarness (helpers_test.go) cannot be reused directly:
// it builds its event bus privately, so nothing outside the task service
// would ever observe a task.created event published through it.
type routineCronHarness struct {
	taskSvc      *taskservice.Service
	taskRepo     *sqlitetaskrepo.Repository
	workflowSvc  *workflowservice.Service
	officeRepo   *officesqlite.Repository
	routineSvc   *officeroutines.RoutineService
	orchestrator *orchestrator.Service
	agentMgr     *stubAgentManager
	workspaceID  string
	db           *sqlx.DB
	// eventBus is the single bus the harness's real taskSvc, workflowSvc,
	// and orchestrator subscribe to. A test needing to observe how the
	// real orchestrator reacts to a run-owned lifecycle event must wire
	// its own office/service.Service onto this same bus rather than
	// standing up a private, disconnected one — TestRoutine_CronFire_LightweightReachesSession
	// builds its own local bus for exactly that reason, so it never
	// actually exercises what the real orchestrator does with a taskless
	// event; this field exists so a different test can.
	eventBus bus.EventBus
}

func newRoutineCronHarness(t *testing.T) *routineCronHarness {
	t.Helper()
	dbConn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "routine-cron.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })

	taskRepo, cleanup, err := taskrepo.Provide(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("task repository: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	if _, err := worktree.NewSQLiteStore(sqlxDB, sqlxDB); err != nil {
		t.Fatalf("worktree store: %v", err)
	}
	workflowRepo, err := workflowrepo.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("workflow repository: %v", err)
	}
	// Office agent CRUD reads/writes the merged agent_profiles table (ADR
	// 0005 Wave C), which only the settings store schema creates —
	// officesqlite.NewWithDB does not create it. See base_test.go's
	// initSharedAgentProfilesSchema in internal/office/service.
	if _, _, err := settingsstore.Provide(sqlxDB, sqlxDB, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	officeRepo, err := officesqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)

	workflowSvc := workflowservice.NewService(workflowRepo, log)
	t.Cleanup(func() { _ = workflowSvc.Close() })
	taskSvc := taskservice.NewService(
		taskservice.Repos{
			Workspaces:       taskRepo,
			Tasks:            taskRepo,
			TaskRepos:        taskRepo,
			Workflows:        taskRepo,
			Messages:         taskRepo,
			Turns:            taskRepo,
			Sessions:         taskRepo,
			GitSnapshots:     taskRepo,
			RepoEntities:     taskRepo,
			RepositorySets:   taskRepo,
			Executors:        taskRepo,
			Environments:     taskRepo,
			TaskEnvironments: taskRepo,
			Reviews:          taskRepo,
			StatusSummaries:  taskRepo,
		},
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{},
	)
	taskSvc.SetWorkflowStepCreator(workflowSvc)
	taskSvc.SetWorkspaceBootstrapper(taskRepo)
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: workflowSvc})
	taskSvc.SetWorkspacePolicyAttacher(testWorkspacePolicyAttacher{})
	workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})

	agentMgr := newStubAgentManager()
	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}
	orchestratorSvc := orchestrator.NewService(
		orchestrator.DefaultServiceConfig(), eventBus, agentMgr,
		taskRepoAdapter, taskRepo, nil, nil, nil, log,
	)
	orchestratorSvc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: workflowSvc})
	orchestratorSvc.SetTurnService(newTurnServiceAdapter(taskSvc))
	orchestratorSvc.SetTaskEventPublisher(taskSvc)

	ctx := context.Background()
	if err := orchestratorSvc.Start(ctx); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() { _ = orchestratorSvc.Stop() })

	routineSvc := officeroutines.NewRoutineService(officeRepo, log, &routineCronNoopActivity{})
	routineSvc.SetWorkflowEnsurer(taskRepo)
	routineSvc.SetTaskCreator(&taskCreatorAdapter{taskSvc: taskSvc})

	workspaces, err := taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("ListWorkspaces: workspaces=%d err=%v", len(workspaces), err)
	}

	return &routineCronHarness{
		taskSvc:      taskSvc,
		taskRepo:     taskRepo,
		workflowSvc:  workflowSvc,
		officeRepo:   officeRepo,
		routineSvc:   routineSvc,
		orchestrator: orchestratorSvc,
		agentMgr:     agentMgr,
		workspaceID:  workspaces[0].ID,
		db:           sqlxDB,
		eventBus:     eventBus,
	}
}

// countRows returns COUNT(*) from table, used to assert the negative half of
// AC-OFFICE-TASKLESS-001.1: a taskless routine fire must create no tasks row
// and no task_sessions row.
func (h *routineCronHarness) countRows(t *testing.T, table string) int {
	t.Helper()
	var count int
	if err := h.db.Get(&count, "SELECT COUNT(*) FROM "+table); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// awaitTaskInProgress polls until the task reaches IN_PROGRESS, the last DB
// write in executor.runAgentProcessAsync's post-LaunchAgent goroutine (see
// its callers for why that goroutine outlives the LaunchAgent call this test
// otherwise synchronizes on).
func (h *routineCronHarness) awaitTaskInProgress(ctx context.Context, t *testing.T, taskID string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		task, err := h.taskRepo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task != nil && task.State == v1.TaskStateInProgress {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for task %q to reach IN_PROGRESS after agent process start", taskID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type routineCronNoopActivity struct{}

func (routineCronNoopActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (routineCronNoopActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// TestRoutine_CronFire_HeavyRoutineReachesSession is the deliverable this
// card exists for: a cron trigger firing a heavy routine (non-empty
// task_template) must reach a real task_sessions row, not stop at the runs
// row the way TestRoutine_CronFire_CreatesTasklessRun_StopsBeforeSchedulerIntegration
// does for the lightweight path.
//
// It also pins the current, wrong-ish launch shape on purpose (see AC-3
// below): a materialized heavy-routine task is NOT an Office task
// (IsFromOffice == false, since it carries no project_id and lives in the
// Routine workflow rather than the workspace's office workflow), so
// autoStartTaskForStep takes the plain kanban StartTask branch. That branch
// builds no Office runtime env — no KANDEV_RUN_ID, no KANDEV_AGENT_ID, no
// KANDEV_WAKE_PAYLOAD_JSON. A future change that "fixes" this silently would
// flip this assertion, which is the point: it forces that change to touch
// this test.
func TestRoutine_CronFire_HeavyRoutineReachesSession(t *testing.T) {
	ctx := context.Background()
	h := newRoutineCronHarness(t)

	routine := &officeroutines.Routine{
		ID:                     "routine-heavy-1",
		WorkspaceID:            h.workspaceID,
		Name:                   "Heavy routine",
		TaskTemplate:           `{"title":"Routine sweep","description":"Do the sweep"}`,
		AssigneeAgentProfileID: "routine-assignee",
		Status:                 "active",
		Variables:              "{}",
	}
	if err := h.routineSvc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	trigger := &officeroutines.RoutineTrigger{
		ID:             "trigger-heavy-1",
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "*/5 * * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := h.routineSvc.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := h.officeRepo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil || len(triggers) == 0 || triggers[0].NextRunAt == nil {
		t.Fatalf("list triggers: triggers=%d err=%v", len(triggers), err)
	}
	fireTime := *triggers[0].NextRunAt

	if err := h.routineSvc.TickScheduledTriggers(ctx, fireTime.Add(time.Second)); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}

	req := h.agentMgr.awaitLaunch(t)

	// LaunchAgent's caller (executor.runAgentProcessAsync) keeps running in a
	// detached goroutine after the buffered channel send above unblocks this
	// test: it still calls StartAgentProcess and then writes the session to
	// RUNNING and the task to IN_PROGRESS. Wait for that goroutine's terminal
	// side effect before any assertion or t.Cleanup can race its DB writes
	// against sqlxDB.Close().
	h.awaitTaskInProgress(ctx, t, req.TaskID)

	// AC-1.2: the deliverable — a session row, not just a run row.
	sessions, err := h.taskRepo.ListTaskSessions(ctx, req.TaskID)
	if err != nil {
		t.Fatalf("list task sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("task_sessions rows for %q = %d, want 1", req.TaskID, len(sessions))
	}

	// The cron trigger source must be the one under test, not a manual fire.
	runs, err := h.officeRepo.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list routine runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want 1", len(runs))
	}
	if runs[0].Source != shared.RoutineSourceCron {
		t.Errorf("run.source = %q, want %q", runs[0].Source, shared.RoutineSourceCron)
	}
	if runs[0].LinkedTaskID != req.TaskID {
		t.Errorf("run.linked_task_id = %q, want %q", runs[0].LinkedTaskID, req.TaskID)
	}

	// AC-3: kanban shape, explicitly — no Office runtime env, no Office
	// identity on the launched task.
	for _, key := range []string{"KANDEV_RUN_ID", "KANDEV_AGENT_ID", "KANDEV_WAKE_PAYLOAD_JSON"} {
		if _, ok := req.Env[key]; ok {
			t.Errorf("req.Env[%q] present, want absent (kanban launch carries no Office runtime env)", key)
		}
	}
	task, err := h.taskRepo.GetTask(ctx, req.TaskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.IsFromOffice {
		t.Errorf("task.IsFromOffice = true, want false (materialized heavy routine run is a kanban task)")
	}

	// AC-4: the routine's assignee is the agent that actually launches,
	// proving the MetaKeyAgentProfileID fallback (the Routine workflow's
	// start step pins no agent).
	if req.AgentProfileID != "routine-assignee" {
		t.Errorf("req.AgentProfileID = %q, want routine-assignee", req.AgentProfileID)
	}
	if !req.StartAgent {
		t.Errorf("req.StartAgent = false, want true (cron-fired heavy routine must start the agent, not prepare-only)")
	}
}

// lightweightRunLauncher is a test double for officeservice.RunSessionLauncher,
// mirroring service_test.tasklessTestLauncher (internal/office/service/
// taskless_lifecycle_test.go): it reserves and starts a real
// office_run_sessions row through the same repo the scheduler under test
// reads, rather than merely recording the call.
type lightweightRunLauncher struct {
	svc   *officeservice.Service
	calls []officeservice.LaunchContext
}

func (l *lightweightRunLauncher) StartRunSession(
	ctx context.Context, run *officemodels.Run, agent *officemodels.AgentInstance,
	launch officeservice.LaunchContext, _ *officeservice.RouteOverride,
) (officeservice.RunSessionLaunch, error) {
	l.calls = append(l.calls, launch)
	attempt := len(l.calls)
	id := fmt.Sprintf("lightweight-run-session-%s-%d", run.ID, attempt)
	now := time.Now().UTC()
	session := &officemodels.RunSession{
		ID: id, WorkspaceID: agent.WorkspaceID, AgentProfileID: agent.ID,
		RunID: run.ID, Attempt: attempt, State: officemodels.RunSessionStatePreparing,
		CreatedAt: now, Version: 1,
	}
	reserved, err := l.svc.RepoForTest().ReserveRunSession(ctx, session)
	if err != nil || !reserved {
		return officeservice.RunSessionLaunch{}, fmt.Errorf(
			"reserve test run session: reserved=%v err=%w", reserved, err)
	}
	acpSessionID := "acp-" + id
	if _, err := l.svc.RepoForTest().BindRunSessionExecution(
		ctx, id, "execution-"+id, launch.ProfileID, "test-adapter", "test-model", acpSessionID,
	); err != nil {
		return officeservice.RunSessionLaunch{}, err
	}
	if _, err := l.svc.RepoForTest().MarkRunSessionStarted(
		ctx, id, "execution-"+id, launch.ProfileID, "test-adapter", "test-model", acpSessionID,
	); err != nil {
		return officeservice.RunSessionLaunch{}, err
	}
	return officeservice.RunSessionLaunch{
		SessionID: id, ExecutionID: "execution-" + id,
		ExecutionProfileID: launch.ProfileID, Adapter: "test-adapter", Model: "test-model",
		ACPSessionID: acpSessionID,
	}, nil
}

// awaitLightweightLaunchCount polls RunSchedulerTick until the launcher has
// been called at least `want` times, bounded by a deadline. TickScheduledTriggers
// and RunSchedulerTick are both synchronous in this harness (no detached
// goroutine, unlike the heavy-routine launch path above), so a single tick
// is expected to suffice; the poll loop exists only to absorb any queue
// interaction it would otherwise be flaky to assume away.
func awaitLightweightLaunchCount(
	t *testing.T, ctx context.Context, officeSvc *officeservice.Service, launcher *lightweightRunLauncher, want int,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		officeservice.RunSchedulerTick(officeSvc, ctx)
		if len(launcher.calls) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d lightweight launch(es), got %d", want, len(launcher.calls))
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRoutine_CronFire_LightweightReachesSession proves
// AC-OFFICE-TASKLESS-001.1 and the first clause of .2 end-to-end: a cron
// trigger firing a lightweight routine (empty task_template) must reach a
// real office_run_sessions row with runs.session_id bound to it — never a
// task_sessions row, which the design forbids for a taskless run — and two
// separate fires must produce two distinct sessions rather than reusing one.
//
// This exercises the exact production wiring
// (routines.RoutineService -> routineWakeupAdapter -> wakeup.Dispatcher ->
// service.Service's scheduler), substituting only the run-session launcher
// (RunSessionLauncher is the seam that boundary exists for) and the budget
// checker (a real costs.CostService, because routine_dispatch_cron
// classifies as unattended provenance and would otherwise be cancelled at
// the no-budget-evaluator gate).
func TestRoutine_CronFire_LightweightReachesSession(t *testing.T) {
	ctx := context.Background()
	h := newRoutineCronHarness(t)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eventBus := bus.NewMemoryEventBus(log)
	officeSvc := officeservice.NewService(officeservice.ServiceOptions{
		Repo: h.officeRepo, Logger: log, EventBus: eventBus,
	})
	// Synchronous handlers + a real event bus let this test simulate the
	// first attempt's completion (AgentCompleted) between the two fires
	// below, the same way an agent process reporting done would free the
	// agent for its next run — ClaimNextEligibleRun refuses to claim a
	// second run for an agent that still has one in status=claimed.
	officeSvc.SetSyncHandlers(true)
	if err := officeSvc.RegisterEventSubscribers(eventBus); err != nil {
		t.Fatalf("register event subscribers: %v", err)
	}
	activity := shared.NewActivityLogger(h.officeRepo, log)
	officeSvc.SetBudgetChecker(officecosts.NewCostService(h.officeRepo, log, activity, officeSvc, officeSvc))
	launcher := &lightweightRunLauncher{svc: officeSvc}
	officeSvc.SetRunSessionLauncher(launcher)

	wakeupDispatcher := officewakeup.NewDispatcher(h.officeRepo, h.officeRepo, log)
	wakeupDispatcher.SetRoutineLookup(h.officeRepo)
	h.routineSvc.SetWakeupEnqueuer(&routineWakeupAdapter{repo: h.officeRepo, dispatcher: wakeupDispatcher})

	// agent_profiles.agent_id is a NOT NULL FK to agents (CLI tool
	// registrations); newRoutineCronHarness opens its DB with FK
	// enforcement on (db.OpenSQLite), unlike office/service's base_test.go
	// (":memory:" with no _foreign_keys pragma). With no CLI tool rows
	// seeded, CreateAgentInstance's DefaultAgentID fallback resolves to ""
	// and the insert violates the FK, so seed one row here.
	if _, err := h.db.Exec(
		`INSERT INTO agents (id, name, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"cli-agent-lightweight", "CLI Agent",
	); err != nil {
		t.Fatalf("seed agents row: %v", err)
	}

	agent := &officemodels.AgentInstance{
		WorkspaceID: h.workspaceID, Name: "lightweight-assignee",
		Role: officemodels.AgentRoleCEO, Status: officemodels.AgentStatusIdle,
		ExecutorPreference: `{"type":"local_pc"}`,
	}
	if err := officeSvc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	routine := &officeroutines.Routine{
		ID:                     "routine-light-1",
		WorkspaceID:            h.workspaceID,
		Name:                   "Lightweight routine",
		TaskTemplate:           "",
		AssigneeAgentProfileID: agent.ID,
		Status:                 "active",
		Variables:              "{}",
		// always_create (normalised to wakeup.PolicyAlwaysEnqueue) so the
		// second fire below creates its own run instead of coalescing into
		// the first fire's still-active one — this test's whole point is
		// asserting two fires produce two distinct sessions.
		ConcurrencyPolicy: "always_create",
	}
	if err := h.routineSvc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	trigger := &officeroutines.RoutineTrigger{
		ID:             "trigger-light-1",
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "*/5 * * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := h.routineSvc.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	// First fire.
	triggers, err := h.officeRepo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil || len(triggers) == 0 || triggers[0].NextRunAt == nil {
		t.Fatalf("list triggers: triggers=%d err=%v", len(triggers), err)
	}
	fireTime := *triggers[0].NextRunAt
	if err := h.routineSvc.TickScheduledTriggers(ctx, fireTime.Add(time.Second)); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}
	awaitLightweightLaunchCount(t, ctx, officeSvc, launcher, 1)

	runs, err := officeSvc.ListRuns(ctx, h.workspaceID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("list runs after first fire: runs=%#v err=%v", runs, err)
	}
	firstRun := runs[0]
	if firstRun.SessionID == "" {
		t.Fatalf("run.session_id is empty after launch, want it bound to an office_run_sessions row")
	}
	firstSession, err := officeSvc.RepoForTest().GetRunSession(ctx, firstRun.SessionID)
	if err != nil || firstSession == nil {
		t.Fatalf("get run session %q: %v", firstRun.SessionID, err)
	}
	if firstSession.RunID != firstRun.ID {
		t.Errorf("session.run_id = %q, want %q", firstSession.RunID, firstRun.ID)
	}

	// Negative half of AC-OFFICE-TASKLESS-001.1: the lightweight fire must
	// create no tasks row and no task_sessions row — that is what
	// distinguishes it from the heavy-routine path above.
	if got := h.countRows(t, "tasks"); got != 0 {
		t.Errorf("tasks row count = %d, want 0 (lightweight fire must not create a task)", got)
	}
	if got := h.countRows(t, "task_sessions"); got != 0 {
		t.Errorf("task_sessions row count = %d, want 0 (taskless run must never create one)", got)
	}

	// Simulate the first attempt completing, the same way a real agent
	// process finishing would: this both frees the agent for the second
	// fire's claim (ClaimNextEligibleRun skips an agent with any run still
	// status=claimed) and mirrors what an actual taskless run's lifecycle
	// looks like end to end.
	if err := eventBus.Publish(ctx, events.AgentCompleted, bus.NewEvent(events.AgentCompleted, "test", lifecycle.AgentEventPayload{
		AgentExecutionID: firstSession.ExecutionID, AgentID: "test-adapter", AgentProfileID: agent.ID,
		RunID: firstRun.ID, RunSessionID: firstRun.SessionID, RunAttempt: 1, OwnerKind: lifecycle.ExecutionOwnerRun,
		WorkspaceID: h.workspaceID, Status: "COMPLETED",
	})); err != nil {
		t.Fatalf("publish first attempt completion: %v", err)
	}

	// Second fire: AC-OFFICE-TASKLESS-001.2's first clause — two fires
	// produce two distinct sessions, with no ACP session id reused.
	triggers, err = h.officeRepo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil || len(triggers) == 0 || triggers[0].NextRunAt == nil {
		t.Fatalf("list triggers before second fire: triggers=%d err=%v", len(triggers), err)
	}
	secondFireTime := *triggers[0].NextRunAt
	if !secondFireTime.After(fireTime) {
		t.Fatalf("second fire's next_run_at = %v, want after first fire's %v", secondFireTime, fireTime)
	}
	if err := h.routineSvc.TickScheduledTriggers(ctx, secondFireTime.Add(time.Second)); err != nil {
		t.Fatalf("tick scheduled triggers (second fire): %v", err)
	}
	awaitLightweightLaunchCount(t, ctx, officeSvc, launcher, 2)

	runs, err = officeSvc.ListRuns(ctx, h.workspaceID)
	if err != nil || len(runs) != 2 {
		t.Fatalf("list runs after second fire: runs=%#v err=%v", runs, err)
	}
	var secondRun *officemodels.Run
	for _, r := range runs {
		if r.ID != firstRun.ID {
			secondRun = r
			break
		}
	}
	if secondRun == nil {
		t.Fatalf("expected a second, distinct run; runs=%#v", runs)
	}
	if secondRun.SessionID == "" {
		t.Fatalf("second run.session_id is empty, want it bound to an office_run_sessions row")
	}
	if secondRun.SessionID == firstRun.SessionID {
		t.Fatalf("second fire reused session id %q from the first fire, want a distinct session", secondRun.SessionID)
	}
	secondSession, err := officeSvc.RepoForTest().GetRunSession(ctx, secondRun.SessionID)
	if err != nil || secondSession == nil {
		t.Fatalf("get run session %q: %v", secondRun.SessionID, err)
	}
	if secondSession.ACPSessionID == firstSession.ACPSessionID {
		t.Errorf("second fire reused ACP session id %q from the first fire, want distinct",
			secondSession.ACPSessionID)
	}

	if got := h.countRows(t, "tasks"); got != 0 {
		t.Errorf("tasks row count after second fire = %d, want 0", got)
	}
	if got := h.countRows(t, "task_sessions"); got != 0 {
		t.Errorf("task_sessions row count after second fire = %d, want 0", got)
	}
}

// stubAgentManager implements executor.AgentManagerClient with just enough
// behavior for LaunchAgent: capture the request and let the test proceed.
// Every other method is a mechanical zero-value stub — the launch path this
// test drives never reaches them.
type stubAgentManager struct {
	launched chan *executor.LaunchAgentRequest
}

func newStubAgentManager() *stubAgentManager {
	return &stubAgentManager{launched: make(chan *executor.LaunchAgentRequest, 4)}
}

func (m *stubAgentManager) awaitLaunch(t *testing.T) *executor.LaunchAgentRequest {
	t.Helper()
	select {
	case req := <-m.launched:
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for LaunchAgent")
		return nil
	}
}

func (m *stubAgentManager) LaunchAgent(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
	m.launched <- req
	return &executor.LaunchAgentResponse{AgentExecutionID: "exec-" + req.SessionID}, nil
}

func (m *stubAgentManager) StartAgentProcess(_ context.Context, _ string) error { return nil }
func (m *stubAgentManager) IsAgentCommandConfigured(_ string) bool              { return true }
func (m *stubAgentManager) StopAgent(_ context.Context, _ string, _ bool) error { return nil }
func (m *stubAgentManager) StopAgentWithReason(_ context.Context, _ string, _ string, _ bool) error {
	return nil
}
func (m *stubAgentManager) PromptAgent(
	_ context.Context, _ string, _ string, _ []v1.MessageAttachment, _ bool,
) (*executor.PromptResult, error) {
	return &executor.PromptResult{}, nil
}
func (m *stubAgentManager) CancelAgent(_ context.Context, _ string) error { return nil }
func (m *stubAgentManager) RespondToPermissionBySessionID(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
func (m *stubAgentManager) ListPendingPermissionsBySessionID(
	_ context.Context, _ string,
) ([]streams.PendingAgentPermission, error) {
	return nil, nil
}
func (m *stubAgentManager) ResolvePermissionBySessionID(
	_ context.Context, _, _, _, _ string,
) (*streams.PermissionResolveResponse, error) {
	return nil, nil
}
func (m *stubAgentManager) CancelPermissionBySessionID(
	_ context.Context, _, _, _ string,
) (*streams.PermissionCancelResponse, error) {
	return nil, nil
}
func (m *stubAgentManager) ProbeBackgroundWorkloads(
	_ context.Context, _ string,
) (client.ProbeResult, error) {
	return client.ProbeResultUnknown, nil
}
func (m *stubAgentManager) IsAgentRunningForSession(_ context.Context, _ string) bool { return false }
func (m *stubAgentManager) IsAgentReadyForPrompt(_ context.Context, _ string) bool    { return false }
func (m *stubAgentManager) ResolveAgentProfile(_ context.Context, profileID string) (*executor.AgentProfileInfo, error) {
	return &executor.AgentProfileInfo{ProfileID: profileID}, nil
}
func (m *stubAgentManager) SetExecutionDescription(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *stubAgentManager) SetExecutionEnv(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (m *stubAgentManager) SetMcpMode(_ context.Context, _ string, _ string) error { return nil }
func (m *stubAgentManager) RestartAgentProcess(_ context.Context, _ string) error  { return nil }
func (m *stubAgentManager) ResetAgentContext(_ context.Context, _ string) error    { return nil }
func (m *stubAgentManager) SetSessionModelBySessionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *stubAgentManager) SetSessionModeBySessionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *stubAgentManager) WasSessionInitialized(_ string) bool { return false }
func (m *stubAgentManager) GetSessionAuthMethods(_ string) []streams.AuthMethodInfo {
	return nil
}
func (m *stubAgentManager) IsPassthroughSession(_ context.Context, _ string) bool { return false }
func (m *stubAgentManager) WritePassthroughStdin(_ context.Context, _ string, _ string) error {
	return nil
}
func (m *stubAgentManager) ResolvePassthroughConfig(_ context.Context, _ string) (agents.PassthroughConfig, error) {
	return agents.PassthroughConfig{}, nil
}
func (m *stubAgentManager) MarkPassthroughRunning(_ string) error { return nil }
func (m *stubAgentManager) GetRemoteRuntimeStatusBySession(
	_ context.Context, _ string,
) (*executor.RemoteRuntimeStatus, error) {
	return nil, nil
}
func (m *stubAgentManager) PollRemoteStatusForRecords(_ context.Context, _ []executor.RemoteStatusPollRequest) {
}
func (m *stubAgentManager) CleanupStaleExecutionBySessionID(_ context.Context, _ string) error {
	return nil
}
func (m *stubAgentManager) EnsureWorkspaceExecutionForSession(_ context.Context, _, _ string) error {
	return nil
}
func (m *stubAgentManager) GetExecutionIDForSession(_ context.Context, _ string) (string, error) {
	return "", nil
}
func (m *stubAgentManager) ListExecutionsForTask(_ string) []lifecycle.ExecutionReference {
	return nil
}
func (m *stubAgentManager) GetGitLog(
	_ context.Context, _, _ string, _ int, _ string,
) (*client.GitLogResult, error) {
	return nil, nil
}
func (m *stubAgentManager) GetCumulativeDiff(
	_ context.Context, _, _ string,
) (*client.CumulativeDiffResult, error) {
	return nil, nil
}
func (m *stubAgentManager) GetGitStatus(_ context.Context, _ string) (*client.GitStatusResult, error) {
	return nil, nil
}
func (m *stubAgentManager) GetGitStatusFresh(_ context.Context, _ string) (*client.GitStatusResult, error) {
	return nil, nil
}
func (m *stubAgentManager) WaitForAgentctlReady(_ context.Context, _ string) error { return nil }

var _ executor.AgentManagerClient = (*stubAgentManager)(nil)
