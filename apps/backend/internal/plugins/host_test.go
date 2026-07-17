package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
)

// newHostTestStateStore returns an in-memory-sqlite-backed *state.Store, for
// pluginHost tests exercising the state capability gate.
func newHostTestStateStore(t *testing.T) *state.Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	st, err := state.NewStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("new state store: %v", err)
	}
	return st
}

// assertPermissionDenied asserts err is a gRPC PermissionDenied status whose
// message matches the frozen contract's wire-level format:
// "capability '<name>' not declared".
func assertPermissionDenied(t *testing.T, err error, capability string) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want PermissionDenied")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("err = %v, want a gRPC status error", err)
	}
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("code = %v, want %v", st.Code(), codes.PermissionDenied)
	}
	want := "capability '" + capability + "' not declared"
	if st.Message() != want {
		t.Fatalf("message = %q, want %q", st.Message(), want)
	}
}

func TestPluginHost_GetState_DeniedWithoutStateCapability(t *testing.T) {
	h := &pluginHost{pluginID: "p1", state: newHostTestStateStore(t)}
	_, _, err := h.GetState(context.Background(), "instance", "", "k")
	assertPermissionDenied(t, err, "state")
}

func TestPluginHost_SetState_DeniedWithoutStateCapability(t *testing.T) {
	h := &pluginHost{pluginID: "p1", state: newHostTestStateStore(t)}
	err := h.SetState(context.Background(), "instance", "", "k", map[string]any{"x": 1.0})
	assertPermissionDenied(t, err, "state")
}

func TestPluginHost_DeleteState_DeniedWithoutStateCapability(t *testing.T) {
	h := &pluginHost{pluginID: "p1", state: newHostTestStateStore(t)}
	err := h.DeleteState(context.Background(), "instance", "", "k")
	assertPermissionDenied(t, err, "state")
}

func TestPluginHost_ListState_DeniedWithoutStateCapability(t *testing.T) {
	h := &pluginHost{pluginID: "p1", state: newHostTestStateStore(t)}
	_, err := h.ListState(context.Background(), "instance", "")
	assertPermissionDenied(t, err, "state")
}

func TestPluginHost_RevealSecret_DeniedWithoutSecretsCapability(t *testing.T) {
	secrets := newFakeSecretRevealer()
	secrets.set("ref", "shh")
	h := &pluginHost{pluginID: "p1", secrets: secrets}
	_, err := h.RevealSecret(context.Background(), "ref")
	assertPermissionDenied(t, err, "secrets")
}

// TestPluginHost_StateMethods_SucceedWithStateCapability proves the state
// RPCs actually work end to end (through the real state.Store) once the
// capability is declared, and that state is scoped to this host's own
// pluginID: a second pluginHost for a different id never sees it.
func TestPluginHost_StateMethods_SucceedWithStateCapability(t *testing.T) {
	st := newHostTestStateStore(t)
	h := &pluginHost{pluginID: "p1", capabilities: manifest.Capabilities{State: true}, state: st}
	ctx := context.Background()

	if err := h.SetState(ctx, "instance", "", "k", map[string]any{"x": 1.0}); err != nil {
		t.Fatalf("SetState() unexpected error: %v", err)
	}

	value, found, err := h.GetState(ctx, "instance", "", "k")
	if err != nil {
		t.Fatalf("GetState() unexpected error: %v", err)
	}
	if !found {
		t.Fatal("GetState() found = false, want true after SetState")
	}
	if value["x"] != 1.0 {
		t.Fatalf("GetState() value = %v, want {x: 1}", value)
	}

	entries, err := h.ListState(ctx, "instance", "")
	if err != nil {
		t.Fatalf("ListState() unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "k" {
		t.Fatalf("ListState() = %v, want one entry for key %q", entries, "k")
	}

	if err := h.DeleteState(ctx, "instance", "", "k"); err != nil {
		t.Fatalf("DeleteState() unexpected error: %v", err)
	}
	if _, found, err := h.GetState(ctx, "instance", "", "k"); err != nil || found {
		t.Fatalf("GetState() after DeleteState found=%v err=%v, want found=false", found, err)
	}

	// A different plugin id must never see p1's state.
	other := &pluginHost{pluginID: "p2", capabilities: manifest.Capabilities{State: true}, state: st}
	if err := h.SetState(ctx, "instance", "", "shared-key", map[string]any{"v": true}); err != nil {
		t.Fatalf("SetState() unexpected error: %v", err)
	}
	if _, found, err := other.GetState(ctx, "instance", "", "shared-key"); err != nil || found {
		t.Fatalf("plugin p2 saw p1's state: found=%v err=%v, want found=false", found, err)
	}
}

// TestPluginHost_RevealSecret_SucceedsWithSecretsCapability proves
// RevealSecret reaches the wired SecretRevealer once the capability is
// declared.
func TestPluginHost_RevealSecret_SucceedsWithSecretsCapability(t *testing.T) {
	secrets := newFakeSecretRevealer()
	secrets.set("ref", "cleartext")
	h := &pluginHost{
		pluginID:     "p1",
		capabilities: manifest.Capabilities{Secrets: true},
		secrets:      secrets,
	}
	got, err := h.RevealSecret(context.Background(), "ref")
	if err != nil {
		t.Fatalf("RevealSecret() unexpected error: %v", err)
	}
	if got != "cleartext" {
		t.Fatalf("RevealSecret() = %q, want %q", got, "cleartext")
	}
}

// TestPluginHost_RevealSecret_ErrorsWithoutVaultWired proves the "vault not
// configured" branch is reachable even when the capability is declared, for
// a Service constructed without SetSecrets (e.g. some tests).
func TestPluginHost_RevealSecret_ErrorsWithoutVaultWired(t *testing.T) {
	h := &pluginHost{pluginID: "p1", capabilities: manifest.Capabilities{Secrets: true}}
	_, err := h.RevealSecret(context.Background(), "ref")
	if err == nil {
		t.Fatal("RevealSecret() with no vault wired unexpectedly succeeded")
	}
}

func TestPluginHost_EmitEvent_PublishesOnPluginNamespacedSubject(t *testing.T) {
	log := testLogger(t)
	memBus := bus.NewMemoryEventBus(log)
	t.Cleanup(memBus.Close)

	received := make(chan *bus.Event, 1)
	sub, err := memBus.Subscribe("plugin.p1.widget.created", func(_ context.Context, e *bus.Event) error {
		received <- e
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe() unexpected error: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	h := &pluginHost{pluginID: "p1", bus: memBus}
	if err := h.EmitEvent(context.Background(), "widget.created", map[string]any{"id": "w1"}); err != nil {
		t.Fatalf("EmitEvent() unexpected error: %v", err)
	}

	select {
	case e := <-received:
		if e.Type != "plugin.p1.widget.created" {
			t.Fatalf("event.Type = %q, want %q", e.Type, "plugin.p1.widget.created")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for EmitEvent's publish to reach the subscriber")
	}
}

// TestPluginHost_EmitEvent_NilBusIsANoOp pins the documented "no event bus
// wired" no-op contract (early boot, or a test Service without one).
func TestPluginHost_EmitEvent_NilBusIsANoOp(t *testing.T) {
	h := &pluginHost{pluginID: "p1"}
	if err := h.EmitEvent(context.Background(), "widget.created", nil); err != nil {
		t.Fatalf("EmitEvent() with nil bus unexpected error: %v", err)
	}
}
