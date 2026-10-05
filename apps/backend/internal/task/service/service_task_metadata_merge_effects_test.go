package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/stepentry"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type metadataPostcommitRepo struct {
	*sqliterepo.Repository
	committed   atomic.Bool
	afterCommit func(context.Context) error
	readError   error
}

func (r *metadataPostcommitRepo) MergeTaskMetadata(ctx context.Context, id string, patch map[string]interface{}) error {
	if err := r.Repository.MergeTaskMetadata(ctx, id, patch); err != nil {
		return err
	}
	if r.afterCommit != nil {
		if err := r.afterCommit(ctx); err != nil {
			return err
		}
	}
	r.committed.Store(true)
	return nil
}
func (r *metadataPostcommitRepo) GetTask(ctx context.Context, id string) (*models.Task, error) {
	if id == "field-task" && r.committed.Load() && r.readError != nil {
		return nil, r.readError
	}
	return r.Repository.GetTask(ctx, id)
}

// @covers AC-TASKS-FIELD-UPDATES-001.12
func TestTaskMetadataMergePostcommitObservation(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "later_commit", true: "read_error"}[failure], func(t *testing.T) {
			injected := errors.New("postcommit read refused")
			svc, bus, repo := createTestServiceWithTaskAndSessionRepos(t, func(r *sqliterepo.Repository) repository.TaskRepository {
				wrapped := &metadataPostcommitRepo{Repository: r}
				if failure {
					wrapped.readError = injected
				} else {
					wrapped.afterCommit = func(ctx context.Context) error {
						title := "Later title"
						_, err := r.UpdateTaskFieldsWithParentAdmission(ctx, "field-task", models.TaskFieldUpdate{Title: &title}, hierarchy.ValidateParent)
						return err
					}
				}
				return wrapped
			}, func(r *sqliterepo.Repository) repository.SessionRepository { return r })
			seedFieldTask(t, repo, nil)
			bus.ClearEvents()
			returned, err := svc.UpdateTaskMetadata(context.Background(), "field-task", map[string]interface{}{"alpha": "accepted"})
			stored, readErr := repo.GetTask(context.Background(), "field-task")
			require.NoError(t, readErr)
			require.Equal(t, "accepted", stored.Metadata["alpha"])
			if failure {
				require.ErrorIs(t, err, injected)
				require.Nil(t, returned)
				require.Empty(t, bus.GetPublishedEvents())
				return
			}
			require.NoError(t, err)
			require.Equal(t, "Later title", returned.Title)
			published := bus.GetPublishedEvents()
			require.Len(t, published, 1)
			require.Equal(t, events.TaskUpdated, published[0].Type)
			data := published[0].Data.(map[string]interface{})
			require.Equal(t, returned.Title, data["title"])
			require.Equal(t, returned.Metadata, data["metadata"])
			require.Equal(t, returned.UpdatedAt.Format(time.RFC3339Nano), data["updated_at"])
		})
	}
}

type metadataEntryProbe struct{ count int }

func (p *metadataEntryProbe) DispatchStepEntry(context.Context, string, string, string, string, int64) {
	p.count++
}

// @covers AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergeEffectsAndFailures(t *testing.T) {
	probe := &metadataEntryProbe{}
	svc, bus, repo := createTestServiceWithTaskAndSessionRepos(t, func(r *sqliterepo.Repository) repository.TaskRepository {
		r.SetStepEntryDispatcher(probe)
		return r
	}, func(r *sqliterepo.Repository) repository.SessionRepository { return r })
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "field-ws", Name: "Fields"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "field-task", WorkspaceID: "field-ws", WorkflowID: "effect-wf", WorkflowStepID: "effect-source", Title: "Current", Priority: "high", AssigneeAgentProfileID: "runner-current", Metadata: map[string]interface{}{"keep": true}}))
	pending, ok := stepentry.BuildPendingAllocation("effect-target", []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterClearDecisions}})
	require.True(t, ok)
	holder := &stepentry.AllocationResult{}
	effectCtx := stepentry.WithResultHolder(stepentry.WithPendingAllocation(ctx, pending), holder)
	bus.ClearEvents()
	baselineDispatch := probe.count
	before := snapshotFieldRow(t, repo)
	var runnersBefore string
	require.NoError(t, repo.DB().QueryRow(`SELECT agent_profile_id FROM workflow_step_participants WHERE task_id='field-task' AND role='runner'`).Scan(&runnersBefore))
	_, err := svc.UpdateTaskMetadata(effectCtx, "field-task", map[string]interface{}{"alpha": "accepted"})
	require.NoError(t, err)
	after := snapshotFieldRow(t, repo)
	require.Equal(t, before.Ledger, after.Ledger)
	require.Equal(t, before.Entries, after.Entries)
	delete(before.Row, "metadata")
	delete(before.Row, "updated_at")
	delete(after.Row, "metadata")
	delete(after.Row, "updated_at")
	require.Equal(t, before.Row, after.Row)
	require.Equal(t, baselineDispatch, probe.count)
	require.Zero(t, holder.EntryID)
	require.Zero(t, holder.TransitionID)
	var runnersAfter string
	require.NoError(t, repo.DB().QueryRow(`SELECT agent_profile_id FROM workflow_step_participants WHERE task_id='field-task' AND role='runner'`).Scan(&runnersAfter))
	require.Equal(t, runnersBefore, runnersAfter)
	require.Len(t, bus.GetPublishedEvents(), 1)
	require.Equal(t, events.TaskUpdated, bus.GetPublishedEvents()[0].Type)
	for _, failure := range []string{"encoding", "store", "cancel", "missing"} {
		t.Run(failure, func(t *testing.T) {
			before := snapshotFieldRow(t, repo)
			bus.ClearEvents()
			patch := map[string]interface{}{"a": true, "b": true}
			callCtx := ctx
			id := "field-task"
			switch failure {
			case "encoding":
				patch["b"] = func() {}
			case "store":
				_, err := repo.DB().Exec(`CREATE TRIGGER reject_merge_service BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT,'rejected'); END`)
				require.NoError(t, err)
				defer func() { _, err := repo.DB().Exec(`DROP TRIGGER reject_merge_service`); require.NoError(t, err) }()
			case "cancel":
				var cancel context.CancelFunc
				callCtx, cancel = context.WithCancel(ctx)
				cancel()
			case "missing":
				id = "missing"
			}
			_, err := svc.UpdateTaskMetadata(callCtx, id, patch)
			require.Error(t, err)
			if failure == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if failure == "missing" {
				require.ErrorIs(t, err, repository.ErrTaskNotFound)
			}
			require.Equal(t, before, snapshotFieldRow(t, repo))
			require.Empty(t, bus.GetPublishedEvents())
			require.Equal(t, baselineDispatch, probe.count)
		})
	}
	// Same pending declaration is reachable through the actual ordinary typed transition.
	target := "effect-target"
	_, err = repo.UpdateTaskFieldsWithParentAdmission(effectCtx, "field-task", models.TaskFieldUpdate{WorkflowStepID: &target}, hierarchy.ValidateParent)
	require.NoError(t, err)
	require.Equal(t, baselineDispatch+1, probe.count)
	require.NotZero(t, holder.EntryID)
	require.NotZero(t, holder.TransitionID)
}

// @covers AC-TASKS-FIELD-UPDATES-001.9, AC-TASKS-FIELD-UPDATES-001.11
func TestTaskMetadataMergeCurrentTransitionAndAssociations(t *testing.T) {
	var repo *sqliterepo.Repository
	var canonical *models.Task
	var before fieldRowSnapshot
	var repositories []*models.TaskRepository
	var folders []*models.TaskWorkspaceFolder
	current := mergeAfterRealRead(t, func(ctx context.Context, _ *Service, r *sqliterepo.Repository) {
		repo = r
		require.NoError(t, r.CreateRepository(ctx, &models.Repository{ID: "merge-source", WorkspaceID: "field-ws", Name: "Source", LocalPath: "/canonical/source"}))
		require.NoError(t, r.CreateTaskRepository(ctx, &models.TaskRepository{ID: "merge-attachment", TaskID: "field-task", RepositoryID: "merge-source", BaseBranch: "current"}))
		require.NoError(t, r.CreateWorkspaceSourceBatch(ctx, &models.WorkspaceSourceBatch{TaskID: "field-task", Sources: []models.WorkspaceSource{{Folder: &models.TaskWorkspaceFolder{LocalPath: "/canonical/docs", DisplayName: "Docs"}}}}))
		target := "current-step"
		state := v1.TaskStateCompleted
		pending, ok := stepentry.BuildPendingAllocation(target, []wfmodels.OnEnterAction{{Type: wfmodels.OnEnterClearDecisions}})
		require.True(t, ok)
		holder := &stepentry.AllocationResult{}
		changeCtx := stepentry.WithResultHolder(stepentry.WithPendingAllocation(ctx, pending), holder)
		_, err := r.UpdateTaskFieldsWithParentAdmission(changeCtx, "field-task", models.TaskFieldUpdate{State: &state, WorkflowStepID: &target}, hierarchy.ValidateParent)
		require.NoError(t, err)
		require.NotZero(t, holder.EntryID)
		require.NotZero(t, holder.TransitionID)
		canonical, err = r.GetTask(ctx, "field-task")
		require.NoError(t, err)
		repositories, err = r.ListTaskRepositories(ctx, "field-task")
		require.NoError(t, err)
		folders, err = r.ListTaskWorkspaceFolders(ctx, "field-task")
		require.NoError(t, err)
		before = snapshotFieldRow(t, r)
	}, nil, map[string]interface{}{"alpha": "accepted"})
	require.Equal(t, v1.TaskStateCompleted, current.State)
	require.Equal(t, "current-step", current.WorkflowStepID)
	canonical.Metadata, canonical.UpdatedAt = current.Metadata, current.UpdatedAt
	require.Equal(t, canonical, current)
	after := snapshotFieldRow(t, repo)
	delete(before.Row, "metadata")
	delete(before.Row, "updated_at")
	delete(after.Row, "metadata")
	delete(after.Row, "updated_at")
	require.Equal(t, before, after)
	actualRepositories, err := repo.ListTaskRepositories(context.Background(), "field-task")
	require.NoError(t, err)
	require.Len(t, actualRepositories, 1)
	require.Equal(t, repositories, actualRepositories)
	actualFolders, err := repo.ListTaskWorkspaceFolders(context.Background(), "field-task")
	require.NoError(t, err)
	require.Len(t, actualFolders, 1)
	require.Equal(t, folders, actualFolders)
}
