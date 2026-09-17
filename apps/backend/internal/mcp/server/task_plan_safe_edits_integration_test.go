package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

type recoveryIntegrationEventBus struct {
	bus.EventBus
	events []*bus.Event
}

func (b *recoveryIntegrationEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	b.events = append(b.events, event)
	return nil
}

func (b *recoveryIntegrationEventBus) ClearEvents() {
	b.events = nil
}

func TestPlanSafeEditsMCPJourney(t *testing.T) {
	log := newTestLogger(t)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "plan-safe-edits.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := sqlite.NewWithDB(database, database, log)
	require.NoError(t, err)
	seedSafeEditsIntegrationTask(t, repo)

	planService := service.NewPlanService(repo, nil, log, 0)
	backendHandlers := backendmcp.NewHandlers(
		nil, nil, nil, nil, nil, nil, nil, nil, planService, nil, nil, nil, log,
	)
	dispatcher := ws.NewDispatcher()
	backendHandlers.RegisterHandlers(dispatcher)
	backend := NewDispatcherBackendClient(dispatcher, log)
	server := New(backend, "integration-session", "task-plan-safe-integration", 10005, log, "", false, ModeTask)

	original := "# Review plan\n\n" + strings.Repeat("- [ ] preserve review detail\n", 120)
	created := callTool(t, server, "create_task_plan_kandev", map[string]interface{}{
		"content": original,
		"title":   "Review plan",
	})
	require.False(t, created.IsError)
	initialVersion := planVersionFromRead(t, callTool(t, server, "get_task_plan_kandev", nil))
	require.NotEmpty(t, initialVersion)

	appended := callTool(t, server, "update_task_plan_kandev", map[string]interface{}{
		"mode": "append", "content": "## Checklist\n\n- [ ] checkbox",
	})
	require.False(t, appended.IsError)
	currentVersion := planVersionFromRead(t, callTool(t, server, "get_task_plan_kandev", nil))
	require.NotEqual(t, initialVersion, currentVersion)

	rejected := callTool(t, server, "update_task_plan_kandev", map[string]interface{}{
		"content":          "- [ ] checkbox",
		"expected_version": currentVersion,
	})
	require.True(t, rejected.IsError)
	rejectedText := firstText(t, rejected)
	require.Contains(t, rejectedText, "plan_truncation_rejected")
	require.Contains(t, rejectedText, "write_applied=false")
	require.Contains(t, rejectedText, "next_action=")
	require.NotContains(t, strings.ToLower(rejectedText), "stop")

	edited := callTool(t, server, "edit_task_plan_kandev", map[string]interface{}{
		"expected_version": currentVersion,
		"old_text":         "- [ ] checkbox",
		"new_text":         "- [x] checkbox",
	})
	require.False(t, edited.IsError)
	currentVersion = planVersionFromRead(t, callTool(t, server, "get_task_plan_kandev", nil))
	readAfterEdit := callTool(t, server, "get_task_plan_kandev", nil)
	require.Equal(t, original[:len(original)-1]+"\n\n## Checklist\n\n- [x] checkbox", planContentFromRead(t, readAfterEdit))

	listed := callTool(t, server, "list_task_plan_revisions_kandev", map[string]interface{}{"limit": 10})
	require.False(t, listed.IsError)
	var listPayload struct {
		Revisions []struct {
			RevisionID     string `json:"revision_id"`
			RevisionNumber int    `json:"revision_number"`
		} `json:"revisions"`
	}
	require.NoError(t, json.Unmarshal([]byte(firstText(t, listed)), &listPayload))
	require.GreaterOrEqual(t, len(listPayload.Revisions), 3)
	sourceID := listPayload.Revisions[len(listPayload.Revisions)-1].RevisionID

	sourceRead := callTool(t, server, "get_task_plan_revision_kandev", map[string]interface{}{
		"revision_id": sourceID,
	})
	require.False(t, sourceRead.IsError)
	sourceVersion, sourceContent := revisionReadValues(t, sourceRead)
	require.Equal(t, original, sourceContent)

	restored := callTool(t, server, "restore_task_plan_revision_kandev", map[string]interface{}{
		"revision_id":               sourceID,
		"expected_version":          currentVersion,
		"expected_revision_version": sourceVersion,
	})
	require.False(t, restored.IsError)
	require.Contains(t, firstText(t, restored), "Plan restored successfully")
	require.Equal(t, original, planContentFromRead(t, callTool(t, server, "get_task_plan_kandev", nil)))
}

// TestPlanRecoveryMCPEnforcesTaskScopeAndAuthorization covers
// AC-TASKS-PLAN-SAFE-004.1, AC-TASKS-PLAN-SAFE-004.3, AC-TASKS-PLAN-SAFE-004.5,
// and AC-TASKS-PLAN-SAFE-004.6 through the real MCP dispatcher. History reads
// authorize before lookup, and a revision from another task is rejected even
// when the addressed task itself is authorized.
func TestPlanRecoveryMCPEnforcesTaskScopeAndAuthorization(t *testing.T) {
	log := newTestLogger(t)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "plan-recovery-scope.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := sqlite.NewWithDB(database, database, log)
	require.NoError(t, err)
	seedSafeEditsIntegrationTask(t, repo)
	seedSafeEditsIntegrationTaskWithID(t, repo, "task-plan-safe-foreign")

	eventBus := &recoveryIntegrationEventBus{}
	planService := service.NewPlanService(repo, eventBus, log, 0)
	ctx := context.Background()
	current, err := planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: "task-plan-safe-integration", Title: "Current", Content: "CURRENT-BODY",
		CreatedBy: "agent", ForceNewRevision: true,
	})
	require.NoError(t, err)
	foreign, err := planService.CreatePlan(ctx, service.CreatePlanRequest{
		TaskID: "task-plan-safe-foreign", Title: "Foreign", Content: "FOREIGN-SECRET",
		CreatedBy: "agent", ForceNewRevision: true,
	})
	require.NoError(t, err)
	currentRevision, err := planService.GetLatestRevision(ctx, "task-plan-safe-integration")
	require.NoError(t, err)
	foreignRevision, err := planService.GetLatestRevision(ctx, "task-plan-safe-foreign")
	require.NoError(t, err)
	foreignRevisionVersion := service.PlanRevisionVersion(foreignRevision)
	eventBus.ClearEvents()

	planService.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		if taskID != "task-plan-safe-integration" {
			return errors.New("task access denied")
		}
		return nil
	})
	backendHandlers := backendmcp.NewHandlers(
		nil, nil, nil, nil, nil, nil, nil, nil, planService, nil, nil, nil, log,
	)
	dispatcher := ws.NewDispatcher()
	backendHandlers.RegisterHandlers(dispatcher)
	backend := NewDispatcherBackendClient(dispatcher, log)
	server := New(backend, "integration-session", "task-plan-safe-integration", 10005, log, "", false, ModeTask)

	listed := callTool(t, server, "list_task_plan_revisions_kandev", nil)
	require.False(t, listed.IsError)
	readCurrent := callTool(t, server, "get_task_plan_revision_kandev", map[string]interface{}{
		"revision_id": currentRevision.ID,
	})
	require.False(t, readCurrent.IsError)
	_, currentContent := revisionReadValues(t, readCurrent)
	require.Equal(t, "CURRENT-BODY", currentContent)

	currentSourceVersion := service.PlanRevisionVersion(currentRevision)
	alreadyCurrent := callTool(t, server, "restore_task_plan_revision_kandev", map[string]interface{}{
		"revision_id":               currentRevision.ID,
		"expected_version":          current.Plan.WriteVersion,
		"expected_revision_version": currentSourceVersion,
	})
	require.False(t, alreadyCurrent.IsError)
	require.Contains(t, firstText(t, alreadyCurrent), "already_current")

	beforeCurrent, err := repo.GetTaskPlan(ctx, "task-plan-safe-integration")
	require.NoError(t, err)
	beforeCurrentHistory, err := repo.ListTaskPlanRevisions(ctx, "task-plan-safe-integration", 0)
	require.NoError(t, err)
	beforeForeign, err := repo.GetTaskPlan(ctx, "task-plan-safe-foreign")
	require.NoError(t, err)
	beforeForeignHistory, err := repo.ListTaskPlanRevisions(ctx, "task-plan-safe-foreign", 0)
	require.NoError(t, err)
	eventsBeforeRejections := len(eventBus.events)

	// The current task is authorized, but the selected revision belongs to a
	// different task. The response must not expose its body or metadata.
	crossTaskRead := callTool(t, server, "get_task_plan_revision_kandev", map[string]interface{}{
		"task_id": current.Plan.TaskID, "revision_id": foreignRevision.ID,
	})
	assertNoForeignRevisionDetails(t, crossTaskRead, foreignRevision, foreign.Plan)

	crossTaskRestore := callTool(t, server, "restore_task_plan_revision_kandev", map[string]interface{}{
		"task_id": current.Plan.TaskID, "revision_id": foreignRevision.ID,
		"expected_version": current.Plan.WriteVersion, "expected_revision_version": foreignRevisionVersion,
	})
	assertNoForeignRevisionDetails(t, crossTaskRestore, foreignRevision, foreign.Plan)

	// An unauthorized task is rejected before list, read, or restore lookup.
	deniedList := callTool(t, server, "list_task_plan_revisions_kandev", map[string]interface{}{
		"task_id": "task-plan-safe-foreign",
	})
	assertNoForeignRevisionDetails(t, deniedList, foreignRevision, foreign.Plan)

	deniedRead := callTool(t, server, "get_task_plan_revision_kandev", map[string]interface{}{
		"task_id": "task-plan-safe-foreign", "revision_id": foreignRevision.ID,
	})
	assertNoForeignRevisionDetails(t, deniedRead, foreignRevision, foreign.Plan)

	deniedRestore := callTool(t, server, "restore_task_plan_revision_kandev", map[string]interface{}{
		"task_id": "task-plan-safe-foreign", "revision_id": foreignRevision.ID,
		"expected_version": foreign.Plan.WriteVersion, "expected_revision_version": foreignRevisionVersion,
	})
	assertNoForeignRevisionDetails(t, deniedRestore, foreignRevision, foreign.Plan)

	afterCurrent, err := repo.GetTaskPlan(ctx, "task-plan-safe-integration")
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(afterCurrent, beforeCurrent), "cross-task rejections changed current HEAD")
	afterCurrentHistory, err := repo.ListTaskPlanRevisions(ctx, "task-plan-safe-integration", 0)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(afterCurrentHistory, beforeCurrentHistory), "cross-task rejections changed current history")
	afterForeign, err := repo.GetTaskPlan(ctx, "task-plan-safe-foreign")
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(afterForeign, beforeForeign), "unauthorized requests changed foreign HEAD")
	afterForeignHistory, err := repo.ListTaskPlanRevisions(ctx, "task-plan-safe-foreign", 0)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(afterForeignHistory, beforeForeignHistory), "unauthorized requests changed foreign history")
	require.Equal(t, eventsBeforeRejections, len(eventBus.events), "rejected recovery calls published events")
}

func seedSafeEditsIntegrationTask(t *testing.T, repo *sqlite.Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-plan-safe-integration", Name: "Plan safety"}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID: "wf-plan-safe-integration", WorkspaceID: "ws-plan-safe-integration", Name: "Plan safety workflow",
	}))
	seedSafeEditsIntegrationTaskWithID(t, repo, "task-plan-safe-integration")
}

func seedSafeEditsIntegrationTaskWithID(t *testing.T, repo *sqlite.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "ws-plan-safe-integration",
		WorkflowID: "wf-plan-safe-integration", Title: "Plan safety task",
		State: v1.TaskStateCreated, Priority: "medium", CreatedAt: now, UpdatedAt: now,
	}))
}

func firstText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func allText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	var out strings.Builder
	for _, content := range result.Content {
		text, ok := content.(mcp.TextContent)
		if ok {
			out.WriteString(text.Text)
		}
	}
	return out.String()
}

func assertNoForeignRevisionDetails(
	t *testing.T,
	result *mcp.CallToolResult,
	revision *models.TaskPlanRevision,
	plan *models.TaskPlan,
) {
	t.Helper()
	require.True(t, result.IsError)
	text := allText(t, result)
	for _, secret := range []string{
		revision.ID, revision.Title, revision.Content, service.PlanRevisionVersion(revision),
		plan.WriteVersion,
	} {
		require.NotContains(t, text, secret)
	}
}

func planVersionFromRead(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.False(t, result.IsError)
	require.Len(t, result.Content, 2)
	var metadata map[string]interface{}
	text := firstText(t, result)
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(text, "Plan metadata:\n")), &metadata))
	version, ok := metadata["version"].(string)
	require.True(t, ok)
	return version
}

func planContentFromRead(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	require.False(t, result.IsError)
	require.Len(t, result.Content, 2)
	text, ok := result.Content[1].(mcp.TextContent)
	require.True(t, ok)
	return text.Text
}

func revisionReadValues(t *testing.T, result *mcp.CallToolResult) (version, content string) {
	t.Helper()
	require.Len(t, result.Content, 2)
	metadataText := result.Content[0].(mcp.TextContent).Text
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(metadataText, "Revision metadata:\n")), &metadata))
	version, ok := metadata["revision_version"].(string)
	require.True(t, ok)
	content = result.Content[1].(mcp.TextContent).Text
	return version, content
}
