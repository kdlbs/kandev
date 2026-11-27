package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

const partialReadTaskID = "task-plan-safe-integration"

func newPartialReadMCPFixture(t *testing.T) (*Server, *service.PlanService, *recoveryIntegrationEventBus) {
	t.Helper()
	log := newTestLogger(t)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "partial-reads.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := sqlite.NewWithDB(database, database, log)
	require.NoError(t, err)
	seedSafeEditsIntegrationTask(t, repo)
	eventBus := &recoveryIntegrationEventBus{}
	svc := service.NewPlanService(repo, eventBus, log, 0)
	handlers := backendmcp.NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, svc, nil, nil, nil, log)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	server := New(NewDispatcherBackendClient(dispatcher, log), "integration-session", partialReadTaskID,
		10005, log, "", false, ModeTask)
	return server, svc, eventBus
}

func partialReadMetadata(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	var metadata map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(firstText(t, result), "Plan metadata:\n")), &metadata))
	return metadata
}

// @covers AC-TASKS-PLAN-READ-002.3
func TestPlanPartialReadMCPJourney(t *testing.T) {
	s, svc, eventBus := newPartialReadMCPFixture(t)
	ctx := context.Background()
	prefix := strings.Repeat("review detail 猫\r\n", 500)
	fragment := "- [ ] unique checkbox"
	original := prefix + fragment + "\r\nTRAILER-KEEP" + strings.Repeat("\n- [ ] duplicated", 2)
	_, err := svc.CreatePlan(ctx, service.CreatePlanRequest{TaskID: partialReadTaskID, Content: original})
	require.NoError(t, err)
	eventBus.ClearEvents()
	read := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{
		"offset": utf8.RuneCountInString(prefix), "limit": len(fragment),
	})
	require.Equal(t, fragment, planContentFromRead(t, read))
	require.NotContains(t, allText(t, read), "TRAILER-KEEP")
	require.Empty(t, eventBus.events)
	version := planVersionFromRead(t, read)
	edited := callTool(t, s, "edit_task_plan_kandev", map[string]interface{}{
		"expected_version": version, "old_text": fragment, "new_text": "- [x] unique checkbox",
	})
	require.False(t, edited.IsError)
	require.NotContains(t, allText(t, edited), prefix)
	head, err := svc.GetPlanSnapshot(ctx, partialReadTaskID)
	require.NoError(t, err)
	require.Equal(t, strings.Replace(original, fragment, "- [x] unique checkbox", 1), head.Content)
	eventBus.ClearEvents()
	stale := callTool(t, s, "edit_task_plan_kandev", map[string]interface{}{
		"expected_version": version, "old_text": "- [x] unique checkbox", "new_text": fragment,
	})
	require.True(t, stale.IsError)
	ambiguous := callTool(t, s, "edit_task_plan_kandev", map[string]interface{}{
		"expected_version": head.WriteVersion, "old_text": "- [ ] duplicated", "new_text": "done",
	})
	require.True(t, ambiguous.IsError)
	require.Contains(t, allText(t, ambiguous), "plan_edit_ambiguous")
	require.Empty(t, eventBus.events)
	full := callTool(t, s, "get_task_plan_kandev", nil)
	require.Equal(t, head.Content, planContentFromRead(t, full))
	require.NotContains(t, partialReadMetadata(t, full), "partial")
}

// @covers AC-TASKS-PLAN-READ-001.4 AC-TASKS-PLAN-READ-001.5 AC-TASKS-PLAN-READ-002.2
func TestPlanPartialReadMCPPaginationConflicts(t *testing.T) {
	s, svc, eventBus := newPartialReadMCPFixture(t)
	ctx := context.Background()
	content := "a猫😀\r\nSECRET-END"
	_, err := svc.CreatePlan(ctx, service.CreatePlanRequest{TaskID: partialReadTaskID, Content: content})
	require.NoError(t, err)
	first := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{"limit": 3})
	require.Equal(t, "a猫😀", planContentFromRead(t, first))
	metadata := partialReadMetadata(t, first)
	second := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{
		"offset": metadata["next_offset"], "limit": 8192, "expected_version": metadata["version"],
	})
	require.Equal(t, "\r\nSECRET-END", planContentFromRead(t, second))
	require.Nil(t, partialReadMetadata(t, second)["next_offset"])
	eof := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{"offset": utf8.RuneCountInString(content)})
	require.Empty(t, planContentFromRead(t, eof))
	require.Equal(t, false, partialReadMetadata(t, eof)["has_more"])
	overrun := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{"offset": 100})
	require.True(t, overrun.IsError)
	require.Contains(t, allText(t, overrun), "plan_read_offset_out_of_range")
	for _, change := range []string{"content", "title", "delete", "recreate"} {
		t.Run(change, func(t *testing.T) {
			read := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{"limit": 1})
			version := planVersionFromRead(t, read)
			head, getErr := svc.GetPlanSnapshot(ctx, partialReadTaskID)
			require.NoError(t, getErr)
			if change == "delete" || change == "recreate" {
				require.NoError(t, svc.DeletePlan(ctx, partialReadTaskID))
				if change == "recreate" {
					_, err = svc.CreatePlan(ctx, service.CreatePlanRequest{TaskID: partialReadTaskID, Content: content})
				}
			} else {
				updatedContent := head.Content
				if change == "content" {
					updatedContent += change
				}
				_, err = svc.UpdatePlan(ctx, service.UpdatePlanRequest{
					TaskID: partialReadTaskID, Title: change, Content: updatedContent,
				})
			}
			require.NoError(t, err)
			eventBus.ClearEvents()
			conflict := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{
				"offset": 1, "limit": 3, "expected_version": version,
			})
			require.True(t, conflict.IsError)
			require.Contains(t, allText(t, conflict), "plan_version_conflict")
			require.NotContains(t, allText(t, conflict), "SECRET-END")
			require.Empty(t, eventBus.events)
			if change == "delete" {
				missing := callTool(t, s, "get_task_plan_kandev", map[string]interface{}{"offset": 0})
				require.Contains(t, allText(t, missing), "No plan exists")
				_, err = svc.CreatePlan(ctx, service.CreatePlanRequest{TaskID: partialReadTaskID, Content: content})
				require.NoError(t, err)
			}
		})
	}
}

// @covers AC-TASKS-PLAN-READ-002.4
func TestPlanPartialReadMCPAuthorization(t *testing.T) {
	s, svc, _ := newPartialReadMCPFixture(t)
	created, err := svc.CreatePlan(context.Background(), service.CreatePlanRequest{
		TaskID: partialReadTaskID, Content: "PRIVATE-PLAN-BODY",
	})
	require.NoError(t, err)
	svc.SetTaskAuthorizer(func(context.Context, string) error { return errors.New("denied") })
	for _, args := range []map[string]interface{}{
		{}, {"offset": 0}, {"offset": 1000}, {"expected_version": "stale"},
	} {
		result := callTool(t, s, "get_task_plan_kandev", args)
		require.True(t, result.IsError)
		text := allText(t, result)
		for _, private := range []string{"PRIVATE-PLAN-BODY", created.Plan.WriteVersion, "next_offset", "total_characters"} {
			require.NotContains(t, text, private)
		}
	}
}
