package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// terminalizeAfterPromotionRepo pauses the prompt claim after the session has
// been promoted, so the test can reproduce a terminal transition in that gap.
type terminalizeAfterPromotionRepo struct {
	sessionExecutorStore
	targetSessionID string
	blockOnGetCall  int
	returnStale     bool
	claimReached    chan struct{}
	allowClaim      chan struct{}
	mu              sync.Mutex
	getCalls        int
	once            sync.Once
}

// postClaimReloadErrorRepo fails the guarded reload that follows a successful
// prompt claim. The turn service arms it only after that claim has persisted
// the RUNNING state and created the turn.
type postClaimReloadErrorRepo struct {
	sessionExecutorStore
	targetSessionID string
	err             error
	mu              sync.Mutex
	failNextReload  bool
}

func (r *postClaimReloadErrorRepo) armReloadFailure() {
	r.mu.Lock()
	r.failNextReload = true
	r.mu.Unlock()
}

func (r *postClaimReloadErrorRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.mu.Lock()
	fail := sessionID == r.targetSessionID && r.failNextReload
	if fail {
		r.failNextReload = false
	}
	r.mu.Unlock()
	if fail {
		return nil, r.err
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

type postClaimReloadErrorTurnService struct {
	*repoBackedTurnService
	repo *postClaimReloadErrorRepo
}

func (s *postClaimReloadErrorTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.repoBackedTurnService.StartTurn(ctx, sessionID)
	if err == nil {
		s.repo.armReloadFailure()
	}
	return turn, err
}

// terminalizeAfterPromptClaimRepo pauses the guarded reload after the prompt
// claim has persisted its RUNNING state and turn, so a competing completion
// can win before agentctl admission.
type terminalizeAfterPromptClaimRepo struct {
	sessionExecutorStore
	targetSessionID string
	claimReached    chan struct{}
	allowReload     chan struct{}
	mu              sync.Mutex
	armed           bool
	once            sync.Once
}

func (r *terminalizeAfterPromptClaimRepo) armTerminalization() {
	r.mu.Lock()
	r.armed = true
	r.mu.Unlock()
}

func (r *terminalizeAfterPromptClaimRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.mu.Lock()
	shouldBlock := sessionID == r.targetSessionID && r.armed
	if shouldBlock {
		r.armed = false
	}
	r.mu.Unlock()
	if shouldBlock {
		r.once.Do(func() { close(r.claimReached) })
		<-r.allowReload
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

type terminalizeAfterPromptClaimTurnService struct {
	*repoBackedTurnService
	repo *terminalizeAfterPromptClaimRepo
}

func (s *terminalizeAfterPromptClaimTurnService) StartTurn(ctx context.Context, sessionID string) (*models.Turn, error) {
	turn, err := s.repoBackedTurnService.StartTurn(ctx, sessionID)
	if err == nil {
		s.repo.armTerminalization()
	}
	return turn, err
}

// dispatchBoundaryAgentManager blocks after agentctl has accepted the prompt,
// giving the test a deterministic way to inspect the guard after admission but
// before the prompt call returns.
type dispatchBoundaryAgentManager struct {
	*mockAgentManager
	dispatched chan struct{}
	release    chan struct{}
}

func (m *dispatchBoundaryAgentManager) PromptAgentWithDispatchCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	onDispatched func(),
) (*executor.PromptResult, error) {
	result, err := m.PromptAgent(ctx, executionID, prompt, attachments, dispatchOnly)
	if err != nil {
		return result, err
	}
	onDispatched()
	close(m.dispatched)
	<-m.release
	return result, nil
}

func (m *dispatchBoundaryAgentManager) PromptAgentWithAdmissionCallback(
	ctx context.Context,
	executionID, prompt string,
	attachments []v1.MessageAttachment,
	dispatchOnly bool,
	beforeAdmission func(context.Context, string, []v1.MessageAttachment) error,
	onDispatched func(),
) (*executor.PromptResult, error) {
	if beforeAdmission != nil {
		if err := beforeAdmission(ctx, prompt, attachments); err != nil {
			return nil, err
		}
	}
	return m.PromptAgentWithDispatchCallback(ctx, executionID, prompt, attachments, dispatchOnly, onDispatched)
}

func (r *terminalizeAfterPromotionRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	r.mu.Lock()
	r.getCalls++
	shouldBlock := sessionID == r.targetSessionID && r.getCalls == r.blockOnGetCall
	r.mu.Unlock()
	if shouldBlock {
		if r.returnStale {
			session, err := r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
			r.once.Do(func() { close(r.claimReached) })
			<-r.allowClaim
			return session, err
		}
		r.once.Do(func() { close(r.claimReached) })
		<-r.allowClaim
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

func TestWorkflowAutoStartPromptClaimRejectsSessionTerminalizedAfterPromotion(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-terminalized-reuse", "session-terminalized-reuse", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-terminalized-reuse")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))

	svc := &Service{repo: repo, logger: testLogger()}
	promoted, err := svc.setNonterminalSessionPrimary(ctx, session.ID)
	requireNoError(t, err)
	if !promoted {
		t.Fatal("expected nonterminal session to be promoted")
	}

	barrierRepo := &terminalizeAfterPromotionRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		blockOnGetCall:       1,
		claimReached:         make(chan struct{}),
		allowClaim:           make(chan struct{}),
	}
	svc.repo = barrierRepo
	errCh := make(chan error, 1)
	go func() {
		_, _, _, _, _, _, claimErr := svc.claimSessionRunningForPrompt(
			ctx, session.TaskID, session.ID, "", false, nil, nil, "", true, nil,
		)
		errCh <- claimErr
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "finished concurrently"))
	close(barrierRepo.allowClaim)

	if err := <-errCh; !errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("claim error = %v, want terminalized workflow auto-start error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("terminalized session state = %s, want COMPLETED", stored.State)
	}
}

func TestWorkflowAutoStartRejectsSessionTerminalizedBeforeResume(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-terminalized", "session-auto-start-terminalized", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-terminalized")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))

	barrierRepo := &terminalizeAfterPromotionRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		blockOnGetCall:       1,
		returnStale:          true,
		claimReached:         make(chan struct{}),
		allowClaim:           make(chan struct{}),
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	if _, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	svc := &Service{
		repo:         barrierRepo,
		logger:       testLogger(),
		agentManager: agentMgr,
		executor:     executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{}),
		messageQueue: queue,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false, nil)
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "finished concurrently"))
	close(barrierRepo.allowClaim)

	if err := <-errCh; !errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("auto-start error = %v, want terminalized workflow auto-start error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("terminalized session state = %s, want COMPLETED", stored.State)
	}
	if status := queue.GetStatus(ctx, session.ID); status.Count != 1 {
		t.Fatalf("queued handoff count = %d, want 1 after terminal auto-start rejection", status.Count)
	}
}

func TestWorkflowAutoStartRollsBackPromptClaimWhenPostClaimReloadFails(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-reload-error", "session-auto-start-reload-error", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-reload-error")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-auto-start-reload-error")

	reloadErr := errors.New("post-claim session reload failed")
	failingRepo := &postClaimReloadErrorRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		err:                  reloadErr,
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	if _, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.repo = failingRepo
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = queue
	svc.turnService = &postClaimReloadErrorTurnService{
		repoBackedTurnService: &repoBackedTurnService{repo: repo},
		repo:                  failingRepo,
	}

	err = svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false, nil)
	if !errors.Is(err, reloadErr) {
		t.Fatalf("auto-start error = %v, want post-claim reload error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after reload failure = %s, want WAITING_FOR_INPUT", stored.State)
	}
	if active, err := repo.GetActiveTurnBySessionID(ctx, session.ID); err == nil || active != nil {
		t.Fatalf("active turn after reload failure = %#v, %v; want no active turn", active, err)
	}
	turns, err := repo.ListTurnsBySession(ctx, session.ID)
	requireNoError(t, err)
	if len(turns) != 1 || turns[0].CompletedAt == nil {
		t.Fatalf("turns after reload failure = %#v, want one completed rollback turn", turns)
	}
	if len(agentMgr.capturedPrompts) != 0 {
		t.Fatalf("prompt dispatches = %d, want none after reload failure", len(agentMgr.capturedPrompts))
	}
	queued, ok := queue.TakeQueued(ctx, session.ID)
	if !ok || queued.Content != "handoff" {
		t.Fatalf("requeued handoff = %#v, ok=%t; want original handoff", queued, ok)
	}
}

func TestWorkflowAutoStartRollsBackPromptClaimWhenTerminalizedAfterClaim(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-terminal-after-claim", "session-auto-start-terminal-after-claim", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-terminal-after-claim")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-auto-start-terminal-after-claim")

	barrierRepo := &terminalizeAfterPromptClaimRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		claimReached:         make(chan struct{}),
		allowReload:          make(chan struct{}),
	}
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true}
	queue := newAuthoritativeMemoryQueue(repo, testLogger())
	if _, err := queue.QueueMessage(ctx, session.ID, session.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.repo = barrierRepo
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageQueue = queue
	svc.turnService = &terminalizeAfterPromptClaimTurnService{
		repoBackedTurnService: &repoBackedTurnService{repo: repo},
		repo:                  barrierRepo,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false, nil)
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "finished concurrently"))
	close(barrierRepo.allowReload)

	if err := <-errCh; !errors.Is(err, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("auto-start error = %v, want terminalized workflow auto-start error", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if stored.State != models.TaskSessionStateCompleted {
		t.Fatalf("terminalized session state = %s, want COMPLETED", stored.State)
	}
	if active, err := repo.GetActiveTurnBySessionID(ctx, session.ID); err == nil || active != nil {
		t.Fatalf("active turn after terminalization = %#v, %v; want no active turn", active, err)
	}
	queued, ok := queue.TakeQueued(ctx, session.ID)
	if !ok || queued.Content != "handoff" {
		t.Fatalf("requeued handoff = %#v, ok=%t; want original handoff", queued, ok)
	}
}

func TestProcessOnEnterDoesNotReplaceCompletedSessionAfterPromptClaim(t *testing.T) {
	testProcessOnEnterTerminalizedAfterPromptClaim(t, models.TaskSessionStateCompleted)
}

func TestProcessOnEnterPreservesCancelledReusedSessionTerminalizedAfterPromptClaim(t *testing.T) {
	testProcessOnEnterTerminalizedAfterPromptClaim(t, models.TaskSessionStateCancelled)
}

func testProcessOnEnterTerminalizedAfterPromptClaim(t *testing.T, terminalState models.TaskSessionState) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-terminalized-replacement", "session-source", "step-source")

	source, err := repo.GetTaskSession(ctx, "session-source")
	requireNoError(t, err)
	source.AgentProfileID = "profile-source"
	source.ExecutorID = "exec-local"
	source.ExecutorProfileID = "ep1"
	source.TaskEnvironmentID = "environment-terminalized-replacement"
	source.IsPrimary = true
	requireNoError(t, repo.UpdateTaskSession(ctx, source))
	requireNoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: source.TaskEnvironmentID, TaskID: source.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))

	target := &models.TaskSession{
		ID:                "session-target",
		TaskID:            source.TaskID,
		AgentProfileID:    "profile-target",
		ExecutorID:        source.ExecutorID,
		ExecutorProfileID: source.ExecutorProfileID,
		TaskEnvironmentID: source.TaskEnvironmentID,
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         source.StartedAt,
		UpdatedAt:         source.UpdatedAt,
	}
	requireNoError(t, repo.CreateTaskSession(ctx, target))
	seedExecutorRunning(t, repo, target.ID, target.TaskID, "execution-target")

	barrierRepo := &terminalizeAfterPromptClaimRepo{
		sessionExecutorStore: repo,
		targetSessionID:      target.ID,
		claimReached:         make(chan struct{}),
		allowReload:          make(chan struct{}),
	}
	var launchPrompts []string
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchPrompts = append(launchPrompts, request.TaskDescription)
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-replacement"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[source.TaskID] = &v1.Task{
		ID: source.TaskID, WorkspaceID: "ws1", WorkflowID: "wf1", Title: "Test Task", Description: "Test",
		State: v1.TaskStateInProgress,
	}
	stepGetter := newMockStepGetter()
	sourceStep := &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", Name: "Source", AgentProfileID: source.AgentProfileID,
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyComplete,
	}
	stepGetter.steps[sourceStep.ID] = sourceStep
	step := &wfmodels.WorkflowStep{
		ID:             "step-target",
		WorkflowID:     "wf1",
		Name:           "Target",
		AgentProfileID: target.AgentProfileID,
		Events: wfmodels.StepEvents{OnEnter: []wfmodels.OnEnterAction{
			{Type: wfmodels.OnEnterAutoStartAgent},
		}},
	}
	stepGetter.steps[step.ID] = step
	dbTask, err := repo.GetTask(ctx, source.TaskID)
	requireNoError(t, err)
	dbTask.WorkflowStepID = step.ID
	requireNoError(t, repo.UpdateTask(ctx, dbTask))
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.repo = barrierRepo
	svc.turnService = &terminalizeAfterPromptClaimTurnService{
		repoBackedTurnService: &repoBackedTurnService{repo: repo},
		repo:                  barrierRepo,
	}
	if _, err := svc.messageQueue.QueueMessage(ctx, source.ID, source.TaskID, "handoff", "", messagequeue.QueuedByUser, true, nil); err != nil {
		t.Fatalf("queue handoff: %v", err)
	}

	done := make(chan struct{})
	go func() {
		svc.processOnEnter(ctx, source.TaskID, source, step, "Test", 0, sourceStep)
		close(done)
	}()

	<-barrierRepo.claimReached
	requireNoError(t, repo.UpdateTaskSessionState(ctx, target.ID, terminalState, "terminalized concurrently"))
	close(barrierRepo.allowReload)
	<-done

	sessions, err := repo.ListTaskSessions(ctx, source.TaskID)
	requireNoError(t, err)
	require.Len(t, sessions, 2, "one accepted entry cannot allocate after terminalization")
	stored, err := repo.GetTaskSession(ctx, target.ID)
	requireNoError(t, err)
	require.Equal(t, terminalState, stored.State)
	require.Empty(t, launchPrompts, "terminalized route must not launch a replacement")
}

func TestWorkflowAutoStartReleasesTerminalGuardAfterPromptAdmission(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-auto-start-dispatch", "session-auto-start-dispatch", "step-one")
	session, err := repo.GetTaskSession(ctx, "session-auto-start-dispatch")
	requireNoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	requireNoError(t, repo.UpdateTaskSession(ctx, session))
	seedExecutorRunning(t, repo, session.ID, session.TaskID, "execution-auto-start-dispatch")

	agentMgr := &dispatchBoundaryAgentManager{
		mockAgentManager: &mockAgentManager{repoForExecutionLookup: repo, isAgentRunning: true},
		dispatched:       make(chan struct{}),
		release:          make(chan struct{}),
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, &wfmodels.WorkflowStep{}, "review the task", false, false, nil)
	}()

	<-agentMgr.dispatched
	guard, release := svc.acquireCancelInFlightGuard(session.ID)
	acquired := make(chan struct{})
	go func() {
		guard.Lock()
		close(acquired)
		guard.Unlock()
		release()
	}()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("workflow auto-start held the terminal guard after agentctl accepted the prompt")
	}
	close(agentMgr.release)
	if err := <-errCh; err != nil {
		t.Fatalf("auto-start prompt: %v", err)
	}
}

// terminalizeCreatedSessionRepo pauses the CREATED-path auto-start just before
// the guard reload, so the test can terminalize the session in that gap and
// assert the guard detects it and returns errWorkflowAutoStartSessionTerminalized.
type terminalizeCreatedSessionRepo struct {
	sessionExecutorStore
	targetSessionID string
	lockAcquired    chan struct{} // closed when the guard lock is acquired
	allowReload     chan struct{} // close to let the reload continue
}

func (r *terminalizeCreatedSessionRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if sessionID == r.targetSessionID {
		select {
		case r.lockAcquired <- struct{}{}:
		default:
		}
		<-r.allowReload
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

// TestWorkflowAutoStartCreatedTerminalizedGuard verifies that the CREATED
// session branch in autoStartStepPrompt checks terminal state under the
// cancel-in-flight guard before calling StartCreatedSession, and returns
// errWorkflowAutoStartSessionTerminalized when the session was terminalized
// concurrently.
func TestWorkflowAutoStartCreatedTerminalizedGuard(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-created-guard", "session-created-guard", "step-created-guard")
	session, err := repo.GetTaskSession(ctx, "session-created-guard")
	requireNoError(t, err)
	session.AgentProfileID = "profile-target"
	session.ExecutorID = "exec-local"
	session.ExecutorProfileID = "ep1"
	session.State = models.TaskSessionStateCreated
	session.IsPrimary = true
	requireNoError(t, repo.UpdateTaskSession(ctx, session))

	barrierRepo := &terminalizeCreatedSessionRepo{
		sessionExecutorStore: repo,
		targetSessionID:      session.ID,
		lockAcquired:         make(chan struct{}, 1),
		allowReload:          make(chan struct{}),
	}

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-created-guard"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[session.TaskID] = &v1.Task{
		ID: session.TaskID, WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test Task", Description: "Test", State: v1.TaskStateInProgress,
	}
	step := &wfmodels.WorkflowStep{
		ID:   "step-created-guard",
		Name: "Target",
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.repo = barrierRepo

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.autoStartStepPrompt(ctx, session.TaskID, session, step, "review the task", false, false, nil)
	}()

	<-barrierRepo.lockAcquired
	requireNoError(t, repo.UpdateTaskSessionState(ctx, session.ID, models.TaskSessionStateCompleted, "concurrent completion"))
	close(barrierRepo.allowReload)

	autoStartErr := <-errCh
	if !errors.Is(autoStartErr, errWorkflowAutoStartSessionTerminalized) {
		t.Fatalf("autoStartStepPrompt error = %v, want errWorkflowAutoStartSessionTerminalized", autoStartErr)
	}

	// The CREATED session must remain terminal (the guard must not have
	// launched it).
	finalSession, err := repo.GetTaskSession(ctx, session.ID)
	requireNoError(t, err)
	if finalSession.State != models.TaskSessionStateCompleted {
		t.Errorf("final session state = %s, must remain COMPLETED (was not launched)", finalSession.State)
	}
}

// terminalizeImplicitSwitchRepo pauses the implicit profile-switch goroutine
// just before autoStartStepPrompt, so the test can terminalize the session
// and assert the goroutine does not allocate again for the same entry.
type terminalizeImplicitSwitchRepo struct {
	sessionExecutorStore
	taskMetadataCarryTaker
	targetSessionID string
	promptCalled    chan struct{} // closed when autoStartStepPrompt is about to run
	allowPrompt     chan struct{} // close to let autoStartStepPrompt proceed
}

func (r *terminalizeImplicitSwitchRepo) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	if sessionID == r.targetSessionID {
		select {
		case r.promptCalled <- struct{}{}:
		default:
		}
		<-r.allowPrompt
	}
	return r.sessionExecutorStore.GetTaskSession(ctx, sessionID)
}

func (r *terminalizeImplicitSwitchRepo) GetTask(ctx context.Context, taskID string) (*models.Task, error) {
	return r.sessionExecutorStore.(interface {
		GetTask(context.Context, string) (*models.Task, error)
	}).GetTask(ctx, taskID)
}

// TestProcessOnEnterImplicitProfileSwitchTerminalizedGuard verifies that the
// implicit profile-switch goroutine does not allocate another destination
// when its recorded session terminalizes before dispatch.
func TestProcessOnEnterImplicitProfileSwitchTerminalizedGuard(t *testing.T) {
	testProcessOnEnterImplicitProfileSwitchTerminalizedGuard(t, models.TaskSessionStateCompleted, false)
}

func TestProcessOnEnterImplicitProfileSwitchPreservesCancellation(t *testing.T) {
	testProcessOnEnterImplicitProfileSwitchTerminalizedGuard(t, models.TaskSessionStateCancelled, false)
}

func TestProcessOnEnterImplicitProfileSwitchSurvivesCallerCancellation(t *testing.T) {
	testProcessOnEnterImplicitProfileSwitchTerminalizedGuard(t, models.TaskSessionStateCompleted, true)
}

func testProcessOnEnterImplicitProfileSwitchTerminalizedGuard(
	t *testing.T,
	terminalState models.TaskSessionState,
	cancelCaller bool,
) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assertCtx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-implicit-switch", "session-implicit-switch-source", "step-source")
	const handoff = "Preserve this completion handoff."
	const visiblePrompt = "Review @principles."
	promptService := newPromptServiceForLaunchFallbackTest(t)
	_, err := promptService.CreatePrompt(ctx, "principles", "Apply the repository principles.")
	require.NoError(t, err)
	_, promptReferenceContext := promptService.AppendReferenceExpansionsWithContext(ctx, visiblePrompt, nil)
	require.NotEmpty(t, promptReferenceContext)

	source, err := repo.GetTaskSession(ctx, "session-implicit-switch-source")
	requireNoError(t, err)
	source.AgentProfileID = "profile-source"
	source.ExecutorID = "exec-local"
	source.ExecutorProfileID = "ep1"
	source.TaskEnvironmentID = "env-implicit-switch"
	source.IsPrimary = true
	requireNoError(t, repo.UpdateTaskSession(ctx, source))
	requireNoError(t, repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: source.TaskEnvironmentID, TaskID: source.TaskID,
		ExecutorType: string(models.ExecutorTypeLocal), Status: models.TaskEnvironmentStatusReady,
	}))

	target := &models.TaskSession{
		ID:                "session-implicit-switch-target",
		TaskID:            source.TaskID,
		AgentProfileID:    "profile-target",
		ExecutorID:        source.ExecutorID,
		ExecutorProfileID: source.ExecutorProfileID,
		TaskEnvironmentID: source.TaskEnvironmentID,
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         source.StartedAt,
		UpdatedAt:         source.UpdatedAt,
	}
	requireNoError(t, repo.CreateTaskSession(ctx, target))
	seedExecutorRunning(t, repo, target.ID, target.TaskID, "execution-implicit-switch")

	barrierRepo := &terminalizeImplicitSwitchRepo{
		sessionExecutorStore:   repo,
		taskMetadataCarryTaker: repo,
		targetSessionID:        target.ID,
		promptCalled:           make(chan struct{}, 1),
		allowPrompt:            make(chan struct{}),
	}

	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		isAgentRunning:         true,
		launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return &executor.LaunchAgentResponse{AgentExecutionID: "execution-implicit-replacement"}, nil
		},
	}
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[source.TaskID] = &v1.Task{
		ID: source.TaskID, WorkspaceID: "ws1", WorkflowID: "wf1",
		Title: "Test Task", Description: "Test", State: v1.TaskStateInProgress,
	}
	stepGetter := newMockStepGetter()
	// Register the existing step-source so the replacement session's task
	// (which inherits dbTask.WorkflowStepID) is resolvable.
	stepGetter.steps["step-source"] = &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", Name: "Source", AgentProfileID: source.AgentProfileID,
		ProfileSessionEndPolicy: models.WorkflowProfileSessionEndPolicyComplete,
	}
	step := &wfmodels.WorkflowStep{
		ID:             "step-implicit-switch",
		WorkflowID:     "wf1",
		Name:           "Target",
		AgentProfileID: target.AgentProfileID,
		Prompt:         visiblePrompt,
		// No auto_start_agent — implicit profile switch path.
	}
	dbTask, err := repo.GetTask(ctx, source.TaskID)
	requireNoError(t, err)
	dbTask.WorkflowStepID = step.ID
	requireNoError(t, repo.UpdateTask(ctx, dbTask))
	seedHandoffCarryToken(t, repo, source.TaskID, step.ID, handoff, "implicit-switch-handoff")
	stepGetter.steps[step.ID] = step
	svc := createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr)
	svc.repo = barrierRepo
	svc.promptExpander = promptService
	core, logs := observer.New(zapcore.InfoLevel)
	observed, err := logger.NewFromZap(zap.New(core))
	requireNoError(t, err)
	svc.logger = observed
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	type launchedPrompt struct {
		sessionID  string
		content    string
		startAgent bool
	}
	launches := make(chan launchedPrompt, 8)
	agentMgr.launchAgentFunc = func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launches <- launchedPrompt{
			sessionID: request.SessionID, content: request.TaskDescription, startAgent: request.StartAgent,
		}
		return &executor.LaunchAgentResponse{AgentExecutionID: "execution-implicit-replacement"}, nil
	}

	done := make(chan struct{})
	go func() {
		svc.processOnEnter(ctx, source.TaskID, source, step, visiblePrompt, 0, stepGetter.steps["step-source"])
		close(done)
	}()

	// Terminalize the reused target session before the goroutine's guarded
	// reload can detect it.
	<-barrierRepo.promptCalled
	if cancelCaller {
		cancel()
	}
	requireNoError(t, repo.UpdateTaskSessionState(context.Background(), target.ID, terminalState, "terminalized concurrently"))
	close(barrierRepo.allowPrompt)

	<-done
	completionMessage := "failed to create replacement after terminalized profile switch"
	if terminalState == models.TaskSessionStateCancelled {
		completionMessage = "implicit profile switch cancelled before dispatch; not creating replacement"
	}
	require.Eventually(t, func() bool {
		return logs.FilterMessage(completionMessage).Len() == 1
	}, 10*time.Second, 10*time.Millisecond, "implicit replacement attempt did not finish")
	sessions, err := repo.ListTaskSessions(assertCtx, source.TaskID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	finalTarget, err := repo.GetTaskSession(assertCtx, target.ID)
	require.NoError(t, err)
	require.Equal(t, terminalState, finalTarget.State)
	finalSource, err := repo.GetTaskSession(assertCtx, source.ID)
	require.NoError(t, err)
	require.False(t, finalSource.IsPrimary)
	require.Empty(t, launches)
	require.Empty(t, messages.userMessages)
}
