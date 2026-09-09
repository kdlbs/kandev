package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type failParkIntentRepo struct {
	repoStore
	remover workflowProfileSwitchStopIntentRemover
	marker  workflowProfileSwitchStopIntentMarker
}

func (r failParkIntentRepo) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	return errors.New("parked switch metadata write failed")
}

func (r failParkIntentRepo) RemoveSessionMetadataKeyIfStamp(ctx context.Context, sessionID, key, stamp string) (bool, error) {
	return r.remover.RemoveSessionMetadataKeyIfStamp(ctx, sessionID, key, stamp)
}

func (r failParkIntentRepo) SetSessionMetadataKeyIfStamp(ctx context.Context, sessionID, key, expectedStamp string, value interface{}) (bool, error) {
	return r.marker.SetSessionMetadataKeyIfStamp(ctx, sessionID, key, expectedStamp, value)
}

type failProfileSwitchPromotionRepo struct {
	repoStore
	err error
}

func (r failProfileSwitchPromotionRepo) SetSessionPrimary(context.Context, string) error {
	return r.err
}

type profileSwitchFixture struct {
	repo        *sqliterepo.Repository
	svc         *Service
	agentMgr    *mockAgentManager
	stepGetter  *mockStepGetter
	current     *models.TaskSession
	startPolicy models.WorkflowProfileSessionStartPolicy
	endPolicy   models.WorkflowProfileSessionEndPolicy
}

func newProfileSwitchFixture(t *testing.T, startPolicy models.WorkflowProfileSessionStartPolicy, endPolicy models.WorkflowProfileSessionEndPolicy) *profileSwitchFixture {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "Workflow", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-a",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "t1", ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	current := &models.TaskSession{
		ID: "session-a", TaskID: "t1", AgentProfileID: "profile-a", ExecutorID: "exec-local",
		ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, current); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	seedExecutorRunning(t, repo, current.ID, current.TaskID, "execution-a")
	running, err := repo.GetExecutorRunningBySessionID(ctx, current.ID)
	if err != nil {
		t.Fatalf("load current execution: %v", err)
	}
	running.ResumeToken = "acp-session-a"
	running.Resumable = true
	if err := repo.UpsertExecutorRunning(ctx, running); err != nil {
		t.Fatalf("persist current resume token: %v", err)
	}

	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-b"
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Test", Description: "Test", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		getExecutionIDForSessionFunc: func(_ context.Context, sessionID string) (string, error) {
			if sessionID == "session-a" {
				return "execution-a", nil
			}
			return "execution-b", nil
		},
		launchAgentFunc: func(_ context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-" + req.AgentProfileID}, nil
		},
	}
	return &profileSwitchFixture{
		repo: repo, stepGetter: stepGetter, current: current, agentMgr: agentMgr, startPolicy: startPolicy, endPolicy: endPolicy,
		svc: createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr),
	}
}

func newParkedProfileSwitchEventFixture(t *testing.T) (*sqliterepo.Repository, *Service, string) {
	t.Helper()
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "session-a", "step-destination")
	session, err := repo.GetTaskSession(ctx, "session-a")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	session.IsPrimary = false
	session.AgentProfileID = "profile-a"
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("park session: %v", err)
	}
	if err := repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyWorkflowProfileSwitchStopIntent, models.WorkflowProfileSwitchStopIntent{
		ExecutionID: "execution-a",
		Stamp:       "stop-stamp-a",
	}); err != nil {
		t.Fatalf("seed stop intent: %v", err)
	}
	turnID := "turn-profile-switch"
	now := time.Now().UTC()
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: turnID, TaskID: "t1", TaskSessionID: session.ID,
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create active turn: %v", err)
	}

	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.turnService = &repoTurnService{repo: repo}
	return repo, svc, turnID
}

func TestSwitchSessionForStep_ReuseOnStartParkOnEnd(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	now := time.Now().UTC()

	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf1", WorkspaceID: "ws1", Name: "Workflow", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", WorkflowStepID: "step-b",
		Title: "Test", Description: "Test", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "t1", ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}

	current := &models.TaskSession{
		ID: "session-a", TaskID: "t1", AgentProfileID: "profile-a",
		ExecutorID: "exec-local", ExecutorProfileID: "ep1", TaskEnvironmentID: "env-1",
		State: models.TaskSessionStateRunning, IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, current); err != nil {
		t.Fatalf("create current session: %v", err)
	}
	seedExecutorRunning(t, repo, current.ID, current.TaskID, "execution-a")

	stepGetter := newMockStepGetter()
	stepGetter.workflowAgentProfileID = "profile-b"
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{
		ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress,
		Title: "Test", Description: "Test",
	}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-b"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	step := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyReuse, ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark}

	selected, switched, err := svc.prepareWorkflowStepSession(ctx, "t1", current, step,
		&wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyPark},
	)
	if err != nil {
		t.Fatalf("prepareWorkflowStepSession: %v", err)
	}
	if !switched {
		t.Fatal("prepareWorkflowStepSession switched = false, want true")
	}
	if selected == nil || selected.ID == current.ID {
		t.Fatalf("selected session = %+v, want a new destination session", selected)
	}

	parked, err := repo.GetTaskSession(ctx, current.ID)
	if err != nil {
		t.Fatalf("reload parked session: %v", err)
	}
	if parked.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("parked session state = %s, want WAITING_FOR_INPUT", parked.State)
	}
	if parked.CompletedAt != nil {
		t.Fatal("parked session must not have CompletedAt")
	}
	if parked.IsPrimary {
		t.Fatal("parked session must not remain primary")
	}
	intent, ok := parked.Metadata["workflow_profile_switch_stop_intent"].(map[string]interface{})
	if !ok {
		t.Fatalf("parked session stop intent = %#v, want stamped metadata", parked.Metadata["workflow_profile_switch_stop_intent"])
	}
	if intent["execution_id"] != "execution-a" || intent["stamp"] == "" {
		t.Fatalf("parked stop intent = %#v, want execution-a and a non-empty stamp", intent)
	}

	agentMgr.mu.Lock()
	defer agentMgr.mu.Unlock()
	if len(agentMgr.stopAgentArgs) != 1 || agentMgr.stopAgentArgs[0].ExecutionID != "execution-a" {
		t.Fatalf("StopAgent calls = %+v, want execution-a", agentMgr.stopAgentArgs)
	}
}

func TestHandleAgentCompleted_ProfileSessionStopIntentSuppressesParkedSwitch(t *testing.T) {
	ctx := context.Background()
	repo, svc, turnID := newParkedProfileSwitchEventFixture(t)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "session-a", AgentExecutionID: "execution-a",
	})

	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("reload turn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("parked-switch completion must not complete the source turn")
	}
	session, err := repo.GetTaskSession(ctx, "session-a")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT", session.State)
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || !intent.Consumed {
		t.Fatalf("matching completion stop intent = %#v, want durable consumed tombstone", session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
}

func TestHandleAgentStopped_ProfileSessionStopIntentSuppressesParkedSwitch(t *testing.T) {
	ctx := context.Background()
	repo, svc, turnID := newParkedProfileSwitchEventFixture(t)

	svc.handleAgentStopped(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "session-a", AgentExecutionID: "execution-a",
	})

	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("reload turn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("parked-switch stopped event must not complete the source turn")
	}
	session, err := repo.GetTaskSession(ctx, "session-a")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state = %s, want WAITING_FOR_INPUT", session.State)
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || !intent.Consumed {
		t.Fatalf("matching stopped stop intent = %#v, want durable consumed tombstone", session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
}

func TestHandleAgentCompleted_ProfileSessionStopIntentSuppressesParkedSwitchAfterRestart(t *testing.T) {
	ctx := context.Background()
	repo, svc, turnID := newParkedProfileSwitchEventFixture(t)
	event := watcher.AgentEventData{
		TaskID: "t1", SessionID: "session-a", AgentExecutionID: "execution-a",
	}

	svc.handleAgentCompleted(ctx, event)

	// A new Service has no in-memory duplicate marker. The persisted consumed
	// tombstone must still suppress a delayed callback after a restart.
	taskRepo := newMockTaskRepo()
	taskRepo.tasks["t1"] = &v1.Task{ID: "t1", WorkspaceID: "ws1", WorkflowID: "wf1", State: v1.TaskStateInProgress}
	svcAfterRestart := createTestServiceWithScheduler(
		repo,
		newMockStepGetter(),
		taskRepo,
		&mockAgentManager{repoForExecutionLookup: repo},
	)
	svcAfterRestart.turnService = &repoTurnService{repo: repo}
	svcAfterRestart.handleAgentCompleted(ctx, event)

	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("reload turn: %v", err)
	}
	if turn.CompletedAt != nil {
		t.Fatal("delayed completion after restart must not complete the source turn")
	}
	session, err := repo.GetTaskSession(ctx, event.SessionID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || !intent.Consumed {
		t.Fatalf("restart stop intent = %#v, want durable consumed tombstone", session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
}

func TestHandleAgentCompleted_ProfileSessionStopIntentAllowsDifferentExecution(t *testing.T) {
	ctx := context.Background()
	repo, svc, turnID := newParkedProfileSwitchEventFixture(t)

	svc.handleAgentCompleted(ctx, watcher.AgentEventData{
		TaskID: "t1", SessionID: "session-a", AgentExecutionID: "execution-new",
	})

	turn, err := repo.GetTurn(ctx, turnID)
	if err != nil {
		t.Fatalf("reload turn: %v", err)
	}
	if turn.CompletedAt == nil {
		t.Fatal("completion from a different execution must remain eligible for normal turn completion")
	}
	session, err := repo.GetTaskSession(ctx, "session-a")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if _, ok := session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; !ok {
		t.Fatal("different execution must not consume the parked-switch stop intent")
	}
}

func TestParkSessionForProfileSwitch_RejectsNaturalCompletionBeforeClaim(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	event := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        fixture.current.ID,
		AgentExecutionID: "execution-a",
	}

	// The natural terminal callback wins the shared session guard before the
	// profile-switch path starts its park claim. The callback records the exact
	// execution as terminal; the later claim must not turn that completed
	// conversation into a reusable parked session.
	fixture.svc.handleAgentCompleted(ctx, event)

	parked, err := fixture.svc.parkSessionForProfileSwitch(ctx, "t1", fixture.current)
	if err == nil {
		t.Fatal("park after natural completion error = nil, want terminal execution rejection")
	}
	if parked {
		t.Fatal("park after natural completion = true, want false")
	}

	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if _, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata); ok {
		t.Fatal("natural completion before park claim must not leave a stop intent")
	}
	fixture.agentMgr.mu.Lock()
	defer fixture.agentMgr.mu.Unlock()
	if len(fixture.agentMgr.stopAgentArgs) != 0 {
		t.Fatalf("StopAgent calls after rejected park = %+v, want none", fixture.agentMgr.stopAgentArgs)
	}
}

func TestParkSessionForProfileSwitch_DelayedTerminalEventsConsumeClaim(t *testing.T) {
	tests := []struct {
		name   string
		handle func(*Service, context.Context, watcher.AgentEventData)
	}{
		{name: "completed", handle: (*Service).handleAgentCompleted},
		{name: "stopped", handle: (*Service).handleAgentStopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
			event := watcher.AgentEventData{
				TaskID:           "t1",
				SessionID:        fixture.current.ID,
				AgentExecutionID: "execution-a",
			}

			// Hold the same guard used by the park claim. The terminal callback
			// can register its waiter, but it cannot inspect or mutate the
			// session until the park claim has committed.
			guard, release := fixture.svc.acquireCancelInFlightGuard(fixture.current.ID)
			guard.Lock()
			guardHeld := true
			t.Cleanup(func() {
				if guardHeld {
					guard.Unlock()
					release()
				}
			})

			eventDone := make(chan struct{})
			go func() {
				tt.handle(fixture.svc, ctx, event)
				close(eventDone)
			}()
			coordinatorStopWaitForGuardRefs(t, fixture.svc, fixture.current.ID, 2)

			parked, _, err := fixture.svc.parkSessionForProfileSwitchClaimLocked(
				withWorkflowProfileSwitchGuardHeld(ctx, fixture.current.ID, ""),
				"t1",
				fixture.current,
			)
			if err != nil {
				t.Fatalf("park before delayed %s event: %v", tt.name, err)
			}
			if !parked {
				t.Fatalf("park before delayed %s event = false, want true", tt.name)
			}
			if !fixture.svc.hasExecutionTeardownOwner(fixture.current.ID, event.AgentExecutionID) {
				t.Fatalf("park before delayed %s event did not claim exact execution teardown ownership", tt.name)
			}

			guard.Unlock()
			guardHeld = false
			release()
			coordinatorStopAwaitSignal(t, eventDone, "delayed "+tt.name+" event")
			require.Eventually(t, func() bool {
				session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
				if err != nil {
					return false
				}
				intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
				return ok && intent.Consumed
			}, 2*time.Second, 10*time.Millisecond, "delayed %s event did not consume the stop intent", tt.name)

			session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			if err != nil {
				t.Fatalf("reload parked session: %v", err)
			}
			if session.State != models.TaskSessionStateWaitingForInput {
				t.Fatalf("session state after delayed %s event = %s, want WAITING_FOR_INPUT", tt.name, session.State)
			}
			intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
			if !ok || !intent.Consumed {
				t.Fatalf("stop intent after delayed %s event = %#v, want consumed durable tombstone", tt.name, session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
			}
		})
	}
}

func TestParkSessionForProfileSwitch_StopsAfterReleasingGuard(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	event := watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        fixture.current.ID,
		AgentExecutionID: "execution-a",
	}
	fixture.agentMgr.stopAgentFunc = func(stopCtx context.Context, _ string, _ bool) error {
		// The lifecycle manager may synchronously publish the stopped event from
		// StopAgent. Calling the real handler here proves the park claim does not
		// hold its guard across runtime teardown.
		fixture.svc.handleAgentStopped(stopCtx, event)
		return nil
	}

	type parkResult struct {
		parked bool
		err    error
	}
	resultCh := make(chan parkResult, 1)
	go func() {
		parked, err := fixture.svc.parkSessionForProfileSwitch(ctx, "t1", fixture.current)
		resultCh <- parkResult{parked: parked, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("park with synchronous stopped callback: %v", result.err)
		}
		if !result.parked {
			t.Fatal("park with synchronous stopped callback = false, want true")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("park remained blocked while StopAgent delivered a synchronous stopped callback")
	}

	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload stopped session: %v", err)
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(session.Metadata)
	if !ok || !intent.Consumed {
		t.Fatalf("stop intent after synchronous stopped callback = %#v, want consumed durable tombstone", session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
}

func TestSwitchSessionForStep_ReuseOnStartParkOnEndRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	stepB := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy, ProfileSessionEndPolicy: fixture.endPolicy}

	profileB, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepB,
		&wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy},
	)
	if err != nil {
		t.Fatalf("switch to profile-b: %v", err)
	}
	if !switched || profileB == nil || profileB.ID == fixture.current.ID {
		t.Fatalf("profile-b selection = session=%+v switched=%t, want new session", profileB, switched)
	}
	originalA, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload parked profile-a session: %v", err)
	}
	if originalA.State != models.TaskSessionStateWaitingForInput || originalA.CompletedAt != nil {
		t.Fatalf("profile-a after first switch = state %s completed_at %v, want parked", originalA.State, originalA.CompletedAt)
	}
	runningA, err := fixture.repo.GetExecutorRunningBySessionID(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload profile-a runtime: %v", err)
	}
	if runningA.ResumeToken != "acp-session-a" {
		t.Fatalf("profile-a resume token = %q, want preserved provider identity", runningA.ResumeToken)
	}

	fixture.stepGetter.workflowAgentProfileID = "profile-a"
	stepA := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 2, ProfileSessionStartPolicy: fixture.startPolicy, ProfileSessionEndPolicy: fixture.endPolicy}
	profileA, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", profileB, stepA,
		&wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy},
	)
	if err != nil {
		t.Fatalf("switch back to profile-a: %v", err)
	}
	if !switched || profileA == nil || profileA.ID != fixture.current.ID {
		t.Fatalf("profile-a re-entry = session=%+v switched=%t, want original session %q", profileA, switched, fixture.current.ID)
	}

	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("list round-trip sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("round-trip session count = %d, want 2", len(sessions))
	}
	activeA, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload active profile-a session: %v", err)
	}
	if !activeA.IsPrimary || activeA.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("active profile-a session = primary %t state %s, want primary waiting", activeA.IsPrimary, activeA.State)
	}
	parkedB, err := fixture.repo.GetTaskSession(ctx, profileB.ID)
	if err != nil {
		t.Fatalf("reload parked profile-b session: %v", err)
	}
	if parkedB.IsPrimary || parkedB.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("parked profile-b session = primary %t state %s, want nonprimary waiting", parkedB.IsPrimary, parkedB.State)
	}
}

func TestSwitchSessionForStep_NewOnStartParkOnEndRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyNew, models.WorkflowProfileSessionEndPolicyPark)
	stepB := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy, ProfileSessionEndPolicy: fixture.endPolicy}

	profileB, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepB,
		&wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy},
	)
	if err != nil {
		t.Fatalf("switch to profile-b: %v", err)
	}
	if !switched || profileB == nil || profileB.ID == fixture.current.ID {
		t.Fatalf("profile-b selection = session=%+v switched=%t, want new session", profileB, switched)
	}

	fixture.stepGetter.workflowAgentProfileID = "profile-a"
	stepA := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 2, ProfileSessionStartPolicy: fixture.startPolicy, ProfileSessionEndPolicy: fixture.endPolicy}
	profileA, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", profileB, stepA,
		&wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy},
	)
	if err != nil {
		t.Fatalf("switch back to profile-a: %v", err)
	}
	if !switched || profileA == nil || profileA.ID == fixture.current.ID {
		t.Fatalf("profile-a re-entry = session=%+v switched=%t, want fresh session", profileA, switched)
	}

	sessions, err := fixture.repo.ListTaskSessions(ctx, "t1")
	if err != nil {
		t.Fatalf("list round-trip sessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("round-trip session count = %d, want 3", len(sessions))
	}
	freshA, err := fixture.repo.GetTaskSession(ctx, profileA.ID)
	if err != nil {
		t.Fatalf("reload fresh profile-a session: %v", err)
	}
	if !freshA.IsPrimary || freshA.AgentProfileID != "profile-a" {
		t.Fatalf("fresh profile-a session = primary %t profile %q, want primary profile-a", freshA.IsPrimary, freshA.AgentProfileID)
	}
	originalA, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload original profile-a session: %v", err)
	}
	if originalA.State != models.TaskSessionStateWaitingForInput || originalA.IsPrimary {
		t.Fatalf("original profile-a session = state %s primary %t, want parked nonprimary", originalA.State, originalA.IsPrimary)
	}
}

func TestSwitchSessionForStep_UsesDestinationStartAndSourceEndIndependently(t *testing.T) {
	tests := []struct {
		name            string
		startPolicy     models.WorkflowProfileSessionStartPolicy
		endPolicy       models.WorkflowProfileSessionEndPolicy
		wantSourceState models.TaskSessionState
		wantExistingID  bool
	}{
		{
			name:            "reuse and complete",
			startPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
			endPolicy:       models.WorkflowProfileSessionEndPolicyComplete,
			wantSourceState: models.TaskSessionStateCompleted,
			wantExistingID:  true,
		},
		{
			name:            "reuse and park",
			startPolicy:     models.WorkflowProfileSessionStartPolicyReuse,
			endPolicy:       models.WorkflowProfileSessionEndPolicyPark,
			wantSourceState: models.TaskSessionStateWaitingForInput,
			wantExistingID:  true,
		},
		{
			name:            "new and complete",
			startPolicy:     models.WorkflowProfileSessionStartPolicyNew,
			endPolicy:       models.WorkflowProfileSessionEndPolicyComplete,
			wantSourceState: models.TaskSessionStateCompleted,
			wantExistingID:  false,
		},
		{
			name:            "new and park",
			startPolicy:     models.WorkflowProfileSessionStartPolicyNew,
			endPolicy:       models.WorkflowProfileSessionEndPolicyPark,
			wantSourceState: models.TaskSessionStateWaitingForInput,
			wantExistingID:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			fixture := newProfileSwitchFixture(t, tt.startPolicy, tt.endPolicy)
			candidate := &models.TaskSession{
				ID: "session-b-existing", TaskID: "t1", AgentProfileID: "profile-b",
				ExecutorID: "exec-local", ExecutorProfileID: "ep1", State: models.TaskSessionStateWaitingForInput,
				StartedAt: time.Now().UTC().Add(-time.Minute), UpdatedAt: time.Now().UTC().Add(-time.Minute),
			}
			require.NoError(t, fixture.repo.CreateTaskSession(ctx, candidate))

			// Deliberately put the opposite end value on the destination. Only
			// the source step is allowed to decide how session-a is retired.
			targetEndPolicy := models.WorkflowProfileSessionEndPolicyComplete
			if tt.endPolicy == models.WorkflowProfileSessionEndPolicyComplete {
				targetEndPolicy = models.WorkflowProfileSessionEndPolicyPark
			}
			target := &wfmodels.WorkflowStep{
				ID: "step-b", WorkflowID: "wf1", Position: 1,
				AgentProfileID: "profile-b", ProfileSessionStartPolicy: tt.startPolicy,
				ProfileSessionEndPolicy: targetEndPolicy,
			}
			source := &wfmodels.WorkflowStep{
				ID: "step-a", WorkflowID: "wf1", Position: 0,
				AgentProfileID: "profile-a", ProfileSessionStartPolicy: models.WorkflowProfileSessionStartPolicyNew,
				ProfileSessionEndPolicy: tt.endPolicy,
			}

			selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, target, source)
			require.NoError(t, err)
			require.True(t, switched)
			require.NotNil(t, selected)
			if tt.wantExistingID {
				require.Equal(t, candidate.ID, selected.ID)
			} else {
				require.NotEqual(t, candidate.ID, selected.ID)
			}

			sourceSession, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
			require.NoError(t, err)
			require.Equal(t, tt.wantSourceState, sourceSession.State)
		})
	}
}

func TestSwitchSessionForStep_ParkOnEndPreservesSourceWhenIntentWriteFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, fixture.current.ID, "t1", "queued handoff", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	fixture.svc.repo = failParkIntentRepo{repoStore: fixture.repo, remover: fixture.repo, marker: fixture.repo}

	_, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy,
	}, &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy})
	if err == nil {
		t.Fatal("parked profile switch error = nil, want stop-intent persistence failure")
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after intent failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	if source.CompletedAt != nil {
		t.Fatal("source after intent failure must not be completed")
	}
	if _, ok := source.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("failed intent write must not leave stop metadata")
	}
	if fixture.svc.hasExecutionTeardownOwner(fixture.current.ID, "execution-a") {
		t.Fatal("failed intent write must release the exact execution teardown claim")
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if status.Count != 1 || status.Entries[0].Content != "queued handoff" {
		t.Fatalf("source queue after intent failure = %+v, want queued handoff restored", status.Entries)
	}
}

func TestSwitchSessionForStep_ParkOnEndRestoresQueueAfterReuseParkingFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	stepB := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy}

	profileB, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepB, &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy,
	})
	if err != nil || !switched || profileB == nil {
		t.Fatalf("initial switch to profile-b = session=%+v switched=%t err=%v", profileB, switched, err)
	}
	currentB, err := fixture.repo.GetTaskSession(ctx, profileB.ID)
	if err != nil {
		t.Fatalf("reload profile-b source: %v", err)
	}
	if _, err := fixture.svc.messageQueue.QueueMessage(
		ctx, currentB.ID, "t1", "queued reuse handoff", "", messagequeue.QueuedByUser, false, nil,
	); err != nil {
		t.Fatalf("queue reuse handoff: %v", err)
	}

	fixture.svc.repo = failParkIntentRepo{repoStore: fixture.repo, remover: fixture.repo, marker: fixture.repo}
	fixture.stepGetter.workflowAgentProfileID = "profile-a"
	_, _, err = fixture.svc.prepareWorkflowStepSession(ctx, "t1", currentB, &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Position: 2, ProfileSessionStartPolicy: fixture.startPolicy,
	}, &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy})
	if err == nil {
		t.Fatal("reuse profile switch error = nil, want stop-intent persistence failure")
	}

	restored, err := fixture.repo.GetTaskSession(ctx, currentB.ID)
	if err != nil {
		t.Fatalf("reload restored source: %v", err)
	}
	if !restored.IsPrimary || restored.State != models.TaskSessionStateCreated {
		t.Fatalf("restored source = primary %t state %s, want primary created", restored.IsPrimary, restored.State)
	}
	status := fixture.svc.messageQueue.GetStatus(ctx, currentB.ID)
	if status.Count != 1 || status.Entries[0].Content != "queued reuse handoff" {
		t.Fatalf("source queue after reuse parking failure = %+v, want queued reuse handoff restored", status.Entries)
	}
	originalAStatus := fixture.svc.messageQueue.GetStatus(ctx, fixture.current.ID)
	if originalAStatus.Count != 0 {
		t.Fatalf("reused destination queue after rollback = %+v, want empty", originalAStatus.Entries)
	}
}

func TestSwitchSessionForStep_ParkOnEndContinuesWhenRuntimeStopFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionStartPolicyReuse, models.WorkflowProfileSessionEndPolicyPark)
	fixture.agentMgr.stopAgentErr = errors.New("runtime teardown failed")

	selected, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1, ProfileSessionStartPolicy: fixture.startPolicy,
	}, &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", ProfileSessionEndPolicy: fixture.endPolicy})
	if err != nil {
		t.Fatalf("profile switch with runtime stop failure: %v", err)
	}
	if !switched || selected == nil || selected.ID == fixture.current.ID {
		t.Fatalf("selected session = %+v switched=%t, want destination session", selected, switched)
	}
	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload parked source: %v", err)
	}
	if source.State != models.TaskSessionStateWaitingForInput || source.IsPrimary {
		t.Fatalf("source after runtime stop failure = state %s primary %t, want parked nonprimary", source.State, source.IsPrimary)
	}
	intent, ok := workflowProfileSwitchStopIntentFromMetadata(source.Metadata)
	if !ok || intent.Consumed {
		t.Fatalf("source stop intent after runtime stop failure = %#v, want active intent", source.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent])
	}
}
