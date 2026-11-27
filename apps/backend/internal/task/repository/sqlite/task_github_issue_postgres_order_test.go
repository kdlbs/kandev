package sqlite_test

import (
	"context"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.2, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3
func TestTaskGitHubIssuePostgresCommitOrders(t *testing.T) {
	for _, partner := range []string{"merge", "link", "unlink"} {
		for _, issueFirst := range []bool{false, true} {
			t.Run(partner+"_"+map[bool]string{false: "partner_first", true: "issue_first"}[issueFirst], func(t *testing.T) {
				a, b, third := newHierarchyPostgresRepoPair(t)
				c := tasksqlite.NewWithInitializedDB(third, third, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "orders-ws", Name: "Orders"}))
				require.NoError(t, a.CreateWorkspace(ctx, &models.Workspace{ID: "other-ws", Name: "Other"}))
				for _, task := range []*models.Task{{ID: "subject", WorkspaceID: "orders-ws", Title: "Subject", Metadata: map[string]interface{}{"keep": true}}, {ID: "sibling", WorkspaceID: "orders-ws", Title: "Sibling", Metadata: map[string]interface{}{"alpha": "sibling"}}, {ID: "other", WorkspaceID: "other-ws", Title: "Other", Metadata: map[string]interface{}{"alpha": "other"}}} {
					require.NoError(t, a.CreateTask(ctx, task))
				}
				holder, err := a.DB().BeginTx(ctx, nil)
				require.NoError(t, err)
				defer func() { _ = holder.Rollback() }()
				_, err = holder.ExecContext(ctx, `SELECT id FROM tasks WHERE id='subject' FOR UPDATE`)
				require.NoError(t, err)
				pids := [2]int{hierarchyBackendPID(t, b.DB()), hierarchyBackendPID(t, c.DB())}
				results := [2]chan error{make(chan error, 1), make(chan error, 1)}
				joined := [2]bool{}
				started := [2]bool{}
				defer func() {
					cancel()
					_ = holder.Rollback()
					for i := range results {
						if started[i] && !joined[i] {
							<-results[i]
						}
					}
				}()
				update := func(repo *tasksqlite.Repository, issue bool) error {
					if issue {
						_, err := repo.UpdateTaskGitHubIssue(ctx, "subject", &models.TaskGitHubIssueLink{URL: "https://github.com/new/tools/issues/42", Number: 42, Owner: "new", Repo: "tools"})
						return err
					}
					if partner == "link" {
						_, err := repo.UpdateTaskGitHubIssue(ctx, "subject", &models.TaskGitHubIssueLink{URL: "https://github.com/other/api/issues/99", Number: 99, Owner: "other", Repo: "api"})
						return err
					}
					if partner == "unlink" {
						_, err := repo.UpdateTaskGitHubIssue(ctx, "subject", nil)
						return err
					}
					return repo.MergeTaskMetadata(ctx, "subject", map[string]interface{}{"alpha": "accepted", "port_forwarding_enabled": true})
				}
				started[0] = true
				go func() { results[0] <- update(b, issueFirst) }()
				waitHierarchyPostgresLocks(t, holder, pids[0], 1, "first physical row wait")
				started[1] = true
				go func() { results[1] <- update(c, !issueFirst) }()
				require.Eventually(t, func() bool {
					var waits int
					err := holder.QueryRowContext(ctx, `SELECT count(*) FROM pg_locks WHERE NOT granted AND locktype='advisory' AND pid=$1`, pids[1]).Scan(&waits)
					return err == nil && waits == 1
				}, 10*time.Second, 10*time.Millisecond)
				t.Logf("actual physical-row blocker then shared metadata advisory wait: firstPID=%d secondPID=%d issueFirst=%t", pids[0], pids[1], issueFirst)
				var schema string
				require.NoError(t, holder.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schema))
				raw, err := internaldb.OpenPostgres(hierarchySchemaDSN(testutil.PostgresDSNFromEnv(t), schema), 1, 1)
				require.NoError(t, err)
				independentDB := sqlx.NewDb(raw, "pgx")
				t.Cleanup(func() { require.NoError(t, independentDB.Close()) })
				independent := tasksqlite.NewWithInitializedDB(independentDB, independentDB, nil)
				for _, id := range []string{"sibling", "other"} {
					_, err = independent.UpdateTaskGitHubIssue(ctx, id, &models.TaskGitHubIssueLink{URL: "independent", Number: 99, Owner: "separate", Repo: "isolated"})
					require.NoError(t, err)
				}
				require.NoError(t, holder.Commit())
				for i := range results {
					err = <-results[i]
					joined[i] = true
					require.NoError(t, err)
				}
				got, err := a.GetTask(ctx, "subject")
				require.NoError(t, err)
				require.Equal(t, true, got.Metadata["keep"])
				if partner == "merge" {
					require.Equal(t, "accepted", got.Metadata["alpha"])
					require.Equal(t, true, got.Metadata["port_forwarding_enabled"])
				}
				expected := &models.TaskGitHubIssueLink{URL: "https://github.com/new/tools/issues/42", Number: 42, Owner: "new", Repo: "tools"}
				if issueFirst && partner == "link" {
					expected = &models.TaskGitHubIssueLink{URL: "https://github.com/other/api/issues/99", Number: 99, Owner: "other", Repo: "api"}
				}
				if issueFirst && partner == "unlink" {
					expected = nil
				}
				for key := range map[string]interface{}{"issue_url": nil, "issue_number": nil, "issue_owner": nil, "issue_repo": nil, "github_issue_linked": nil} {
					if expected == nil {
						require.NotContains(t, got.Metadata, key)
						continue
					}
					value := map[string]interface{}{"issue_url": expected.URL, "issue_number": float64(expected.Number), "issue_owner": expected.Owner, "issue_repo": expected.Repo, "github_issue_linked": true}[key]
					require.Equal(t, value, got.Metadata[key])
				}
				for _, id := range []string{"sibling", "other"} {
					task, err := a.GetTask(ctx, id)
					require.NoError(t, err)
					require.Equal(t, id, task.Metadata["alpha"])
					require.Equal(t, float64(99), task.Metadata["issue_number"])
					require.Equal(t, "isolated", task.Metadata["issue_repo"])
				}
			})
		}
	}
}
