package backendapp

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	userdto "github.com/kandev/kandev/internal/user/dto"
	usermodels "github.com/kandev/kandev/internal/user/models"
)

func TestBootTaskDetailSessionSelectionRequiresTaskMembership(t *testing.T) {
	task := &models.Task{ID: "task-1"}
	sessions := []*models.TaskSession{
		{ID: "session-primary", TaskID: "task-1", IsPrimary: true},
		{ID: "session-sibling", TaskID: "task-1"},
		{ID: "foreign-primary", TaskID: "task-2", IsPrimary: true},
	}

	if got := resolveTaskDetailSessionID(task, sessions, "session-sibling"); got != "session-sibling" {
		t.Fatalf("requested session = %q, want authorized sibling", got)
	}
	if got := resolveTaskDetailSessionID(task, sessions, "foreign-primary"); got != "session-primary" {
		t.Fatalf("foreign requested session = %q, want task primary fallback", got)
	}
	if got := resolveTaskDetailSessionID(task, sessions, ""); got != "session-primary" {
		t.Fatalf("default session = %q, want task primary", got)
	}
}

func TestTaskDetailSidebarQueryUsesWorkspaceDraftAndPreferences(t *testing.T) {
	settings := &userdto.UserSettingsDTO{
		SidebarViews: []usermodels.SidebarView{{ID: "global", Group: "none"}},
		SidebarViewsByWorkspace: map[string]usermodels.SidebarWorkspaceState{
			"workspace-1": {
				Views: []usermodels.SidebarView{{
					ID: "view-1", Group: "workflow", CollapsedGroups: []string{"workflow:done"},
					Filters: []usermodels.SidebarViewClause{{
						Dimension: "archived", Op: "is", Value: json.RawMessage("false"),
					}},
					Sort: usermodels.SidebarViewSort{Key: "state", Direction: "asc"},
				}},
				ActiveViewID: "view-1",
				Draft: &usermodels.SidebarViewDraft{
					BaseViewID: "view-1", Group: "repository",
					Filters: []usermodels.SidebarViewClause{{
						Dimension: "state", Op: "is", Value: json.RawMessage(`"in_progress"`),
					}},
					Sort: usermodels.SidebarViewSort{Key: "updatedAt", Direction: "desc"},
				},
			},
		},
		SidebarTaskPrefs: usermodels.SidebarTaskPrefs{
			PinnedTaskIDs: []string{"task-pinned"}, OrderedTaskIDs: []string{"task-ordered"},
			SubtaskOrderByParentID: map[string][]string{"task-parent": {"task-child"}},
		},
	}
	query, prefs := taskDetailSidebarQuery(settings, "workspace-1", "ja")
	if err := query.Validate(); err != nil {
		t.Fatalf("workspace sidebar query invalid: %v", err)
	}
	if query.Page != 1 || query.PageSize != 100 || query.Locale != "ja" || query.Group != "repository" {
		t.Fatalf("sidebar query scope = %#v", query)
	}
	if query.Sort.Key != "updatedAt" || query.Sort.Direction != "desc" || len(query.Filters) != 1 || query.Filters[0].Dimension != "state" {
		t.Fatalf("effective sidebar draft = %#v", query)
	}
	if len(query.CollapsedGroupKeys) != 1 || query.CollapsedGroupKeys[0] != "workflow:done" {
		t.Fatalf("collapsed groups = %#v", query.CollapsedGroupKeys)
	}
	if len(prefs.PinnedTaskIDs) != 1 || prefs.PinnedTaskIDs[0] != "task-pinned" || len(prefs.OrderedTaskIDs) != 1 {
		t.Fatalf("sidebar preferences = %#v", prefs)
	}
}

func TestBootTaskDetailSiblingRetainsPendingAction(t *testing.T) {
	fixture := newJourneyReadFixture(t, 1)
	taskID := fixture.taskIDs[0]
	primary := &models.TaskSession{ID: "boot-selected", TaskID: taskID, State: models.TaskSessionStateWaitingForInput, IsPrimary: true}
	sibling := &models.TaskSession{ID: "boot-hidden", TaskID: taskID, State: models.TaskSessionStateWaitingForInput}
	for _, session := range []*models.TaskSession{primary, sibling} {
		if err := fixture.taskRepo.CreateTaskSession(t.Context(), session); err != nil {
			t.Fatal(err)
		}
	}
	turn := &models.Turn{ID: "boot-hidden-turn", TaskID: taskID, TaskSessionID: sibling.ID}
	if err := fixture.taskRepo.CreateTurn(t.Context(), turn); err != nil {
		t.Fatal(err)
	}
	if err := fixture.taskRepo.CreateMessage(t.Context(), &models.Message{
		ID: "boot-hidden-permission", TaskID: taskID, TaskSessionID: sibling.ID, TurnID: turn.ID,
		AuthorType: models.MessageAuthorAgent, Type: models.MessageTypePermissionRequest,
		Content: "Permission required", Metadata: map[string]any{},
	}); err != nil {
		t.Fatal(err)
	}
	state := map[string]any{}
	builder := bootStateBuilder{p: fixture.params}
	builder.addTaskDetailSessionsState(t.Context(), state, taskID, []*models.TaskSession{primary, sibling}, primary, primary.ID, nil)
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		TaskSessions struct {
			Items map[string]struct {
				PendingAction *string `json:"pending_action"`
			}
		}
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	action := response.TaskSessions.Items[sibling.ID].PendingAction
	if action == nil || *action != string(models.TaskPendingActionPermission) {
		t.Fatalf("hidden sibling lost pending permission in boot: %s", raw)
	}
}
