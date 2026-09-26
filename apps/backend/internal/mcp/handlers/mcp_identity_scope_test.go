package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// In-session MCP tool calls arrive over the agent's WebSocket stream, which
// carries no credential of its own. Before the stream dispatch was scoped to
// the task owner, the task service saw "no identity" — its signal for an
// internal caller — and granted unscoped access, so an agent that supplied
// another user's task_id/workflow_id was served their data.
//
// These tests drive the same handler→service path production uses, with the
// stream context scoped by mcpscope.Resolver the way the lifecycle stream
// manager scopes it.

type scopedOwner struct {
	userID      string
	workspaceID string
	workflowID  string
	taskID      string
	sessionID   string
}

// stubIdentityLookup stands in for *auth.Service. The real implementation is
// covered by TestIdentityForUser* in internal/auth.
type stubIdentityLookup map[string]authn.Identity

func (s stubIdentityLookup) IdentityForUser(_ context.Context, userID string) (authn.Identity, bool) {
	identity, ok := s[userID]
	return identity, ok
}

func seedOwnedTask(t *testing.T, svc *service.Service, repo *sqliterepo.Repository, suffix, ownerID string) scopedOwner {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	owned := scopedOwner{
		userID:      ownerID,
		workspaceID: "ws-" + suffix,
		workflowID:  "wf-" + suffix,
		taskID:      "task-" + suffix,
		sessionID:   "session-" + suffix,
	}
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{
		ID:        owned.workspaceID,
		Name:      "Workspace " + suffix,
		OwnerID:   ownerID,
		CreatedAt: now,
		UpdatedAt: now,
	}))
	require.NoError(t, repo.CreateWorkflow(ctx, &models.Workflow{
		ID:          owned.workflowID,
		WorkspaceID: owned.workspaceID,
		Name:        "Board " + suffix,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          owned.taskID,
		WorkspaceID: owned.workspaceID,
		WorkflowID:  owned.workflowID,
		Title:       "Task " + suffix,
		State:       v1.TaskStateInProgress,
		CreatedAt:   now,
		UpdatedAt:   now,
	}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:        owned.sessionID,
		TaskID:    owned.taskID,
		IsPrimary: true,
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
	}))
	_, err := svc.CreateMessage(ctx, &service.CreateMessageRequest{
		TaskSessionID: owned.sessionID,
		TaskID:        owned.taskID,
		AuthorType:    "user",
		Content:       "secret plan for " + suffix,
	})
	require.NoError(t, err)
	return owned
}

// scopeFixture wires the production dispatch chain: MCP handlers registered on
// a real dispatcher, reached through a stream context scoped to one task's
// owner.
type scopeFixture struct {
	dispatcher *ws.Dispatcher
	scope      func(ctx context.Context, taskID string) (context.Context, error)
	userA      scopedOwner
	userB      scopedOwner
	// unowned is a workspace with an empty owner_id — created before auth was
	// enabled, or since by an internal (unscoped) caller, because
	// CreateWorkspace only stamps an owner for scoped callers.
	unowned scopedOwner
}

func newScopeFixture(t *testing.T, authEnabled bool) *scopeFixture {
	t.Helper()
	return newScopeFixtureWith(t, authEnabled, stubIdentityLookup{
		"user-a": {UserID: "user-a", Role: authn.RoleMember},
		"user-b": {UserID: "user-b", Role: authn.RoleAdmin},
	})
}

// newScopeFixtureWithoutAccounts models both owners having been disabled or
// deleted while their agents kept running.
func newScopeFixtureWithoutAccounts(t *testing.T) *scopeFixture {
	t.Helper()
	return newScopeFixtureWith(t, true, stubIdentityLookup{})
}

func newScopeFixtureWith(t *testing.T, authEnabled bool, identities stubIdentityLookup) *scopeFixture {
	t.Helper()
	svc, repo := newTestTaskService(t)
	userA := seedOwnedTask(t, svc, repo, "a", "user-a")
	userB := seedOwnedTask(t, svc, repo, "b", "user-b")
	unowned := seedOwnedTask(t, svc, repo, "unowned", "")

	h := &Handlers{taskSvc: svc, logger: testLogger(t).WithFields()}
	dispatcher := ws.NewDispatcher()
	dispatcher.RegisterFunc(ws.ActionMCPListTasks, h.handleListTasks)
	dispatcher.RegisterFunc(ws.ActionMCPGetTaskConversation, h.handleGetTaskConversation)
	dispatcher.RegisterFunc(ws.ActionMCPListTaskSessions, h.handleListTaskSessions)

	resolver := mcpscope.NewResolver(repo, identities, func() bool { return authEnabled }, testLogger(t))
	return &scopeFixture{
		dispatcher: dispatcher,
		scope:      resolver.Scope,
		userA:      userA,
		userB:      userB,
		unowned:    unowned,
	}
}

// dispatchAs runs one MCP request over the stream of the agent session that
// belongs to streamTaskID.
func (f *scopeFixture) dispatchAs(t *testing.T, streamTaskID, action string, payload map[string]interface{}) *ws.Message {
	t.Helper()
	ctx, err := f.scope(context.Background(), streamTaskID)
	require.NoError(t, err)
	resp, err := f.dispatcher.Dispatch(ctx, makeWSMessage(t, action, payload))
	require.NoError(t, err)
	require.NotNil(t, resp)
	return resp
}

// TestInSessionMCPListTasksDeniesForeignWorkflow is the regression pin for the
// leak: user A's agent naming user B's workflow_id must not be served B's
// tasks, even though the agent knows the ID.
func TestInSessionMCPListTasksDeniesForeignWorkflow(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userB.workflowID,
	})

	assert.Equal(t, ws.MessageTypeError, resp.Type, "user A's agent must not read user B's workflow")
}

func TestInSessionMCPListTasksAllowsOwnWorkflow(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userA.workflowID,
	})

	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, float64(1), payload["total"])
}

// TestInSessionMCPConversationDeniesForeignTask covers the read that exposes
// message content rather than task metadata.
func TestInSessionMCPConversationDeniesForeignTask(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": f.userB.taskID,
		"limit":   10,
	})

	assert.Equal(t, ws.MessageTypeError, resp.Type, "user A's agent must not read user B's conversation")
}

func TestInSessionMCPConversationAllowsOwnTask(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": f.userA.taskID,
		"limit":   10,
	})

	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, f.userA.sessionID, payload["session_id"])
	assert.Equal(t, float64(1), payload["total"])
}

// TestInSessionMCPListSessionsDeniesForeignTask covers session discovery: the
// session IDs it returns are the keys to another user's conversations and
// message delivery, so it must be scoped like the reads it feeds.
func TestInSessionMCPListSessionsDeniesForeignTask(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": f.userB.taskID,
	})

	assert.Equal(t, ws.MessageTypeError, resp.Type, "user A's agent must not enumerate user B's sessions")
}

func TestInSessionMCPListSessionsAllowsOwnTask(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTaskSessions, map[string]interface{}{
		"task_id": f.userA.taskID,
	})

	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, float64(1), payload["total"])
}

// TestInSessionMCPUnscopedWhenAuthDisabled pins the opt-out path: with the
// auth feature off, dispatch stays unscoped exactly as it did before per-user
// scoping existed, so single-user instances keep cross-workspace access.
func TestInSessionMCPUnscopedWhenAuthDisabled(t *testing.T) {
	f := newScopeFixture(t, false)

	resp := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userB.workflowID,
	})

	assert.Equal(t, ws.MessageTypeResponse, resp.Type)
}

// TestInSessionMCPUnownedStreamCannotReadOwnedWorkflow closes the gap where an
// agent whose own workspace has no owner was handed an identity-free context.
// The task service reads "no identity" as an internal caller and serves
// everything, so such a stream could name any user's workflow. It must reach
// only unowned rows.
func TestInSessionMCPUnownedStreamCannotReadOwnedWorkflow(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.unowned.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userB.workflowID,
	})

	assert.Equal(t, ws.MessageTypeError, resp.Type,
		"an unowned-workspace agent must not read an owned workflow")
}

// TestInSessionMCPUnownedStreamReadsOwnUnownedWorkflow is the other half: the
// pre-auth compatibility contract keeps unowned rows readable, so bounding the
// scope must not lock the agent out of its own workspace.
func TestInSessionMCPUnownedStreamReadsOwnUnownedWorkflow(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.unowned.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.unowned.workflowID,
	})

	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, float64(1), payload["total"])
}

// TestInSessionMCPUnownedStreamReadsOwnConversation guards the message-content
// path for the same stream.
func TestInSessionMCPUnownedStreamReadsOwnConversation(t *testing.T) {
	f := newScopeFixture(t, true)

	resp := f.dispatchAs(t, f.unowned.taskID, ws.ActionMCPGetTaskConversation, map[string]interface{}{
		"task_id": f.unowned.taskID,
		"limit":   10,
	})

	require.Equal(t, ws.MessageTypeResponse, resp.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, float64(1), payload["total"])
}

// TestInSessionMCPDeniesWhenOwnerAccountInactive covers an admin disabling a
// user while their agent is still running: revoking sessions and PATs must also
// cut off the in-session MCP surface.
func TestInSessionMCPDeniesWhenOwnerAccountInactive(t *testing.T) {
	f := newScopeFixtureWithoutAccounts(t)

	_, err := f.scope(context.Background(), f.userA.taskID)

	require.Error(t, err, "an inactive owner must deny the dispatch, not scope to them")
}

// TestInSessionMCPIgnoresPayloadSessionForScoping is the privilege-escalation
// pin: scoping keys off the stream's own task, so naming another user's task
// in the payload cannot swap in their identity. Both requests below claim
// user B's workflow; only the one whose *stream* belongs to B is served.
func TestInSessionMCPIgnoresPayloadSessionForScoping(t *testing.T) {
	f := newScopeFixture(t, true)

	fromA := f.dispatchAs(t, f.userA.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userB.workflowID,
		"task_id":     f.userB.taskID,
		"session_id":  f.userB.sessionID,
	})
	assert.Equal(t, ws.MessageTypeError, fromA.Type)

	fromB := f.dispatchAs(t, f.userB.taskID, ws.ActionMCPListTasks, map[string]interface{}{
		"workflow_id": f.userB.workflowID,
	})
	assert.Equal(t, ws.MessageTypeResponse, fromB.Type)
}
