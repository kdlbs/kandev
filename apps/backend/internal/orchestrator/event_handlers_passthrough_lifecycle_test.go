package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type countingLifecycleStepGetter struct {
	*mockStepGetter
	getStepCalls int
}

func (g *countingLifecycleStepGetter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	g.getStepCalls++
	return g.mockStepGetter.GetStep(ctx, stepID)
}

// A durable lifecycle reservation made by handleAgentReady must reserve its
// dispatch token while the ready handler still owns the session guard. If it
// waits until after releasing the guard, a concurrent manual drain can reserve
// the same durable row and start a second delivery attempt.
func TestHandleAgentReady_PassthroughLifecycleReservationBlocksConcurrentDrain(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
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

	deferred := make(chan struct{})
	releaseDeferred := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseDeferred:
		default:
			close(releaseDeferred)
		}
	})
	svc.afterReadyLifecycleReservation = func() {
		close(deferred)
		<-releaseDeferred
	}
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(readyDone)
	}()
	select {
	case <-deferred:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deferred lifecycle dispatch")
	}

	drained, err := svc.DrainQueuedMessage(ctx, "s1")
	if err != nil {
		t.Fatalf("concurrent manual drain: %v", err)
	}
	if drained {
		t.Fatal("concurrent manual drain started a duplicate lifecycle dispatch")
	}
	close(releaseDeferred)
	select {
	case <-readyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lifecycle dispatch")
	}

	if got := len(messages.userMessages); got != 1 {
		t.Fatalf("visible lifecycle messages = %d, want 1", got)
	}
	if got := len(agentMgr.passthroughStdinCalls); got != 1 {
		t.Fatalf("PTY lifecycle prompts = %d, want 1", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("durable lifecycle queue entries after acceptance = %d, want 0", got)
	}

	// The following ready event completes the accepted prompt. The durable row
	// has already been acknowledged, so it must not deliver another prompt.
	svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
	if got := len(messages.userMessages); got != 1 {
		t.Fatalf("visible lifecycle messages after next ready = %d, want 1", got)
	}
	if got := len(agentMgr.passthroughStdinCalls); got != 1 {
		t.Fatalf("PTY lifecycle prompts after next ready = %d, want 1", got)
	}
}

// Archive may win after handleAgentReady reserves a durable lifecycle row and
// releases its session guard, but before the deferred dispatch begins. That
// stale entry must be discarded before it can trigger a workflow turn-start,
// resume runtime state, create a visible message, or retry itself.
func TestHandleAgentReady_PassthroughLifecycleArchiveBeforeDeferredDispatchHasNoSideEffects(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedExecutorRunning(t, repo, "s1", "t1", "exec-1")

	agentMgr := &mockAgentManager{
		isPassthrough:          true,
		isAgentRunning:         true,
		repoForExecutionLookup: repo,
	}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, "t1", v1.TaskStateReview)
	steps := &countingLifecycleStepGetter{mockStepGetter: newMockStepGetter()}
	steps.steps["step1"] = &wfmodels.WorkflowStep{ID: "step1", WorkflowID: "wf1"}
	svc := createTestServiceWithAgent(repo, steps.mockStepGetter, taskRepo, agentMgr)
	// Preserve the counter wrapper while retaining the test helper's normal
	// construction of every other service dependency.
	svc.workflowStepGetter = steps
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

	reserved := make(chan struct{})
	releaseDispatch := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-releaseDispatch:
		default:
			close(releaseDispatch)
		}
	})
	svc.afterReadyLifecycleReservation = func() {
		close(reserved)
		<-releaseDispatch
	}
	readyDone := make(chan struct{})
	go func() {
		svc.handleAgentReady(ctx, watcher.AgentEventData{TaskID: "t1", SessionID: "s1"})
		close(readyDone)
	}()
	select {
	case <-reserved:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deferred lifecycle reservation")
	}
	turnStartCallsBeforeArchive := steps.getStepCalls

	if err := repo.ArchiveTask(ctx, "t1"); err != nil {
		t.Fatalf("archive task after lifecycle reservation: %v", err)
	}
	close(releaseDispatch)
	select {
	case <-readyDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for deferred lifecycle dispatch")
	}

	if got := steps.getStepCalls; got != turnStartCallsBeforeArchive {
		t.Fatalf("stale lifecycle dispatch invoked workflow step lookup %d additional times, want 0", got-turnStartCallsBeforeArchive)
	}
	if got := len(messages.userMessages); got != 0 {
		t.Fatalf("visible lifecycle messages after archive = %d, want 0", got)
	}
	if got := len(agentMgr.passthroughStdinCalls); got != 0 {
		t.Fatalf("PTY lifecycle prompts after archive = %d, want 0", got)
	}
	if got := svc.messageQueue.GetStatus(ctx, "s1").Count; got != 0 {
		t.Fatalf("stale lifecycle queue entries after archive = %d, want 0", got)
	}
}
