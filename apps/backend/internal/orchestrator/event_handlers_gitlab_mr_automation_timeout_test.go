package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/gitlab"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRunTaskMRAutomationFollowUp_BoundsCanceledParentContext(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	started := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() {
		result <- runTaskMRAutomationFollowUp(parent, 10*time.Millisecond, func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("follow-up operation did not start")
	}

	select {
	case err := <-result:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("follow-up operation did not stop at its deadline")
	}
}

func TestTaskMRAutomationCheckpointFollowUpTimeout_ReleasesInFlight(t *testing.T) {
	oldTimeout := ciAutomationFollowUpTimeout
	ciAutomationFollowUpTimeout = 10 * time.Millisecond
	t.Cleanup(func() { ciAutomationFollowUpTimeout = oldTimeout })

	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "task-1", "session-1", models.TaskSessionStateRunning)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = &recordingEventBus{}
	automation := &mockGitLabMRAutomationService{
		options:           &gitlab.TaskMRAutomationResponse{PromptOnMerged: true},
		checkpoint:        &gitlab.TaskMRLifecycleState{LastObservedState: gitlabMRStateOpen},
		recordPromptBlock: true,
		recordPromptStart: make(chan struct{}, 1),
	}
	svc.gitlabMRAutomation = automation
	mr := &gitlab.TaskMR{
		TaskID: "task-1", RepositoryID: "repo-1", Host: "https://gitlab.example.com",
		ProjectPath: "group/project", MRIID: 42, State: gitlabMRStateMerged,
	}

	parent, cancel := context.WithCancel(context.Background())
	svc.startTaskMRLifecycleAutomation(parent, mr)
	select {
	case <-automation.recordPromptStart:
		cancel()
	case <-time.After(time.Second):
		cancel()
		t.Fatal("lifecycle evaluation did not reach the post-dispatch checkpoint")
	}

	key := mrAutomationInFlightKey(mr)
	require.Eventually(t, func() bool {
		_, loaded := svc.mrAutomationInFlight.Load(key)
		return !loaded
	}, time.Second, time.Millisecond, "timed-out follow-up kept the MR single-flight entry in flight")

	if got := svc.messageQueue.GetStatus(context.Background(), "session-1").Count; got != 1 {
		t.Fatalf("durable prompt count = %d, want 1 after checkpoint timeout", got)
	}
	if got := len(automation.recordedErrors()); got != 1 {
		t.Fatalf("recorded automation errors = %d, want 1 after checkpoint timeout", got)
	}
}

func TestRecordMRAutomationError_FollowUpTimeout(t *testing.T) {
	oldTimeout := ciAutomationFollowUpTimeout
	ciAutomationFollowUpTimeout = 10 * time.Millisecond
	t.Cleanup(func() { ciAutomationFollowUpTimeout = oldTimeout })

	automation := &mockGitLabMRAutomationService{
		recordErrorBlock: true,
		recordErrorStart: make(chan struct{}, 1),
	}
	svc := &Service{logger: testLogger(), gitlabMRAutomation: automation}
	mr := &gitlab.TaskMR{TaskID: "task-1", RepositoryID: "repo-1", ProjectPath: "group/project", MRIID: 42}
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		svc.recordMRAutomationError(parent, mr, errors.New("checkpoint failed"))
		close(done)
	}()

	select {
	case <-automation.recordErrorStart:
	case <-time.After(time.Second):
		t.Fatal("error persistence did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("error persistence ignored its follow-up deadline")
	}
}

func TestPublishTaskMRAutomationState_ResponseLoadFollowUpTimeout(t *testing.T) {
	oldTimeout := ciAutomationFollowUpTimeout
	ciAutomationFollowUpTimeout = 10 * time.Millisecond
	t.Cleanup(func() { ciAutomationFollowUpTimeout = oldTimeout })

	automation := &mockGitLabMRAutomationService{
		responseBlocks:  true,
		responseStarted: make(chan struct{}, 1),
	}
	svc := &Service{logger: testLogger(), gitlabMRAutomation: automation, eventBus: &recordingEventBus{}}
	done := make(chan struct{})
	go func() {
		svc.publishTaskMRAutomationState(context.Background(), "task-1")
		close(done)
	}()

	select {
	case <-automation.responseStarted:
	case <-time.After(time.Second):
		t.Fatal("state response load did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("state response load ignored its follow-up deadline")
	}
}

type blockingTaskMRAutomationEventBus struct {
	recordingEventBus
	started chan struct{}
}

func (b *blockingTaskMRAutomationEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestPublishTaskMRAutomationState_EventPublishFollowUpTimeout(t *testing.T) {
	oldTimeout := ciAutomationFollowUpTimeout
	ciAutomationFollowUpTimeout = 10 * time.Millisecond
	t.Cleanup(func() { ciAutomationFollowUpTimeout = oldTimeout })

	automation := &mockGitLabMRAutomationService{
		options: &gitlab.TaskMRAutomationResponse{TaskID: "task-1"},
	}
	eventBus := &blockingTaskMRAutomationEventBus{started: make(chan struct{}, 1)}
	svc := &Service{logger: testLogger(), gitlabMRAutomation: automation, eventBus: eventBus}
	done := make(chan struct{})
	go func() {
		svc.publishTaskMRAutomationState(context.Background(), "task-1")
		close(done)
	}()

	select {
	case <-eventBus.started:
	case <-time.After(time.Second):
		t.Fatal("state event publication did not start")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("state event publication ignored its follow-up deadline")
	}
}
