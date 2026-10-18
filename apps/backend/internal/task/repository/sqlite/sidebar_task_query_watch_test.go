package sqlite

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type sidebarWatcherCase struct {
	name     string
	metadata map[string]interface{}
	issue    bool
	review   bool
}

func sidebarWatcherCases() []sidebarWatcherCase {
	cases := []sidebarWatcherCase{
		{"github-issue", map[string]interface{}{"issue_watch_id": "watch-1"}, true, false},
		{"github-review", map[string]interface{}{"review_watch_id": "watch-1"}, false, true},
		{"gitlab-issue", map[string]interface{}{"gitlab_issue_watch_id": "watch-1"}, true, false},
		{"gitlab-review", map[string]interface{}{"gitlab_review_watch_id": "watch-1"}, false, true},
		{"jira", map[string]interface{}{"jira_issue_watch_id": "watch-1"}, true, false},
		{"linear", map[string]interface{}{"linear_issue_watch_id": "watch-1"}, true, false},
		{"sentry", map[string]interface{}{"sentry_issue_watch_id": "watch-1"}, true, false},
		{"azure-work-item", map[string]interface{}{"azure_devops_work_item_watch_id": "watch-1"}, true, false},
		{"azure-pr", map[string]interface{}{"azure_devops_pr_watch_id": "watch-1"}, false, true},
		{"ordinary", nil, false, false},
		{"manual-links", map[string]interface{}{"issue_url": "https://example.test/issues/1", "pr_url": "https://example.test/pulls/2"}, false, false},
		{"unknown-keys", map[string]interface{}{"custom_issue_watch_id": "watch-1", "custom_review_watch_id": "watch-1"}, false, false},
		{"mixed", map[string]interface{}{"issue_watch_id": "", "gitlab_issue_watch_id": "watch-1", "review_watch_id": nil, "azure_devops_pr_watch_id": "watch-2"}, true, true},
		{"opaque-strings", map[string]interface{}{"issue_watch_id": " ", "review_watch_id": "null"}, true, true},
	}
	for index, value := range []interface{}{nil, "", "undefined", false, true, 0, 42, []string{"id"}, map[string]string{"id": "watch"}} {
		cases = append(cases, sidebarWatcherCase{
			name: fmt.Sprintf("invalid-%d", index),
			metadata: map[string]interface{}{
				"issue_watch_id": value, "review_watch_id": value,
				"gitlab_issue_watch_id": value, "gitlab_review_watch_id": value,
			},
		})
	}
	return cases
}

func TestQuerySidebarTaskPageWatcherTaskTypes(t *testing.T) {
	// @covers AC-UI-PROVIDER-WATCHER-TYPES-001.1 through .6
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			for _, item := range sidebarWatcherCases() {
				t.Run(item.name, func(t *testing.T) {
					workspace := "watch-" + item.name
					seedWorkspace(t, repo, workspace)
					require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
						ID: item.name, WorkspaceID: workspace, Title: item.name, Metadata: item.metadata,
					}))
					for dimension, expected := range map[string]bool{"isIssueWatch": item.issue, "isPRReview": item.review} {
						for _, negative := range []bool{false, true} {
							query := sidebarTaskQuery(1)
							op := "is"
							if negative {
								op = "is_not"
							}
							query.Filters = []models.SidebarTaskViewClause{{Dimension: dimension, Op: op, Value: json.RawMessage(`true`)}}
							page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, models.SidebarTaskViewPreferences{})
							require.NoError(t, err)
							want := []string{}
							if expected != negative {
								want = append(want, item.name)
							}
							require.Equal(t, want, sidebarTaskIDs(page.Tasks), "%s %s true", dimension, op)
							require.EqualValues(t, len(want), page.TotalTasks)
							require.EqualValues(t, len(want), page.TotalVisibleTasks)
						}
					}
					page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, sidebarTaskQuery(1), models.SidebarTaskViewPreferences{})
					require.NoError(t, err)
					require.Equal(t, []string{item.name}, sidebarTaskIDs(page.Tasks))
				})
			}
		})
	}
}

func TestSidebarWatchFieldsDialects(t *testing.T) {
	for _, driver := range []string{"sqlite3", "pgx"} {
		t.Run(driver, func(t *testing.T) {
			require.Empty(t, sidebarWatchFields(driver, sidebarBaseNeeds{}))
			for _, item := range []struct {
				needs sidebarBaseNeeds
				alias string
				keys  []string
			}{
				{sidebarBaseNeeds{issueWatch: true}, "is_issue_watch", []string{"issue_watch_id", "gitlab_issue_watch_id", "jira_issue_watch_id", "linear_issue_watch_id", "sentry_issue_watch_id", "azure_devops_work_item_watch_id"}},
				{sidebarBaseNeeds{reviewWatch: true}, "is_pr_review", []string{"review_watch_id", "gitlab_review_watch_id", "azure_devops_pr_watch_id"}},
			} {
				fields := sidebarWatchFields(driver, item.needs)
				require.Len(t, fields, 1)
				require.Contains(t, fields[0], " AS "+item.alias)
				for _, key := range item.keys {
					typeCheck := fmt.Sprintf("json_type(t.metadata, '$.%s') = 'text'", key)
					extract := fmt.Sprintf("json_extract(t.metadata, '$.%s')", key)
					if driver == "pgx" {
						typeCheck = fmt.Sprintf("jsonb_typeof(t.metadata::jsonb->'%s') = 'string'", key)
						extract = fmt.Sprintf("t.metadata::jsonb->>'%s'", key)
					}
					require.Contains(t, fields[0], typeCheck)
					require.Contains(t, fields[0], extract)
				}
			}
		})
	}
}
