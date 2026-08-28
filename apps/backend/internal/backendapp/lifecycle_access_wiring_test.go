package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// recordingAuthorizer records which authorizer the lifecycle manager reached
// for each kind of check.
type recordingAuthorizer struct{ calls []string }

func (r *recordingAuthorizer) AuthorizeSessionAccess(_ context.Context, sessionID string) error {
	r.calls = append(r.calls, "session:"+sessionID)
	return nil
}

func (r *recordingAuthorizer) AuthorizeEnvironmentAccess(_ context.Context, envID string) error {
	r.calls = append(r.calls, "environment:"+envID)
	return nil
}

func (r *recordingAuthorizer) AuthorizeTaskAccess(_ context.Context, taskID string) error {
	r.calls = append(r.calls, "task:"+taskID)
	return nil
}

func (r *recordingAuthorizer) AuthorizeTaskEnvironmentAccess(_ context.Context, taskID, envID string) error {
	r.calls = append(r.calls, "pair:"+taskID+"/"+envID)
	return nil
}

// TestWireLifecycleAccessCheckers pins the hop between the task service and the
// lifecycle manager. The three single-ID checkers share the signature
// `func(context.Context, string) error`, so wiring session visibility into the
// task slot compiles cleanly and authorizes the wrong resource for every
// caller downstream (the SSR terminal routes, the vscode and port proxies).
// Checking each check reaches its own authorizer is the only thing that catches
// a crossed or dropped wire.
func TestWireLifecycleAccessCheckers(t *testing.T) {
	authz := &recordingAuthorizer{}
	mgr := &lifecycle.Manager{}

	wireLifecycleAccessCheckers(mgr, authz)

	ctx := context.Background()
	if err := mgr.CheckSessionAccess(ctx, "sess-1"); err != nil {
		t.Fatalf("CheckSessionAccess: %v", err)
	}
	if err := mgr.CheckEnvironmentAccess(ctx, "env-1"); err != nil {
		t.Fatalf("CheckEnvironmentAccess: %v", err)
	}
	if err := mgr.CheckTaskAccess(ctx, "task-1"); err != nil {
		t.Fatalf("CheckTaskAccess: %v", err)
	}
	if err := mgr.CheckTaskEnvironmentAccess(ctx, "task-1", "env-1"); err != nil {
		t.Fatalf("CheckTaskEnvironmentAccess: %v", err)
	}

	want := []string{"session:sess-1", "environment:env-1", "task:task-1", "pair:task-1/env-1"}
	if len(authz.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", authz.calls, want)
	}
	for i, call := range want {
		if authz.calls[i] != call {
			t.Errorf("call %d = %q, want %q (crossed wire)", i, authz.calls[i], call)
		}
	}
}
