package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Dispatched WS actions were previously left to authorize themselves at the
// service layer, which was missed for a dozen session-mutating actions. These
// tests pin the gateway backstop: an action naming a foreign task or session is
// refused before its handler runs.

// dispatchScopeFixture wires a hub whose auth policy grants only user-a's
// task-a / sess-a and denies everything else.
type dispatchScopeFixture struct {
	client     *Client
	handled    *bool
	dispatcher *ws.Dispatcher
}

func newDispatchScopeFixture(t *testing.T, identity authn.Identity) *dispatchScopeFixture {
	t.Helper()
	h := newTestHub(t)
	dispatcher := ws.NewDispatcher()
	h.dispatcher = dispatcher
	h.mu.Lock()
	h.dispatchCtx = context.Background()
	h.mu.Unlock()

	// Only "sess-a" / "task-a" belong to user-a; everything else is denied.
	h.setAuthPolicy(AuthPolicy{
		Enforced: func() bool { return true },
		Subscriptions: SubscriptionAccessPolicy{
			Session: func(_ context.Context, sessionID string) error {
				if sessionID == "sess-a" {
					return nil
				}
				return repoerrors.ErrTaskNotFound
			},
			Task: func(_ context.Context, taskID string) error {
				if taskID == "task-a" {
					return nil
				}
				return repoerrors.ErrTaskNotFound
			},
		},
		ActionEnvironment: func(_ context.Context, envID string) error {
			if envID == "env-a" {
				return nil
			}
			return repoerrors.ErrTaskNotFound
		},
	})

	handled := false
	dispatcher.RegisterFunc("test.scoped", func(context.Context, *ws.Message) (*ws.Message, error) {
		handled = true
		return nil, nil
	})

	c := newTestClient("c-scope")
	c.hub = h
	c.identity = identity
	return &dispatchScopeFixture{client: c, handled: &handled, dispatcher: dispatcher}
}

// dispatchAs sends payload under a real action name, so tests can exercise the
// rules that key off the action rather than only the payload.
func (f *dispatchScopeFixture) dispatchAs(t *testing.T, action, payload string) {
	t.Helper()
	f.dispatcher.RegisterFunc(action, func(context.Context, *ws.Message) (*ws.Message, error) {
		*f.handled = true
		return nil, nil
	})
	f.client.handleMessage(&ws.Message{
		Action:  action,
		Type:    ws.MessageTypeRequest,
		Payload: json.RawMessage(payload),
	})
}

func (f *dispatchScopeFixture) dispatch(t *testing.T, payload string) {
	t.Helper()
	f.client.handleMessage(&ws.Message{
		Action:  "test.scoped",
		Type:    ws.MessageTypeRequest,
		Payload: json.RawMessage(payload),
	})
}

func realIdentity() authn.Identity {
	return authn.Identity{UserID: "user-a", Role: authn.RoleMember}
}

func TestDispatchScopeDeniesForeignSession(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"session_id":"sess-b"}`)

	if *f.handled {
		t.Error("handler ran for a foreign session; the backstop did not fire")
	}
}

func TestDispatchScopeDeniesForeignTask(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_id":"task-b"}`)

	if *f.handled {
		t.Error("handler ran for a foreign task")
	}
}

// TestDispatchScopeDeniesForeignSessionEvenWithOwnTask covers the mixed payload
// (session.launch / task session status carry both): owning the task must not
// license operating on someone else's session.
func TestDispatchScopeDeniesForeignSessionEvenWithOwnTask(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_id":"task-a","session_id":"sess-b"}`)

	if *f.handled {
		t.Error("handler ran; a foreign session_id must deny even beside an owned task_id")
	}
}

func TestDispatchScopeAllowsOwnResources(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_id":"task-a","session_id":"sess-a"}`)

	if !*f.handled {
		t.Error("handler did not run for the caller's own task and session")
	}
}

// TestDispatchScopeUnscopedIdentitiesPassThrough pins the compatibility
// contract: the synthetic identity injected while auth is disabled, and
// identity-less connections, keep the pre-auth see-everything behavior.
func TestDispatchScopeUnscopedIdentitiesPassThrough(t *testing.T) {
	cases := map[string]authn.Identity{
		"synthetic":   {UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true},
		"no identity": {},
	}
	for name, identity := range cases {
		t.Run(name, func(t *testing.T) {
			f := newDispatchScopeFixture(t, identity)

			f.dispatch(t, `{"session_id":"sess-b","task_id":"task-b"}`)

			if !*f.handled {
				t.Errorf("%s caller was denied; pre-auth behavior must be preserved", name)
			}
		})
	}
}

// TestDispatchScopeIgnoresPayloadsWithoutRefs makes sure the backstop does not
// interfere with actions that name no task or session, including non-object
// payloads (it must not panic or deny them).
func TestDispatchScopeIgnoresPayloadsWithoutRefs(t *testing.T) {
	for _, payload := range []string{`{}`, `{"other":"x"}`, `[]`, `"str"`, `null`, ``} {
		t.Run("payload="+payload, func(t *testing.T) {
			f := newDispatchScopeFixture(t, realIdentity())

			f.dispatch(t, payload)

			if !*f.handled {
				t.Errorf("payload %q names no resource and must dispatch", payload)
			}
		})
	}
}

// TestDispatchScopeNoOpWithoutPolicy keeps an unwired gateway permissive, so
// embedded/test setups behave as before.
func TestDispatchScopeNoOpWithoutPolicy(t *testing.T) {
	h := newTestHub(t)
	dispatcher := ws.NewDispatcher()
	h.dispatcher = dispatcher
	h.mu.Lock()
	h.dispatchCtx = context.Background()
	h.mu.Unlock()
	handled := false
	dispatcher.RegisterFunc("test.scoped", func(context.Context, *ws.Message) (*ws.Message, error) {
		handled = true
		return nil, nil
	})
	c := newTestClient("c-nopolicy")
	c.hub = h
	c.identity = realIdentity()

	c.handleMessage(&ws.Message{
		Action:  "test.scoped",
		Type:    ws.MessageTypeRequest,
		Payload: json.RawMessage(`{"session_id":"sess-b"}`),
	})

	if !handled {
		t.Error("no auth policy wired; the action must dispatch")
	}
}

func TestParseScopedActionRefs(t *testing.T) {
	cases := []struct {
		payload  string
		wantOK   bool
		wantTask string
		wantSess string
		wantEnv  string
		wantID   string
	}{
		{`{"task_id":"t","session_id":"s"}`, true, "t", "s", "", ""},
		{`{"task_id":"t"}`, true, "t", "", "", ""},
		{`{"session_id":"s"}`, true, "", "s", "", ""},
		{`{"task_environment_id":"e"}`, true, "", "", "e", ""},
		{`{"task_id":"","task_environment_id":"e"}`, true, "", "", "e", ""},
		{`{"id":"t"}`, true, "", "", "", "t"},
		{`{"id":"t","state":"COMPLETED"}`, true, "", "", "", "t"},
		{`{}`, false, "", "", "", ""},
		{`{"task_id":""}`, false, "", "", "", ""},
		{`{"id":""}`, false, "", "", "", ""},
		{`[1,2]`, false, "", "", "", ""},
		{``, false, "", "", "", ""},
	}
	for _, tc := range cases {
		refs, ok := parseScopedActionRefs(json.RawMessage(tc.payload))
		if ok != tc.wantOK {
			t.Errorf("payload %q ok = %v, want %v", tc.payload, ok, tc.wantOK)
		}
		if refs.TaskID != tc.wantTask || refs.SessionID != tc.wantSess ||
			refs.TaskEnvironmentID != tc.wantEnv || refs.ID != tc.wantID {
			t.Errorf("payload %q refs = %+v, want task=%q session=%q env=%q id=%q",
				tc.payload, refs, tc.wantTask, tc.wantSess, tc.wantEnv, tc.wantID)
		}
	}
}

func TestIdentityIsScoped(t *testing.T) {
	if identityIsScoped(authn.Identity{}) {
		t.Error("empty identity must not be scoped")
	}
	if identityIsScoped(authn.Identity{UserID: "u", Synthetic: true}) {
		t.Error("synthetic identity must not be scoped")
	}
	if !identityIsScoped(authn.Identity{UserID: "u"}) {
		t.Error("real identity must be scoped")
	}
}

// TestDispatchScopeDeniesForeignEnvironment covers the user-shell actions, which
// name a task environment and treat task_id as optional. Checking only task_id
// let `user_shell.stop` tear down another user's terminal with task_id empty.
func TestDispatchScopeDeniesForeignEnvironment(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_environment_id":"env-b","terminal_id":"shell-1"}`)

	if *f.handled {
		t.Error("handler ran for a foreign task environment")
	}
}

// TestDispatchScopeDeniesForeignEnvironmentWithEmptyTaskID is the exact bypass:
// omitting task_id must not skip the check.
func TestDispatchScopeDeniesForeignEnvironmentWithEmptyTaskID(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_id":"","task_environment_id":"env-b","terminal_id":"shell-1"}`)

	if *f.handled {
		t.Error("empty task_id must not bypass the environment check")
	}
}

func TestDispatchScopeAllowsOwnEnvironment(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatch(t, `{"task_environment_id":"env-a","terminal_id":"shell-1"}`)

	if !*f.handled {
		t.Error("handler did not run for the caller's own task environment")
	}
}

// A top-level task.<verb> action names its task `id`, not `task_id` — which is
// how task.state and task.move mutated any user's task with the backstop finding
// no refs to check and allowing the dispatch.
//
// The rule keys off namespace depth rather than a list of action names, so a
// future task.<verb> is covered without an edit. Deeper namespaces are excluded
// on purpose: their `id` names a plan, revision or finding, and checking it as a
// task ID would deny legitimate reads — see the allow cases below.
func TestDispatchScopeDeniesForeignTaskByIDOnTopLevelTaskActions(t *testing.T) {
	for _, action := range []string{ws.ActionTaskState, ws.ActionTaskMove, ws.ActionTaskArchive, ws.ActionTaskGet} {
		t.Run(action, func(t *testing.T) {
			f := newDispatchScopeFixture(t, realIdentity())

			f.dispatchAs(t, action, `{"id":"task-b","state":"COMPLETED"}`)

			if *f.handled {
				t.Errorf("handler ran for %s naming a foreign task by id", action)
			}
		})
	}
}

func TestDispatchScopeAllowsOwnTaskByIDOnTopLevelTaskActions(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatchAs(t, ws.ActionTaskState, `{"id":"task-a","state":"COMPLETED"}`)

	if !*f.handled {
		t.Error("the owner's own task.state was denied; the rule must not be a blanket deny")
	}
}

// TestDispatchScopeIgnoresIDOnNestedTaskActions is the regression this rule
// could plausibly cause: in task.plan.* and task.review.* the `id` is a plan,
// revision or finding ID, and treating it as a task ID would refuse every read.
func TestDispatchScopeIgnoresIDOnNestedTaskActions(t *testing.T) {
	nested := map[string]string{
		ws.ActionTaskPlanRevisionGet:     `{"id":"revision-7"}`,
		ws.ActionTaskReviewFindingUpdate: `{"id":"finding-3","status":"resolved"}`,
		ws.ActionTaskPlanGet:             `{"id":"plan-1"}`,
	}
	for action, payload := range nested {
		t.Run(action, func(t *testing.T) {
			f := newDispatchScopeFixture(t, realIdentity())

			f.dispatchAs(t, action, payload)

			if !*f.handled {
				t.Errorf("%s was denied: its id names a nested resource, not a task", action)
			}
		})
	}
}

// TestDispatchScopeIgnoresIDOnNonTaskActions keeps every other namespace out of
// the rule — `id` there is a workflow, executor or agent profile.
func TestDispatchScopeIgnoresIDOnNonTaskActions(t *testing.T) {
	for _, action := range []string{ws.ActionWorkflowGet, "executor.get", "tasks.something"} {
		t.Run(action, func(t *testing.T) {
			f := newDispatchScopeFixture(t, realIdentity())

			f.dispatchAs(t, action, `{"id":"task-b"}`)

			if !*f.handled {
				t.Errorf("%s was denied; its id does not name a task", action)
			}
		})
	}
}

// task.create / task.list carry no id, so the rule is a no-op for them.
func TestDispatchScopeAllowsTopLevelTaskActionsWithoutID(t *testing.T) {
	f := newDispatchScopeFixture(t, realIdentity())

	f.dispatchAs(t, ws.ActionTaskCreate, `{"workspace_id":"ws-a","title":"New"}`)

	if !*f.handled {
		t.Error("task.create names no task id and must dispatch")
	}
}

func TestIsTopLevelTaskAction(t *testing.T) {
	cases := map[string]bool{
		ws.ActionTaskState:               true,
		ws.ActionTaskMove:                true,
		ws.ActionTaskGet:                 true,
		ws.ActionTaskArchive:             true,
		ws.ActionTaskPlanGet:             false,
		ws.ActionTaskPlanRevisionGet:     false,
		ws.ActionTaskReviewFindingUpdate: false,
		ws.ActionTaskSessionList:         false,
		ws.ActionWorkflowGet:             false,
		"task":                           false,
		"task.":                          true,
		"tasks.get":                      false,
		"":                               false,
	}
	for action, want := range cases {
		if got := isTopLevelTaskAction(action); got != want {
			t.Errorf("isTopLevelTaskAction(%q) = %v, want %v", action, got, want)
		}
	}
}
