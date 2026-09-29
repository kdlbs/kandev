package handlers

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type allowAllAuthorizer struct{}

func (allowAllAuthorizer) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error {
	return nil
}

type guardFixture struct {
	h      *Handlers
	svc    *coordinator.Service
	c      *coordinator.Coordinator
	rawDB  *sqlx.DB
	repo   *sqliterepo.Repository
	store  *coordinator.Store
	ctxFor func(binding *mcpprofile.CoordinatorToolPolicy, required bool) context.Context
}

// newPhase2GuardFixture wires a phase-2 coordinator service over a real store
// and a guard whose principal is that coordinator's conversation session.
func newPhase2GuardFixture(t *testing.T) *guardFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coordinator.db")
	writerConn, err := db.OpenSQLite(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writerConn.Close() })
	readerConn, err := db.OpenSQLiteReader(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = readerConn.Close() })
	writer := sqlx.NewDb(writerConn, "sqlite3")
	store, err := coordinator.NewStore(writer, sqlx.NewDb(readerConn, "sqlite3"))
	require.NoError(t, err)
	svc := coordinator.NewService(store, coordinator.NewValidator(nil, nil), allowAllAuthorizer{}, testLogger(t), coordinator.WithPhase2(true))

	taskSvc, repo := newTestTaskService(t)
	h := &Handlers{taskSvc: taskSvc, logger: testLogger(t).WithFields()}
	h.SetCoordinatorService(svc)

	workspaces, err := taskSvc.ListWorkspaces(context.Background())
	require.NoError(t, err)
	require.Len(t, workspaces, 1)
	c := &coordinator.Coordinator{WorkspaceID: workspaces[0].ID, Name: "c", AgentProfileID: "ap", ExecutorProfileID: "ep"}
	require.NoError(t, store.CreateCoordinator(context.Background(), c))

	f := &guardFixture{h: h, svc: svc, c: c, rawDB: writer, repo: repo, store: store}
	f.ctxFor = func(binding *mcpprofile.CoordinatorToolPolicy, required bool) context.Context {
		principal := coordinatorTestPrincipal(c.WorkspaceID)
		principal.CoordinatorID = c.ID
		ctx := mcpscope.WithPrincipal(context.Background(), principal)
		return streams.WithMCPExecutionContext(ctx, streams.MCPExecutionContext{
			ExecutionID: "exec-1", TaskID: principal.CallerTaskID, SessionID: principal.CallerSessionID,
			CoordinatorToolPolicy: binding, CoordinatorToolPolicyRequired: required,
		})
	}
	return f
}

func (f *guardFixture) binding(names ...string) *mcpprofile.CoordinatorToolPolicy {
	return &mcpprofile.CoordinatorToolPolicy{
		Version: 1, CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID,
		ConversationTaskID: "coordinator-conversation-task", ToolNames: names,
	}
}

func (f *guardFixture) save(t *testing.T, createTask string) {
	t.Helper()
	body := `{"policy":{"actions":{"create_task":"` + createTask + `","start_agent":"denied","message":"denied","move":"denied","resume":"denied","stop":"denied"}}}`
	_, err := f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID, []byte(body))
	require.NoError(t, err)
}

func (f *guardFixture) refusals(t *testing.T) []struct{ Class, Reason string } {
	t.Helper()
	var rows []struct {
		Class  string `db:"action_class"`
		Reason string `db:"reason_code"`
	}
	require.NoError(t, f.rawDB.Select(&rows, `SELECT action_class, reason_code FROM coordinator_activity WHERE coordinator_id = ? AND outcome = 'refused'`, f.c.ID))
	out := make([]struct{ Class, Reason string }, len(rows))
	for i, r := range rows {
		out[i] = struct{ Class, Reason string }{r.Class, r.Reason}
	}
	return out
}

func (f *guardFixture) call(t *testing.T, ctx context.Context, action string) *ws.Message {
	t.Helper()
	guarded, _, err := f.h.authorizeCoordinatorRequest(ctx, makeWSMessage(t, action, map[string]interface{}{}))
	require.NoError(t, err)
	return guarded
}

var phase1Names = []string{
	"list_tasks_kandev", "get_task_conversation_kandev", "list_workflows_kandev",
	"list_workflow_steps_kandev", "list_repositories_kandev", "get_coordinator_item_kandev", "propose_task_kandev",
}

// Row 1: S tightens and commits before the guard's policy read.
func TestGuardInterleaving1_TightenedBeforeReadRefusesPolicyDenied(t *testing.T) {
	f := newPhase2GuardFixture(t)
	f.save(t, "denied")
	guarded := f.call(t, f.ctxFor(f.binding(phase1Names...), true), coordinator.ActionProposeTask)
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	require.Equal(t, []struct{ Class, Reason string }{{"create_task", "policy_denied"}}, f.refusals(t))
}

// Row 3: S loosens while a conversation runs. The bound names are unchanged,
// so a direct call to a tool outside them fails check 2.
func TestGuardInterleaving3_LoosenedSettingDoesNotWidenBoundNames(t *testing.T) {
	f := newPhase2GuardFixture(t)
	_, err := f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID,
		[]byte(`{"policy":{"actions":{"create_task":"requires_approval","start_agent":"denied","message":"requires_approval","move":"denied","resume":"denied","stop":"denied"}}}`))
	require.NoError(t, err)
	guarded := f.call(t, f.ctxFor(f.binding(phase1Names...), true), "coordinator.propose_message")
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	require.Equal(t, []struct{ Class, Reason string }{{"message", "not_in_profile"}}, f.refusals(t))
}

// Row 4: the guard reads denied, S loosens afterwards; the call stays refused.
func TestGuardInterleaving4_RefusalStandsAfterLoosening(t *testing.T) {
	f := newPhase2GuardFixture(t)
	f.save(t, "denied")
	ctx := f.ctxFor(f.binding(phase1Names...), true)
	assertWSError(t, f.call(t, ctx, coordinator.ActionProposeTask), ws.ErrorCodeUnknownAction)
	f.save(t, "requires_approval")
	require.Equal(t, []struct{ Class, Reason string }{{"create_task", "policy_denied"}}, f.refusals(t))
}

// An action outside the bound names, sent straight to the endpoint.
func TestGuard_ActionOutsideBoundNamesIsNotInProfile(t *testing.T) {
	f := newPhase2GuardFixture(t)
	guarded := f.call(t, f.ctxFor(f.binding("list_tasks_kandev"), true), coordinator.ActionProposeTask)
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	require.Equal(t, []struct{ Class, Reason string }{{"create_task", "not_in_profile"}}, f.refusals(t))
}

// A present but unparsable binding refuses every action and records the
// coordinator and workspace ids of the principal.
func TestGuard_InvalidBindingRefusesEveryAction(t *testing.T) {
	f := newPhase2GuardFixture(t)
	ctx := f.ctxFor(nil, true)
	assertWSError(t, f.call(t, ctx, ws.ActionMCPListTasks), ws.ErrorCodeUnknownAction)
	assertWSError(t, f.call(t, ctx, coordinator.ActionProposeTask), ws.ErrorCodeUnknownAction)
	got := f.refusals(t)
	require.Len(t, got, 2)
	for _, r := range got {
		require.Equal(t, "binding_invalid", r.Reason)
	}
}

// A conversation opened before phase 2 has no binding: the phase-1 seven.
func TestGuard_NoBindingIsPhaseOneSeven(t *testing.T) {
	f := newPhase2GuardFixture(t)
	require.Nil(t, f.call(t, f.ctxFor(nil, false), ws.ActionMCPListRepositories))
	require.Empty(t, f.refusals(t))
}

func (f *guardFixture) savePolicy(t *testing.T, overrides map[string]string) {
	t.Helper()
	actions := map[string]string{"create_task": "requires_approval", "start_agent": "denied", "message": "denied", "move": "denied", "resume": "denied", "stop": "denied"}
	for k, v := range overrides {
		actions[k] = v
	}
	body, err := json.Marshal(map[string]any{"policy": map[string]any{"actions": actions}})
	require.NoError(t, err)
	_, err = f.svc.SaveSettings(context.Background(), f.c.WorkspaceID, f.c.ID, body)
	require.NoError(t, err)
}

func TestGuard_ProposeKindActionsPassWhenBoundAndAllowed(t *testing.T) {
	f := newPhase2GuardFixture(t)
	f.savePolicy(t, map[string]string{"move": "requires_approval", "message": "requires_approval", "resume": "requires_approval"})
	names := append(append([]string{}, phase1Names...), "propose_move_kandev", "propose_message_kandev", "propose_resume_kandev")
	ctx := f.ctxFor(f.binding(names...), true)
	for _, action := range []string{coordinator.ActionProposeMove, coordinator.ActionProposeMessage, coordinator.ActionProposeResume} {
		msg := makeWSMessage(t, action, map[string]interface{}{"task_id": "task-not-in-this-workspace"})
		guarded, replacement, err := f.h.authorizeCoordinatorRequest(ctx, msg)
		require.NoError(t, err)
		require.Nil(t, guarded, "%s must reach the service, which names the field", action)
		require.NotNil(t, replacement)
	}
	require.Empty(t, f.refusals(t))
}

func TestGuard_ProposeKindDeniedByPolicyRecordsItsOwnClass(t *testing.T) {
	f := newPhase2GuardFixture(t)
	names := append(append([]string{}, phase1Names...), "propose_move_kandev")
	guarded := f.call(t, f.ctxFor(f.binding(names...), true), coordinator.ActionProposeMove)
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	require.Equal(t, []struct{ Class, Reason string }{{"move", "policy_denied"}}, f.refusals(t))
}
