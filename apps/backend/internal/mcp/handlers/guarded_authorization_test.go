package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type guardedFixture struct {
	guards    *fakeGuardSource
	repo      *sqliterepo.Repository
	h         *Handlers
	principal mcpscope.Principal
}

const (
	fxCaller   = "task-caller"
	fxFloored  = "task-floored"
	fxPlain    = "task-plain"
	fxForeign  = "task-foreign"
	fxFloorOth = "task-floor-other"
	fxSession  = "sess-caller"
)

// fakeGuardSource reports the listed tasks as guarded; err makes every lookup fail.
type fakeGuardSource struct {
	guarded map[string]bool
	err     error
}

func (f *fakeGuardSource) TaskGuarded(_ context.Context, _, taskID string) (bool, error) {
	return f.guarded[taskID], f.err
}

func newGuardedFixture(t *testing.T) *guardedFixture {
	t.Helper()
	svc, repo := newTestTaskService(t)
	ctx := context.Background()
	workspaces, err := svc.ListWorkspaces(ctx)
	require.NoError(t, err)
	wsID := workspaces[0].ID
	now := time.Now().UTC()
	mk := func(id, workspace, parent string) {
		task := &models.Task{ID: id, WorkspaceID: workspace, ParentID: parent, Title: id,
			State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, repo.CreateTask(ctx, task))
	}
	foreignWS := &models.Workspace{ID: "ws-other", Name: "Other", CreatedAt: now, UpdatedAt: now}
	require.NoError(t, repo.CreateWorkspace(ctx, foreignWS))
	mk(fxCaller, wsID, "")
	mk(fxFloored, wsID, fxCaller)
	mk(fxFloorOth, wsID, fxCaller)
	mk(fxPlain, wsID, fxCaller)
	mk(fxForeign, foreignWS.ID, "")
	requireSession(t, repo, fxSession, fxCaller)
	requireSession(t, repo, "sess-floored", fxFloored)
	requireSession(t, repo, "sess-foreign", fxForeign)
	guards := &fakeGuardSource{guarded: map[string]bool{fxFloored: true, fxFloorOth: true}}
	return &guardedFixture{
		guards: guards,
		repo:   repo,
		h:      &Handlers{taskSvc: svc, guardSource: guards, logger: testLogger(t).WithFields()},
		principal: mcpscope.Principal{
			WorkspaceID: wsID, CallerTaskID: fxCaller, CallerSessionID: fxSession,
			Surface: mcpprofile.SurfaceKanbanTask, Guarded: true,
		},
	}
}

func requireSession(t *testing.T, repo *sqliterepo.Repository, id, taskID string) {
	t.Helper()
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: id, TaskID: taskID, State: models.TaskSessionStateRunning, StartedAt: now, UpdatedAt: now,
	}))
}

func (f *guardedFixture) call(t *testing.T, principal mcpscope.Principal, action string, payload any) (*ws.Message, *ws.Message) {
	t.Helper()
	var msg *ws.Message
	if raw, ok := payload.(json.RawMessage); ok {
		msg = &ws.Message{ID: "m1", Type: ws.MessageTypeRequest, Action: action, Payload: raw}
	} else {
		msg = makeWSMessage(t, action, payload)
	}
	guarded, replacement, err := f.h.authorizeGuardedRequest(mcpscope.WithPrincipal(context.Background(), principal), msg)
	require.NoError(t, err)
	return guarded, replacement
}

func errorReason(t *testing.T, msg *ws.Message) string {
	t.Helper()
	var p ws.ErrorPayload
	require.NoError(t, json.Unmarshal(msg.Payload, &p))
	reason, _ := p.Details["reason"].(string)
	return reason
}

func replacementFields(t *testing.T, msg *ws.Message) map[string]json.RawMessage {
	t.Helper()
	fields := map[string]json.RawMessage{}
	require.NoError(t, json.Unmarshal(msg.Payload, &fields))
	return fields
}

func TestGuardedGuard_PassesUnguardedAndOtherPrincipals(t *testing.T) {
	f := newGuardedFixture(t)
	cases := map[string]mcpscope.Principal{
		"unguarded": func() mcpscope.Principal { p := f.principal; p.Guarded = false; return p }(),
		"automation": func() mcpscope.Principal {
			p := f.principal
			p.AutomationID, p.Surface = "auto", mcpprofile.SurfaceAutomation
			return p
		}(),
		"coordinator": func() mcpscope.Principal {
			p := f.principal
			p.CoordinatorID, p.Surface = "coord", mcpprofile.SurfaceCoordinator
			return p
		}(),
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) {
			guarded, replacement := f.call(t, p, ws.ActionMCPArchiveTask, map[string]any{"task_id": fxForeign})
			require.Nil(t, guarded)
			require.NotNil(t, replacement)
		})
	}
}

func TestGuardedGuard_UnknownAndDeniedActions(t *testing.T) {
	f := newGuardedFixture(t)
	for _, action := range []string{ws.ActionMCPArchiveTask, ws.ActionMCPDeleteTask, "mcp.not_a_tool", "mcp.create_agent"} {
		guarded, _ := f.call(t, f.principal, action, map[string]any{})
		assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
	}
}

func TestGuardedGuard_ChildWriteScope(t *testing.T) {
	f := newGuardedFixture(t)
	allowed := []string{fxCaller, fxFloored, fxFloorOth}
	for _, id := range allowed {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": id})
		require.Nil(t, guarded, id)
	}
	for _, id := range []string{fxForeign, "missing", fxPlain} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": id})
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
		require.Equal(t, reasonGuardedScope, errorReason(t, guarded), id)
	}
}

func TestGuardedGuard_UnguardedDirectChildIsRefused(t *testing.T) {
	f := newGuardedFixture(t)
	requireSession(t, f.repo, "sess-plain", fxPlain)
	cases := map[string]map[string]any{
		ws.ActionMCPMessageTask:  {"task_id": fxPlain},
		ws.ActionMCPSpawnSession: {"task_id": fxPlain},
		ws.ActionMCPUpdateTask:   {"task_id": fxPlain},
		ws.ActionMCPStopTask:     {"task_id": fxPlain},
	}
	for action, payload := range cases {
		guarded, replacement := f.call(t, f.principal, action, payload)
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
		require.Equal(t, reasonGuardedScope, errorReason(t, guarded), action)
		require.Nil(t, replacement, action)
	}
	guarded, _ := f.call(t, f.principal, ws.ActionMCPMessageTask, map[string]any{"session_id": "sess-plain"})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
	guarded, _ = f.call(t, f.principal, ws.ActionMCPAddTaskDependency, map[string]any{"task_id": fxCaller, "depends_on_task_id": fxPlain})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}

func TestGuardedGuard_ChildGuardLookupFailureIsUnavailable(t *testing.T) {
	f := newGuardedFixture(t)
	f.guards.err = errors.New("guard store down")
	guarded, replacement := f.call(t, f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": fxFloored})
	assertWSError(t, guarded, ws.ErrorCodeUnavailable)
	require.Equal(t, reasonLookupFailed, errorReason(t, guarded))
	require.Nil(t, replacement)

	f.h.guardSource = nil
	guarded, _ = f.call(t, f.principal, ws.ActionMCPUpdateTask, map[string]any{"task_id": fxFloored})
	assertWSError(t, guarded, ws.ErrorCodeUnavailable)
}

func TestGuardedGuard_SessionAndOtherReferenceFields(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, _ := f.call(t, f.principal, ws.ActionMCPMessageTask, map[string]any{"session_id": "sess-floored"})
	require.Nil(t, guarded)
	for _, payload := range []map[string]any{
		{"session_id": "sess-foreign"},
		{"session_id": "missing"},
		{"task_id": fxCaller, "depends_on_task_id": fxForeign},
		{"task_id": fxCaller, "blocked_by": []string{fxFloored, fxForeign}},
	} {
		guarded, _ = f.call(t, f.principal, ws.ActionMCPAddTaskDependency, payload)
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
	}
}

func TestGuardedGuard_WrongTypesAndShapes(t *testing.T) {
	f := newGuardedFixture(t)
	raws := []string{
		`{"task_id": 5}`, `{"task_id": ["a"]}`, `{"blocked_by": "x"}`, `{"blocked_by": ["` + fxCaller + `", 5]}`,
		`{"blocked_by": [null]}`, `{"session_id": {}}`, `[1]`, `"x"`, `7`,
		`{"task_id":"` + fxCaller + `","TASK_ID":"` + fxCaller + `"}`,
		`{"task_id":"` + fxCaller + `","task_id":"` + fxCaller + `"}`,
	}
	for _, raw := range raws {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPUpdateTask, json.RawMessage(raw))
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
		require.Equal(t, reasonGuardedScope, errorReason(t, guarded), raw)
	}
}

func TestGuardedGuard_NullAndEmptyPayloadIsEmptyObject(t *testing.T) {
	f := newGuardedFixture(t)
	for _, tool := range []string{ws.ActionMCPListWorkspaces} {
		for _, raw := range []string{"", "null", "{}"} {
			guarded, replacement := f.call(t, f.principal, tool, json.RawMessage(raw))
			require.Nil(t, guarded, tool+raw)
			require.JSONEq(t, `{}`, string(replacement.Payload))
		}
	}
}

func TestGuardedGuard_CaseVariantKeysAreCanonicalised(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, replacement := f.call(t, f.principal, ws.ActionMCPUpdateTask,
		json.RawMessage(`{"Task_ID":"`+fxFloored+`","title":"t"}`))
	require.Nil(t, guarded)
	fields := replacementFields(t, replacement)
	require.Contains(t, fields, "task_id")
	require.NotContains(t, fields, "Task_ID")
	// A variant spelling naming a foreign task is checked, not ignored.
	guarded, _ = f.call(t, f.principal, ws.ActionMCPUpdateTask, json.RawMessage(`{"TASK_id":"`+fxForeign+`"}`))
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}

func TestGuardedGuard_AttributionRewrite(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, replacement := f.call(t, f.principal, ws.ActionMCPCreateTask, map[string]any{
		"title": "x", "source_task_id": "forged", "SOURCE_SESSION_ID": "forged",
	})
	require.Nil(t, guarded)
	fields := replacementFields(t, replacement)
	require.JSONEq(t, `"`+fxCaller+`"`, string(fields["source_task_id"]))
	require.JSONEq(t, `"`+fxSession+`"`, string(fields["source_session_id"]))
	require.NotContains(t, fields, "SOURCE_SESSION_ID")

	guarded, replacement = f.call(t, f.principal, ws.ActionMCPSpawnSession, map[string]any{
		"task_id": fxFloored, "sender_task_id": "forged",
	})
	require.Nil(t, guarded)
	fields = replacementFields(t, replacement)
	require.JSONEq(t, `"`+fxCaller+`"`, string(fields["sender_task_id"]))
	require.JSONEq(t, `"`+fxSession+`"`, string(fields["sender_session_id"]))

	_, replacement = f.call(t, f.principal, ws.ActionMCPListRelatedTasks, map[string]any{"caller_task_id": "forged"})
	require.JSONEq(t, `"`+fxCaller+`"`, string(replacementFields(t, replacement)["caller_task_id"]))
}

func TestGuardedGuard_SelfClassAcceptsOnlyOwnTask(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, _ := f.call(t, f.principal, ws.ActionMCPCreateTaskPlan, map[string]any{"task_id": fxCaller})
	require.Nil(t, guarded)
	guarded, _ = f.call(t, f.principal, ws.ActionMCPCreateTaskPlan, map[string]any{})
	require.Nil(t, guarded)
	guarded, _ = f.call(t, f.principal, ws.ActionMCPCreateTaskPlan, map[string]any{"task_id": fxFloored})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}

func TestGuardedGuard_CreateTaskNormalisesWorkspaceAndParent(t *testing.T) {
	f := newGuardedFixture(t)
	for _, parent := range []any{nil, "", "self", fxCaller} {
		payload := map[string]any{"title": "x", "parent_id": parent, "parent_is_self": true}
		guarded, replacement := f.call(t, f.principal, ws.ActionMCPCreateTask, payload)
		require.Nil(t, guarded, parent)
		fields := replacementFields(t, replacement)
		require.JSONEq(t, `"`+fxCaller+`"`, string(fields["parent_id"]))
		require.JSONEq(t, `"`+f.principal.WorkspaceID+`"`, string(fields["workspace_id"]))
		require.NotContains(t, fields, "parent_is_self")
	}
	guarded, replacement := f.call(t, f.principal, ws.ActionMCPCreateTask, map[string]any{"title": "x"})
	require.Nil(t, guarded)
	require.JSONEq(t, `"`+fxCaller+`"`, string(replacementFields(t, replacement)["parent_id"]))

	for _, payload := range []map[string]any{
		{"parent_id": fxFloored}, {"parent_id": fxForeign}, {"parent_id": 3},
		{"workspace_id": "ws-other"}, {"workspace_id": 4},
	} {
		guarded, _ = f.call(t, f.principal, ws.ActionMCPCreateTask, payload)
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
		require.Equal(t, reasonGuardedScope, errorReason(t, guarded))
	}
}

func TestGuardedGuard_CreateTaskFromSubtaskPinsParentToCaller(t *testing.T) {
	f := newGuardedFixture(t)
	p := f.principal
	p.CallerTaskID, p.CallerSessionID = fxFloored, "sess-floored"
	guarded, replacement := f.call(t, p, ws.ActionMCPCreateTask, map[string]any{"title": "x"})
	require.Nil(t, guarded)
	require.JSONEq(t, `"`+fxFloored+`"`, string(replacementFields(t, replacement)["parent_id"]))

	guarded, _ = f.call(t, p, ws.ActionMCPCreateTask, map[string]any{"title": "x", "parent_id": fxCaller})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
	require.Equal(t, reasonGuardedScope, errorReason(t, guarded))
}

func TestGuardedGuard_MessageToParentRefused(t *testing.T) {
	f := newGuardedFixture(t)
	p := f.principal
	p.CallerTaskID, p.CallerSessionID = fxFloored, "sess-floored"
	guarded, _ := f.call(t, p, ws.ActionMCPMessageTask, map[string]any{"task_id": fxCaller})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
	require.Equal(t, reasonGuardedScope, errorReason(t, guarded))
}

func TestGuardedGuard_InternalActionRequiresTrustedInternalCall(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, _ := f.call(t, f.principal, ws.ActionMCPAuditTaskTransferAttempt, json.RawMessage(`[1]`))
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)

	msg := &ws.Message{ID: "m1", Type: ws.MessageTypeRequest, Action: ws.ActionMCPAuditTaskTransferAttempt, Payload: json.RawMessage(`[1]`)}
	ctx := mcporigin.WithTrustedInternalCall(mcpscope.WithPrincipal(context.Background(), f.principal))
	guarded, replacement, err := f.h.authorizeGuardedRequest(ctx, msg)
	require.NoError(t, err)
	require.Nil(t, guarded)
	require.JSONEq(t, `[1]`, string(replacement.Payload))
}
