package instance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

const concurrencyTestWait = 5 * time.Second

// bindablePortRange returns a base port whose next size ports can all be bound
// on loopback right now.
func bindablePortRange(t *testing.T, size int) int {
	t.Helper()
	for attempt := 0; attempt < 10; attempt++ {
		probe, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen for port range: %v", err)
		}
		base := probe.Addr().(*net.TCPAddr).Port
		_ = probe.Close()
		if base+size-1 > 65535 {
			continue
		}
		ok := true
		for port := base; port < base+size; port++ {
			ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
			if err != nil {
				ok = false
				break
			}
			_ = ln.Close()
		}
		if ok {
			return base
		}
	}
	t.Skip("could not find a bindable loopback port range")
	return 0
}

// newConcurrencyTestManager returns a manager whose instances answer 200 OK and
// which is shut down when the test ends. The shutdown is bounded so a stuck
// creation fails the test instead of hanging the package.
func newConcurrencyTestManager(t *testing.T, ports int) *Manager {
	t.Helper()
	base := bindablePortRange(t, ports)
	mgr := NewManager(&config.Config{
		Ports:    config.PortConfig{Base: base, Max: base + ports - 1},
		Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP},
	}, newTestLogger(t))
	t.Cleanup(func() {
		if !finishesWithin(func() { _ = mgr.Shutdown(context.Background()) }) {
			t.Error("Shutdown did not finish during cleanup")
		}
	})
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})
	return mgr
}

// startupGate holds every creation at the end of its startup until released.
type startupGate struct {
	reached     chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func holdStartup(t *testing.T, mgr *Manager) *startupGate {
	t.Helper()
	gate := &startupGate{reached: make(chan struct{}, 16), release: make(chan struct{})}
	mgr.afterTrackerStart = func() {
		gate.reached <- struct{}{}
		<-gate.release
	}
	t.Cleanup(gate.open)
	return gate
}

func (g *startupGate) open() {
	g.releaseOnce.Do(func() { close(g.release) })
}

// waitReached reports whether n creations reached the gate within the bound.
func (g *startupGate) waitReached(n int) bool {
	timeout := time.After(concurrencyTestWait)
	for i := 0; i < n; i++ {
		select {
		case <-g.reached:
		case <-timeout:
			return false
		}
	}
	return true
}

// finishesWithin runs fn and reports whether it returned within the bound.
func finishesWithin(fn func()) bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
		return true
	case <-time.After(concurrencyTestWait):
		return false
	}
}

// receiveWithin returns the next value from ch, or false after the bound.
func receiveWithin[T any](ch <-chan T) (T, bool) {
	select {
	case v := <-ch:
		return v, true
	case <-time.After(concurrencyTestWait):
		var zero T
		return zero, false
	}
}

func TestConcurrentCreationsOverlapStartup(t *testing.T) {
	const concurrent = 4
	mgr := newConcurrencyTestManager(t, concurrent+4)
	gate := holdStartup(t, mgr)

	var wg sync.WaitGroup
	resps := make([]*CreateResponse, concurrent)
	errs := make([]error, concurrent)
	for i := range concurrent {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resps[i], errs[i] = mgr.CreateInstance(t.Context(), &CreateRequest{
				ID:            fmt.Sprintf("concurrent-%d", i),
				WorkspacePath: t.TempDir(),
			})
		}(i)
	}

	overlapped := gate.waitReached(concurrent)
	gate.open()
	if !finishesWithin(wg.Wait) {
		t.Fatal("creations did not finish after the gate opened")
	}
	if !overlapped {
		t.Fatalf("fewer than %d creations reached the end of startup together", concurrent)
	}

	client := &http.Client{Timeout: concurrencyTestWait}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("CreateInstance %d: %v", i, err)
		}
		resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", resps[i].Port))
		if err != nil {
			t.Fatalf("published instance %d is not serving: %v", i, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("instance %d status = %d, want 200", i, resp.StatusCode)
		}
	}
}

func TestCreateInstanceRejectsDuplicateIDDuringStartup(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	gate := holdStartup(t, mgr)

	firstErr := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(t.Context(), &CreateRequest{ID: "dup", WorkspacePath: t.TempDir()})
		firstErr <- err
	}()
	if !gate.waitReached(1) {
		t.Fatal("first creation did not reach the end of startup")
	}

	var dupErr error
	prompt := finishesWithin(func() {
		_, dupErr = mgr.CreateInstance(t.Context(), &CreateRequest{ID: "dup", WorkspacePath: t.TempDir()})
	})
	gate.open()
	if !prompt {
		t.Fatal("duplicate creation waited for the first creation's startup")
	}
	if dupErr == nil {
		t.Fatal("duplicate creation succeeded while the first owned the ID")
	}
	if err, ok := receiveWithin(firstErr); !ok {
		t.Fatal("first creation did not finish")
	} else if err != nil {
		t.Fatalf("first creation: %v", err)
	}
	if _, ok := mgr.GetInstance("dup"); !ok {
		t.Error("first creation was not published")
	}
}

func TestShutdownWaitsForInProgressCreation(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	gate := holdStartup(t, mgr)

	createDone := make(chan struct{})
	go func() {
		defer close(createDone)
		_, _ = mgr.CreateInstance(t.Context(), &CreateRequest{ID: "inflight", WorkspacePath: t.TempDir()})
	}()
	if !gate.waitReached(1) {
		t.Fatal("creation did not reach the end of startup")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- mgr.Shutdown(context.Background()) }()
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned while a creation was in progress: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	gate.open()
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(concurrencyTestWait):
		t.Fatal("Shutdown did not finish after the creation was released")
	}
	if _, ok := receiveWithin(createDone); !ok {
		t.Fatal("creation did not finish after Shutdown")
	}

	mgr.mu.RLock()
	published, provisional := len(mgr.instances), len(mgr.provisional)
	mgr.mu.RUnlock()
	if published != 0 || provisional != 0 {
		t.Errorf("after Shutdown: %d published, %d provisional; want none", published, provisional)
	}
	mgr.portAlloc.mu.Lock()
	reserved := len(mgr.portAlloc.allocated)
	mgr.portAlloc.mu.Unlock()
	if reserved != 0 {
		t.Errorf("after Shutdown: %d ports reserved, want 0", reserved)
	}
}

// TestCreateInstanceNotPublishedWhenCancelledBeforePublication cancels the
// caller after the post-startup context check, while the manager lock that
// publication needs is held elsewhere.
func TestCreateInstanceNotPublishedWhenCancelledBeforePublication(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	handlerBuilt := make(chan struct{})
	resume := make(chan struct{})
	mgr.SetServerFactory(func(*config.InstanceConfig, *process.Manager, *logger.Logger) http.Handler {
		close(handlerBuilt)
		<-resume
		return http.NotFoundHandler()
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	createErr := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(ctx, &CreateRequest{ID: "cancelled", WorkspacePath: t.TempDir()})
		createErr <- err
	}()
	select {
	case <-handlerBuilt:
	case <-time.After(concurrencyTestWait):
		close(resume)
		t.Fatal("creation did not reach handler construction")
	}

	mgr.mu.Lock()
	cancel()
	close(resume)
	time.Sleep(50 * time.Millisecond)
	mgr.mu.Unlock()

	select {
	case err := <-createErr:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want wrapped context.Canceled", err)
		}
	case <-time.After(concurrencyTestWait):
		t.Fatal("creation did not finish")
	}
	if _, ok := mgr.GetInstance("cancelled"); ok {
		t.Fatal("instance was published after its caller gave up")
	}
	if !finishesWithin(func() { _ = mgr.Shutdown(context.Background()) }) {
		t.Fatal("Shutdown did not finish")
	}
	mgr.portAlloc.mu.Lock()
	reserved := len(mgr.portAlloc.allocated)
	mgr.portAlloc.mu.Unlock()
	if reserved != 0 {
		t.Errorf("%d ports reserved after the cancelled creation was cleaned up, want 0", reserved)
	}
}

func TestOtherInstanceOperationsDoNotWaitForStartup(t *testing.T) {
	mgr := newConcurrencyTestManager(t, 4)
	if _, err := mgr.CreateInstance(t.Context(), &CreateRequest{ID: "other", WorkspacePath: t.TempDir()}); err != nil {
		t.Fatalf("create other instance: %v", err)
	}
	gate := holdStartup(t, mgr)

	slowErr := make(chan error, 1)
	go func() {
		_, err := mgr.CreateInstance(t.Context(), &CreateRequest{ID: "slow", WorkspacePath: t.TempDir()})
		slowErr <- err
	}()
	if !gate.waitReached(1) {
		t.Fatal("slow creation did not reach the end of startup")
	}

	var found bool
	var listed int
	var stopErr error
	prompt := finishesWithin(func() {
		_, found = mgr.GetInstance("other")
		listed = len(mgr.ListInstances())
		stopErr = mgr.StopInstance(context.Background(), "other")
	})
	gate.open()
	if !prompt {
		t.Fatal("operations on another instance waited for a creation's startup")
	}
	if !found || listed != 1 || stopErr != nil {
		t.Errorf("found=%v listed=%d stopErr=%v; want true, 1, nil", found, listed, stopErr)
	}
	if err, ok := receiveWithin(slowErr); !ok {
		t.Fatal("slow creation did not finish")
	} else if err != nil {
		t.Fatalf("slow creation: %v", err)
	}
}
