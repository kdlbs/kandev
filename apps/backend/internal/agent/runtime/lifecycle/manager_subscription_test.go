package lifecycle

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/common/logger"
)

// newTestManagerForAggregator builds a minimal Manager with just the bits the
// aggregator needs: an executionStore and a logger. We don't need the real
// dependency graph because the aggregator only reads execution metadata and
// dispatches to AgentExecution.GetAgentCtlClient (which we leave nil — pushAsync
// will then no-op, and the test inspects sessionModes/lastPushed directly).
func newTestManagerForAggregator(t *testing.T) *Manager {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	mgr := &Manager{
		logger:         log,
		executionStore: NewExecutionStore(),
	}
	mgr.pollAggregator = newWorkspacePollAggregator(mgr)
	return mgr
}

// addExecution attaches a session→workspace mapping so the aggregator can
// resolve it. agentctl is left nil; pushAsync becomes a no-op for these tests.
func addExecution(mgr *Manager, sessionID, workspacePath string) {
	mgr.executionStore.Add(&AgentExecution{
		ID:            "exec-" + sessionID,
		SessionID:     sessionID,
		WorkspacePath: workspacePath,
	})
}

func TestAggregator_SingleSessionPushesItsMode(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")

	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)

	// Inspect the recorded last-pushed state to confirm the workspace mode.
	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeFast {
		t.Errorf("last pushed mode = %q, want fast", got)
	}
}

func TestAggregator_TwoSessionsSameWorkspace_TakesMax(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")
	addExecution(mgr, "s2", "/tmp/ws1")

	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeSlow)
	mgr.pollAggregator.HandleSessionMode("s2", WorkspacePollModeFast)

	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeFast {
		t.Errorf("with one fast and one slow session, expected fast; got %q", got)
	}
}

func TestAggregator_DowngradeAfterFastSessionLeaves(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")
	addExecution(mgr, "s2", "/tmp/ws1")

	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)
	mgr.pollAggregator.HandleSessionMode("s2", WorkspacePollModeSlow)

	// s1 unfocuses (drops to slow). Workspace should now be slow.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeSlow)

	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeSlow {
		t.Errorf("after fast session drops to slow, workspace mode = %q, want slow", got)
	}
}

func TestAggregator_AllSessionsPausedYieldsPaused(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")
	addExecution(mgr, "s2", "/tmp/ws1")

	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeSlow)
	mgr.pollAggregator.HandleSessionMode("s2", WorkspacePollModeSlow)
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModePaused)
	mgr.pollAggregator.HandleSessionMode("s2", WorkspacePollModePaused)

	mgr.pollAggregator.mu.Lock()
	_, hasEntry := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	sessionEntries := len(mgr.pollAggregator.sessionModes)
	mgr.pollAggregator.mu.Unlock()

	// When the workspace becomes paused we drop the lastPushed entry to keep
	// the map bounded over a long-running gateway. The push itself happens via
	// pushAsync (test inspects the bookkeeping, not the actual RPC).
	if hasEntry {
		t.Errorf("expected lastPushed entry for /tmp/ws1 to be deleted when workspace fully paused")
	}
	// Same cleanup applies to per-session entries.
	if sessionEntries != 0 {
		t.Errorf("expected sessionModes to be empty when all sessions paused, got %d entries", sessionEntries)
	}
}

func TestAggregator_NoExecutionIsNoOp(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	// No execution registered — call should not panic, should not push anything.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)

	mgr.pollAggregator.mu.Lock()
	defer mgr.pollAggregator.mu.Unlock()
	if len(mgr.pollAggregator.lastPushed) != 0 {
		t.Errorf("expected no pushes when execution is absent, got %+v", mgr.pollAggregator.lastPushed)
	}
}

func TestAggregator_DifferentWorkspacesAreIndependent(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")
	addExecution(mgr, "s2", "/tmp/ws2")

	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)
	mgr.pollAggregator.HandleSessionMode("s2", WorkspacePollModeSlow)

	mgr.pollAggregator.mu.Lock()
	gotWS1 := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	gotWS2 := mgr.pollAggregator.lastPushed["/tmp/ws2"]
	mgr.pollAggregator.mu.Unlock()

	if gotWS1 != WorkspacePollModeFast {
		t.Errorf("ws1 mode = %q, want fast", gotWS1)
	}
	if gotWS2 != WorkspacePollModeSlow {
		t.Errorf("ws2 mode = %q, want slow", gotWS2)
	}
}

func TestAggregator_FlushSessionMode_AppliesCachedModeAfterExecutionReady(t *testing.T) {
	mgr := newTestManagerForAggregator(t)

	// Gateway sends focus BEFORE execution is registered — mode is cached,
	// not pushed.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)

	mgr.pollAggregator.mu.Lock()
	_, pushedBeforeReady := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()
	if pushedBeforeReady {
		t.Fatal("expected no lastPushed entry before execution exists")
	}

	// Execution is created (simulates waitForAgentctlReady completing).
	addExecution(mgr, "s1", "/tmp/ws1")
	mgr.pollAggregator.FlushSessionMode("s1")

	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeFast {
		t.Errorf("after FlushSessionMode, workspace mode = %q, want fast", got)
	}
}

func TestAggregator_FlushSessionMode_NoOpWhenNothingCached(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")

	// No prior HandleSessionMode call — nothing cached.
	mgr.pollAggregator.FlushSessionMode("s1")

	mgr.pollAggregator.mu.Lock()
	_, pushed := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()
	if pushed {
		t.Error("FlushSessionMode should not push anything when nothing cached")
	}
}

func TestAggregator_RedundantEventDoesNotRepush(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")

	// Repeated identical events should be a no-op for the workspace mode.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)

	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeFast {
		t.Errorf("after redundant events, workspace mode = %q, want fast", got)
	}
}

// mockSessionModeQuerier is a test double for SessionModeQuerier that returns
// a fixed mode for any session ID.
type mockSessionModeQuerier struct {
	mode WorkspacePollMode
}

func (m *mockSessionModeQuerier) GetSessionMode(_ string) WorkspacePollMode {
	return m.mode
}

func TestAggregator_FlushSessionMode_QueriesHubWhenWired(t *testing.T) {
	mgr := newTestManagerForAggregator(t)

	// Wire a mock hub that always reports fast.
	mock := &mockSessionModeQuerier{mode: WorkspacePollModeFast}
	mgr.SetSessionModeQuerier(mock)

	// Gateway sends focus BEFORE execution exists — cached but not pushed.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeSlow)

	mgr.pollAggregator.mu.Lock()
	_, pushedBeforeReady := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	mgr.pollAggregator.mu.Unlock()
	if pushedBeforeReady {
		t.Fatal("expected no lastPushed entry before execution exists")
	}

	// Execution becomes ready — FlushSessionMode should query the hub (fast),
	// not use the cached value (slow).
	addExecution(mgr, "s1", "/tmp/ws1")
	mgr.pollAggregator.FlushSessionMode("s1")

	mgr.pollAggregator.mu.Lock()
	got := mgr.pollAggregator.lastPushed["/tmp/ws1"]
	cachedMode := mgr.pollAggregator.sessionModes["s1"]
	mgr.pollAggregator.mu.Unlock()

	if got != WorkspacePollModeFast {
		t.Errorf("FlushSessionMode should use hub-queried mode; lastPushed = %q, want fast", got)
	}
	if cachedMode != WorkspacePollModeFast {
		t.Errorf("FlushSessionMode should update sessionModes to hub-queried mode; got %q, want fast", cachedMode)
	}
}

// addExecutionWithClient is like addExecution but wires a real agentctl.Client
// pointing at the supplied URL. Needed for tests that observe the HTTP RPC.
func addExecutionWithClient(t *testing.T, mgr *Manager, sessionID, workspacePath, agentctlURL string) {
	t.Helper()
	host, port := parseHTTPURL(t, agentctlURL)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	c := agentctl.NewClient(host, port, log)
	if err := mgr.executionStore.Add(&AgentExecution{
		ID:            "exec-" + sessionID,
		SessionID:     sessionID,
		WorkspacePath: workspacePath,
		agentctl:      c,
	}); err != nil {
		t.Fatalf("executionStore.Add: %v", err)
	}
}

func parseHTTPURL(t *testing.T, raw string) (host string, port int) {
	t.Helper()
	stripped := strings.TrimPrefix(raw, "http://")
	parts := strings.Split(stripped, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected URL shape: %q", raw)
	}
	host = parts[0]
	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("port parse: %v", err)
	}
	return host, port
}

// Regression for #982: slow-then-fast mode events must not leave the tracker stuck in slow.
func TestAggregator_PushAsync_LastWriteWinsUnderRace(t *testing.T) {
	type call struct {
		Mode string
		At   time.Time
	}
	var (
		mu         sync.Mutex
		calls      []call
		slowCalls  atomic.Int32
		slowDelay  = 80 * time.Millisecond
		serverSeen string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Mode string `json:"mode"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Mode == "slow" {
			slowCalls.Add(1)
			time.Sleep(slowDelay)
		}
		mu.Lock()
		calls = append(calls, call{Mode: body.Mode, At: time.Now()})
		serverSeen = body.Mode
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	mgr := newTestManagerForAggregator(t)
	addExecutionWithClient(t, mgr, "s1", "/tmp/ws1", srv.URL)

	// Fire subscribe (slow) then focus (fast) microseconds apart, matching
	// what the gateway does when a page loads.
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeSlow)
	mgr.pollAggregator.HandleSessionMode("s1", WorkspacePollModeFast)

	// Wait for both pushes to land — the server records each call, so we
	// can poll the recorded length instead of sleeping a fixed amount. The
	// old fire-and-forget impl sent both slow and fast (2 calls); the fix
	// may collapse slow before dispatch (1 call). Either way we wait until
	// the pusher goroutine has drained, plus a brief settle window for any
	// in-flight slow request to complete.
	deadline := time.NewTimer(slowDelay + 2*time.Second)
	defer deadline.Stop()
waitLoop:
	for {
		mu.Lock()
		n := len(calls)
		seen := serverSeen
		mu.Unlock()
		// 1 call is enough only when it's already "fast" — in the buggy
		// path a late-arriving slow can still overwrite, so wait for the
		// drain window or until both arrived.
		if n >= 2 || (n >= 1 && seen == "fast") {
			break
		}
		select {
		case <-deadline.C:
			break waitLoop
		case <-time.After(20 * time.Millisecond):
		}
	}
	// Add a brief settle so any racing slow push has time to overwrite if
	// the bug is present.
	time.Sleep(slowDelay + 100*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if serverSeen != "fast" {
		t.Errorf("agentctl tracker final mode = %q, want fast (calls: %+v)", serverSeen, calls)
	}
	// The fix collapses back-to-back slow→fast in the pending queue when the
	// pusher hasn't dispatched yet, so we expect at most one slow push. More
	// than one would mean the queue stopped collapsing (a regression).
	if n := slowCalls.Load(); n > 1 {
		t.Errorf("slow push count = %d, want <= 1 (queue should collapse duplicates)", n)
	}
}

// TestAggregator_ConcurrentEvents asserts no races. We don't assert ordering
// — only that the aggregator survives concurrent calls without data races
// (run with -race).
func TestAggregator_ConcurrentEvents(t *testing.T) {
	mgr := newTestManagerForAggregator(t)
	addExecution(mgr, "s1", "/tmp/ws1")
	addExecution(mgr, "s2", "/tmp/ws1")

	var wg sync.WaitGroup
	const goroutines = 20
	wg.Add(goroutines)
	modes := []WorkspacePollMode{WorkspacePollModeFast, WorkspacePollModeSlow, WorkspacePollModePaused}
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			session := "s1"
			if i%2 == 0 {
				session = "s2"
			}
			mgr.pollAggregator.HandleSessionMode(session, modes[i%len(modes)])
		}(i)
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		// no-op; we just want to be sure it returns
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent HandleSessionMode calls did not return in time")
	}
}
