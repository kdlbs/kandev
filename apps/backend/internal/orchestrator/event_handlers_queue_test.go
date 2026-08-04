package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func queuedReferenceFixture() v1.EntityReference {
	return v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      "mention:v1:kandev:task:workspace-1:task-2",
		Provider: "kandev",
		Kind:     "task",
		ID:       "task-2",
		Key:      "TASK-2",
		Title:    "Referenced task",
		URL:      "/t/task-2",
		Scope:    "workspace-1",
	}
}

type lifecycleClaimSequenceRepository struct {
	repoStore
	claimErrors []error
	claimCalls  int
}

func (r *lifecycleClaimSequenceRepository) ClaimPromptableTaskSessionIfActive(
	ctx context.Context, sessionID string,
) (models.PromptableTaskSessionClaim, error) {
	if r.claimCalls < len(r.claimErrors) {
		err := r.claimErrors[r.claimCalls]
		r.claimCalls++
		if err != nil {
			return models.PromptableTaskSessionClaim{}, err
		}
	}
	return r.repoStore.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
}

type lifecycleClaimBarrierRepository struct {
	repoStore
	claimEntered chan struct{}
	allowClaim   chan struct{}
}

// lifecycleClaimHookRepository runs its hook only after the real SQLite
// lifecycle claim has committed. It makes the gap between that claim and the
// final prompt dispatch reproducible without timing sleeps.
type lifecycleClaimHookRepository struct {
	repoStore
	afterClaim func()
}

// lifecycleResolveHookRepository changes task-session selection at the exact
// final lifecycle-resolution boundary, after prompt preparation but before the
// active-task claim. It avoids timing-dependent races in queue tests.
type lifecycleResolveHookRepository struct {
	repoStore
	beforeResolve func()
}

type lifecycleResolveBarrierRepository struct {
	repoStore
	resolveEntered chan struct{}
	allowResolve   chan struct{}
	once           sync.Once
}

type lifecycleWorkflowBarrierRepository struct {
	repoStore
	sessionReadEntered chan struct{}
	allowSessionRead   chan struct{}
	once               sync.Once
}

type hookedMessageCreator struct {
	*mockMessageCreator
	beforeCreate func()
}

// archiveBeforeLifecycleRetryRepository commits an archive after a lifecycle
// entry has been dequeued and its claim failed, but before the generic retry
// insert runs. The channels make that schedule explicit without sleeps.
type archiveBeforeLifecycleRetryRepository struct {
	messagequeue.Repository
	archive          func(context.Context) error
	requeueStarted   chan struct{}
	archiveCommitted chan struct{}
	lifecycleCalls   int
}

func (r *archiveBeforeLifecycleRetryRepository) InsertOrReplaceLifecycleByCoalesceKey(
	ctx context.Context,
	msg *messagequeue.QueuedMessage,
	coalesceKey string,
	maxPerSession int,
	allowInsert bool,
) (*messagequeue.QueuedMessage, bool, error) {
	r.lifecycleCalls++
	if r.lifecycleCalls == 1 {
		return r.Repository.InsertOrReplaceLifecycleByCoalesceKey(ctx, msg, coalesceKey, maxPerSession, allowInsert)
	}
	r.requeueStarted <- struct{}{}
	if err := r.archive(ctx); err != nil {
		return nil, false, err
	}
	r.archiveCommitted <- struct{}{}
	return nil, false, messagequeue.ErrTaskInactive
}

func (r *lifecycleClaimBarrierRepository) ClaimPromptableTaskSessionIfActive(
	ctx context.Context, sessionID string,
) (models.PromptableTaskSessionClaim, error) {
	r.claimEntered <- struct{}{}
	<-r.allowClaim
	return r.repoStore.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
}

func (r *lifecycleClaimHookRepository) ClaimPromptableTaskSessionIfActive(
	ctx context.Context, sessionID string,
) (models.PromptableTaskSessionClaim, error) {
	claim, err := r.repoStore.ClaimPromptableTaskSessionIfActive(ctx, sessionID)
	if err == nil && claim.Status == models.PromptableTaskSessionClaimed && r.afterClaim != nil {
		r.afterClaim()
	}
	return claim, err
}

func (r *lifecycleResolveHookRepository) ListTaskSessions(ctx context.Context, taskID string) ([]*models.TaskSession, error) {
	if r.beforeResolve != nil {
		r.beforeResolve()
		r.beforeResolve = nil
	}
	return r.repoStore.ListTaskSessions(ctx, taskID)
}

func (r *lifecycleResolveBarrierRepository) ListTaskSessions(
	ctx context.Context, taskID string,
) ([]*models.TaskSession, error) {
	r.once.Do(func() {
		close(r.resolveEntered)
		<-r.allowResolve
	})
	return r.repoStore.ListTaskSessions(ctx, taskID)
}

func (r *lifecycleWorkflowBarrierRepository) GetTaskSession(
	ctx context.Context,
	sessionID string,
) (*models.TaskSession, error) {
	r.once.Do(func() {
		close(r.sessionReadEntered)
		<-r.allowSessionRead
	})
	return r.repoStore.GetTaskSession(ctx, sessionID)
}

func (m *hookedMessageCreator) CreateUserMessage(
	ctx context.Context,
	taskID, content, sessionID, turnID string,
	metadata map[string]interface{},
) error {
	if m.beforeCreate != nil {
		m.beforeCreate()
	}
	return m.mockMessageCreator.CreateUserMessage(ctx, taskID, content, sessionID, turnID, metadata)
}

func TestExecuteQueuedMessage_LifecycleRequeueAfterArchiveIsDiscardedBeforeUnarchiveDrain(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	seedExecutorRunning(t, baseRepo, "s1", "t1", "exec-1")
	session, err := baseRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := baseRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptDone:             make(chan struct{}),
		repoForExecutionLookup: baseRepo,
	}
	svc := createTestServiceWithAgent(baseRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, baseRepo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &lifecycleClaimSequenceRepository{
		repoStore:   baseRepo,
		claimErrors: []error{errors.New("transient lifecycle claim error")},
	}
	queueRepo := &archiveBeforeLifecycleRetryRepository{
		Repository:       messagequeue.NewMemoryRepository(),
		archive:          func(ctx context.Context) error { return baseRepo.ArchiveTask(ctx, "t1") },
		requeueStarted:   make(chan struct{}, 1),
		archiveCommitted: make(chan struct{}, 1),
	}
	svc.messageQueue = messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())

	// Start with a lifecycle-specific accepted entry, then take it as the
	// normal ready-drain path does before executeQueuedMessage retries it.
	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "stale lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle entry: accepted=%v err=%v", accepted, err)
	}
	queued, ok := svc.messageQueue.TakeQueued(ctx, "s1")
	if !ok {
		t.Fatal("lifecycle entry was not dequeued")
	}

	// The failed claim reaches the generic requeue. Its repository barrier
	// commits the archive before that insert is allowed to proceed.
	svc.executeQueuedMessage("s1", queued)
	<-queueRepo.requeueStarted
	<-queueRepo.archiveCommitted

	task, err := baseRepo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get archived task: %v", err)
	}
	if task.ArchivedAt == nil {
		t.Fatal("archive did not commit before the lifecycle retry insert")
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Errorf("archived lifecycle retry entries = %d, want 0", got)
	}

	if _, err := baseRepo.UnarchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	if dispatched := svc.drainQueuedMessageForPromptableSession(ctx, "s1"); dispatched {
		<-agentMgr.promptDone
		t.Error("unarchive readiness drained a lifecycle retry that archive should have discarded")
	}
	if got := len(agentMgr.capturedPrompts); got != 0 {
		t.Errorf("lifecycle prompts after archive/unarchive = %d, want 0", got)
	}
}

func TestExecuteQueuedMessage_LifecycleClaimErrorClearsInFlightTokenForRetryDrain(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	seedExecutorRunning(t, baseRepo, "s1", "t1", "exec-1")
	session, err := baseRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := baseRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: baseRepo}
	svc := createTestServiceWithAgent(baseRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, baseRepo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &lifecycleClaimSequenceRepository{
		repoStore:   baseRepo,
		claimErrors: []error{errors.New("transient claim database error")},
	}
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-claim-error", SessionID: "s1", TaskID: "t1", Content: "retry lifecycle prompt",
		Metadata: map[string]interface{}{"origin": githubPRAutomationOrigin},
	}

	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("claim-error lifecycle retry entries = %d, want 1", got)
	}
	if svc.isQueuedDispatchInFlight("s1") {
		t.Fatal("claim-error lifecycle dispatch left an in-flight token that blocks retry")
	}
	if !svc.drainQueuedMessageForPromptableSession(ctx, "s1") {
		t.Fatal("retry lifecycle queue did not drain after transient claim error")
	}
}

func TestExecuteQueuedMessage_LifecycleDispatchFailureRestoresStateAndRetriesWithoutDuplicateMessage(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	seedExecutorRunning(t, baseRepo, "s1", "t1", "exec-1")
	session, err := baseRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := baseRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptErr:              fmt.Errorf("agent stream disconnected while prompting"),
		repoForExecutionLookup: baseRepo,
	}
	svc := createTestServiceWithAgent(baseRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, baseRepo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-dispatch-failure", SessionID: "s1", TaskID: "t1", Content: "retry lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin":                         githubPRAutomationOrigin,
			messagequeue.MetadataCoalesceKey: "github-pr:repo:1:merged",
		},
	}

	svc.executeQueuedMessage("s1", queued)
	persisted, err := baseRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session after failed lifecycle dispatch: %v", err)
	}
	if persisted.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after failed lifecycle dispatch = %s, want WAITING_FOR_INPUT", persisted.State)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("failed lifecycle dispatch retry entries = %d, want 1", got)
	}

	retry, ok := svc.messageQueue.TakeQueued(ctx, "s1")
	if !ok {
		t.Fatal("failed lifecycle dispatch did not retain a retry entry")
	}
	agentMgr.promptErr = nil
	svc.executeQueuedMessage("s1", retry)
	if got := len(svc.messageCreator.(*mockMessageCreator).userMessages); got != 1 {
		t.Fatalf("lifecycle retry created %d visible messages, want 1", got)
	}
}

// A lifecycle executor rejection is not a normal completed turn: the runtime
// claim moved the task to IN_PROGRESS solely to make the pending delivery
// observable. It must be rolled back to the captured pre-claim state rather
// than sent to REVIEW like an ordinary user prompt failure.
func TestExecuteQueuedMessage_LifecycleExecutorFailureRestoresPreClaimTaskState(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateTODO)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptErr:              errors.New("executor rejected lifecycle prompt"),
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	svc.executeQueuedMessage("s1", &messagequeue.QueuedMessage{
		ID: "lifecycle-executor-failure", SessionID: "s1", TaskID: "t1", Content: "retry lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:merged",
		},
	})

	task, err := taskRepo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.State != v1.TaskStateTODO {
		t.Fatalf("task state after rejected lifecycle prompt = %s, want restored TODO", task.State)
	}
	persisted, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if persisted.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("session state after rejected lifecycle prompt = %s, want WAITING_FOR_INPUT", persisted.State)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("rejected lifecycle prompt retries = %d, want 1", got)
	}
}

// A lifecycle delivery must not complete a turn that belonged to an earlier
// dispatch. It adopts such a turn only to associate the visible message; on
// executor rejection, no lifecycle-owned turn exists to roll back.
func TestExecuteQueuedMessage_LifecycleExecutorFailurePreservesPreexistingTurn(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	turns := &repoTurnService{repo: repo}
	preexisting, err := turns.StartTurn(ctx, "s1")
	if err != nil {
		t.Fatalf("create preexisting turn: %v", err)
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		promptErr:              errors.New("executor rejected lifecycle prompt"),
		repoForExecutionLookup: repo,
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.turnService = turns
	svc.messageCreator = &mockMessageCreator{}

	svc.executeQueuedMessage("s1", &messagequeue.QueuedMessage{
		ID: "lifecycle-existing-turn", SessionID: "s1", TaskID: "t1", Content: "retry lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:merged",
		},
	})

	active, err := turns.GetActiveTurn(ctx, "s1")
	if err != nil {
		t.Fatalf("get active turn: %v", err)
	}
	if active == nil || active.ID != preexisting.ID {
		t.Fatalf("active turn after rejected lifecycle prompt = %+v, want preexisting %q still open", active, preexisting.ID)
	}
}

// A concurrent task transition owns its result. Lifecycle error recovery may
// restore only its own IN_PROGRESS claim, never overwrite the newer state.
func TestExecuteQueuedMessage_LifecycleExecutorFailureDoesNotClobberConcurrentTaskTransition(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set session waiting: %v", err)
	}

	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateTODO)
	agentMgr := &mockAgentManager{
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		promptAgentFunc: func(context.Context, string, string, []v1.MessageAttachment, bool) (*executor.PromptResult, error) {
			if err := taskRepo.UpdateTaskState(ctx, "t1", v1.TaskStateFailed); err != nil {
				t.Errorf("concurrent task transition: %v", err)
			}
			return nil, errors.New("executor rejected lifecycle prompt")
		},
	}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.messageCreator = &mockMessageCreator{}

	svc.executeQueuedMessage("s1", &messagequeue.QueuedMessage{
		ID: "lifecycle-concurrent-transition", SessionID: "s1", TaskID: "t1", Content: "retry lifecycle prompt",
		Metadata: map[string]interface{}{
			"origin": githubPRAutomationOrigin, messagequeue.MetadataCoalesceKey: "github-pr:repo:1:merged",
		},
	})

	task, err := taskRepo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.State != v1.TaskStateFailed {
		t.Fatalf("concurrent task transition was clobbered: got %s, want FAILED", task.State)
	}
}

func TestHandleAgentReady_PassthroughAcknowledgesLifecycleOnlyAfterPTYAcceptance(t *testing.T) {
	tests := []struct {
		name           string
		lifecycle      bool
		passthroughErr error
		wantQueued     int
	}{
		{
			name:           "lifecycle PTY failure retains durable entry",
			lifecycle:      true,
			passthroughErr: errors.New("PTY write failed"),
			wantQueued:     1,
		},
		{
			name:       "lifecycle PTY acceptance acknowledges durable entry",
			lifecycle:  true,
			wantQueued: 0,
		},
		{
			name:           "ordinary PTY failure keeps legacy destructive dequeue",
			passthroughErr: errors.New("PTY write failed"),
			wantQueued:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			agentMgr := &mockAgentManager{
				isPassthrough:          true,
				passthroughStdinErr:    tt.passthroughErr,
				isAgentRunning:         true,
				repoForExecutionLookup: repo,
			}
			steps := newMockStepGetter()
			steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}
			taskRepo := newMockTaskRepo()
			seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
			svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
			svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
			svc.messageCreator = &mockMessageCreator{}
			if tt.lifecycle {
				seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
			}

			if tt.lifecycle {
				_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
					ctx, "s1", "t1", "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
					false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
				)
				if err != nil || !accepted {
					t.Fatalf("queue lifecycle prompt: accepted=%v err=%v", accepted, err)
				}
			} else if _, err := svc.messageQueue.QueueMessage(ctx, "s1", "t1", "ordinary prompt", "", "user", false, nil); err != nil {
				t.Fatalf("queue ordinary prompt: %v", err)
			}

			svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})

			if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != tt.wantQueued {
				t.Fatalf("queued entries after PTY result = %d, want %d", got, tt.wantQueued)
			}
		})
	}
}

func TestHandleAgentReady_PassthroughLifecyclePersistsVisibleMessageBeforePTYAcceptance(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	ptyEntered := make(chan struct{})
	releasePTY := make(chan struct{})
	ptyAccepted := make(chan struct{})
	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
		passthroughStdinFunc: func(context.Context, string, string) error {
			close(ptyEntered)
			<-releasePTY
			close(ptyAccepted)
			return nil
		},
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	steps := newMockStepGetter()
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}
	svc := createTestServiceWithAgent(repo, steps, taskRepo, agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	messages := &mockMessageCreator{}
	svc.messageCreator = messages
	_, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle prompt: accepted=%v err=%v", accepted, err)
	}

	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(readyDone)
	}()
	select {
	case <-ptyEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lifecycle PTY dispatch")
	}

	if got := len(messages.userMessages); got != 1 {
		t.Fatalf("visible lifecycle messages before PTY acceptance = %d, want 1", got)
	}
	close(releasePTY)
	<-ptyAccepted
	<-readyDone
}

func TestArchiveTask_PersistentQueueCallbackDoesNotPurgeReacceptedGeneration(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")

	db := sqlx.NewDb(repo.DB(), "sqlite3")
	persistentRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("new persistent queue repository: %v", err)
	}
	persistentQueue := messagequeue.NewService(persistentRepo, messagequeue.DefaultMaxPerSession, testLogger())
	// An explicitly registered ephemeral mirror runs after the task mutation
	// commits. It may observe an unarchive and a fresh G+1 lifecycle entry;
	// NewService must leave that callback intact instead of replacing it with
	// a second purge of the shared SQLite queue.
	repo.SetTaskQueuePurger(func(hookCtx context.Context, taskID string) {
		if _, err := repo.UnarchiveTask(hookCtx, taskID); err != nil {
			t.Fatalf("unarchive during post-commit queue callback: %v", err)
		}
		_, _, accepted, err := persistentQueue.QueueLifecycleMessageWithCoalesceKey(
			hookCtx, "s1", taskID, "fresh lifecycle prompt", "", messagequeue.QueuedByWorkflow,
			false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
		)
		if err != nil || !accepted {
			t.Fatalf("accept fresh lifecycle generation: accepted=%v err=%v", accepted, err)
		}
	})

	// A supplied queue is production-owned and shares SQLite with the task
	// repository, so NewService must not replace the separately registered
	// ephemeral-mirror callback above.
	_ = NewService(ServiceConfig{}, nil, &mockAgentManager{}, newMockTaskRepo(), repo, nil, nil, persistentQueue, testLogger())
	_, _, accepted, err := persistentQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "old lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("accept original lifecycle generation: accepted=%v err=%v", accepted, err)
	}

	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if got := persistentQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("fresh lifecycle entries after archive callback = %d, want 1", got)
	}
	generation, err := persistentRepo.LifecycleGeneration(ctx, "t1")
	if err != nil {
		t.Fatalf("read lifecycle generation: %v", err)
	}
	if generation != 1 {
		t.Fatalf("lifecycle generation after archive = %d, want 1", generation)
	}
}

// Archive cancels accepted lifecycle work even when the currently selected
// session is busy and therefore cannot drain it immediately. Unarchiving must
// not resurrect that historical observation into a fresh agent prompt.
func TestDispatchTaskPRAgentPrompt_ArchivePurgesBusyAcceptedLifecycleEntry(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	pr := &github.TaskPR{TaskID: "t1", RepositoryID: "repo-1", Owner: "acme", Repo: "widget", PRNumber: 42}

	if _, err := svc.dispatchTaskPRAgentPrompt(ctx, pr, "merged lifecycle prompt", taskPRAgentEventMerged); err != nil {
		t.Fatalf("accept lifecycle prompt while busy: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("accepted lifecycle entries = %d, want 1", got)
	}
	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("archived task retained %d accepted lifecycle entries, want 0", got)
	}
	if _, err := repo.UnarchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("unarchive task: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("unarchive resurrected %d lifecycle entries, want 0", got)
	}
}

// Pending lifecycle rows are visible and cancellable by an authorized session
// owner. Task deletion independently purges any remaining lifecycle work so no
// orphan prompt survives after the task is gone.
func TestHandleTaskDeleted_PurgesLifecycleRowsAfterUserCancellation(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{})
	queued, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "merged lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:1:merged", true,
	)
	if err != nil || !accepted {
		t.Fatalf("queue lifecycle prompt: accepted=%v err=%v", accepted, err)
	}
	if err := svc.messageQueue.RemoveEntry(ctx, "s1", queued.ID); err != nil {
		t.Fatalf("remove visible lifecycle row: %v", err)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("user cancellation retained %d lifecycle queue rows, want 0", got)
	}
	if _, _, accepted, err := svc.messageQueue.QueueLifecycleMessageWithCoalesceKey(
		ctx, "s1", "t1", "remaining lifecycle prompt", "", messagequeue.QueuedByWorkflow,
		false, nil, map[string]interface{}{"origin": githubPRAutomationOrigin}, "github-pr:repo:2:merged", true,
	); err != nil || !accepted {
		t.Fatalf("queue lifecycle prompt for task cleanup: accepted=%v err=%v", accepted, err)
	}
	if err := repo.DeleteTask(ctx, "t1"); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	svc.handleTaskDeleted(ctx, watcher.TaskEventData{TaskID: "t1"})
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("deleted task retained %d lifecycle queue rows, want 0", got)
	}
}

func TestExecuteQueuedMessage_LifecycleBusyClaimRequeuesInsteadOfDropping(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedSession(t, baseRepo, "t1", "s1", "step1")
	seedExecutorRunning(t, baseRepo, "s1", "t1", "exec-1")
	session, err := baseRepo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := baseRepo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: baseRepo}
	svc := createTestServiceWithAgent(baseRepo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, baseRepo, testLogger(), executor.ExecutorConfig{})
	claimRepo := &lifecycleClaimBarrierRepository{
		repoStore: baseRepo, claimEntered: make(chan struct{}), allowClaim: make(chan struct{}),
	}
	svc.repo = claimRepo
	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-busy-claim", SessionID: "s1", TaskID: "t1", Content: "preserve lifecycle prompt",
		Metadata: map[string]interface{}{"origin": githubPRAutomationOrigin},
	}

	done := make(chan struct{})
	go func() {
		svc.executeQueuedMessage("s1", queued)
		close(done)
	}()
	<-claimRepo.claimEntered
	if err := baseRepo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateRunning, "direct prompt won"); err != nil {
		t.Fatalf("direct prompt claim: %v", err)
	}
	close(claimRepo.allowClaim)
	<-done

	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Fatalf("busy lifecycle claim retained %d queue entries, want 1", got)
	}
}

// The lifecycle claim currently writes RUNNING before ensureSessionRunning and
// executor dispatch. If reset starts in that interval, the claim must be
// rolled back to its prior promptable state so the retained entry can drain
// once reset completes.
func TestExecuteQueuedMessage_LifecycleResetAfterClaimRestoresStateAndRetryDrains(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &lifecycleClaimHookRepository{
		repoStore: repo,
		afterClaim: func() {
			svc.setSessionResetInProgress("s1", true)
		},
	}
	defer svc.setSessionResetInProgress("s1", false)

	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-reset-claim", SessionID: "s1", TaskID: "t1", Content: "retry after reset",
		Metadata: map[string]interface{}{"origin": githubPRAutomationOrigin},
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	persisted, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("reload claimed session: %v", err)
	}
	if persisted.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("state after reset won post-claim = %s, want WAITING_FOR_INPUT", persisted.State)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Errorf("retained lifecycle entries after reset = %d, want 1", got)
	}

	svc.setSessionResetInProgress("s1", false)
	if !svc.drainQueuedMessageForPromptableSession(ctx, "s1") {
		t.Error("reset retry was not drainable after the session became promptable")
	}
}

// A newer queued dispatch can supersede the entry after lifecycle's database
// claim commits but before executor.Prompt. The stale worker must requeue its
// entry and never prompt the agent; the succeeding dispatch token remains the
// only active owner.
func TestExecuteQueuedMessage_LifecycleSupersededClaimBeforePromptDoesNotDispatch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")
	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	session.State = models.TaskSessionStateWaitingForInput
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("set waiting session: %v", err)
	}

	agentMgr := &mockAgentManager{isAgentRunning: true, repoForExecutionLookup: repo}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)
	svc.executor = executor.NewExecutor(agentMgr, repo, testLogger(), executor.ExecutorConfig{})
	svc.repo = &lifecycleClaimHookRepository{
		repoStore: repo,
		afterClaim: func() {
			svc.markQueuedDispatchInFlight("s1", "newer-dispatch")
		},
	}

	queued := &messagequeue.QueuedMessage{
		ID: "lifecycle-stale-claim", SessionID: "s1", TaskID: "t1", Content: "do not stale-dispatch",
		Metadata: map[string]interface{}{"origin": githubPRAutomationOrigin},
	}
	svc.markQueuedDispatchInFlight("s1", queued.ID)
	svc.executeQueuedMessage("s1", queued)

	if got := len(agentMgr.capturedPrompts); got != 0 {
		t.Errorf("stale lifecycle worker called executor.Prompt %d times, want 0", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 1 {
		t.Errorf("superseded lifecycle entry count = %d, want 1 retry entry", got)
	}
	if !svc.isCurrentQueuedDispatch("s1", "newer-dispatch") {
		t.Error("stale lifecycle cleanup replaced the newer dispatch token")
	}
}

// A lifecycle entry is task-owned, rather than permanently owned by the
// session that happened to be selected when it was accepted. If task session
// selection changes before the final lifecycle claim, retain the same
// coalesced event for the replacement session instead of treating the old
// session as an inactive task and silently discarding it.
