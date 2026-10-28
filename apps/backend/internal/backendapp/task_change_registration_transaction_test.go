package backendapp

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/gitlab"
	mcp "github.com/kandev/kandev/internal/mcp/handlers"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func TestTaskChangeRegistrationCommitsAssociationAndIntentTogether(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "commit"
		if conflict {
			name = "receipt conflict rolls back association"
		}
		t.Run(name, func(t *testing.T) {
			taskSvc, repos, database := newTaskChangeCoordinatorHarnessWithDB(t)
			seedTaskChangeCoordinatorTask(t, repos, "ws-registration-tx", "task-registration-tx", "repo-registration-tx",
				"https://gitlab.example.test", "group", "project")
			_, err := database.Exec(`CREATE TABLE test_provider_associations (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, repository_id TEXT NOT NULL, mr_iid INTEGER NOT NULL)`)
			require.NoError(t, err)
			if conflict {
				seed := &tasksqlite.TaskChangeRegistrationIntent{
					ID: "prior-intent", TaskID: "task-registration-tx", WorkspaceID: "ws-registration-tx",
					SourceKind: "agent_registration", OriginID: "operation-1", SourceRevision: "0",
					ReferenceRef: "gitlab:provider-42#42", ReferenceSnapshot: `{"url":"different"}`,
					Provider: "gitlab", ProviderHost: "https://gitlab.example.test", ConnectionScope: "builtin:ws-registration-tx",
					ProviderRepositoryID: "provider-42", ChangeRequestID: "42", OperationID: "operation-1",
					OperationFingerprint: "different", State: "succeeded",
				}
				tx, err := database.BeginTxx(context.Background(), nil)
				require.NoError(t, err)
				_, _, err = repos.CreateTaskChangeRegistrationIntentTx(context.Background(), tx, seed)
				require.NoError(t, err)
				require.NoError(t, tx.Commit())
			}

			provider := &transactionalRegistrationGitLab{db: database}
			coordinator := taskChangeLinkCoordinator{
				tasks: taskSvc, gitlab: provider, db: database, registrationIntents: repos,
			}
			ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})
			links, err := coordinator.LinkTaskChange(ctx, mcp.TaskChangeLinkRequest{
				TaskID: "task-registration-tx", URL: "https://gitlab.example.test/group/project/-/merge_requests/42",
				OperationID: "operation-1", AssociationMode: "observation",
			})
			count := 0
			require.NoError(t, database.Get(&count, `SELECT COUNT(*) FROM test_provider_associations`))
			if conflict {
				require.ErrorIs(t, err, tasksqlite.ErrTaskChangeRegistrationConflict)
				require.Zero(t, count, "provider association must roll back when its intent receipt conflicts")
				stored, getErr := repos.GetTaskChangeRegistrationIntent(context.Background(), "task-registration-tx", "prior-intent")
				require.NoError(t, getErr)
				require.Equal(t, "succeeded", stored.State)
				return
			}
			require.NoError(t, err)
			require.Len(t, links, 1)
			require.Equal(t, 1, count)
			var intentID string
			require.NoError(t, database.Get(&intentID, `SELECT id FROM task_change_registration_intents WHERE task_id = ? AND operation_id = ?`,
				"task-registration-tx", "operation-1"))
			stored, getErr := repos.GetTaskChangeRegistrationIntent(context.Background(), "task-registration-tx", intentID)
			require.NoError(t, getErr)
			require.NotNil(t, stored)
			require.Equal(t, "succeeded", stored.State)
			require.Equal(t, "operation-1", stored.OperationID)
			require.Equal(t, int64(1), *stored.AssociationRevision)
		})
	}
}

type transactionalRegistrationGitLab struct {
	db   *sqlx.DB
	last *gitlab.TaskMR
}

func (f *transactionalRegistrationGitLab) AssociateExistingMRByURL(context.Context, string, string, string, string) (*gitlab.TaskMR, error) {
	return nil, nil
}

func (f *transactionalRegistrationGitLab) RegisterExistingMRByURL(
	context.Context, string, string, string, string, bool, string,
) (*gitlab.TaskMR, error) {
	return nil, nil
}

func (f *transactionalRegistrationGitLab) RegisterExistingMRByURLWithCommit(
	ctx context.Context, workspaceID, taskID, repositoryID, mrURL string, restore bool, operationID string,
	commit gitlab.TaskMRObservationCommitter,
) (*gitlab.TaskMR, error) {
	mr := &gitlab.TaskMR{
		ID: "association-1", TaskID: taskID, RepositoryID: repositoryID, Host: "https://gitlab.example.test",
		ProjectPath: "group/project", RepositoryPath: "group/project", MRIID: 42, MRURL: mrURL,
		ProviderRepositoryID: "provider-42", ConnectionScope: "builtin:" + workspaceID,
		Source: gitlab.TaskMRSourceAgentRegistration, ComparisonPolicy: gitlab.TaskMRComparisonPolicyPreserve,
		AssociationRevision: 1, FirstOriginKind: "agent_registration", FirstOriginID: operationID,
	}
	registered, _, err := commit(ctx, mr, restore, 0)
	if err == nil {
		f.last = registered
	}
	return registered, err
}

func (f *transactionalRegistrationGitLab) RegisterTaskMRObservationTx(
	ctx context.Context, tx *sqlx.Tx, mr *gitlab.TaskMR, _ bool, _ int64,
) (*gitlab.TaskMR, bool, error) {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO test_provider_associations (id, task_id, repository_id, mr_iid) VALUES (?, ?, ?, ?)`),
		mr.ID, mr.TaskID, mr.RepositoryID, mr.MRIID); err != nil {
		return nil, false, err
	}
	return mr, true, nil
}

func (f *transactionalRegistrationGitLab) ListTaskMRsByTask(_ context.Context, taskID string) ([]*gitlab.TaskMR, error) {
	if f.last == nil || f.last.TaskID != taskID {
		return nil, nil
	}
	return []*gitlab.TaskMR{f.last}, nil
}

func (f *transactionalRegistrationGitLab) UnlinkTaskMR(context.Context, string, string) error {
	return nil
}
