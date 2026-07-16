package runtime

import (
	"context"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fakeHost is a minimal in-memory pluginsdk.Host for end-to-end tests: it
// only implements SetState (with a channel so tests can synchronize on
// delivery without a sleep), and errors on everything else.
type fakeHost struct {
	mu     sync.Mutex
	states map[string]map[string]any
	setCh  chan struct{}
}

func newFakeHost() *fakeHost {
	return &fakeHost{states: map[string]map[string]any{}, setCh: make(chan struct{}, 16)}
}

func (h *fakeHost) GetState(context.Context, string, string, string) (map[string]any, bool, error) {
	return nil, false, nil
}

func (h *fakeHost) SetState(_ context.Context, scope, scopeID, key string, value map[string]any) error {
	h.mu.Lock()
	h.states[scope+"|"+scopeID+"|"+key] = value
	h.mu.Unlock()
	select {
	case h.setCh <- struct{}{}:
	default:
	}
	return nil
}

func (h *fakeHost) DeleteState(context.Context, string, string, string) error { return nil }
func (h *fakeHost) ListState(context.Context, string, string) ([]pluginsdk.StateEntry, error) {
	return nil, nil
}
func (h *fakeHost) RevealSecret(context.Context, string) (string, error)    { return "", nil }
func (h *fakeHost) EmitEvent(context.Context, string, map[string]any) error { return nil }

func (h *fakeHost) get(scope, scopeID, key string) (map[string]any, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	v, ok := h.states[scope+"|"+scopeID+"|"+key]
	return v, ok
}

// buildFixtureRecord copies the real fixture plugin binary into a fresh
// InstallPath and returns a *store.Record ready for Manager.Start,
// skipping the test if -short suppressed the fixture build.
func buildFixtureRecord(t *testing.T, id string) *store.Record {
	t.Helper()
	if fixtureBinPath == "" {
		t.Skip("fixture plugin binary not built (test run with -short)")
	}

	installPath := t.TempDir()
	platformKey := goruntime.GOOS + "-" + goruntime.GOARCH
	relExec := filepath.Join("server", "plugin-"+platformKey)
	destExec := filepath.Join(installPath, relExec)
	if err := os.MkdirAll(filepath.Dir(destExec), 0o755); err != nil {
		t.Fatalf("mkdir server dir: %v", err)
	}
	copyFile(t, fixtureBinPath, destExec, 0o755)

	return &store.Record{
		Manifest: manifest.Manifest{
			ID:         id,
			APIVersion: 1,
			Version:    "1.0.0",
			Runtime: manifest.Runtime{
				Type:        "binary",
				Executables: map[string]string{platformKey: relExec},
			},
		},
		Status:      store.StatusRegistered,
		InstallPath: installPath,
	}
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

func TestManager_StartDeliverEventInvokeToolStop(t *testing.T) {
	rec := buildFixtureRecord(t, "kandev-fixture-plugin")
	pluginsDir := t.TempDir()
	host := newFakeHost()

	m := NewManager(pluginsDir, nil, testLogger(t))
	t.Cleanup(m.StopAll)

	ctx := context.Background()
	if err := m.Start(ctx, rec, func(string) pluginsdk.Host { return host }); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}
	if !m.Running(rec.ID) {
		t.Fatal("Running() = false right after a successful Start()")
	}

	remote, ok := m.Get(rec.ID)
	if !ok {
		t.Fatal("Get() ok = false right after a successful Start()")
	}

	t.Run("InvokeTool echoes input over the real subprocess", func(t *testing.T) {
		resp, err := remote.InvokeTool(ctx, &pluginsdk.ToolRequest{ToolName: "echo", Input: map[string]any{"x": float64(1)}})
		if err != nil {
			t.Fatalf("InvokeTool() unexpected error: %v", err)
		}
		if resp.Output["x"] != float64(1) {
			t.Fatalf("InvokeTool().Output = %v, want echoed input", resp.Output)
		}
	})

	t.Run("DeliverEvent reaches the plugin, which calls back into Host.SetState", func(t *testing.T) {
		err := remote.DeliverEvent(ctx, &pluginsdk.Event{EventID: "e1", EventType: "task.created"})
		if err != nil {
			t.Fatalf("DeliverEvent() unexpected error: %v", err)
		}

		select {
		case <-host.setCh:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for the plugin's Host.SetState callback")
		}

		value, found := host.get("instance", "", "last_event")
		if !found {
			t.Fatal("fakeHost never recorded the delivered event")
		}
		if value["event_type"] != "task.created" {
			t.Fatalf("recorded event_type = %v, want %q", value["event_type"], "task.created")
		}
	})

	t.Run("Ping succeeds against the live subprocess", func(t *testing.T) {
		if err := m.Ping(rec.ID); err != nil {
			t.Fatalf("Ping() unexpected error: %v", err)
		}
	})

	m.Stop(rec.ID)
	if m.Running(rec.ID) {
		t.Fatal("Running() = true after Stop()")
	}
}

func TestManager_CrashTriggersAutomaticRestart(t *testing.T) {
	rec := buildFixtureRecord(t, "kandev-fixture-plugin-crash")
	pluginsDir := t.TempDir()
	host := newFakeHost()

	var mu sync.Mutex
	var transitions []bool
	onStatusChange := func(_ string, healthy bool) {
		mu.Lock()
		transitions = append(transitions, healthy)
		mu.Unlock()
	}

	m := NewManager(pluginsDir, onStatusChange, testLogger(t),
		WithPingInterval(20*time.Millisecond),
		WithMaxConsecutiveFailures(1),
		WithRestartBackoff([]time.Duration{10 * time.Millisecond}),
		WithMaxRestartAttempts(3),
	)
	t.Cleanup(m.StopAll)

	ctx := context.Background()
	if err := m.Start(ctx, rec, func(string) pluginsdk.Host { return host }); err != nil {
		t.Fatalf("Start() unexpected error: %v", err)
	}

	remote, ok := m.Get(rec.ID)
	if !ok {
		t.Fatal("Get() ok = false right after Start()")
	}
	// Invoking the "crash" tool exits the subprocess immediately; the RPC
	// call itself is expected to error (connection dropped mid-call) or
	// hang up cleanly - either way we only assert on the recovery below.
	_, _ = remote.InvokeTool(ctx, &pluginsdk.ToolRequest{ToolName: "crash"})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(transitions) >= 2 && !transitions[0] && transitions[1]
	}, 10*time.Second, 20*time.Millisecond,
		"onStatusChange should observe degraded (false) then recovered (true) after the crash")
}
