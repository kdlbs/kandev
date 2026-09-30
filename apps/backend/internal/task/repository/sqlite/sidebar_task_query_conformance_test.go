package sqlite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type sidebarLocalFixtureTask struct {
	models.Task
	Summary       json.RawMessage `json:"summary"`
	ExecutorType  string          `json:"executor_type"`
	RepositoryIDs []string        `json:"repository_ids"`
}
type sidebarLocalFixtureCase struct {
	Name          string                            `json:"name"`
	Query         models.SidebarTaskViewQuery       `json:"query"`
	Preferences   models.SidebarTaskViewPreferences `json:"preferences"`
	IDs           []string                          `json:"expected_task_ids"`
	Groups        []string                          `json:"expected_group_keys"`
	Queues        map[string][2]int                 `json:"expected_queues"`
	Depths        map[string]int                    `json:"expected_depths"`
	SubtaskCounts map[string]int                    `json:"expected_subtask_counts"`
	Continuation  string                            `json:"expected_continuation"`
	TotalVisible  *int                              `json:"expected_total_visible"`
}

func TestSidebarLocalViewSQLiteConformance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "sidebar-local-conformance.json"))
	require.NoError(t, err)
	var fixture struct {
		Workflows []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			StepID string `json:"step_id"`
		} `json:"workflows"`
		Repositories []*models.Repository `json:"repositories"`
		Scenarios    []struct {
			Name  string                    `json:"name"`
			Tasks []sidebarLocalFixtureTask `json:"tasks"`
			Cases []sidebarLocalFixtureCase `json:"cases"`
		} `json:"scenarios"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	for _, scenario := range fixture.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			repo := newRepoForEntityTests(t)
			seedWorkspace(t, repo, "conformance")
			for _, workflow := range fixture.Workflows {
				require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{ID: workflow.ID, Name: workflow.Name, WorkspaceID: "conformance"}))
				seedCASWorkflowStep(t, repo, workflow.ID, workflow.StepID, 0)
			}
			for _, repository := range fixture.Repositories {
				copy := *repository
				copy.WorkspaceID = "conformance"
				require.NoError(t, repo.CreateRepository(t.Context(), &copy))
			}
			for _, row := range scenario.Tasks {
				task := row.Task
				task.ParentID = ""
				require.NoError(t, repo.CreateTask(t.Context(), &task))
			}
			for _, row := range scenario.Tasks {
				seedSidebarLocalProjection(t, repo, row)
			}
			for _, item := range scenario.Cases {
				t.Run(item.Name, func(t *testing.T) {
					page, err := repo.QuerySidebarTaskPage(t.Context(), "conformance", item.Query, item.Preferences)
					require.NoError(t, err)
					assertSidebarLocalConformance(t, page, item)
				})
			}
		})
	}
}

func seedSidebarLocalProjection(t *testing.T, repo *Repository, row sidebarLocalFixtureTask) {
	t.Helper()
	_, err := repo.db.Exec(`UPDATE tasks SET parent_id=?, state=?, created_at=?, updated_at=?, archived_at=?, position=?, queued_for_step_id=?, queued_at=?, wip_admitted=? WHERE id=?`,
		row.ParentID, row.State, row.CreatedAt, row.UpdatedAt, row.ArchivedAt, row.Position, row.QueuedForStepID, row.QueuedAt, row.WIPAdmitted, row.ID)
	require.NoError(t, err)
	for index, id := range row.RepositoryIDs {
		require.NoError(t, repo.CreateTaskRepository(t.Context(), &models.TaskRepository{ID: row.ID + "-" + id, TaskID: row.ID, RepositoryID: id, Position: index}))
	}
	if row.ExecutorType != "" {
		id := row.ID + "-executor"
		require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: id, Name: id, Type: models.ExecutorType(row.ExecutorType)}))
		require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{ID: row.ID + "-session", TaskID: row.ID, ExecutorID: id, State: models.TaskSessionStateCompleted}))
	}
	if len(row.Summary) > 0 {
		_, err := repo.db.Exec(`INSERT INTO task_status_summaries (task_id, workspace_id, revision, summary, updated_at) VALUES (?, ?, 1, ?, ?)`, row.ID, row.WorkspaceID, string(row.Summary), time.Now())
		require.NoError(t, err)
	}
	// Session insertion can update the task's activity projection; fixture task timestamps remain independent.
	_, err = repo.db.Exec(`UPDATE tasks SET updated_at=? WHERE id=?`, row.UpdatedAt, row.ID)
	require.NoError(t, err)
}

func assertSidebarLocalConformance(t *testing.T, page *models.SidebarTaskPageResult, expected sidebarLocalFixtureCase) {
	t.Helper()
	require.Equal(t, expected.IDs, sidebarTaskIDs(page.Tasks))
	groups := []string{}
	continuations := []string{}
	for _, entry := range page.Entries {
		if entry.Kind == "group" {
			groups = append(groups, entry.GroupKey)
		}
		if entry.Kind == "continuation" {
			continuations = append(continuations, entry.ParentID)
		}
		if queue, ok := expected.Queues[entry.TaskID]; ok {
			require.Equal(t, queue, [2]int{entry.WIPQueuePosition, entry.WIPQueueTotal})
		}
		if depth, ok := expected.Depths[entry.TaskID]; ok {
			require.Equal(t, depth, entry.Depth)
		}
		if count, ok := expected.SubtaskCounts[entry.TaskID]; ok {
			require.Equal(t, count, entry.SubtaskCount)
		}
	}
	if expected.Groups != nil {
		require.Equal(t, expected.Groups, groups)
	}
	if expected.Continuation != "" {
		require.Contains(t, continuations, expected.Continuation)
	}
	if expected.TotalVisible != nil {
		require.Equal(t, *expected.TotalVisible, page.TotalVisibleTasks)
	}
}
