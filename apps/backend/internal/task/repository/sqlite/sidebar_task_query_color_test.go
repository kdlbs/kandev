package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	usermodels "github.com/kandev/kandev/internal/user/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func sidebarColorPreferencesForTest() models.SidebarTaskViewPreferences {
	return models.SidebarTaskViewPreferences{ColorSettings: &models.SidebarTaskColorSettings{
		ManualColors: map[string]*string{},
		Automation:   json.RawMessage(`{"enabled":false,"rules":[]}`),
	}}
}

func TestSidebarEffectiveColorRanksRootsBeforePagination(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			const workspace = "sidebar-color-rank"
			seedWorkspace(t, repo, workspace)
			base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			for _, row := range []struct {
				id, parent, state string
				updated           time.Duration
			}{
				{"red-root", "", "TODO", time.Minute},
				{"new-neutral", "", "TODO", 10 * time.Minute},
				{"auto-blue", "", "FAILED", 9 * time.Minute},
				{"parent", "", "TODO", 2 * time.Minute},
				{"red-child", "parent", "TODO", 20 * time.Minute},
			} {
				require.NoError(t, repo.CreateTask(t.Context(), &models.Task{
					ID: row.id, WorkspaceID: workspace, Title: row.id, ParentID: row.parent,
					State: v1.TaskState(row.state), CreatedAt: base, UpdatedAt: base.Add(row.updated),
				}))
			}
			for _, row := range []struct {
				id      string
				updated time.Duration
			}{
				{"red-root", time.Minute},
				{"new-neutral", 10 * time.Minute},
				{"auto-blue", 9 * time.Minute},
				{"parent", 2 * time.Minute},
				{"red-child", 20 * time.Minute},
			} {
				_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind("UPDATE tasks SET updated_at = ? WHERE id = ?"), base.Add(row.updated), row.id)
				require.NoError(t, err)
			}

			automation, err := json.Marshal(usermodels.SidebarTaskColorAutomation{
				Enabled: true,
				Rules: []usermodels.SidebarTaskColorRule{{
					ID: "failed-blue", Enabled: true,
					Condition: usermodels.SidebarTaskColorCondition{
						Dimension: usermodels.SidebarTaskColorDimensionTaskState,
						Value:     "FAILED", Label: "Failed",
					},
					Output: usermodels.SidebarTaskColorOutput{Kind: usermodels.SidebarTaskColorOutputFixed, Color: "blue"},
				}},
			})
			require.NoError(t, err)
			prefs := sidebarColorPreferencesForTest()
			prefs.ColorSettings = &models.SidebarTaskColorSettings{
				ManualColors: map[string]*string{
					"red-root":  strptr("red"),
					"auto-blue": strptr("red"),
					"red-child": strptr("red"),
				},
				Automation: automation,
			}
			query := sidebarTaskQuery(1)
			query.Sort = models.SidebarTaskViewSort{
				Key: "color", Color: "red", Direction: "desc",
				ThenBy: []models.SidebarTaskViewSortCriterion{{Key: "updatedAt", Direction: "desc"}},
			}
			query.PageSize = 1
			first, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, prefs)
			require.NoError(t, err)
			require.Equal(t, []string{"red-root"}, sidebarTaskIDs(first.Tasks), "the preferred row ranks ahead of newer uncolored rows before pagination")
			require.True(t, first.HasNext)

			query.PageSize = models.MaxSidebarTaskPageSize
			full, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, prefs)
			require.NoError(t, err)
			require.Equal(t, []string{"red-root", "new-neutral", "auto-blue", "parent", "red-child"}, sidebarTaskIDs(full.Tasks),
				"automatic blue overrides manual red, and a red child does not promote its parent")
		})
	}
}

func TestSidebarColorQueryKeyTracksOnlyColorSortSettings(t *testing.T) {
	colorQuery := sidebarTaskQuery(1)
	colorQuery.Sort = models.SidebarTaskViewSort{Key: "color", Color: "red", Direction: "desc"}
	one := sidebarColorPreferencesForTest()
	two := sidebarColorPreferencesForTest()
	one.ColorSettings.ManualColors["task"] = strptr("red")
	two.ColorSettings.ManualColors["task"] = strptr("blue")
	require.NotEqual(t, sidebarTaskQueryKey("workspace", colorQuery, one), sidebarTaskQueryKey("workspace", colorQuery, two))

	nonColorQuery := sidebarTaskQuery(1)
	require.Equal(t, sidebarTaskQueryKey("workspace", nonColorQuery, one), sidebarTaskQueryKey("workspace", nonColorQuery, two))
}

func TestSidebarColorRuleConditionsCompileEveryDimension(t *testing.T) {
	conditions := []usermodels.SidebarTaskColorCondition{
		{Dimension: "workflow_step", Value: map[string]any{"workspace_id": "ws", "step_id": "step"}},
		{Dimension: "workflow", Value: map[string]any{"workspace_id": "ws", "workflow_id": "flow"}},
		{Dimension: "repository", Value: map[string]any{"kind": "workspace", "workspace_id": "ws", "repository_id": "repo"}},
		{Dimension: "executor_profile", Value: "profile"},
		{Dimension: "task_state", Value: "FAILED"},
		{Dimension: "priority", Value: "high"},
		{Dimension: "origin", Value: "kanban"},
	}
	for _, driver := range []string{"sqlite3", "postgres"} {
		for _, condition := range conditions {
			t.Run(fmt.Sprintf("%s/%s", driver, condition.Dimension), func(t *testing.T) {
				expression, args, err := sidebarColorRuleCondition(driver, condition)
				require.NoError(t, err)
				require.NotEmpty(t, expression)
				require.NotEmpty(t, args)
			})
		}
	}
}

func TestSidebarColorConformanceFixture(t *testing.T) {
	type fixtureTask struct {
		WorkflowID                 string                 `json:"workflow_id"`
		WorkflowStepID             string                 `json:"workflow_step_id"`
		State                      string                 `json:"state"`
		Priority                   string                 `json:"priority"`
		Origin                     string                 `json:"origin"`
		Metadata                   map[string]interface{} `json:"metadata"`
		RepositoryLink             bool                   `json:"repository_link"`
		PrimarySessionProfileID    string                 `json:"primary_session_profile_id"`
		NonPrimarySessionProfileID string                 `json:"non_primary_session_profile_id"`
	}
	type fixtureStep struct {
		ID         string `json:"id"`
		WorkflowID string `json:"workflow_id"`
		Color      string `json:"color"`
	}
	type fixtureRepository struct {
		ID             string `json:"id"`
		LocalPath      string `json:"local_path"`
		Provider       string `json:"provider"`
		ProviderRepoID string `json:"provider_repo_id"`
		ProviderHost   string `json:"provider_host"`
		ProviderScope  string `json:"provider_scope"`
		ProviderOwner  string `json:"provider_owner"`
		RemoteURL      string `json:"remote_url"`
	}
	var fixture struct {
		Cases []struct {
			Name          string             `json:"name"`
			WorkspaceID   string             `json:"workspace_id"`
			Task          fixtureTask        `json:"task"`
			Steps         []fixtureStep      `json:"steps"`
			Repository    *fixtureRepository `json:"repository"`
			Automation    json.RawMessage    `json:"automation"`
			ManualColor   *string            `json:"manual_color"`
			Preferred     string             `json:"preferred_color"`
			ExpectedToken *string            `json:"expected_token"`
			ExpectedMatch bool               `json:"expected_match"`
		} `json:"cases"`
	}
	data, err := os.ReadFile(filepath.Join("..", "testdata", "sidebar-color-conformance.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &fixture))

	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			repo := newRepoForSidebarConformance(t, backend)
			for _, testCase := range fixture.Cases {
				t.Run(testCase.Name, func(t *testing.T) {
					workspace := testCase.WorkspaceID
					workflowID := func(id string) string {
						if id == "" {
							return ""
						}
						return workspace + "-" + id
					}
					stepID := func(id string) string {
						if id == "" {
							return ""
						}
						return workspace + "-" + id
					}
					seedWorkspace(t, repo, workspace)
					workflowIDs := map[string]struct{}{}
					if testCase.Task.WorkflowID != "" || len(testCase.Steps) > 0 {
						workflowIDs[workflowID("other-flow")] = struct{}{}
					}
					if testCase.Task.WorkflowID != "" {
						workflowIDs[workflowID(testCase.Task.WorkflowID)] = struct{}{}
					}
					for _, step := range testCase.Steps {
						workflowIDs[workflowID(step.WorkflowID)] = struct{}{}
					}
					for workflowID := range workflowIDs {
						require.NoError(t, repo.CreateWorkflow(t.Context(), &models.Workflow{
							ID: workflowID, WorkspaceID: workspace, Name: workflowID,
						}))
					}
					for index, step := range testCase.Steps {
						seedCASWorkflowStep(t, repo, workflowID(step.WorkflowID), stepID(step.ID), index)
						if step.Color != "" {
							_, err := repo.db.ExecContext(t.Context(), repo.db.Rebind("UPDATE workflow_steps SET color = ? WHERE id = ?"), step.Color, stepID(step.ID))
							require.NoError(t, err)
						}
					}
					if _, ok := workflowIDs[workflowID("other-flow")]; ok {
						seedCASWorkflowStep(t, repo, workflowID("other-flow"), stepID("other-step"), 0)
					}
					repositoryID := ""
					if testCase.Repository != nil {
						repositoryID = workspace + "-" + testCase.Repository.ID
						require.NoError(t, repo.CreateRepository(t.Context(), &models.Repository{
							ID: repositoryID, WorkspaceID: workspace,
							Name: repositoryID, LocalPath: testCase.Repository.LocalPath,
							Provider: testCase.Repository.Provider, ProviderRepoID: testCase.Repository.ProviderRepoID,
							ProviderHost: testCase.Repository.ProviderHost, ProviderScope: testCase.Repository.ProviderScope,
							ProviderOwner: testCase.Repository.ProviderOwner, RemoteURL: testCase.Repository.RemoteURL,
						}))
					}
					base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
					state := testCase.Task.State
					if state == "" {
						state = "TODO"
					}
					metadata := testCase.Task.Metadata
					if metadata == nil {
						metadata = map[string]interface{}{}
					}
					targetID := workspace + "-target"
					controlID := workspace + "-control"
					target := &models.Task{
						ID: targetID, WorkspaceID: workspace,
						WorkflowID: workflowID(testCase.Task.WorkflowID), WorkflowStepID: stepID(testCase.Task.WorkflowStepID),
						Title: "Target", State: v1.TaskState(state), Priority: testCase.Task.Priority,
						Origin: testCase.Task.Origin, Metadata: metadata,
						CreatedAt: base, UpdatedAt: base,
					}
					require.NoError(t, repo.CreateTask(t.Context(), target))
					if testCase.Task.RepositoryLink {
						require.NotNil(t, testCase.Repository)
						require.NoError(t, repo.CreateTaskRepository(t.Context(), &models.TaskRepository{
							ID: workspace + "-target-repo-link", TaskID: target.ID,
							RepositoryID: repositoryID,
						}))
					}
					for _, session := range []struct {
						id      string
						profile string
						primary bool
					}{
						{id: "primary", profile: testCase.Task.PrimarySessionProfileID, primary: true},
						{id: "secondary", profile: testCase.Task.NonPrimarySessionProfileID},
					} {
						if session.profile == "" {
							continue
						}
						require.NoError(t, repo.CreateTaskSession(t.Context(), &models.TaskSession{
							ID: workspace + "-" + session.id, TaskID: target.ID,
							ExecutorProfileID: session.profile, State: models.TaskSessionStateRunning,
							IsPrimary: session.primary,
						}))
					}
					control := &models.Task{
						ID: controlID, WorkspaceID: workspace, Title: "Control",
						State: "TODO", Priority: "low", Origin: "kanban",
						CreatedAt: base, UpdatedAt: base.Add(time.Hour),
					}
					if _, ok := workflowIDs[workflowID("other-flow")]; ok {
						control.WorkflowID, control.WorkflowStepID = workflowID("other-flow"), stepID("other-step")
					}
					require.NoError(t, repo.CreateTask(t.Context(), control))

					var automation usermodels.SidebarTaskColorAutomation
					require.NoError(t, json.Unmarshal(testCase.Automation, &automation))
					for index := range automation.Rules {
						rule := &automation.Rules[index]
						target, ok := rule.Condition.Value.(map[string]interface{})
						if !ok {
							continue
						}
						switch rule.Condition.Dimension {
						case usermodels.SidebarTaskColorDimensionWorkflowStep:
							if id, ok := target["step_id"].(string); ok {
								target["step_id"] = stepID(id)
							}
						case usermodels.SidebarTaskColorDimensionWorkflow:
							if id, ok := target["workflow_id"].(string); ok {
								target["workflow_id"] = workflowID(id)
							}
						case usermodels.SidebarTaskColorDimensionRepository:
							if id, ok := target["repository_id"].(string); ok {
								target["repository_id"] = workspace + "-" + id
							}
						}
					}
					automationJSON, err := json.Marshal(automation)
					require.NoError(t, err)
					manualColors := map[string]*string{}
					if testCase.ManualColor != nil {
						manualColors[target.ID] = testCase.ManualColor
					}
					prefs := models.SidebarTaskViewPreferences{ColorSettings: &models.SidebarTaskColorSettings{
						ManualColors: manualColors, Automation: automationJSON,
					}}
					query := sidebarTaskQuery(1)
					query.Sort = models.SidebarTaskViewSort{
						Key: "color", Color: testCase.Preferred, Direction: "desc",
					}
					page, err := repo.QuerySidebarTaskPage(t.Context(), workspace, query, prefs)
					require.NoError(t, err)
					ids := sidebarTaskIDs(page.Tasks)
					if testCase.ExpectedMatch {
						require.Equal(t, []string{targetID, controlID}, ids)
					} else {
						require.Equal(t, []string{controlID, targetID}, ids)
					}
					if testCase.ExpectedToken != nil {
						workflowOutput := false
						for _, rule := range automation.Rules {
							workflowOutput = workflowOutput || rule.Output.Kind == usermodels.SidebarTaskColorOutputWorkflowStep
						}
						if workflowOutput {
							stepColor := ""
							for _, step := range testCase.Steps {
								if step.ID == testCase.Task.WorkflowStepID {
									stepColor = step.Color
								}
							}
							expression := sidebarWorkflowColorToken(repo.ro.DriverName(), "'"+strings.ReplaceAll(stepColor, "'", "''")+"'")
							var token string
							err := repo.ro.QueryRowxContext(t.Context(), "SELECT "+expression).Scan(&token)
							require.NoError(t, err)
							require.Equal(t, *testCase.ExpectedToken, token)
						}
					}
				})
			}
		})
	}
}
