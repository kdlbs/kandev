package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// ceilingWriteRaceRepo wraps the real repository and, on the first GetTask
// call for a watched task, writes a fresh deferred_launch record through the
// repository's own CAS primitive before returning the pre-race snapshot —
// modeling the session ceiling's admission controller mutating
// deferred_launch in the window between a read-modify-write caller's own
// GetTask and its later write-back.
type ceilingWriteRaceRepo struct {
	repository.TaskRepository
	t           *testing.T
	watchTaskID string
	fired       bool
}

func (r *ceilingWriteRaceRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.TaskRepository.GetTask(ctx, id)
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

// TestUpdateTaskMetadataSurvivesAConcurrentCeilingWrite pins RV3-B at
// UpdateTaskMetadata: the session ceiling admission controller's own CAS
// write to deferred_launch, landing between this method's GetTask and its
// own write-back, must survive. Before the fix, the plain UpdateTask call
// wrote the stale in-memory metadata verbatim and silently erased the
// concurrently-written ceiling_deferred record.
func TestUpdateTaskMetadataSurvivesAConcurrentCeilingWrite(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-meta-race", Name: "Race"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-meta-race", WorkspaceID: "ws-meta-race", Name: "flow"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: "task-meta-race", WorkspaceID: "ws-meta-race", WorkflowID: "wf-meta-race",
		Title: "Race", State: v1.TaskStateCreated,
	}))
	_, lostCompare, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "task-meta-race",
		sqliterepo.AbsentDeferredLaunch(), map[string]interface{}{"prompt": "original"})
	require.NoError(t, err)
	require.False(t, lostCompare)

	raceRepo := &ceilingWriteRaceRepo{TaskRepository: repo, t: t, watchTaskID: "task-meta-race"}
	svc.tasks = raceRepo

	_, err = svc.UpdateTaskMetadata(ctx, "task-meta-race", map[string]interface{}{"ordinary": "value"})
	require.NoError(t, err)
	require.True(t, raceRepo.fired, "the concurrent race must have actually been injected")

	current, err := repo.GetTask(ctx, "task-meta-race")
	require.NoError(t, err)
	require.Equal(t, "value", current.Metadata["ordinary"], "the caller's own metadata update must still apply")

	deferred, ok := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "deferred_launch missing or wrong shape: %#v", current.Metadata[models.MetaKeyDeferredLaunch])
	require.Equal(t, true, deferred["ceiling_deferred"],
		"UpdateTaskMetadata must not clobber a concurrent ceiling CAS write")
}

// TestMoveTaskWithOptionsSameStepSurvivesAConcurrentCeilingWrite pins RV3-B
// at updateMovedTaskSameStep's plain-UpdateTask branch (no ExpectedWorkflowID,
// the ordinary reorder/same-step-move path): the same ceiling CAS race must
// survive a same-step move just as it does an ordinary metadata edit.
func TestMoveTaskWithOptionsSameStepSurvivesAConcurrentCeilingWrite(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	seedMoveSteps(svc)
	createMoveTask(t, ctx, repo, "task-move-race", "wf-source", "step-source", nil)
	_, lostCompare, err := repo.SetTaskDeferredLaunchIfUnchanged(ctx, "task-move-race",
		sqliterepo.AbsentDeferredLaunch(), map[string]interface{}{"prompt": "original"})
	require.NoError(t, err)
	require.False(t, lostCompare)

	raceRepo := &ceilingWriteRaceRepo{TaskRepository: repo, t: t, watchTaskID: "task-move-race"}
	svc.tasks = raceRepo

	_, err = svc.MoveTaskWithOptions(ctx, "task-move-race", "wf-source", "step-source", 1, MoveTaskOptions{})
	require.NoError(t, err)
	require.True(t, raceRepo.fired, "the concurrent race must have actually been injected")

	current, err := repo.GetTask(ctx, "task-move-race")
	require.NoError(t, err)
	require.Equal(t, 1, current.Position, "the caller's own move must still apply")

	deferred, ok := current.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, ok, "deferred_launch missing or wrong shape: %#v", current.Metadata[models.MetaKeyDeferredLaunch])
	require.Equal(t, true, deferred["ceiling_deferred"],
		"a same-step move must not clobber a concurrent ceiling CAS write")
}
