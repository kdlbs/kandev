package sqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSidebarTreeActivityAndStateShareCompleteAncestors(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			ctx := t.Context()
			seedWorkspace(t, repo, "ancestors")
			base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			for _, row := range []struct {
				id, parent, state, primary string
				activity                   time.Duration
			}{
				{"root", "", "TODO", "", 0},
				{"middle", "root", "TODO", "", time.Minute},
				{"leaf", "middle", "COMPLETED", "RUNNING", 10 * time.Minute},
				{"peer", "", "IN_PROGRESS", "", 5 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(ctx, &models.Task{
					ID: row.id, WorkspaceID: "ancestors", Title: row.id, ParentID: row.parent,
					State: v1.TaskState(row.state), CreatedAt: base, UpdatedAt: base,
				}))
				summary := fmt.Sprintf(`{"last_activity_at":%q,"primary_session":{"state":%q}}`,
					base.Add(row.activity).Format(time.RFC3339Nano), row.primary)
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`INSERT INTO task_status_summaries
					(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
					row.id, "ancestors", summary, base)
				require.NoError(t, err)
			}
			query := sidebarTaskQuery(1)
			query.Group = "state"
			query.Sort = models.SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}
			query.PageSize = 2
			first, err := repo.QuerySidebarTaskPage(ctx, "ancestors", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"root", "middle"}, sidebarTaskIDs(first.Tasks))
			require.Equal(t, "IN_PROGRESS", first.Entries[0].GroupKey)
			for _, entry := range first.Entries {
				if entry.TaskID == "root" {
					require.Equal(t, 2, entry.SubtaskCount)
				}
			}
			query.Page = 2
			second, err := repo.QuerySidebarTaskPage(ctx, "ancestors", query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"leaf", "peer"}, sidebarTaskIDs(second.Tasks))
			require.Equal(t, 4, second.TotalVisibleTasks)
		})
	}
}

func TestSidebarTreeActivityKeepsNewerParentActivity(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			seedWorkspace(t, repo, "parent-activity")
			base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			for _, row := range []struct {
				id, parent string
				activity   time.Duration
			}{
				{"root", "", 10 * time.Minute},
				{"child", "root", time.Minute},
				{"peer", "", 5 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: row.id, WorkspaceID: "parent-activity", Title: row.id, ParentID: row.parent,
					State: "TODO", CreatedAt: base,
				}))
				_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`UPDATE tasks SET updated_at = ? WHERE id = ?`),
					base.Add(row.activity), row.id)
				require.NoError(t, err)
			}
			for _, group := range []string{"none", "state"} {
				query := sidebarTaskQuery(1)
				query.Group = group
				query.Sort = models.SidebarTaskViewSort{Key: "lastActivityAt", Direction: "desc"}
				page, err := repo.QuerySidebarTaskPage(t.Context(), "parent-activity", query, models.SidebarTaskViewPreferences{})
				require.NoError(t, err)
				require.Equal(t, []string{"root", "child", "peer"}, sidebarTaskIDs(page.Tasks), group)
			}
		})
	}
}

func TestSidebarComposableRunningAndActivitySortsBeforePaging(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "sort-chain"
			seedWorkspace(t, repo, workspace)
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			for _, row := range []struct {
				id, parent, primary string
				activity            time.Duration
			}{
				{"running-parent", "", "", 0},
				{"running-child", "running-parent", "RUNNING", 20 * time.Minute},
				{"running-root", "", "RUNNING", 10 * time.Minute},
				{"recent-waiting", "", "WAITING_FOR_INPUT", 30 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: row.id, WorkspaceID: workspace, Title: row.id, ParentID: row.parent,
					State: "TODO", CreatedAt: base, UpdatedAt: base,
				}))
				primary := ""
				if row.primary != "" {
					primary = fmt.Sprintf(`,"primary_session":{"state":%q}`, row.primary)
				}
				summary := fmt.Sprintf(`{"last_activity_at":%q%s}`,
					base.Add(row.activity).Format(time.RFC3339Nano), primary)
				_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_status_summaries
					(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
					row.id, workspace, summary, base)
				require.NoError(t, err)
			}
			query := sidebarTaskQuery(1)
			query.Group = "none"
			query.Sort = models.SidebarTaskViewSort{
				Key: "running", Direction: "desc",
				ThenBy: []models.SidebarTaskViewSortCriterion{{Key: "lastActivityAt", Direction: "desc"}},
			}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"running-parent", "running-child", "running-root", "recent-waiting"}, sidebarTaskIDs(page.Tasks))
		})
	}
}

func TestSidebarTreeStateKeepsDescendantsWhenFilteringPromotesARoot(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "state-forest"
			seedWorkspace(t, repo, workspace)
			for _, row := range []struct{ id, title, parent, state string }{
				{"root", "Excluded root", "", "TODO"},
				{"middle", "Middle", "root", "TODO"},
				{"leaf", "Leaf", "middle", "REVIEW"},
				{"peer", "Peer", "", "TODO"},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: row.id, WorkspaceID: workspace, Title: row.title,
					ParentID: row.parent, State: v1.TaskState(row.state),
				}))
			}
			query := sidebarTaskQuery(1)
			query.Group = "state"
			query.Sort = models.SidebarTaskViewSort{Key: "title", Direction: "asc"}
			query.Filters = []models.SidebarTaskViewClause{{
				Dimension: "titleMatch", Op: "not_matches", Value: []byte(`"Excluded root"`),
			}}
			page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, models.SidebarTaskViewPreferences{})
			require.NoError(t, err)
			require.Equal(t, []string{"peer", "middle", "leaf"}, sidebarTaskIDs(page.Tasks))
			require.Equal(t, 3, page.TotalVisibleTasks)
			for _, entry := range page.Entries {
				if entry.TaskID == "middle" {
					require.Equal(t, "REVIEW", entry.GroupKey)
					require.Equal(t, 1, entry.SubtaskCount)
					require.Zero(t, entry.Depth)
				}
			}
		})
	}
}

func TestSidebarUniformAndMixedTreeStates(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			for _, item := range []struct {
				name, primary, expected string
				parent, child           v1.TaskState
			}{
				{"uniform", "", "TODO", "TODO", "TODO"},
				{"completed-child", "", "TODO", "TODO", "COMPLETED"},
				{"completed-tree", "", "COMPLETED", "COMPLETED", "COMPLETED"},
				{"mixed-state", "", "REVIEW", "TODO", "REVIEW"},
				{"mixed-bucket", "WAITING_FOR_INPUT", "TODO", "BLOCKED", "TODO"},
				{"scheduling", "", "SCHEDULING", "TODO", "SCHEDULING"},
				{"active-primary", "RUNNING", "IN_PROGRESS", "TODO", "COMPLETED"},
			} {
				t.Run(item.name, func(t *testing.T) {
					seedWorkspace(t, repo, item.name)
					parentID, childID := item.name+"-parent", item.name+"-child"
					require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
						ID: parentID, WorkspaceID: item.name, Title: parentID, State: item.parent,
					}))
					require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
						ID: childID, WorkspaceID: item.name, Title: childID, ParentID: parentID, State: item.child,
					}))
					if item.primary != "" {
						_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind(`INSERT INTO task_status_summaries
							(task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`),
							childID, item.name, fmt.Sprintf(`{"primary_session":{"state":%q}}`, item.primary), time.Now())
						require.NoError(t, err)
					}
					query := sidebarTaskQuery(1)
					query.Group = "state"
					page, err := repo.QuerySidebarTaskPage(t.Context(), item.name, query, models.SidebarTaskViewPreferences{})
					require.NoError(t, err)
					require.Equal(t, []string{parentID, childID}, sidebarTaskIDs(page.Tasks))
					require.Equal(t, item.expected, page.Entries[0].GroupKey)
				})
			}
		})
	}
}

func TestSidebarPostgresJITSettingIsTransactionLocal(t *testing.T) {
	repo := newRepoForSidebarConformance(t, "postgres")
	_, err := repo.db.ExecContext(t.Context(), "SET jit = on")
	require.NoError(t, err)
	snapshot, err := beginSidebarQuerySnapshot(t.Context(), repo.ro)
	require.NoError(t, err)
	defer snapshot.close()
	_, _, err = snapshot.prepare(t.Context(), repo.ro.DriverName(), "workspace", sidebarTaskQuery(1))
	require.NoError(t, err)
	var setting string
	require.NoError(t, snapshot.tx.QueryRowContext(t.Context(), "SHOW jit").Scan(&setting))
	require.Equal(t, "off", setting)
	require.NoError(t, snapshot.tx.Rollback())
	require.NoError(t, snapshot.conn.QueryRowContext(t.Context(), "SHOW jit").Scan(&setting))
	require.Equal(t, "on", setting)
}
