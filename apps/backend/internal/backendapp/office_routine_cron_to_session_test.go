package backendapp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/agents"
	client "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	officeroutines "github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/shared"
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
	}
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

// TestRoutine_CronFire_LightweightReachesSession is deliberately skipped:
// the lightweight (taskless) routine path has no session to reach at all
// today (see card 49894d63 — gap 1, the taskless execution model). Writing
// it out in full, rather than leaving the gap unrepresented, means the skip
// carries the assertion that will need to pass once that card lands instead
// of silently having no coverage for either shape.
func TestRoutine_CronFire_LightweightReachesSession(t *testing.T) {
	t.Skip("blocked on card 49894d63: the lightweight (taskless) routine flow has no task_sessions row to assert on yet")

	ctx := context.Background()
	h := newRoutineCronHarness(t)

	routine := &officeroutines.Routine{
		ID:                     "routine-light-1",
		WorkspaceID:            h.workspaceID,
		Name:                   "Lightweight routine",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "routine-assignee",
		Status:                 "active",
		Variables:              "{}",
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
	triggers, err := h.officeRepo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil || len(triggers) == 0 || triggers[0].NextRunAt == nil {
		t.Fatalf("list triggers: triggers=%d err=%v", len(triggers), err)
	}
	fireTime := *triggers[0].NextRunAt

	if err := h.routineSvc.TickScheduledTriggers(ctx, fireTime.Add(time.Second)); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}

	// Once card 49894d63 lands, assert a task_sessions row exists for the
	// agent the lightweight wakeup dispatched to.
	t.Fatal("unreachable: update this assertion when the lightweight path reaches a session")
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
