package executor

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TestPersistTaskEnvironmentBindsCanonicalWorkspaceOnEverySuccessPath pins
// the session-row invariant consumed by shared-environment Git status source
// selection. The launch request may still carry the source checkout (or no
// path at all), but a successful environment bind must persist the effective
// environment workspace on the session before persistLaunchState writes it.
func TestPersistTaskEnvironmentBindsCanonicalWorkspaceOnEverySuccessPath(t *testing.T) {
	const canonicalPath = "/tasks/task-1/materialized"
	const sourcePath = "/source/checkout"

	tests := []struct {
		name string
		run  func(t *testing.T, repo *mockRepository, exec *Executor, session *models.TaskSession)
	}{
		{
			name: "new environment",
			run: func(t *testing.T, repo *mockRepository, exec *Executor, session *models.TaskSession) {
				err := exec.persistTaskEnvironment(
					context.Background(), session.TaskID, session, nil,
					&LaunchAgentRequest{
						TaskID:         session.TaskID,
						ExecutorType:   string(models.ExecutorTypeWorktree),
						RepositoryPath: sourcePath,
					},
					&LaunchAgentResponse{WorkspacePath: canonicalPath},
					executorConfig{ExecutorID: models.ExecutorIDWorktree},
				)
				if err != nil {
					t.Fatalf("persistTaskEnvironment: %v", err)
				}
				if got := repo.taskEnvironments[session.TaskEnvironmentID].WorkspacePath; got != canonicalPath {
					t.Fatalf("new environment workspace = %q, want %q", got, canonicalPath)
				}
			},
		},
		{
			name: "initial materializer",
			run: func(t *testing.T, repo *mockRepository, exec *Executor, session *models.TaskSession) {
				env := &models.TaskEnvironment{
					ID:                       "env-materializer",
					TaskID:                   session.TaskID,
					ExecutorType:             string(models.ExecutorTypeWorktree),
					Status:                   models.TaskEnvironmentStatusCreating,
					MaterializationSessionID: session.ID,
				}
				repo.taskEnvironments[env.ID] = env
				session.TaskEnvironmentID = env.ID
				err := exec.persistTaskEnvironment(
					context.Background(), session.TaskID, session, env,
					&LaunchAgentRequest{
						TaskID:         session.TaskID,
						ExecutorType:   string(models.ExecutorTypeWorktree),
						RepositoryPath: sourcePath,
					},
					&LaunchAgentResponse{
						WorktreePath: canonicalPath,
						Worktrees: []RepoWorktreeResult{{
							RepositoryID: "repo-1", WorktreePath: canonicalPath,
						}},
					},
					executorConfig{ExecutorID: models.ExecutorIDWorktree},
				)
				if err != nil {
					t.Fatalf("persistTaskEnvironment: %v", err)
				}
			},
		},
		{
			name: "ready sibling",
			run: func(t *testing.T, repo *mockRepository, exec *Executor, session *models.TaskSession) {
				env := &models.TaskEnvironment{
					ID:            "env-ready",
					TaskID:        session.TaskID,
					ExecutorType:  string(models.ExecutorTypeWorktree),
					Status:        models.TaskEnvironmentStatusReady,
					WorkspacePath: canonicalPath,
				}
				repo.taskEnvironments[env.ID] = env
				session.TaskEnvironmentID = env.ID
				err := exec.persistTaskEnvironment(
					context.Background(), session.TaskID, session, env,
					&LaunchAgentRequest{TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeWorktree)},
					&LaunchAgentResponse{WorkspacePath: canonicalPath},
					executorConfig{ExecutorID: models.ExecutorIDWorktree},
				)
				if err != nil {
					t.Fatalf("persistTaskEnvironment: %v", err)
				}
			},
		},
		{
			name: "cross-task guest",
			run: func(t *testing.T, repo *mockRepository, exec *Executor, session *models.TaskSession) {
				env := &models.TaskEnvironment{
					ID:            "env-owner",
					TaskID:        "task-owner",
					ExecutorType:  string(models.ExecutorTypeWorktree),
					Status:        models.TaskEnvironmentStatusReady,
					WorkspacePath: canonicalPath,
				}
				repo.taskEnvironments[env.ID] = env
				session.TaskEnvironmentID = env.ID
				err := exec.persistTaskEnvironment(
					context.Background(), session.TaskID, session, env,
					&LaunchAgentRequest{TaskID: session.TaskID, ExecutorType: string(models.ExecutorTypeWorktree)},
					&LaunchAgentResponse{}, executorConfig{ExecutorID: models.ExecutorIDWorktree},
				)
				if err != nil {
					t.Fatalf("persistTaskEnvironment: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newMockRepository()
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			session := &models.TaskSession{
				ID:            "session-" + test.name,
				TaskID:        "task-1",
				State:         models.TaskSessionStateCompleted,
				WorkspacePath: sourcePath,
			}
			repo.sessions[session.ID] = &models.TaskSession{
				ID: session.ID, TaskID: session.TaskID, State: session.State,
			}
			test.run(t, repo, exec, session)
			if session.WorkspacePath != canonicalPath {
				t.Fatalf("session workspace = %q, want canonical %q", session.WorkspacePath, canonicalPath)
			}
			if err := exec.persistLaunchState(
				context.Background(), session.TaskID, session.ID, session,
				&LaunchAgentResponse{}, false, time.Now().UTC(),
			); err != nil {
				t.Fatalf("persistLaunchState: %v", err)
			}
			if got := repo.sessions[session.ID].WorkspacePath; got != canonicalPath {
				t.Fatalf("persisted session workspace = %q, want canonical %q", got, canonicalPath)
			}
		})
	}
}
