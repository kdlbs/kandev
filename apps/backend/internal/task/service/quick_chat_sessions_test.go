package service

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type quickChatFixture struct {
	id             string
	workspaceID    string
	workflowID     string
	title          string
	agentProfileID string
	origin         string
	metadata       map[string]interface{}
	updatedAt      time.Time
	withoutPrimary bool
}

func seedQuickChatTask(t *testing.T, svc *Service, sqlxDB *sqlx.DB, fixture quickChatFixture) {
	t.Helper()
	ctx := context.Background()
	workspaceID := fixture.workspaceID
	if workspaceID == "" {
		workspaceID = "ws-qc"
	}
	metadata := map[string]interface{}{}
	if fixture.agentProfileID != "" {
		metadata[models.MetaKeyAgentProfileID] = fixture.agentProfileID
	}
	for key, value := range fixture.metadata {
		metadata[key] = value
	}
	if err := svc.tasks.CreateTask(ctx, &models.Task{
		ID:          fixture.id,
		WorkspaceID: workspaceID,
		WorkflowID:  fixture.workflowID,
		Title:       fixture.title,
		State:       v1.TaskStateTODO,
		Priority:    "medium",
		IsEphemeral: true,
		Origin:      fixture.origin,
		Metadata:    metadata,
	}); err != nil {
		t.Fatalf("CreateTask(%s): %v", fixture.id, err)
	}
	// CreateTask stamps its own timestamps; backdate so ordering is deterministic.
	if _, err := sqlxDB.ExecContext(ctx,
		`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`,
		fixture.updatedAt.Add(-time.Hour), fixture.updatedAt, fixture.id,
	); err != nil {
		t.Fatalf("backdate task(%s): %v", fixture.id, err)
	}
	if fixture.withoutPrimary {
		return
	}
	if err := svc.sessions.CreateTaskSession(ctx, &models.TaskSession{
		ID:             fixture.id + "-session",
		TaskID:         fixture.id,
		AgentProfileID: fixture.agentProfileID,
		State:          models.TaskSessionStateCompleted,
		StartedAt:      fixture.updatedAt.Add(-time.Hour),
		UpdatedAt:      fixture.updatedAt,
		IsPrimary:      true,
	}); err != nil {
		t.Fatalf("CreateTaskSession(%s): %v", fixture.id, err)
	}
}

// TestListQuickChatSessionsIsWorkspaceScopedAndOrdered pins the contract the
// boot payload and the resync endpoint share: only restorable quick chats for
// the requested workspace, in a stable creation baseline. Both clients read
// this list, so activity cannot reorder tabs on different devices.
func TestListQuickChatSessionsIsWorkspaceScopedAndOrdered(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	ctx := context.Background()

	for _, ws := range []string{"ws-qc", "ws-other"} {
		if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: ws, Name: ws}); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", ws, err)
		}
	}
	if err := svc.workflows.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-qc", WorkspaceID: "ws-qc", Name: "Workflow",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}

	base := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-old", title: "Older Chat", agentProfileID: "agent-old",
		updatedAt: base.Add(-3 * time.Hour),
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-new", title: "Newer Chat", agentProfileID: "agent-new",
		updatedAt: base.Add(-time.Hour),
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-config", title: "Config", agentProfileID: "agent-config",
		metadata: map[string]interface{}{"config_mode": true}, updatedAt: base,
	})
	// Placeholder title is not a tab label.
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-unnamed", title: quickChatDefaultTitle, agentProfileID: "agent-unnamed",
		updatedAt: base.Add(-4 * time.Hour),
	})
	// Excluded: no primary session, other workspace, automation run, workflow-bound.
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-no-primary", title: "No Primary", updatedAt: base, withoutPrimary: true,
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-foreign", workspaceID: "ws-other", title: "Foreign", updatedAt: base,
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-automation", title: "Automation", updatedAt: base,
		origin: models.TaskOriginAutomationRun,
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-workflow", title: "Workflow Ephemeral", updatedAt: base, workflowID: "wf-qc",
	})

	items, err := svc.ListQuickChatSessions(ctx, "ws-qc")
	if err != nil {
		t.Fatalf("ListQuickChatSessions: %v", err)
	}

	gotIDs := make([]string, 0, len(items))
	for _, item := range items {
		gotIDs = append(gotIDs, item.SessionID)
	}
	wantIDs := []string{
		"task-unnamed-session", "task-old-session", "task-new-session", "task-config-session",
	}
	if len(gotIDs) != len(wantIDs) {
		t.Fatalf("session ids = %v, want %v", gotIDs, wantIDs)
	}
	for i, want := range wantIDs {
		if gotIDs[i] != want {
			t.Fatalf("session ids = %v, want %v", gotIDs, wantIDs)
		}
	}
	if items[3].Kind != QuickChatKindConfig || items[1].Kind != QuickChatKindChat {
		t.Fatalf("kinds = %q/%q, want config/chat", items[3].Kind, items[1].Kind)
	}
	if items[3].Name != "Config" || items[3].AgentProfileID != "agent-config" {
		t.Fatalf("config tab = %#v, want title and agent profile preserved", items[3])
	}
	if items[0].Name != "" {
		t.Fatalf("placeholder-titled tab name = %q, want empty", items[0].Name)
	}
	if items[3].WorkspaceID != "ws-qc" || items[3].Session == nil {
		t.Fatalf("tab payload = %#v, want workspace and session attached", items[3])
	}
}

// TestListQuickChatSessionsEmptyWorkspace guards the empty case: a client that
// resyncs into a workspace with no quick chats must get an empty list (which
// clears its stale tabs), not an error it would silently ignore.
func TestListQuickChatSessionsEmptyWorkspace(t *testing.T) {
	svc := createOfficeIntegrationService(t)
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-empty", Name: "Empty"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	items, err := svc.ListQuickChatSessions(ctx, "ws-empty")
	if err != nil {
		t.Fatalf("ListQuickChatSessions: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %#v, want empty", items)
	}
}

func TestIsRestorableQuickChatTask(t *testing.T) {
	tests := []struct {
		name string
		task *models.Task
		want bool
	}{
		{name: "nil", task: nil, want: false},
		{name: "ephemeral chat", task: &models.Task{IsEphemeral: true}, want: true},
		{name: "not ephemeral", task: &models.Task{}, want: false},
		{name: "workflow bound", task: &models.Task{IsEphemeral: true, WorkflowID: "wf"}, want: false},
		{
			name: "automation run",
			task: &models.Task{IsEphemeral: true, Origin: models.TaskOriginAutomationRun},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRestorableQuickChatTask(tt.task); got != tt.want {
				t.Fatalf("IsRestorableQuickChatTask() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestQuickChatRenamePublishesTabFields pins the cross-device rename contract:
// updating a quick chat's title must publish a task.updated carrying everything
// a client needs to re-label its tab. Clients restore quick-chat tabs from this
// payload, so dropping any of these fields silently strands renames (and new
// chats) on the device that made them.
func TestQuickChatRenamePublishesTabFields(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	ctx := context.Background()
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: "ws-qc", Name: "ws-qc"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "task-chat", title: "Claude - Chat 1", agentProfileID: "agent-1",
		updatedAt: time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC),
	})
	eventBus, ok := svc.eventBus.(*MockEventBus)
	if !ok {
		t.Fatalf("event bus = %T, want *MockEventBus", svc.eventBus)
	}
	eventBus.ClearEvents()

	renamed := "My renamed chat"
	if _, err := svc.UpdateTask(ctx, "task-chat", &UpdateTaskRequest{Title: &renamed}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	var updated map[string]interface{}
	for _, event := range eventBus.GetPublishedEvents() {
		data, dataOK := event.Data.(map[string]interface{})
		if !dataOK || data["task_id"] != "task-chat" {
			continue
		}
		if event.Type == events.TaskUpdated {
			updated = data
		}
	}
	if updated == nil {
		t.Fatalf("no task.updated published; events = %#v", eventBus.GetPublishedEvents())
	}
	if updated["title"] != renamed {
		t.Fatalf("title = %v, want %q", updated["title"], renamed)
	}
	if updated["is_ephemeral"] != true {
		t.Fatalf("is_ephemeral = %v, want true", updated["is_ephemeral"])
	}
	if updated["workspace_id"] != "ws-qc" {
		t.Fatalf("workspace_id = %v, want ws-qc", updated["workspace_id"])
	}
	if updated["primary_session_id"] != "task-chat-session" {
		t.Fatalf("primary_session_id = %v, want task-chat-session", updated["primary_session_id"])
	}
	if _, has := updated["origin"]; !has {
		t.Fatalf("origin missing; restorable-quick-chat filtering needs it")
	}
}
