package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// deferredLaunchRaceRepo wraps the real repository and, on the first GetTask
// call for a watched task, writes a fresh deferred_launch record through the
// repository's own CAS primitive before returning — modeling the session
// ceiling's admission controller mutating deferred_launch in the window
// between an orchestrator handler's read and its write-back.
type deferredLaunchRaceRepo struct {
	*sqliterepo.Repository
	t           *testing.T
	watchTaskID string
	fired       bool
}

func (r *deferredLaunchRaceRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.Repository.GetTask(ctx, id)
	if id == r.watchTaskID && !r.fired {
		r.fired = true
		_, prior, priorErr := r.GetTaskDeferredLaunch(ctx, id)
		require.NoError(r.t, priorErr)
		stored, lostCompare, setErr := r.SetTaskDeferredLaunchIfUnchanged(ctx, id, prior,
			map[string]interface{}{"prompt": "original", "ceiling_deferred": true})
		require.NoError(r.t, setErr)
		require.False(r.t, lostCompare)
		require.True(r.t, stored)
	}
	return task, err
}

// TestMarkTaskCompletedForTerminalStep_PreservesConcurrentDeferredLaunchWrite
// pins R2R-B at a real orchestrator entry point (markTaskCompletedForTerminalStep,
// one of the nine call sites that used to write through plain UpdateTask):
// the ceiling admission controller's own CAS write to deferred_launch, landing
// between this handler's GetTask and its own write-back, must survive. Before
// the fix, UpdateTask wrote the handler's stale in-memory metadata verbatim
// and silently erased the concurrently-written ceiling_deferred record.
func TestMarkTaskCompletedForTerminalStep_PreservesConcurrentDeferredLaunchWrite(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "term-deferred-race", "term-deferred-race-session", models.TaskSessionStateCompleted)

	task, err := repo.GetTask(ctx, "term-deferred-race")
	require.NoError(t, err)
	task.WorkflowStepID = "term-step"
	task.UpdatedAt = time.Now().UTC()
	require.NoError(t, repo.UpdateTask(ctx, task))

	stored, lostCompare, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "term-deferred-race",
		sqliterepo.AbsentDeferredLaunch(), map[string]interface{}{"prompt": "original"})
	require.NoError(t, err)
	require.False(t, lostCompare)
	require.True(t, stored)

	raceRepo := &deferredLaunchRaceRepo{Repository: repo, t: t, watchTaskID: "term-deferred-race"}
	svc := &Service{repo: raceRepo, logger: testLogger()}

	svc.markTaskCompletedForTerminalStep(ctx, "term-deferred-race", "term-step")

	require.True(t, raceRepo.fired, "the concurrent race must have actually been injected")

	current, err := repo.GetTask(ctx, "term-deferred-race")
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateCompleted, current.State, "the terminal-step completion must still apply")

	deferred, ok := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "deferred_launch missing or wrong shape: %#v", current.Metadata[models.MetaKeyDeferredLaunch])
	require.Equal(t, true, deferred["ceiling_deferred"],
		"a plain UpdateTask through this orchestrator path must not clobber a concurrent ceiling CAS write")
}
