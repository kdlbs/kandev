package backendapp

import (
	"encoding/json"
	"testing"

	taskdto "github.com/kandev/kandev/internal/task/dto"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestMapKanbanStateIncludesWIPAdmissionFields(t *testing.T) {
	step := mapKanbanStepState(taskdto.WorkflowStepDTO{
		ID:             "step-review",
		Name:           "Review",
		WIPLimit:       1,
		PullFromStepID: "step-backlog",
	})
	if step["wip_limit"] != 1 || step["pull_from_step_id"] != "step-backlog" {
		t.Fatalf("kanban step WIP fields = %#v, want limit 1 and feeder step", step)
	}

	task := mapKanbanTaskState(taskdto.TaskDTO{
		ID:              "task-queued",
		WorkflowStepID:  "step-review",
		WIPAdmitted:     false,
		QueuedForStepID: "step-review",
	})
	if task["wipAdmitted"] != false || task["queuedForStepId"] != "step-review" {
		t.Fatalf("kanban task WIP fields = %#v, want queued task metadata", task)
	}
}

func TestMapKanbanStepStateIncludesProfileSessionPolicies(t *testing.T) {
	step := mapKanbanStepState(taskdto.WorkflowStepDTO{
		ID:                        "step-policy",
		ProfileSessionStartPolicy: "new",
		ProfileSessionEndPolicy:   "park",
	})
	if step["profile_session_start_policy"] != "new" || step["profile_session_end_policy"] != "park" {
		t.Fatalf("profile session policies = %#v/%#v, want new/park", step["profile_session_start_policy"], step["profile_session_end_policy"])
	}
}

// TestMapKanbanTaskStateIncludesAutoStartFailed regression-tests Review round
// 2's MAJOR finding: mapKanbanTaskState is a camelCase whitelist that omitted
// auto_start_failed, so a task whose auto-start already failed rendered with
// no badge until the WS path (now fixed separately) delivered an update — the
// very first paint was wrong for a task that failed before this boot payload
// was ever built.
func TestMapKanbanTaskStateIncludesAutoStartFailed(t *testing.T) {
	task := mapKanbanTaskState(taskdto.TaskDTO{
		ID:              "task-auto-start-failed",
		WorkflowStepID:  "step-review",
		AutoStartFailed: true,
	})
	if task["autoStartFailed"] != true {
		t.Fatalf("kanban task autoStartFailed = %#v, want true", task["autoStartFailed"])
	}

	cleared := mapKanbanTaskState(taskdto.TaskDTO{
		ID:             "task-auto-start-ok",
		WorkflowStepID: "step-review",
	})
	if cleared["autoStartFailed"] != false {
		t.Fatalf("kanban task autoStartFailed = %#v, want false for a task without the marker", cleared["autoStartFailed"])
	}
}

func TestMapUserSettingsStateIncludesAzureDevOpsBrowsePreferences(t *testing.T) {
	preferences := json.RawMessage(`{"workspace-1":{"mode":"board","filters":{"projectId":"project-2"},"board":{"teamId":"team-2","boardId":"board-2","focusedColumnId":"done"}}}`)
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{AzureDevOpsBrowsePreferences: preferences},
	}, "workspace-1")

	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal boot settings: %v", err)
	}
	var payload struct {
		Loaded      bool `json:"loaded"`
		Preferences map[string]struct {
			Mode    string `json:"mode"`
			Filters struct {
				ProjectID string `json:"projectId"`
			} `json:"filters"`
			Board struct {
				TeamID  string `json:"teamId"`
				BoardID string `json:"boardId"`
			} `json:"board"`
		} `json:"azureDevOpsBrowsePreferences"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode boot settings: %v", err)
	}

	preference := payload.Preferences["workspace-1"]
	if !payload.Loaded || preference.Mode != "board" || preference.Filters.ProjectID != "project-2" || preference.Board.TeamID != "team-2" || preference.Board.BoardID != "board-2" {
		t.Fatalf("Azure browse preferences missing from loaded boot settings: %s", encoded)
	}
}

func TestMapUserSettingsStateIncludesPortableTaskAndSidebarSettings(t *testing.T) {
	maxColumns := 3
	state := mapUserSettingsState(userdto.UserSettingsResponse{
		Settings: userdto.UserSettingsDTO{
			SidebarViews: []usermodels.SidebarView{{
				ID:   "view-1",
				Name: "My view",
				TaskRow: &usermodels.SidebarTaskRowPresentation{
					DetailsEnabled: true,
					DetailOrder:    []string{"repository", "relative_time"},
					VisibleDetails: []string{"repository"},
					Trailing:       "relative_time",
				},
			}},
			SidebarActiveViewID: "view-1",
			SidebarDraft: &usermodels.SidebarViewDraft{
				BaseViewID: "view-1",
				Group:      "repository",
				TaskRow: &usermodels.SidebarTaskRowPresentation{
					DetailsEnabled: false,
					DetailOrder:    []string{"relative_time", "repository", "pull_request_number"},
					VisibleDetails: []string{},
					Trailing:       "none",
				},
			},
			ThreadViews: []usermodels.ThreadView{{
				ID:         "thread-view-1",
				Name:       "Thread view",
				TaskScope:  usermodels.ThreadTaskScope{Mode: usermodels.ThreadTaskScopeSelected, TaskIDs: []string{"task-1"}},
				Filters:    []usermodels.ThreadViewClause{},
				Sort:       usermodels.ThreadViewSort{Key: "attention", Direction: "asc"},
				MaxColumns: &maxColumns,
			}},
			ThreadActiveViewID: "thread-view-1",
			ThreadViewDraft: &usermodels.ThreadViewDraft{
				BaseViewID: "thread-view-1",
				TaskScope:  usermodels.ThreadTaskScope{Mode: usermodels.ThreadTaskScopeAll, TaskIDs: []string{}},
				Filters:    []usermodels.ThreadViewClause{},
				Sort:       usermodels.ThreadViewSort{Key: "attention", Direction: "asc"},
			},
			SidebarTaskPrefs: usermodels.SidebarTaskPrefs{
				PinnedTaskIDs:          []string{"task-1"},
				OrderedTaskIDs:         []string{"task-2"},
				SubtaskOrderByParentID: map[string][]string{"task-1": {"task-3"}},
			},
			TaskCreateLastUsed: usermodels.TaskCreateLastUsed{
				RepositoryID:           "repo-1",
				Branch:                 "main",
				AgentProfileID:         "agent-1",
				ExecutorProfileID:      "executor-1",
				WorkflowIDsByWorkspace: map[string]string{"workspace-1": "workflow-1"},
			},
		},
	}, "workspace-1")

	if state["sidebarActiveViewId"] != "view-1" {
		t.Fatalf("sidebarActiveViewId = %#v, want view-1", state["sidebarActiveViewId"])
	}
	if state["threadActiveViewId"] != "thread-view-1" {
		t.Fatalf("threadActiveViewId = %#v, want thread-view-1", state["threadActiveViewId"])
	}
	threadViews, ok := state["threadViews"].([]map[string]any)
	if !ok || len(threadViews) != 1 {
		t.Fatalf("threadViews = %#v, want one mapped view", state["threadViews"])
	}
	threadScope, ok := threadViews[0]["taskScope"].(map[string]any)
	if !ok || threadScope["mode"] != "selected" {
		t.Fatalf("thread taskScope = %#v, want selected scope", threadViews[0]["taskScope"])
	}
	threadDraft, ok := state["threadViewDraft"].(map[string]any)
	if !ok || threadDraft["baseViewId"] != "thread-view-1" {
		t.Fatalf("threadViewDraft = %#v, want mapped draft", state["threadViewDraft"])
	}
	draft, ok := state["sidebarDraft"].(map[string]any)
	if !ok || draft["baseViewId"] != "view-1" || draft["group"] != "repository" {
		t.Fatalf("sidebarDraft = %#v, want mapped draft", state["sidebarDraft"])
	}
	view, ok := state["sidebarViews"].([]map[string]any)
	if !ok || len(view) != 1 {
		t.Fatalf("sidebarViews = %#v, want one mapped view", state["sidebarViews"])
	}
	viewTaskRow, ok := view[0]["taskRow"].(map[string]any)
	if !ok || viewTaskRow["trailing"] != "relative_time" {
		t.Fatalf("sidebarViews taskRow = %#v, want relative_time trailing", view[0]["taskRow"])
	}
	draftTaskRow, ok := draft["taskRow"].(map[string]any)
	if !ok || draftTaskRow["detailsEnabled"] != false || draftTaskRow["trailing"] != "none" {
		t.Fatalf("sidebarDraft taskRow = %#v, want disabled none presentation", draft["taskRow"])
	}
	prefs, ok := state["sidebarTaskPrefs"].(map[string]any)
	if !ok || len(prefs["pinnedTaskIds"].([]string)) != 1 {
		t.Fatalf("sidebarTaskPrefs = %#v, want mapped preferences", state["sidebarTaskPrefs"])
	}
	lastUsed, ok := state["taskCreateLastUsed"].(map[string]any)
	if !ok || lastUsed["repositoryId"] != "repo-1" || lastUsed["synced"] != true {
		t.Fatalf("taskCreateLastUsed = %#v, want mapped settings", state["taskCreateLastUsed"])
	}
	workflowIDs, ok := lastUsed["workflowIdsByWorkspace"].(map[string]string)
	if !ok || workflowIDs["workspace-1"] != "workflow-1" {
		t.Fatalf("workflowIdsByWorkspace = %#v, want workspace-1 mapping", lastUsed["workflowIdsByWorkspace"])
	}
}

func TestMapUserSettingsStateNormalizesNilSubtaskOrder(t *testing.T) {
	state := mapUserSettingsState(userdto.UserSettingsResponse{}, "workspace-1")
	prefs, ok := state["sidebarTaskPrefs"].(map[string]any)
	if !ok {
		t.Fatalf("sidebarTaskPrefs = %#v, want map[string]any", state["sidebarTaskPrefs"])
	}
	order, ok := prefs["subtaskOrderByParentId"].(map[string][]string)
	if !ok || order == nil || len(order) != 0 {
		t.Fatalf("subtaskOrderByParentId = %#v, want empty map", prefs["subtaskOrderByParentId"])
	}
}
