package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type failParkIntentRepo struct {
	repoStore
	remover workflowProfileSwitchStopIntentRemover
}

func (r failParkIntentRepo) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	return errors.New("parked switch metadata write failed")
}

func (r failParkIntentRepo) RemoveSessionMetadataKeyIfStamp(ctx context.Context, sessionID, key, stamp string) (bool, error) {
	return r.remover.RemoveSessionMetadataKeyIfStamp(ctx, sessionID, key, stamp)
}

type failProfileSwitchPromotionRepo struct {
	repoStore
	err error
}

func (r failProfileSwitchPromotionRepo) SetSessionPrimary(context.Context, string) error {
	return r.err
}

type profileSwitchFixture struct {
	repo       *sqliterepo.Repository
	svc        *Service
	stepGetter *mockStepGetter
	current    *models.TaskSession
}

func newProfileSwitchFixture(t *testing.T, policy models.WorkflowProfileSessionPolicy) *profileSwitchFixture {
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
	stepGetter.workflowProfileSessionPolicy = policy
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
		repo: repo, stepGetter: stepGetter, current: current,
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

func TestSwitchSessionForStep_ParkReuseProfileSessionPolicy(t *testing.T) {
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
	stepGetter.workflowProfileSessionPolicy = models.WorkflowProfileSessionPolicyParkReuse
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
	step := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1}

	selected, switched, err := svc.prepareWorkflowStepSession(ctx, "t1", current, step)
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

func TestHandleAgentCompleted_ProfileSessionPolicySuppressesParkedSwitch(t *testing.T) {
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
	if _, ok := session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("matching completion must consume the parked-switch stop intent")
	}
}

func TestHandleAgentStopped_ProfileSessionPolicySuppressesParkedSwitch(t *testing.T) {
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
	if _, ok := session.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("matching stopped event must consume the parked-switch stop intent")
	}
}

func TestHandleAgentCompleted_ProfileSessionPolicyAllowsDifferentExecution(t *testing.T) {
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

func TestSwitchSessionForStep_ParkReuseProfileSessionPolicyRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionPolicyParkReuse)
	stepB := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1}

	profileB, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepB)
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
	stepA := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 2}
	profileA, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", profileB, stepA)
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

func TestSwitchSessionForStep_ParkNewProfileSessionPolicyRoundTrip(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionPolicyParkNew)
	stepB := &wfmodels.WorkflowStep{ID: "step-b", WorkflowID: "wf1", Position: 1}

	profileB, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, stepB)
	if err != nil {
		t.Fatalf("switch to profile-b: %v", err)
	}
	if !switched || profileB == nil || profileB.ID == fixture.current.ID {
		t.Fatalf("profile-b selection = session=%+v switched=%t, want new session", profileB, switched)
	}

	fixture.stepGetter.workflowAgentProfileID = "profile-a"
	stepA := &wfmodels.WorkflowStep{ID: "step-a", WorkflowID: "wf1", Position: 2}
	profileA, switched, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", profileB, stepA)
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

func TestSwitchSessionForStep_ParkProfileSessionPolicyPreservesSourceWhenIntentWriteFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionPolicyParkReuse)
	fixture.svc.repo = failParkIntentRepo{repoStore: fixture.repo, remover: fixture.repo}

	_, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
	})
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
}

func TestSwitchSessionForStep_ParkProfileSessionPolicyPreservesSourceWhenPromotionFails(t *testing.T) {
	ctx := context.Background()
	fixture := newProfileSwitchFixture(t, models.WorkflowProfileSessionPolicyParkReuse)
	promotionErr := errors.New("destination promotion failed")
	fixture.svc.repo = failProfileSwitchPromotionRepo{repoStore: fixture.repo, err: promotionErr}

	_, _, err := fixture.svc.prepareWorkflowStepSession(ctx, "t1", fixture.current, &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Position: 1,
	})
	if !errors.Is(err, promotionErr) {
		t.Fatalf("parked profile switch error = %v, want promotion failure", err)
	}

	source, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	if err != nil {
		t.Fatalf("reload source session: %v", err)
	}
	if source.State != models.TaskSessionStateRunning || !source.IsPrimary {
		t.Fatalf("source after promotion failure = state %s primary %t, want running primary", source.State, source.IsPrimary)
	}
	if source.CompletedAt != nil {
		t.Fatal("source after promotion failure must not be completed")
	}
	if _, ok := source.Metadata[models.SessionMetaKeyWorkflowProfileSwitchStopIntent]; ok {
		t.Fatal("promotion failure must not leave stop metadata")
	}
}
