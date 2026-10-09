package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/automation"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type automationScopeFixture struct{}

func (automationScopeFixture) GetTask(context.Context, string) (*models.Task, error) {
	return &models.Task{ID: "config-task", WorkspaceID: "ws"}, nil
}
func (automationScopeFixture) GetWorkspace(context.Context, string) (*models.Workspace, error) {
	return &models.Workspace{ID: "ws", OwnerID: "owner"}, nil
}
func (automationScopeFixture) IdentityForUser(context.Context, string) (authn.Identity, bool) {
	return authn.Identity{UserID: "owner", Role: authn.RoleMember}, true
}
func (automationScopeFixture) WorkflowWorkspaceID(context.Context, string) (string, error) {
	return "foreign", nil
}
func (automationScopeFixture) GetRepository(context.Context, string) (string, string, bool) {
	return "foreign", "main", true
}

type automationDispatchBackend struct {
	dispatcher *ws.Dispatcher
	resolver   *mcpscope.Resolver
}

func (b *automationDispatchBackend) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	scoped, err := b.resolver.Scope(ctx, "config-task")
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	response, err := b.dispatcher.Dispatch(scoped, &ws.Message{ID: "create", Type: ws.MessageTypeRequest, Action: action, Payload: raw})
	if err != nil {
		return err
	}
	if response.Type == ws.MessageTypeError {
		var failure ws.ErrorPayload
		if err := json.Unmarshal(response.Payload, &failure); err != nil {
			return err
		}
		return fmt.Errorf("%s", failure.Message)
	}
	return json.Unmarshal(response.Payload, result)
}

func newAutomationIntegration(t *testing.T) (*Server, *automation.Service) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(1)
	store, err := automation.NewStore(db, db)
	require.NoError(t, err)
	log := newTestLogger(t)
	svc := automation.NewService(store, nil, log)
	svc.SetWorkspaceAuthorizer(func(ctx context.Context, workspaceID string) error {
		identity, ok := authn.IdentityFromContext(ctx)
		if !ok || identity.UserID != "owner" || workspaceID != "ws" {
			return fmt.Errorf("workspace access denied")
		}
		return nil
	})
	svc.SetWorkflowLocator(automationScopeFixture{})
	svc.SetRepositoryLookup(automationScopeFixture{})
	h := mcphandlers.NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	h.SetAutomationCreator(svc)
	dispatcher := ws.NewDispatcher()
	h.RegisterHandlers(dispatcher)
	resolver := mcpscope.NewResolver(automationScopeFixture{}, automationScopeFixture{}, func() bool { return true }, log)
	return newTestServer(t, &automationDispatchBackend{dispatcher: dispatcher, resolver: resolver}), svc
}

// @covers AC-OFFICE-CONFIG-AUTOMATION-001.2
// @covers AC-OFFICE-CONFIG-AUTOMATION-001.4
func TestConfigAutomationMCPCreateAndRead(t *testing.T) {
	s, svc := newAutomationIntegration(t)
	result := callTool(t, s, "create_automation_kandev", map[string]interface{}{
		"workspace_id": "ws", "name": "Daily report", "prompt": "Summarize progress",
		"triggers": []interface{}{map[string]interface{}{"type": "scheduled", "config": map[string]interface{}{"cron_expression": "0 9 * * *", "timezone": "UTC"}, "enabled": true}},
	})
	require.False(t, result.IsError, "%+v", result.Content)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	saved, err := svc.ListAutomations(ctx, "ws")
	require.NoError(t, err)
	require.Len(t, saved, 1)
	a, err := svc.GetAutomation(ctx, saved[0].ID)
	require.NoError(t, err)
	require.NotNil(t, a)
	require.Len(t, result.Content, 1)
	text, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	var response map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(text.Text), &response))
	assert.Equal(t, a.ID, response["id"])
	assert.Equal(t, a.WebhookSecret, response["webhook_secret"])

	assert.Equal(t, "Daily report", a.Name)
	assert.Equal(t, "Summarize progress", a.Prompt)
	assert.Equal(t, automation.TaskModeAutomationRun, a.TaskMode)
	assert.Equal(t, automation.ContinuationPolicyNewTask, a.ContinuationPolicy)
	assert.Equal(t, 1, a.MaxConcurrentRuns)
	assert.True(t, a.Enabled)
	assert.Empty(t, a.Repositories)
	require.Len(t, a.Triggers, 1)
	assert.True(t, a.Triggers[0].Enabled)
	assert.JSONEq(t, `{"cron_expression":"0 9 * * *","timezone":"UTC"}`, string(a.Triggers[0].Config))
	runs, err := svc.Store().ListRuns(ctx, a.ID, 10)
	require.NoError(t, err)
	assert.Empty(t, runs)
	raw, err := json.Marshal(a)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "webhook_secret")
}

// @covers AC-OFFICE-CONFIG-AUTOMATION-001.3
func TestConfigAutomationMCPRejectsInvalidCreation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		fields map[string]interface{}
	}{
		{"foreign workspace", map[string]interface{}{"workspace_id": "foreign"}},
		{"foreign workflow", map[string]interface{}{"workflow_id": "foreign"}},
		{"foreign repository", map[string]interface{}{"repository_ids": []interface{}{"foreign"}}},
		{"missing workflow", map[string]interface{}{"task_mode": "normal_task"}},
		{"invalid cron", map[string]interface{}{"triggers": []interface{}{map[string]interface{}{"type": "scheduled", "config": map[string]interface{}{"cron_expression": "invalid"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, svc := newAutomationIntegration(t)
			args := map[string]interface{}{"workspace_id": "ws", "name": "Daily"}
			for k, v := range tc.fields {
				args[k] = v
			}
			result := callTool(t, s, "create_automation_kandev", args)
			require.True(t, result.IsError)
			saved, err := svc.Store().ListAutomations(context.Background(), "ws")
			require.NoError(t, err)
			assert.Empty(t, saved)
			foreign, err := svc.Store().ListAutomations(context.Background(), "foreign")
			require.NoError(t, err)
			assert.Empty(t, foreign)
		})
	}
}

func TestConfigAutomationMCPOmittedTriggerEnablement(t *testing.T) {
	s, svc := newAutomationIntegration(t)
	result := callTool(t, s, "create_automation_kandev", map[string]interface{}{
		"workspace_id": "ws", "name": "Inactive schedule",
		"triggers": []interface{}{map[string]interface{}{"type": "scheduled", "config": map[string]interface{}{"cron_expression": "0 9 * * *"}}},
	})
	require.False(t, result.IsError)
	saved, err := svc.Store().ListAutomations(context.Background(), "ws")
	require.NoError(t, err)
	require.Len(t, saved, 1)
	a, err := svc.Store().GetAutomation(context.Background(), saved[0].ID)
	require.NoError(t, err)
	require.Len(t, a.Triggers, 1)
	assert.True(t, a.Enabled)
	assert.False(t, a.Triggers[0].Enabled)
}
