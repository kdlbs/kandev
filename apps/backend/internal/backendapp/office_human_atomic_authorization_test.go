package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type officeAuthorizationReadWindow struct {
	*tasksqlite.Repository
	afterRead func()
}

func (r *officeAuthorizationReadWindow) GetTask(ctx context.Context, id string) (*models.Task, error) {
	task, err := r.Repository.GetTask(ctx, id)
	if err == nil && r.afterRead != nil {
		fn := r.afterRead
		r.afterRead = nil
		fn()
	}
	return task, err
}

// The authorization read and the write snapshot must refer to the same scope.
func TestOfficeHumanCompletionAuthorizesObservedWorkspace(t *testing.T) {
	var window *officeAuthorizationReadWindow
	f := newOfficeAtomicFixture(t, func(r *tasksqlite.Repository) taskrepo.TaskRepository {
		window = &officeAuthorizationReadWindow{Repository: r}
		return window
	})
	ctx := context.Background()
	_, err := f.h.db.Exec(`UPDATE workspaces SET owner_id=CASE WHEN id='ws-1' THEN 'owner-a' ELSE 'owner-b' END WHERE id IN ('ws-1','ws-2')`)
	require.NoError(t, err)
	require.NoError(t, f.workflow.CreateStep(ctx, &wfmodels.WorkflowStep{ID: "foreign-step", WorkflowID: "wf-other-workspace", Name: "Foreign", Position: 0}))
	window.afterRead = func() {
		task, err := f.h.repo.GetTask(ctx, "atomic-task")
		require.NoError(t, err)
		task.WorkspaceID = "ws-2"
		task.WorkflowID = "wf-other-workspace"
		task.WorkflowStepID = "foreign-step"
		require.NoError(t, f.h.repo.UpdateTask(ctx, task))
	}
	caller := authn.WithIdentity(ctx, authn.Identity{UserID: "owner-a", Role: authn.RoleMember})
	_, err = f.svc.UpdateTaskStateAsHuman(caller, "atomic-task", v1.TaskStateCompleted)
	require.ErrorIs(t, err, repoerrors.ErrWorkflowChangeConflict, "a pre-read authorization must not lend access to the later foreign snapshot")
	foreign, err := f.h.repo.GetTask(ctx, "atomic-task")
	require.NoError(t, err)
	require.Equal(t, "ws-2", foreign.WorkspaceID)
	require.Equal(t, v1.TaskStateTODO, foreign.State)
}
