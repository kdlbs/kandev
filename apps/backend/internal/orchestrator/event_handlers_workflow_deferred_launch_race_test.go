package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// deferredLaunchPublishRaceRepo wraps the orchestrator's repo handle and, on
// the first GetTask call for the watched task, writes a fresh
// deferred_launch record through the repository's own CAS primitive before
// returning — modeling the session ceiling admission controller deferring a
// concurrent launch in the window between launchDeferredTask's detached
// goroutine finishing LaunchSession and reloading the task to publish.
type deferredLaunchPublishRaceRepo struct {
	sessionExecutorStore
	t           *testing.T
	watchTaskID string
	mu          sync.Mutex
	fired       bool
}

func (r *deferredLaunchPublishRaceRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.sessionExecutorStore.GetTask(ctx, id)
	r.mu.Lock()
	shouldFire := id == r.watchTaskID && !r.fired
	if shouldFire {
		r.fired = true
	}
	r.mu.Unlock()
	if shouldFire {
		_, prior, priorErr := r.GetTaskDeferredLaunch(ctx, id)
		require.NoError(r.t, priorErr)
		stored, lostCompare, setErr := r.SetTaskDeferredLaunchIfUnchanged(ctx, id, prior,
			map[string]interface{}{"prompt": "concurrent", "ceiling_deferred": true})
		require.NoError(r.t, setErr)
		require.False(r.t, lostCompare)
		require.True(r.t, stored)
	}
	return task, err
}

// capturingTaskEvents records every PublishTaskUpdated call's full task
// argument. recordingTaskEventPublisher (office_task_status_test.go) only
// keeps the task ID, which cannot pin an assertion on deferred_launch's
// published contents.
type capturingTaskEvents struct {
	mu    sync.Mutex
	tasks []*models.Task
}

func (c *capturingTaskEvents) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tasks = append(c.tasks, task)
}

func (c *capturingTaskEvents) PublishTaskStateChanged(context.Context, *models.Task, v1.TaskState) {}

func (c *capturingTaskEvents) PublishTaskActivityIfChanged(context.Context, string) {}

func (c *capturingTaskEvents) last() *models.Task {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.tasks) == 0 {
		return nil
	}
	return c.tasks[len(c.tasks)-1]
}

// TestLaunchDeferredTaskPublishesTheReloadedTaskAfterAConcurrentCeilingWrite
// pins the fix at launchDeferredTask's post-launch publish: a concurrent
// ceiling deferral writing a fresh deferred_launch record while
// LaunchSession runs must survive the publish, not be discarded by the
// pre-claim in-memory snapshot the closure captured.
func TestLaunchDeferredTaskPublishesTheReloadedTaskAfterAConcurrentCeilingWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)

	svc := newDeferredLaunchTestService(t, repo, newLaunchCounter())
	raceRepo := &deferredLaunchPublishRaceRepo{sessionExecutorStore: svc.repo, t: t, watchTaskID: deferredChainTaskID}
	svc.repo = raceRepo
	events := &capturingTaskEvents{}
	svc.SetTaskEventPublisher(events)

	require.True(t, svc.launchDeferredTask(ctx, task, "test.deferred_launch", false))
	awaitLaunchedSession(t, repo, deferredChainTaskID)

	deadline := time.Now().Add(2 * time.Second)
	var published *models.Task
	for time.Now().Before(deadline) {
		if published = events.last(); published != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.NotNil(t, published, "launchDeferredTask must publish the reloaded task after a successful launch")
	require.True(t, raceRepo.fired, "the concurrent ceiling race must have actually been injected")

	deferred, ok := published.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "published task's deferred_launch missing or wrong shape: %#v", published.Metadata[models.MetaKeyDeferredLaunch])
	require.Equal(t, true, deferred["ceiling_deferred"],
		"launchDeferredTask must publish a task reflecting the concurrent ceiling CAS write, not the stale pre-launch snapshot")
}
