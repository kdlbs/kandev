package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"

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

func TestGuardedGuard_ReadRefusesCaseVariantReferences(t *testing.T) {
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

func seedWorkflows(t *testing.T, f *guardedFixture) (own, foreign string) {
	t.Helper()
	now := time.Now().UTC()
	for _, wf := range []*models.Workflow{
		{ID: "wf-own", WorkspaceID: f.principal.WorkspaceID, Name: "own", CreatedAt: now, UpdatedAt: now},
		{ID: "wf-foreign", WorkspaceID: "ws-other", Name: "foreign", CreatedAt: now, UpdatedAt: now},
	} {
		require.NoError(t, f.repo.CreateWorkflow(context.Background(), wf))
	}
	return "wf-own", "wf-foreign"
}

func TestGuardedGuard_ReadIsConfinedToCallerWorkflows(t *testing.T) {
	f := newGuardedFixture(t)
	own, foreign := seedWorkflows(t, f)
	for _, action := range []string{ws.ActionMCPListTasks, ws.ActionMCPListWorkflowSteps} {
		guarded, replacement := f.call(t, f.principal, action, map[string]any{"workflow_id": own})
		require.Nil(t, guarded, action)
		require.NotNil(t, replacement)
		for _, payload := range []any{
			map[string]any{"workflow_id": foreign},
			map[string]any{"workflow_id": 5},
			json.RawMessage(`{"workflow_id":"` + own + `","workflow_id":"` + foreign + `"}`),
			json.RawMessage(`{"Workflow_ID":"` + foreign + `"}`),
			json.RawMessage(`{"workflow_id":"` + own + `","WORKFLOW_ID":"` + own + `"}`),
		} {
			guarded, _ = f.call(t, f.principal, action, payload)
			assertWSError(t, guarded, ws.ErrorCodeForbidden)
			require.Equal(t, reasonGuardedScope, errorReason(t, guarded), action)
		}
	}
}

func TestGuardedGuard_ReadLookupFailureIsUnavailableForEveryReference(t *testing.T) {
	for name, payload := range map[string]map[string]any{
		"session":  {"session_id": "sess-floored"},
		"workflow": {"workflow_id": "wf-own"},
	} {
		f := newGuardedFixture(t)
		seedWorkflows(t, f)
		require.NoError(t, f.repo.DB().Close())
		guarded, replacement := f.call(t, f.principal, ws.ActionMCPGetTaskConversation, payload)
		assertWSError(t, guarded, ws.ErrorCodeUnavailable)
		require.Equal(t, reasonLookupFailed, errorReason(t, guarded), name)
		require.Nil(t, replacement)
	}
}

func TestGuardedGuard_EnumeratingReadIsPinnedToCallerWorkspace(t *testing.T) {
	f := newGuardedFixture(t)
	for _, payload := range []any{map[string]any{}, json.RawMessage(`null`), map[string]any{"limit": 5}} {
		guarded, replacement := f.call(t, f.principal, ws.ActionMCPListPendingQuestions, payload)
		require.Nil(t, guarded)
		var fields map[string]any
		require.NoError(t, json.Unmarshal(replacement.Payload, &fields))
		require.Equal(t, f.principal.WorkspaceID, fields["workspace_id"])
	}
	guarded, _ := f.call(t, f.principal, ws.ActionMCPListPendingQuestions, map[string]any{"workspace_id": "ws-other"})
	assertWSError(t, guarded, ws.ErrorCodeForbidden)
}

func TestGuardedGuard_ExecutorProfileListingIsDenied(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, _ := f.call(t, f.principal, ws.ActionMCPListExecutorProfiles, map[string]any{"executor_id": "exec-local"})
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
}

func TestGuardedGuard_AgentListingIsDenied(t *testing.T) {
	f := newGuardedFixture(t)
	guarded, _ := f.call(t, f.principal, ws.ActionMCPListAgents, map[string]any{})
	assertWSError(t, guarded, ws.ErrorCodeUnknownAction)
}

func TestListWorkspacesIsConfinedForGuardedPrincipal(t *testing.T) {
	f := newGuardedFixture(t)
	list := func(principal mcpscope.Principal) []dto.WorkspaceDTO {
		msg := makeWSMessage(t, ws.ActionMCPListWorkspaces, map[string]any{})
		resp, err := f.h.handleListWorkspaces(mcpscope.WithPrincipal(context.Background(), principal), msg)
		require.NoError(t, err)
		var out dto.ListWorkspacesResponse
		require.NoError(t, json.Unmarshal(resp.Payload, &out))
		return out.Workspaces
	}
	got := list(f.principal)
	require.Len(t, got, 1)
	require.Equal(t, f.principal.WorkspaceID, got[0].ID)
	unguarded := f.principal
	unguarded.Guarded = false
	require.Greater(t, len(list(unguarded)), 1)
}
