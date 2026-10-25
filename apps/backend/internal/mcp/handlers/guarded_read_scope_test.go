package handlers

import (
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

func TestGuardedGuard_ReadIsConfinedToCallerWorkspace(t *testing.T) {
	f := newGuardedFixture(t)
	for _, payload := range []map[string]any{
		{"task_id": fxCaller},
		{"task_id": fxPlain},
		{"session_id": "sess-floored"},
		{"workspace_id": f.principal.WorkspaceID},
		{},
	} {
		guarded, replacement := f.call(t, f.principal, ws.ActionMCPGetTaskConversation, payload)
		require.Nil(t, guarded, payload)
		require.NotNil(t, replacement)
	}
	for _, payload := range []map[string]any{
		{"task_id": fxForeign},
		{"task_id": "missing"},
		{"session_id": "sess-foreign"},
		{"session_id": "missing"},
		{"workspace_id": "ws-other"},
		{"workflow_id": "missing"},
		{"task_id": 5},
		{"session_id": []string{"x"}},
		{"workspace_id": 4},
	} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPGetTaskConversation, payload)
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
		require.Equal(t, reasonGuardedScope, errorReason(t, guarded), payload)
	}
}

func TestGuardedGuard_ReadRefusesDuplicateAndCaseVariantReferences(t *testing.T) {
	f := newGuardedFixture(t)
	for _, raw := range []string{
		`{"task_id":"` + fxCaller + `","Task_Id":"` + fxForeign + `"}`,
		`{"TASK_ID":"` + fxForeign + `"}`,
	} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPListRelatedTasks, json.RawMessage(raw))
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
	}
}

func TestGuardedGuard_DiagnosticBundleIsSelfClass(t *testing.T) {
	f := newGuardedFixture(t)
	for _, payload := range []map[string]any{{"source": "backend"}, {"source": "backend", "task_id": fxCaller}} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPGetDiagnosticBundle, payload)
		require.Nil(t, guarded, payload)
	}
	for _, payload := range []map[string]any{
		{"task_id": fxPlain}, {"task_id": fxForeign}, {"session_id": "sess-floored"},
	} {
		guarded, _ := f.call(t, f.principal, ws.ActionMCPGetDiagnosticBundle, payload)
		assertWSError(t, guarded, ws.ErrorCodeForbidden)
	}
}

func TestGuardedGuard_RepositoryFailureIsUnavailable(t *testing.T) {
	f := newGuardedFixture(t)
	require.NoError(t, f.repo.DB().Close())
	cases := map[string]map[string]any{
		ws.ActionMCPGetTaskConversation: {"task_id": fxPlain},
		ws.ActionMCPUpdateTask:          {"task_id": fxPlain},
		ws.ActionMCPMessageTask:         {"session_id": "sess-floored"},
	}
	for action, payload := range cases {
		guarded, replacement := f.call(t, f.principal, action, payload)
		assertWSError(t, guarded, ws.ErrorCodeUnavailable)
		require.Equal(t, reasonLookupFailed, errorReason(t, guarded), action)
		require.Nil(t, replacement)
	}
}

func TestGuardedGuard_MissingTaskServiceIsUnavailable(t *testing.T) {
	f := newGuardedFixture(t)
	f.h.taskSvc = nil
	for action, payload := range map[string]map[string]any{
		ws.ActionMCPUpdateTask:          {"task_id": fxFloored},
		ws.ActionMCPGetTaskConversation: {"task_id": fxFloored},
	} {
		guarded, _ := f.call(t, f.principal, action, payload)
		assertWSError(t, guarded, ws.ErrorCodeUnavailable)
		require.Equal(t, reasonLookupFailed, errorReason(t, guarded), action)
	}
	guarded, _ := f.call(t, f.principal, ws.ActionMCPListWorkspaces, map[string]any{})
	require.Nil(t, guarded)
}

func TestGuardedGuard_ParentReportingRoutesStayOpenForSubtask(t *testing.T) {
	f := newGuardedFixture(t)
	p := f.principal
	p.CallerTaskID, p.CallerSessionID = fxFloored, "sess-floored"
	for _, action := range []string{ws.ActionMCPStepComplete, ws.ActionMCPAskParentQuestion, ws.ActionMCPAskUserQuestion} {
		guarded, _ := f.call(t, p, action, map[string]any{})
		require.Nil(t, guarded, action)
	}
	guarded, _ := f.call(t, p, ws.ActionMCPMessageTask, map[string]any{"session_id": fxSession})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}

func TestGuardedGuard_UnicodeFoldedKeyIsChecked(t *testing.T) {
	f := newGuardedFixture(t)
	kelvin := "Key"
	_ = kelvin
	// U+017F (long s) folds to "s": "ſession_id" is the session_id field.
	guarded, _ := f.call(t, f.principal, ws.ActionMCPMessageTask, json.RawMessage(`{"ſession_id":"sess-foreign"}`))
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}
