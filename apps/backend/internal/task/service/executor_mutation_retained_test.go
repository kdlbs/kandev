package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// retainedFixture is a task in Alice's workspace, shared with collaborator
// Carol, whose metadata and active session still name an executor profile
// that was deleted, a parent executor that was retired, and an environment
// row that no longer exists.
type retainedFixture struct {
	workspaceID string
	workflowID  string
	taskID      string
	sessionID   string
	profileID   string
	executorID  string
	envID       string
}

func retainedUser(userID string) authn.Identity {
	return authn.Identity{UserID: userID, Role: authn.RoleMember}
}

func seedRetainedExecutorTask(t *testing.T, repo *sqliterepo.Repository, id string) retainedFixture {
	t.Helper()
	ctx := context.Background()
	f := retainedFixture{
		workspaceID: "ws-" + id, workflowID: "wf-" + id, taskID: "task-" + id, sessionID: "session-" + id,
		profileID: "gone-profile-" + id, executorID: "retired-" + id, envID: "missing-env-" + id,
	}
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: f.workspaceID, Name: id, OwnerID: "alice"}))
	require.NoError(t, repo.UpsertWorkspaceMember(ctx, &models.WorkspaceMember{
		WorkspaceID: f.workspaceID, UserID: "carol", Role: "collaborator",
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{ID: f.workflowID, WorkspaceID: f.workspaceID, Name: id}))
	require.NoError(t, repo.CreateExecutor(ctx, &models.Executor{
		ID: f.executorID, Name: "Retired", Type: models.ExecutorTypeSSH,
		Status: models.ExecutorStatusActive, OwnerUserID: "alice",
	}))
	require.NoError(t, repo.CreateExecutorProfile(ctx, &models.ExecutorProfile{
		ID: f.profileID, ExecutorID: f.executorID, Name: "Deleted", OwnerUserID: "alice",
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: f.taskID, WorkspaceID: f.workspaceID, WorkflowID: f.workflowID, WorkflowStepID: "step-1",
		Title: "Original title", Description: "Original description", State: v1.TaskStateCreated, Priority: "medium",
		Metadata: map[string]interface{}{
			models.MetaKeyExecutorProfileID: f.profileID, models.MetaKeyExecutorID: f.executorID, "note": "kept",
		},
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: f.sessionID, TaskID: f.taskID, ExecutorID: f.executorID, ExecutorProfileID: f.profileID,
		TaskEnvironmentID: f.envID, State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.DeleteExecutorProfile(ctx, f.profileID))
	require.NoError(t, repo.DeleteExecutor(ctx, f.executorID))
	return f
}

func createPrivateExecutor(t *testing.T, repo *sqliterepo.Repository, id string) (*models.Executor, *models.ExecutorProfile) {
	t.Helper()
	executor := &models.Executor{
		ID: id, Name: "Private", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive, OwnerUserID: "alice",
	}
	require.NoError(t, repo.CreateExecutor(context.Background(), executor))
	profile := &models.ExecutorProfile{ID: id + "-profile", ExecutorID: id, Name: "Private", OwnerUserID: "alice"}
	require.NoError(t, repo.CreateExecutorProfile(context.Background(), profile))
	return executor, profile
}

// requireRetainedEvidence proves an edit left the stale bindings exactly as
// they were: the task metadata, the session row and the absent rows.
func requireRetainedEvidence(t *testing.T, repo *sqliterepo.Repository, f retainedFixture) {
	t.Helper()
	ctx := context.Background()
	task, err := repo.GetTask(ctx, f.taskID)
	require.NoError(t, err)
	require.Equal(t, f.profileID, task.Metadata[models.MetaKeyExecutorProfileID])
	require.Equal(t, f.executorID, task.Metadata[models.MetaKeyExecutorID])
	session, err := repo.GetTaskSession(ctx, f.sessionID)
	require.NoError(t, err)
	require.Equal(t, f.executorID, session.ExecutorID)
	require.Equal(t, f.profileID, session.ExecutorProfileID)
	require.Equal(t, f.envID, session.TaskEnvironmentID)
	require.Equal(t, models.TaskSessionStateWaitingForInput, session.State)
	_, err = repo.GetExecutorProfile(ctx, f.profileID)
	require.ErrorIs(t, err, repoerrors.ErrExecutorProfileNotFound)
	_, err = repo.GetExecutor(ctx, f.executorID)
	require.ErrorIs(t, err, models.ErrExecutorNotFound)
	retained, err := repo.HasExecutorRecord(ctx, f.executorID)
	require.NoError(t, err)
	require.True(t, retained)
}

func requireTaskDescription(t *testing.T, repo *sqliterepo.Repository, taskID, want string) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	require.Equal(t, want, task.Description)
}

func TestRetainedExecutorBindingsDoNotBlockContentEdits(t *testing.T) {
	for _, user := range []string{"alice", "carol"} {
		t.Run(user, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			f := seedRetainedExecutorTask(t, repo, "content-"+user)
			ctx := authn.WithIdentity(t.Context(), retainedUser(user))

			description, title := "Edited by "+user, "Retitled by "+user
			updated, err := svc.UpdateTask(ctx, f.taskID, &UpdateTaskRequest{Description: &description, Title: &title})
			require.NoError(t, err)
			require.Equal(t, description, updated.Description)
			require.Equal(t, title, updated.Title)
			require.NoError(t, svc.AuthorizeWorkflowExecutorMutation(ctx, f.workflowID))
			requireRetainedEvidence(t, repo, f)
		})
	}
}

func TestRetainedExecutorLiveBindingsStillDenyCollaborator(t *testing.T) {
	cases := []struct {
		name string
		bind func(t *testing.T, repo *sqliterepo.Repository, f retainedFixture)
	}{
		{"metadata profile", func(t *testing.T, repo *sqliterepo.Repository, f retainedFixture) {
			_, profile := createPrivateExecutor(t, repo, "live-meta-profile")
			setTaskMetadata(t, repo, f.taskID, map[string]interface{}{models.MetaKeyExecutorProfileID: profile.ID})
		}},
		{"metadata executor behind a deleted profile", func(t *testing.T, repo *sqliterepo.Repository, f retainedFixture) {
			executor, _ := createPrivateExecutor(t, repo, "live-meta-executor")
			setTaskMetadata(t, repo, f.taskID, map[string]interface{}{
				models.MetaKeyExecutorProfileID: f.profileID, models.MetaKeyExecutorID: executor.ID,
			})
		}},
		{"active session executor", func(t *testing.T, repo *sqliterepo.Repository, f retainedFixture) {
			executor, _ := createPrivateExecutor(t, repo, "live-session-executor")
			updateSession(t, repo, f.sessionID, func(s *models.TaskSession) { s.ExecutorID = executor.ID })
		}},
		{"active session environment", func(t *testing.T, repo *sqliterepo.Repository, f retainedFixture) {
			executor, _ := createPrivateExecutor(t, repo, "live-env-executor")
			env := &models.TaskEnvironment{ID: "live-env", TaskID: f.taskID, ExecutorID: executor.ID}
			require.NoError(t, repo.CreateTaskEnvironment(context.Background(), env))
			updateSession(t, repo, f.sessionID, func(s *models.TaskSession) { s.TaskEnvironmentID = env.ID })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			f := seedRetainedExecutorTask(t, repo, "live")
			tc.bind(t, repo, f)
			carol := authn.WithIdentity(t.Context(), retainedUser("carol"))

			description := "Run my commands"
			_, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Description: &description})
			require.ErrorIs(t, err, repoerrors.ErrExecutorProfileNotFound)
			requireTaskDescription(t, repo, f.taskID, "Original description")
			err = svc.AuthorizeWorkflowExecutorMutation(carol, f.workflowID)
			require.ErrorIs(t, err, models.ErrExecutorMutationDenied)

			alice := authn.WithIdentity(t.Context(), retainedUser("alice"))
			_, err = svc.UpdateTask(alice, f.taskID, &UpdateTaskRequest{Description: &description})
			require.NoError(t, err)
		})
	}
}

func TestRetainedExecutorSelectionChangesAreChecked(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := seedRetainedExecutorTask(t, repo, "selection")
	privateExecutor, privateProfile := createPrivateExecutor(t, repo, "selection-private")
	shared := &models.Executor{ID: "selection-shared", Name: "Shared", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	require.NoError(t, repo.CreateExecutor(t.Context(), shared))
	sharedProfile := &models.ExecutorProfile{ID: "selection-shared-profile", ExecutorID: shared.ID, Name: "Shared"}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), sharedProfile))
	carol := authn.WithIdentity(t.Context(), retainedUser("carol"))
	stale := func(extra map[string]interface{}) map[string]interface{} {
		out := map[string]interface{}{models.MetaKeyExecutorProfileID: f.profileID, models.MetaKeyExecutorID: f.executorID}
		for k, v := range extra {
			out[k] = v
		}
		return out
	}

	for name, metadata := range map[string]map[string]interface{}{
		"foreign private profile":  stale(map[string]interface{}{models.MetaKeyExecutorProfileID: privateProfile.ID}),
		"foreign private executor": stale(map[string]interface{}{models.MetaKeyExecutorID: privateExecutor.ID}),
		"new missing profile":      stale(map[string]interface{}{models.MetaKeyExecutorProfileID: "never-existed"}),
		"new missing executor":     stale(map[string]interface{}{models.MetaKeyExecutorID: "never-existed"}),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Metadata: metadata})
			require.Error(t, err)
			requireRetainedEvidence(t, repo, f)
		})
	}

	t.Run("unchanged stale bindings", func(t *testing.T) {
		updated, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Metadata: stale(map[string]interface{}{"note": "edited"})})
		require.NoError(t, err)
		require.Equal(t, "edited", updated.Metadata["note"])
		requireRetainedEvidence(t, repo, f)
	})
	t.Run("eligible shared profile", func(t *testing.T) {
		metadata := map[string]interface{}{models.MetaKeyExecutorProfileID: sharedProfile.ID, models.MetaKeyExecutorID: shared.ID}
		updated, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Metadata: metadata})
		require.NoError(t, err)
		require.Equal(t, sharedProfile.ID, updated.Metadata[models.MetaKeyExecutorProfileID])
	})
	t.Run("owner selects own private profile", func(t *testing.T) {
		alice := authn.WithIdentity(t.Context(), retainedUser("alice"))
		metadata := map[string]interface{}{models.MetaKeyExecutorProfileID: privateProfile.ID, models.MetaKeyExecutorID: privateExecutor.ID}
		_, err := svc.UpdateTask(alice, f.taskID, &UpdateTaskRequest{Metadata: metadata})
		require.NoError(t, err)
	})
}

func TestRetainedExecutorEditKeepsWorkspaceAuthorization(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := seedRetainedExecutorTask(t, repo, "workspace")
	other := seedRetainedExecutorTask(t, repo, "elsewhere")
	require.NoError(t, repo.UpsertWorkspaceMember(t.Context(), &models.WorkspaceMember{
		WorkspaceID: f.workspaceID, UserID: "victor", Role: "viewer",
	}))
	_, err := repo.DB().ExecContext(t.Context(),
		`DELETE FROM workspace_members WHERE workspace_id = ? AND user_id = 'carol'`, other.workspaceID)
	require.NoError(t, err)
	description := "Run my commands"

	dave := authn.WithIdentity(t.Context(), retainedUser("dave"))
	_, err = svc.UpdateTask(dave, f.taskID, &UpdateTaskRequest{Description: &description})
	require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)

	carol := authn.WithIdentity(t.Context(), retainedUser("carol"))
	_, err = svc.UpdateTask(carol, other.taskID, &UpdateTaskRequest{Description: &description})
	require.ErrorIs(t, err, repoerrors.ErrTaskNotFound)

	victor := authn.WithIdentity(t.Context(), retainedUser("victor"))
	_, err = svc.UpdateTask(victor, f.taskID, &UpdateTaskRequest{Description: &description})
	require.Error(t, err)

	requireTaskDescription(t, repo, f.taskID, "Original description")
	requireTaskDescription(t, repo, other.taskID, "Original description")
}

func TestRetainedExecutorEndedSessionsAreNotConsidered(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := seedRetainedExecutorTask(t, repo, "ended")
	executor, profile := createPrivateExecutor(t, repo, "ended-private")
	for _, state := range []models.TaskSessionState{models.TaskSessionStateCompleted, models.TaskSessionStateFailed} {
		require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{
			ID: "old-" + string(state), TaskID: f.taskID, ExecutorID: executor.ID, ExecutorProfileID: profile.ID, State: state,
		}))
	}
	carol := authn.WithIdentity(t.Context(), retainedUser("carol"))
	description := "Edited"
	_, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Description: &description})
	require.NoError(t, err)
	requireRetainedEvidence(t, repo, f)
}

type failingExecutorReads struct {
	repository.ExecutorRepository
	profileErr  error
	executorErr error
}

func (r failingExecutorReads) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	if r.profileErr != nil {
		return nil, r.profileErr
	}
	return r.ExecutorRepository.GetExecutorProfile(ctx, id)
}

func (r failingExecutorReads) GetExecutor(ctx context.Context, id string) (*models.Executor, error) {
	if r.executorErr != nil {
		return nil, r.executorErr
	}
	return r.ExecutorRepository.GetExecutor(ctx, id)
}

type failingEnvironmentReads struct {
	repository.TaskEnvironmentRepository
	err error
}

func (r failingEnvironmentReads) GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error) {
	return nil, r.err
}

func TestRetainedExecutorStorageErrorsFailClosed(t *testing.T) {
	storageErr := errors.New("storage offline")
	cases := map[string]func(svc *Service){
		"profile read": func(svc *Service) {
			svc.executors = failingExecutorReads{ExecutorRepository: svc.executors, profileErr: storageErr}
		},
		"executor read": func(svc *Service) {
			svc.executors = failingExecutorReads{ExecutorRepository: svc.executors, executorErr: storageErr}
		},
		"environment read": func(svc *Service) { svc.taskEnvironments = failingEnvironmentReads{svc.taskEnvironments, storageErr} },
	}
	for name, inject := range cases {
		t.Run(name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			f := seedRetainedExecutorTask(t, repo, "storage")
			inject(svc)
			carol := authn.WithIdentity(t.Context(), retainedUser("carol"))
			description := "Edited"
			_, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Description: &description})
			require.ErrorIs(t, err, storageErr)
			err = svc.AuthorizeWorkflowExecutorMutation(carol, f.workflowID)
			require.ErrorIs(t, err, storageErr)
			require.NotErrorIs(t, err, models.ErrExecutorMutationDenied)
			requireTaskDescription(t, repo, f.taskID, "Original description")
		})
	}
}

func TestRetainedExecutorCanceledEditFailsClosed(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := seedRetainedExecutorTask(t, repo, "canceled")
	ctx, cancel := context.WithCancel(authn.WithIdentity(t.Context(), retainedUser("carol")))
	cancel()
	description := "Edited"
	_, err := svc.UpdateTask(ctx, f.taskID, &UpdateTaskRequest{Description: &description})
	require.Error(t, err)
	requireTaskDescription(t, repo, f.taskID, "Original description")
}

// Edits racing a profile deletion either observe the live private profile
// and are refused, or observe its absence and succeed; no edit is admitted
// with any other outcome.
func TestRetainedExecutorEditsRacingProfileDeletion(t *testing.T) {
	svc, _, repo := createTestService(t)
	f := seedRetainedExecutorTask(t, repo, "race")
	_, profile := createPrivateExecutor(t, repo, "race-private")
	setTaskMetadata(t, repo, f.taskID, map[string]interface{}{models.MetaKeyExecutorProfileID: profile.ID})
	carol := authn.WithIdentity(t.Context(), retainedUser("carol"))

	const editors = 8
	var wg sync.WaitGroup
	errs := make([]error, editors)
	wg.Add(editors + 1)
	go func() {
		defer wg.Done()
		require.NoError(t, repo.DeleteExecutorProfile(context.Background(), profile.ID))
	}()
	for i := range editors {
		go func() {
			defer wg.Done()
			description := "Edited"
			_, errs[i] = svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Description: &description})
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			require.ErrorIs(t, err, repoerrors.ErrExecutorProfileNotFound)
		}
	}
	description := "After deletion"
	_, err := svc.UpdateTask(carol, f.taskID, &UpdateTaskRequest{Description: &description})
	require.NoError(t, err)
	requireTaskDescription(t, repo, f.taskID, description)
}

func setTaskMetadata(t *testing.T, repo *sqliterepo.Repository, taskID string, metadata map[string]interface{}) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	require.NoError(t, err)
	task.Metadata = metadata
	require.NoError(t, repo.UpdateTask(context.Background(), task))
}

func updateSession(t *testing.T, repo *sqliterepo.Repository, sessionID string, mutate func(*models.TaskSession)) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), sessionID)
	require.NoError(t, err)
	mutate(session)
	require.NoError(t, repo.UpdateTaskSession(context.Background(), session))
}
