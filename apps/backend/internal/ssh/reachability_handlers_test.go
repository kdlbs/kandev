package ssh

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/kandev/kandev/internal/common/logger"
	reachabilitypkg "github.com/kandev/kandev/internal/executors/reachability"
	"github.com/kandev/kandev/internal/task/models"
)

func testHandlerLogger() *logger.Logger {
	return logger.Default()
}

type fakeReachabilityExecutorFetcher struct {
	executors map[string]*models.Executor
}

func (f *fakeReachabilityExecutorFetcher) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	executor, ok := f.executors[id]
	if !ok {
		return nil, models.ErrExecutorNotFound
	}
	return executor, nil
}

type fakeReachabilityLister struct {
	executors []*models.Executor
	records   []*models.ExecutorReachability
	byID      map[string]*models.ExecutorReachability
}

func (f *fakeReachabilityLister) ListExecutors(_ context.Context) ([]*models.Executor, error) {
	return f.executors, nil
}

func (f *fakeReachabilityLister) ListExecutorReachability(_ context.Context) ([]*models.ExecutorReachability, error) {
	return f.records, nil
}

func (f *fakeReachabilityLister) GetExecutorReachability(_ context.Context, executorID string) (*models.ExecutorReachability, error) {
	if record, ok := f.byID[executorID]; ok {
		return record, nil
	}
	return nil, models.ErrExecutorReachabilityNotFound
}

func sshExecutor(id string, status models.ExecutorStatus, config map[string]string) *models.Executor {
	return &models.Executor{ID: id, Type: models.ExecutorTypeSSH, Status: status, Config: config}
}

func resolvableSSHConfig() map[string]string {
	return map[string]string{
		"ssh_host":             "10.0.0.1",
		"ssh_user":             "deploy",
		"ssh_host_fingerprint": "SHA256:test",
	}
}

// fakeReachabilityProber models the exact guarantee *reachability.Poller
// makes for concurrent callers of the same executor id: it coalesces
// through the same golang.org/x/sync/singleflight primitive the production
// Poller uses, so this test proves the handler's own plumbing (always
// routing through the one shared prober instance) rather than re-deriving
// singleflight's own coalescing guarantee, which is already covered by
// TestPollerProbeAndWait_CoalescesConcurrentCallsForSameExecutor.
type fakeReachabilityProber struct {
	group        singleflight.Group
	dialCount    int32
	callsEntered int32
	gate         chan struct{}
	result       reachabilitypkg.ProbeResult
	interval     int
}

func (f *fakeReachabilityProber) ProbeAndWait(executor *models.Executor) (reachabilitypkg.ProbeResult, bool) {
	atomic.AddInt32(&f.callsEntered, 1)
	v, _, _ := f.group.Do(executor.ID, func() (any, error) {
		atomic.AddInt32(&f.dialCount, 1)
		<-f.gate
		return f.result, nil
	})
	return v.(reachabilitypkg.ProbeResult), true
}

func (f *fakeReachabilityProber) EffectiveIntervalSeconds() int { return f.interval }

func waitForCallsEntered(t *testing.T, prober *fakeReachabilityProber, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&prober.callsEntered) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("callsEntered did not reach %d within 1s (got %d)", want, atomic.LoadInt32(&prober.callsEntered))
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002 (coalescing)
//
// Two overlapping probe requests for the same executor must share one dial
// and receive one identical record. Written first as a RED test against a
// handler that has not yet been implemented, per the task-04 work order.
func TestProbeReachability_CoalescesConcurrentProbes(t *testing.T) {
	executor := &models.Executor{ID: "exec-1", Type: models.ExecutorTypeSSH, Status: models.ExecutorStatusActive}
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	updatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	prober := &fakeReachabilityProber{
		gate: make(chan struct{}),
		result: reachabilitypkg.ProbeResult{
			Record: &models.ExecutorReachability{
				ExecutorID: "exec-1",
				State:      models.ExecutorReachabilityStateReachable,
				UpdatedAt:  updatedAt,
			},
			Persisted: true,
		},
		interval: 30,
	}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), nil, prober)

	const callers = 2
	dtos := make([]*reachabilitypkg.RecordDTO, callers)
	statuses := make([]int, callers)
	errs := make([]error, callers)

	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dto, status, err := h.probeReachability(context.Background(), "exec-1")
			dtos[i] = dto
			statuses[i] = status
			errs[i] = err
		}(i)
	}

	waitForCallsEntered(t, prober, callers)
	close(prober.gate)
	wg.Wait()

	if got := atomic.LoadInt32(&prober.dialCount); got != 1 {
		t.Fatalf("dialCount = %d, want 1 (concurrent probes must coalesce into one dial)", got)
	}
	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, errs[i])
		}
		if statuses[i] != 200 {
			t.Fatalf("caller %d: status = %d, want 200", i, statuses[i])
		}
	}
	if !reflect.DeepEqual(dtos[0], dtos[1]) {
		t.Fatalf("responses differ: %+v vs %+v", dtos[0], dtos[1])
	}
	if dtos[0].State != string(models.ExecutorReachabilityStateReachable) {
		t.Fatalf("state = %q, want %q", dtos[0].State, models.ExecutorReachabilityStateReachable)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.5
func TestGetReachability_NoRecordAndResolvableConfigReturnsUnknown(t *testing.T) {
	executor := sshExecutor("exec-1", models.ExecutorStatusActive, resolvableSSHConfig())
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	lister := &fakeReachabilityLister{}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), lister, nil)

	dto, status, err := h.getReachability(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if dto.State != string(models.ExecutorReachabilityStateUnknown) {
		t.Fatalf("state = %q, want %q", dto.State, models.ExecutorReachabilityStateUnknown)
	}
	if dto.Reason != "" {
		t.Fatalf("reason = %q, want empty", dto.Reason)
	}
	if dto.Persisted {
		t.Fatal("persisted = true for a synthesized never-probed record")
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.5
//
// An ssh executor whose config cannot resolve a target (no ssh_host here)
// must answer 200 with reason "config", never 400 — resolveSSHTarget's own
// 400-on-unresolvable-config mapping does not apply to the reachability
// routes.
func TestGetReachability_NoRecordAndUnresolvableConfigReturnsReasonConfig(t *testing.T) {
	executor := sshExecutor("exec-1", models.ExecutorStatusActive, map[string]string{})
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	lister := &fakeReachabilityLister{}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), lister, nil)

	dto, status, err := h.getReachability(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if dto.Reason != string(models.ExecutorReachabilityReasonConfig) {
		t.Fatalf("reason = %q, want %q", dto.Reason, models.ExecutorReachabilityReasonConfig)
	}
	if dto.Persisted {
		t.Fatal("persisted = true for a synthesized record that was never written")
	}
}

func TestGetReachability_ReturnsStoredRecordVerbatim(t *testing.T) {
	executor := sshExecutor("exec-1", models.ExecutorStatusActive, resolvableSSHConfig())
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	stored := &models.ExecutorReachability{
		ExecutorID: "exec-1",
		State:      models.ExecutorReachabilityStateReachable,
		Host:       "10.0.0.1",
	}
	lister := &fakeReachabilityLister{byID: map[string]*models.ExecutorReachability{"exec-1": stored}}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), lister, nil)

	dto, status, err := h.getReachability(context.Background(), "exec-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if !dto.Persisted {
		t.Fatal("persisted = false for an actually-stored record")
	}
	if dto.State != string(models.ExecutorReachabilityStateReachable) {
		t.Fatalf("state = %q, want %q", dto.State, models.ExecutorReachabilityStateReachable)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.5
func TestGetReachability_NonexistentExecutorReturns404(t *testing.T) {
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{}}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), &fakeReachabilityLister{}, nil)

	_, status, err := h.getReachability(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for a nonexistent executor")
	}
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.5
//
// A wrong-type executor id is a 400, distinct from the config-unresolvable
// case (which is a 200 with reason config).
func TestGetReachability_WrongTypeExecutorReturns400(t *testing.T) {
	executor := &models.Executor{ID: "exec-1", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), &fakeReachabilityLister{}, nil)

	_, status, err := h.getReachability(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected an error for a non-ssh executor")
	}
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.4
//
// The list route orders by executor_id ascending, includes a deactivated
// ssh executor's retained record, and omits non-ssh executors entirely.
func TestListReachability_OrdersFiltersAndIncludesDeactivated(t *testing.T) {
	sshA := sshExecutor("ssh-b", models.ExecutorStatusActive, resolvableSSHConfig())
	sshB := sshExecutor("ssh-a", models.ExecutorStatusDisabled, resolvableSSHConfig())
	nonSSH := &models.Executor{ID: "local-1", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	fetcher := &fakeReachabilityExecutorFetcher{}
	lister := &fakeReachabilityLister{
		executors: []*models.Executor{sshA, sshB, nonSSH},
		records: []*models.ExecutorReachability{
			{ExecutorID: "ssh-a", State: models.ExecutorReachabilityStateUnreachable, Reason: models.ExecutorReachabilityReasonTimeout},
		},
	}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), lister, nil)

	dtos, status, err := h.listReachability(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != 200 {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(dtos) != 2 {
		t.Fatalf("len(dtos) = %d, want 2 (non-ssh executor must be omitted)", len(dtos))
	}
	if dtos[0].ExecutorID != "ssh-a" || dtos[1].ExecutorID != "ssh-b" {
		t.Fatalf("order = [%s, %s], want [ssh-a, ssh-b] (executor_id ascending)", dtos[0].ExecutorID, dtos[1].ExecutorID)
	}
	if dtos[0].State != string(models.ExecutorReachabilityStateUnreachable) {
		t.Fatalf("deactivated executor's record state = %q, want %q (retained record)", dtos[0].State, models.ExecutorReachabilityStateUnreachable)
	}
}

// @covers AC-EXECUTORS-SSH-REACHABILITY-002.6
//
// A POST against a deactivated ssh executor answers 409 without probing.
func TestProbeReachability_InactiveExecutorReturns409WithoutProbing(t *testing.T) {
	executor := sshExecutor("exec-1", models.ExecutorStatusDisabled, resolvableSSHConfig())
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	prober := &fakeReachabilityProber{gate: make(chan struct{})}
	close(prober.gate) // would return immediately if ever called

	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), nil, prober)
	_, status, err := h.probeReachability(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected an error for a probe against an inactive executor")
	}
	if status != 409 {
		t.Fatalf("status = %d, want 409", status)
	}
	if got := atomic.LoadInt32(&prober.callsEntered); got != 0 {
		t.Fatalf("callsEntered = %d, want 0 (must not probe an inactive executor)", got)
	}
}

func TestProbeReachability_NonexistentExecutorReturns404(t *testing.T) {
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{}}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), nil, &fakeReachabilityProber{})

	_, status, err := h.probeReachability(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected an error for a nonexistent executor")
	}
	if status != 404 {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestProbeReachability_WrongTypeExecutorReturns400(t *testing.T) {
	executor := &models.Executor{ID: "exec-1", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive}
	fetcher := &fakeReachabilityExecutorFetcher{executors: map[string]*models.Executor{"exec-1": executor}}
	h := NewHandler(nil, fetcher, nil, nil, testHandlerLogger(), nil, &fakeReachabilityProber{})

	_, status, err := h.probeReachability(context.Background(), "exec-1")
	if err == nil {
		t.Fatal("expected an error for a non-ssh executor")
	}
	if status != 400 {
		t.Fatalf("status = %d, want 400", status)
	}
}
