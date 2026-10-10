package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.3, AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.4
func TestTaskGitHubIssueSQLiteValues(t *testing.T) {
	runTaskGitHubIssueValues(t, newRepoForEntityTests(t))
}
func TestTaskGitHubIssuePostgresValues(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runTaskGitHubIssueValues(t, newPostgresMetadataCASRepo(t, database))
}
func runTaskGitHubIssueValues(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "issue-values", Name: "Issue values"}))
	complex := `{"false":false,"zero":0,"empty":"","array":[],"large":9007199254740993,"nullable":null,"nested":{"large":9007199254740993,"nil":null,"array":[0,false,{},[]]},"literal.dot\"key":null,"deferred_launch":{"prompt":"current"},"step_handoff_carry":{"step":"current"},"handoff_source":{"task_id":"source"},"handoffs":[{"task_id":"child"}],"office_carrier_causation_depth":3,"workspace":{"mode":"shared_group","group_id":"group","other":null},"issue_watch_id":"watch","issue_author":"author","issue_repo":"acme/api"}`
	for variant, raw := range []interface{}{nil, "", "null", `{}`, complex, complex[:len(complex)-1] + `,"agent_title_pending":true,"agent_title_owner_session_id":"owner"}`} {
		for _, remove := range []bool{false, true} {
			t.Run(fmt.Sprintf("variant_%d_remove_%t", variant, remove), func(t *testing.T) {
				id := "value-task"
				require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "issue-values", Title: "Current", Priority: "high", Metadata: map[string]interface{}{"seed": true}}))
				before, err := repo.GetTask(ctx, id)
				require.NoError(t, err)
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), raw, id)
				require.NoError(t, err)
				var expected map[string]json.RawMessage
				if text, ok := raw.(string); ok && text != "" && text != "null" {
					require.NoError(t, json.Unmarshal([]byte(text), &expected))
				}
				if expected == nil {
					expected = make(map[string]json.RawMessage)
				}
				var link *models.TaskGitHubIssueLink
				if !remove {
					link = &models.TaskGitHubIssueLink{URL: "https://github.com/new/tools/issues/42", Number: 42, Owner: "new", Repo: "tools"}
				}
				returned, err := repo.UpdateTaskGitHubIssue(ctx, id, link)
				require.NoError(t, err)
				var encoded string
				require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata FROM tasks WHERE id=?`), id).Scan(&encoded))
				var current map[string]json.RawMessage
				require.NoError(t, json.Unmarshal([]byte(encoded), &current))
				assertStoredIssueKeys(t, current, remove)
				for _, key := range []string{"issue_url", "issue_number", "issue_owner", "issue_repo", "github_issue_linked"} {
					delete(current, key)
					delete(expected, key)
				}
				require.Equal(t, expected, current)
				stored, err := repo.GetTask(ctx, id)
				require.NoError(t, err)
				require.Equal(t, stored, returned)
				before.Metadata = stored.Metadata
				before.UpdatedAt = stored.UpdatedAt
				require.Equal(t, before, stored)
				require.NoError(t, repo.DeleteTask(ctx, id))
			})
		}
	}
}
func assertStoredIssueKeys(t *testing.T, current map[string]json.RawMessage, remove bool) {
	t.Helper()
	expected := map[string]string{"issue_url": `"https://github.com/new/tools/issues/42"`, "issue_number": "42", "issue_owner": `"new"`, "issue_repo": `"tools"`, "github_issue_linked": "true"}
	for key, value := range expected {
		if remove {
			require.NotContains(t, current, key)
		} else {
			require.JSONEq(t, value, string(current[key]))
		}
	}
}
