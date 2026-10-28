package backendapp

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/models"
)

func TestMCPAssignmentOwnerRejectsArchivedTask(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	if err != nil || len(workspaces) == 0 {
		t.Fatalf("workspaces: %v", err)
	}
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, false)
	if err != nil || len(workflows) == 0 {
		t.Fatalf("workflows: %v", err)
	}
	now := time.Now().UTC()
	task := &models.Task{ID: "mcp-archived-owner", Title: "Archived", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID, ArchivedAt: &now}
	if err := harness.taskRepo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := harness.taskRepo.ArchiveTask(ctx, task.ID); err != nil {
		t.Fatal(err)
	}
	validator := mcpSelectionAdditionValidator(routeParams{taskSvc: harness.taskSvc})
	if err := validator(ctx, mcpconfig.SelectionScopeTask, task.WorkspaceID, task.ID); err == nil {
		t.Fatal("archived owner is eligible")
	}
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.2
func TestMCPAssignmentSessionAdmission(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	repo, cleanup, err := settingsstore.Provide(harness.db, harness.db, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	require.NoError(t, repo.CreateAgent(ctx, &settingsmodels.Agent{ID: "assignment-agent", Name: "assignment-agent", SupportsMCP: true}))
	profile := &settingsmodels.AgentProfile{ID: "assignment-profile", AgentID: "assignment-agent", Name: "Profile", Enabled: true}
	require.NoError(t, repo.CreateAgentProfile(ctx, profile))
	task := &models.Task{ID: "assignment-task", Title: "Task", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID}
	require.NoError(t, harness.taskRepo.CreateTask(ctx, task))
	validator := mcpSelectionAdditionValidator(routeParams{taskSvc: harness.taskSvc, agentSettingsRepo: repo})
	for _, tc := range []struct {
		name           string
		state          models.TaskSessionState
		passthrough    bool
		disabled       bool
		missingProfile bool
		wantError      bool
	}{
		{name: "running", state: models.TaskSessionStateRunning},
		{name: "idle", state: models.TaskSessionStateWaitingForInput},
		{name: "completed", state: models.TaskSessionStateCompleted, wantError: true},
		{name: "failed", state: models.TaskSessionStateFailed, wantError: true},
		{name: "cancelled", state: models.TaskSessionStateCancelled, wantError: true},
		{name: "passthrough", state: models.TaskSessionStateRunning, passthrough: true, wantError: true},
		{name: "disabled-profile", state: models.TaskSessionStateRunning, disabled: true, wantError: true},
		{name: "removed-profile", state: models.TaskSessionStateRunning, missingProfile: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, !tc.disabled)
			require.NoError(t, err)
			profileID := profile.ID
			if tc.missingProfile {
				profileID = "missing"
			}
			session := &models.TaskSession{ID: "assignment-" + tc.name, TaskID: task.ID, AgentProfileID: profileID, State: tc.state, IsPassthrough: tc.passthrough}
			require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, session))
			err = validator(ctx, mcpconfig.SelectionScopeTaskSession, task.WorkspaceID, session.ID)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMCPInternalLaunchSelectionsPreserveIneligibleSession(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	workspaces, err := harness.taskSvc.ListWorkspaces(ctx)
	require.NoError(t, err)
	workflows, err := harness.taskSvc.ListWorkflows(ctx, workspaces[0].ID, false)
	require.NoError(t, err)
	settingsDB, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	settingsDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, settingsDB.Close()) })
	repo, cleanup, err := settingsstore.Provide(settingsDB, settingsDB, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, cleanup()) })
	task := &models.Task{ID: "launch-mcp-task", Title: "Task", WorkspaceID: workspaces[0].ID, WorkflowID: workflows[0].ID}
	require.NoError(t, harness.taskRepo.CreateTask(ctx, task))
	session := &models.TaskSession{ID: "launch-mcp-session", TaskID: task.ID, IsPassthrough: true, State: models.TaskSessionStateStarting}
	require.NoError(t, harness.taskRepo.CreateTaskSession(ctx, session))
	definition := &mcpconfig.MCPServerDefinition{ID: "launch-docs", WorkspaceID: task.WorkspaceID, RuntimeName: "docs", NormalizedRuntimeName: "docs", DisplayName: "Docs", Enabled: true, ExecutionMode: "remote", Transport: "http", Configuration: mcpconfig.MCPServerConfiguration{URL: "https://mcp.example.test/docs"}}
	require.NoError(t, repo.CreateMCPServerDefinition(ctx, definition))
	service := mcpconfig.NewSelectionService(repo, repo)
	params := routeParams{taskSvc: harness.taskSvc, agentSettingsRepo: repo}
	service.SetOwnerValidator(mcpSelectionOwnerValidator(params))
	service.SetAdditionValidator(mcpSelectionAdditionValidator(params))
	service.SetSessionMCPStateRepository(repo)
	origin := mcpconfig.SelectionOrigin{Scope: mcpconfig.SelectionScopeTaskSession, WorkspaceID: task.WorkspaceID, OwnerID: session.ID}
	_, err = service.ReplaceConditional(ctx, origin, []string{definition.ID}, mcpconfig.SelectionCondition{})
	require.ErrorIs(t, err, mcpconfig.ErrMCPSelectionOwnerAccess)
	require.NoError(t, service.Replace(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID, []string{definition.ID}))
	snapshot, err := service.ReadSnapshot(ctx, origin)
	require.NoError(t, err)
	require.Equal(t, []string{definition.ID}, snapshot.DefinitionIDs)
	require.Equal(t, int64(1), snapshot.State.DesiredRevision)
	require.Error(t, service.Replace(ctx, origin.Scope, "foreign-workspace", origin.OwnerID, []string{definition.ID}))
	require.NoError(t, service.Replace(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID, nil))
	definition.Enabled = false
	require.NoError(t, repo.UpdateMCPServerDefinition(ctx, definition, definition.Revision))
	require.ErrorIs(t, service.Replace(ctx, origin.Scope, origin.WorkspaceID, origin.OwnerID, []string{definition.ID}), mcpconfig.ErrMCPDefinitionDisabled)
}
