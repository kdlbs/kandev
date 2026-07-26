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
	client  *Client
	handled *bool
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
	return &dispatchScopeFixture{client: c, handled: &handled}
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
	}{
		{`{"task_id":"t","session_id":"s"}`, true, "t", "s", ""},
		{`{"task_id":"t"}`, true, "t", "", ""},
		{`{"session_id":"s"}`, true, "", "s", ""},
		{`{"task_environment_id":"e"}`, true, "", "", "e"},
		{`{"task_id":"","task_environment_id":"e"}`, true, "", "", "e"},
		{`{}`, false, "", "", ""},
		{`{"task_id":""}`, false, "", "", ""},
		{`[1,2]`, false, "", "", ""},
		{``, false, "", "", ""},
	}
	for _, tc := range cases {
		refs, ok := parseScopedActionRefs(json.RawMessage(tc.payload))
		if ok != tc.wantOK {
			t.Errorf("payload %q ok = %v, want %v", tc.payload, ok, tc.wantOK)
		}
		if refs.TaskID != tc.wantTask || refs.SessionID != tc.wantSess || refs.TaskEnvironmentID != tc.wantEnv {
			t.Errorf("payload %q refs = %+v, want task=%q session=%q env=%q",
				tc.payload, refs, tc.wantTask, tc.wantSess, tc.wantEnv)
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
