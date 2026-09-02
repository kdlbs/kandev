package handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// This file covers AC-9/AC-16's fail-closed guarantee for
// authorizeHandoffCall (handoff_task_handlers.go): every way the trusted
// caller identity can be absent, malformed, or unresolvable must refuse with
// Forbidden and create nothing, never fall through to a permission grant.

// --- no principal attached to the context at all ---

func TestHandleHandoffTask_NoPrincipalIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)

	resp, err := f.h.handleHandoffTask(context.Background(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}

// --- principal present but missing the caller task/session identity ---

func TestHandleHandoffTask_EmptyCallerTaskIDIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    "",
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	})

	resp, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}

func TestHandleHandoffTask_EmptyCallerSessionIDIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: "",
		Surface:         mcpprofile.SurfaceOfficeTask,
	})

	resp, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}

// --- the claimed session cannot be resolved, or resolves to a different task ---

func TestHandleHandoffTask_UnresolvableSessionIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    f.sourceTaskID,
		CallerSessionID: "session-does-not-exist",
		Surface:         mcpprofile.SurfaceOfficeTask,
	})

	resp, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}

func TestHandleHandoffTask_SessionTaskIDMismatchIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)
	// f.sourceSessionID genuinely resolves, but to f.sourceTaskID, not the
	// task_id this principal claims to be calling from.
	ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{
		WorkspaceID:     f.sourceWorkspaceID,
		CallerTaskID:    "task-a-different-task-entirely",
		CallerSessionID: f.sourceSessionID,
		Surface:         mcpprofile.SurfaceOfficeTask,
	})

	resp, err := f.h.handleHandoffTask(ctx, makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}

// --- the permission lookup itself fails (not merely resolves to false) ---

func TestHandleHandoffTask_PermissionLookupFailureIsForbidden(t *testing.T) {
	f := newHandoffFixture(t, agentsettingsmodels.AgentRoleCEO, "", nil)
	before := countTasksInWorkspace(t, f.svc, f.targetWorkspaceID)
	_, err := f.agentDB.Exec("DROP TABLE agent_profiles")
	require.NoError(t, err, "precondition: must be able to break the agent_profiles table out from under the fixture")

	resp, err := f.h.handleHandoffTask(f.ctx(), makeWSMessage(t, ws.ActionMCPHandoffTask, f.validPayload(nil)))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeForbidden)
	require.Equal(t, before, countTasksInWorkspace(t, f.svc, f.targetWorkspaceID), "no task should have been created")
}
