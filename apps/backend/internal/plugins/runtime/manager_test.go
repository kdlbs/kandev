package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// fakeHost is a minimal in-memory pluginsdk.Host for end-to-end tests: it
// only implements SetState (with a channel so tests can synchronize on
// delivery without a sleep), and errors on everything else. It embeds
// UnimplementedHostData to satisfy the Host data API (ADR 0042)
// sub-accessors without wiring them; see docs/plans/plugins/host-data-api/
// task-04-host-data-impl.md for the real implementation.
type fakeHost struct {
	pluginsdk.UnimplementedHostData

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

// TestManager_ConcurrentStartOnlySpawnsOnce pins the fix for the TOCTOU
// double-spawn race: two Start calls for the same plugin id racing the
// "already running" check must not both reach spawnFn. The first goroutine
// is deliberately paused *inside* its (slow) spawn — the exact window the
// old code released the manager lock across — before the second goroutine's
// startProcess call is allowed to race in.
func TestManager_ConcurrentStartOnlySpawnsOnce(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t))
	t.Cleanup(m.StopAll)

	var spawnCalls int32
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once

	spawnFn := func() (spawnedProcess, error) {
		if atomic.AddInt32(&spawnCalls, 1) == 1 {
			closeOnce.Do(func() { close(firstEntered) })
			<-release // hold the first spawn open so the second Start can race in
		}
		return &fakeSpawnedProcess{}, nil
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		errs[0] = m.startProcess("race-plugin", spawnFn)
	}()

	<-firstEntered // the first Start is now mid-spawn, before it has registered anything

	errs[1] = m.startProcess("race-plugin", spawnFn)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&spawnCalls); got != 1 {
		t.Fatalf("spawnFn called %d times for a concurrent Start of the same id, want exactly 1 (no double-spawn)", got)
	}
	if errs[0] != nil {
		t.Fatalf("first startProcess() unexpected error: %v", errs[0])
	}
	if errs[1] == nil {
		t.Fatal("second concurrent startProcess() for the same id succeeded, want an error (already starting)")
	}
	if !m.Running("race-plugin") {
		t.Fatal("Running() = false after the winning Start() completed")
	}
}

// TestManager_StopDuringStartAbortsAndLeavesNoOrphanProcess pins the fix for
// the Stop-during-Start orphan race: Stop previously only consulted
// m.processes, so a Disable/Uninstall racing an in-flight Start was a
// silent no-op — the spawn then completed and inserted a live supervised
// process for a plugin whose record/files may already be gone by the time
// the spawn finished. claimStart's "starting" reservation now doubles as
// the hook Stop uses to request an abort: startProcess checks it right
// after the (slow) spawn succeeds and, if set, kills the freshly spawned
// process and never inserts it into m.processes.
func TestManager_StopDuringStartAbortsAndLeavesNoOrphanProcess(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t))
	t.Cleanup(m.StopAll)

	entered := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	spawned := &fakeSpawnedProcess{}

	spawnFn := func() (spawnedProcess, error) {
		closeOnce.Do(func() { close(entered) })
		<-release // hold the spawn open until the test has called Stop
		return spawned, nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.startProcess("racing-plugin", spawnFn)
	}()

	<-entered // the spawn is in flight, before startProcess has registered anything

	m.Stop("racing-plugin") // races the in-flight Start: must not be a no-op
	close(release)          // let the spawn finish

	err := <-errCh
	if err == nil {
		t.Fatal("startProcess() expected an error when Stop() raced an in-flight Start, got nil")
	}
	if m.Running("racing-plugin") {
		t.Fatal("Running() = true after Stop() raced an in-flight Start; an orphan process was left registered")
	}
	if !spawned.isKilled() {
		t.Fatal("the spawned process was never killed after Stop() raced an in-flight Start (orphan subprocess)")
	}
}

// TestManager_StopAllDuringStartAbortsAndLeavesNoOrphanProcess is the same
// race as TestManager_StopDuringStartAbortsAndLeavesNoOrphanProcess, but
// through StopAll (graceful shutdown), which previously only iterated
// m.processes too.
func TestManager_StopAllDuringStartAbortsAndLeavesNoOrphanProcess(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t))
	t.Cleanup(m.StopAll)

	entered := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	spawned := &fakeSpawnedProcess{}

	spawnFn := func() (spawnedProcess, error) {
		closeOnce.Do(func() { close(entered) })
		<-release
		return spawned, nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- m.startProcess("racing-plugin-2", spawnFn)
	}()

	<-entered

	m.StopAll()
	close(release)

	err := <-errCh
	if err == nil {
		t.Fatal("startProcess() expected an error when StopAll() raced an in-flight Start, got nil")
	}
	if m.Running("racing-plugin-2") {
		t.Fatal("Running() = true after StopAll() raced an in-flight Start; an orphan process was left registered")
	}
	if !spawned.isKilled() {
		t.Fatal("the spawned process was never killed after StopAll() raced an in-flight Start (orphan subprocess)")
	}
}

// TestManager_StartAfterExhaustionRespawns pins the fix that lets a fresh
// Start cleanly replace an entry whose restart attempts were exhausted
// (gaveUp==true, current==nil): before the fix, Start saw the stale map
// entry and always errored "already running" instead of respawning.
func TestManager_StartAfterExhaustionRespawns(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t),
		WithPingInterval(5*time.Millisecond),
		WithMaxConsecutiveFailures(1),
		WithRestartBackoff([]time.Duration{0}),
		WithMaxRestartAttempts(1),
	)
	t.Cleanup(m.StopAll)

	var mu sync.Mutex
	firstSpawnDone := false
	spawnFn := func() (spawnedProcess, error) {
		mu.Lock()
		defer mu.Unlock()
		if !firstSpawnDone {
			firstSpawnDone = true
			return &fakeSpawnedProcess{exited: true}, nil // dies immediately, forcing an exhausting restart loop
		}
		return nil, errors.New("respawn always fails") // every restart attempt fails too
	}

	if err := m.startProcess("exhaust-plugin", spawnFn); err != nil {
		t.Fatalf("startProcess() unexpected error: %v", err)
	}

	// Wait for the supervision loop to actually finish giving up (gaveUp),
	// not just for current to go nil — current is cleared at the start of
	// handleFailureAndRestart, before the (failing) respawn attempt(s) run,
	// so polling only Running()==false races the loop's own completion.
	require.Eventually(t, func() bool {
		m.mu.Lock()
		p, ok := m.processes["exhaust-plugin"]
		m.mu.Unlock()
		return ok && p.isGaveUp()
	}, 2*time.Second, 5*time.Millisecond,
		"plugin's process should have given up once restart attempts are exhausted")

	healthy := &fakeSpawnedProcess{}
	respawnCalls := 0
	freshSpawnFn := func() (spawnedProcess, error) {
		respawnCalls++
		return healthy, nil
	}
	if err := m.startProcess("exhaust-plugin", freshSpawnFn); err != nil {
		t.Fatalf("startProcess() after exhaustion unexpected error: %v", err)
	}
	if respawnCalls != 1 {
		t.Fatalf("respawn spawnFn called %d times, want 1", respawnCalls)
	}
	if !m.Running("exhaust-plugin") {
		t.Fatal("Running() = false after a fresh Start() following exhaustion")
	}
}

// TestManager_DefaultStartTimeoutIs30s pins the fix for bounded startup:
// Manager.Start previously spawned via a hcplugin.Client with no configured
// StartTimeout, defaulting to go-plugin's own 1-minute handshake timeout, so
// a hung plugin binary could block Enable/Install for up to a minute. A
// fresh Manager must default to a much shorter, explicit bound.
func TestManager_DefaultStartTimeoutIs30s(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t))
	if m.startTimeout != 30*time.Second {
		t.Fatalf("default startTimeout = %v, want 30s", m.startTimeout)
	}
}

// TestManager_WithStartTimeoutOverridesDefault pins the escape hatch for
// callers (and this test file) that need a different bound.
func TestManager_WithStartTimeoutOverridesDefault(t *testing.T) {
	m := NewManager(t.TempDir(), nil, testLogger(t), WithStartTimeout(5*time.Second))
	if m.startTimeout != 5*time.Second {
		t.Fatalf("startTimeout = %v, want 5s (WithStartTimeout override)", m.startTimeout)
	}
}

// TestManager_StartFailsFastOnHandshakeTimeout proves the configured
// startTimeout actually reaches the underlying hcplugin.Client: an
// unreasonably short timeout must make Start() return an error quickly
// instead of ever completing the handshake, even against a real, working
// fixture binary.
func TestManager_StartFailsFastOnHandshakeTimeout(t *testing.T) {
	rec := buildFixtureRecord(t, "kandev-fixture-plugin-timeout")
	m := NewManager(t.TempDir(), nil, testLogger(t), WithStartTimeout(1*time.Nanosecond))
	t.Cleanup(m.StopAll)

	started := time.Now()
	err := m.Start(context.Background(), rec, func(string) pluginsdk.Host { return newFakeHost() })
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("Start() with a 1ns startTimeout unexpectedly succeeded")
	}
	if elapsed > 10*time.Second {
		t.Fatalf("Start() took %v to fail, want well under go-plugin's 1-minute default handshake timeout", elapsed)
	}
}
