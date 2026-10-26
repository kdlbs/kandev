package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// nomadFixtureCoordinator runs the production cleanup claim wiring against the
// shared cleanup fixture, so recovery and rollback reach the real cleanup code.
type nomadFixtureCoordinator struct {
	fixture  *nomadCreateFixture
	manager  *Manager
	mu       sync.Mutex
	launches int
	resumes  int
	lastErr  error
}

func newNomadFixtureCoordinator(f *nomadCreateFixture, claims ExecutorRunningWriter) *nomadFixtureCoordinator {
	c := &nomadFixtureCoordinator{fixture: f, manager: &Manager{runningWriter: claims, logger: newTestLogger()}}
	f.runtime.setCleanupCoordinator(c)
	return c
}

func (c *nomadFixtureCoordinator) DestroyInstance(context.Context, *ExecutorInstance) error {
	return nil
}

func (c *nomadFixtureCoordinator) DestroyLaunch(ctx context.Context, req *ExecutorCreateRequest) error {
	c.mu.Lock()
	c.launches++
	c.mu.Unlock()
	if req.ReleaseRuntimeInventory == nil {
		req.ReleaseRuntimeInventory = func(context.Context) error { return nil }
	}
	err := c.manager.withNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string) error {
		return c.fixture.runtime.destroyNomadOperation(claimed, req, claimID)
	})
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
	return err
}

func (c *nomadFixtureCoordinator) ResumeCleanup(ctx context.Context, record *models.ExecutorRunning) (bool, error) {
	state, err := nomad.EnvironmentStateFromMetadata(record.Metadata)
	if err != nil || !nomadCleanupResumable(state) {
		return false, nil
	}
	c.mu.Lock()
	c.resumes++
	c.mu.Unlock()
	req := c.fixture.request
	return true, c.manager.withNomadCleanupClaim(ctx, req, func(claimed context.Context, claimID string) error {
		return c.fixture.runtime.resumeNomadCleanup(claimed, req, claimID)
	})
}

func setNomadFixtureState(t *testing.T, f *nomadCreateFixture, mutate func(*nomad.EnvironmentState)) {
	t.Helper()
	state, err := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	mutate(&state)
	if err := nomad.PutEnvironmentState(f.request.Metadata, state); err != nil {
		t.Fatal(err)
	}
}

func withReceipt(state *nomad.EnvironmentState) {
	state.DrainReceiptDigest = strings.Repeat("a", 64)
	state.DrainReceiptGeneration = state.Generation
}

func nomadFixtureRecord(f *nomadCreateFixture) *models.ExecutorRunning {
	return &models.ExecutorRunning{
		SessionID: f.request.SessionID, TaskID: f.request.TaskID, AgentExecutionID: f.request.InstanceID,
		Runtime: agentruntime.RuntimeNomad, Status: models.ExecutorRunningStatusStarting, Metadata: f.request.Metadata,
	}
}

// guardUnclaimedCheckpoints counts durable writes made without the cleanup claim.
func guardUnclaimedCheckpoints(f *nomadCreateFixture) *int {
	unclaimed := new(int)
	inner := f.request.CheckpointRuntimeInventory
	f.request.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		if recoveryclaim.ClaimFromContext(ctx) == nil {
			*unclaimed++
			return errors.New("checkpoint without a cleanup claim")
		}
		return inner(ctx, metadata)
	}
	return unclaimed
}

func countCalls(calls []string, want string) int {
	n := 0
	for _, call := range calls {
		if call == want {
			n++
		}
	}
	return n
}

func recoverNomadRecord(t *testing.T, f *nomadCreateFixture) {
	t.Helper()
	instances, err := f.runtime.RecoverInstances(context.Background(), []*models.ExecutorRunning{nomadFixtureRecord(f)})
	if err != nil || len(instances) != 0 {
		t.Fatalf("RecoverInstances() = %v, %v; a cleanup row must not become a connected instance", instances, err)
	}
}

func TestNomadWorkerQuarantineReconciledOnRecoverInstances(t *testing.T) {
	t.Run("pending quarantine without receipt reconciles only", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		newNomadFixtureCoordinator(f, &claimStoreWriter{})
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
			s.Phase, s.QuarantineState = nomadPhaseCleanupPending, nomad.QuarantinePending
		})
		unclaimed := guardUnclaimedCheckpoints(f)
		recoverNomadRecord(t, f)
		state := f.lastCheckpoint(t)
		if state.QuarantineState != nomad.QuarantineMarked || state.Phase != nomadPhaseCleanupPending {
			t.Fatalf("phase=%q quarantine=%q, want cleanup_pending and marked", state.Phase, state.QuarantineState)
		}
		calls := f.fake.callLog()
		if countCalls(calls, "node:eligibility") != 1 || countCalls(calls, "marker:put") != 1 || f.fake.marker == "" || f.fake.eligibility != "ineligible" {
			t.Fatalf("node not fenced: calls=%v marker=%q", calls, f.fake.marker)
		}
		if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 || *unclaimed != 0 {
			t.Fatalf("drain=%d stop=%d release=%d unclaimed=%d", f.cleanup.drainCalls, f.cleanup.stopCalls, f.cleanup.releaseCalls, *unclaimed)
		}
	})
	t.Run("receipt resumes the tail without a second drain", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		newNomadFixtureCoordinator(f, &claimStoreWriter{})
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
			s.Phase, s.QuarantineState = nomadPhaseCleanupPending, nomad.QuarantinePending
			withReceipt(s)
		})
		unclaimed := guardUnclaimedCheckpoints(f)
		recoverNomadRecord(t, f)
		calls := f.fake.callLog()
		order := []string{"node:eligibility", "job:stop", "ssh:fence-release", "inventory:release"}
		last := -1
		for _, want := range order {
			at := indexOfCall(calls, want)
			if at <= last {
				t.Fatalf("call %q out of order in %v", want, calls)
			}
			last = at
		}
		final := f.lastCheckpoint(t)
		if final.Phase != nomadPhaseAbsent || final.QuarantineState != nomad.QuarantineMarked {
			t.Fatalf("phase=%q quarantine=%q", final.Phase, final.QuarantineState)
		}
		if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 1 || *unclaimed != 0 {
			t.Fatalf("drain=%d stop=%d unclaimed=%d", f.cleanup.drainCalls, f.cleanup.stopCalls, *unclaimed)
		}
	})
	t.Run("cleanup pending without receipt or quarantine fences the node and never drains", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) { s.Phase = nomadPhaseCleanupPending })
		unclaimed := guardUnclaimedCheckpoints(f)
		recoverNomadRecord(t, f)
		state := f.lastCheckpoint(t)
		if state.QuarantineState != nomad.QuarantineMarked || state.Phase != nomadPhaseCleanupPending {
			t.Fatalf("phase=%q quarantine=%q, want cleanup_pending and marked", state.Phase, state.QuarantineState)
		}
		if f.fake.eligibility != "ineligible" || !strings.Contains(f.fake.marker, nomadCleanupReasonInterrupted) {
			t.Fatalf("node not fenced: eligibility=%s marker=%q", f.fake.eligibility, f.fake.marker)
		}
		if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 || *unclaimed != 0 {
			t.Fatalf("drain=%d stop=%d release=%d unclaimed=%d", f.cleanup.drainCalls, f.cleanup.stopCalls, f.cleanup.releaseCalls, *unclaimed)
		}
		if coordinator.resumes != 1 {
			t.Fatalf("resume attempts = %d, want the row handled once", coordinator.resumes)
		}
	})
	t.Run("an already marked cleanup pending row without receipt writes nothing", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		newNomadFixtureCoordinator(f, &claimStoreWriter{})
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
			s.Phase, s.QuarantineState = nomadPhaseCleanupPending, nomad.QuarantineMarked
		})
		recoverNomadRecord(t, f)
		if len(f.checkpoints) != 0 || f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || len(f.fake.callLog()) != 0 {
			t.Fatalf("writes=%d drain=%d stop=%d calls=%v", len(f.checkpoints), f.cleanup.drainCalls, f.cleanup.stopCalls, f.fake.callLog())
		}
	})
	t.Run("receipt resume removes the staged bootstrap environment first", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		newNomadFixtureCoordinator(f, &claimStoreWriter{})
		f.cleanup.failBootstrapRm = true
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
			s.Phase = nomadPhaseCleanupPending
			withReceipt(s)
		})
		state, _ := nomad.EnvironmentStateFromMetadata(f.request.Metadata)
		recoverNomadRecord(t, f)
		calls := f.fake.callLog()
		rm, stop := indexOfCall(calls, "ssh:bootstrap-rm"), indexOfCall(calls, "job:stop")
		if rm < 0 || stop < 0 || rm > stop || countCalls(calls, "ssh:bootstrap-rm") != 1 {
			t.Fatalf("bootstrap removal not before settle: %v", calls)
		}
		var removed string
		for _, command := range f.cleanup.ssh {
			if strings.HasPrefix(command, "rm -f -- ") {
				removed = command
			}
		}
		if !strings.Contains(removed, state.BootstrapPath+".env") {
			t.Fatalf("removed %q, want the owned bootstrap env file", removed)
		}
		if f.lastCheckpoint(t).Phase != nomadPhaseAbsent || f.cleanup.drainCalls != 0 {
			t.Fatalf("a failed bootstrap removal masked the tail: phase=%q drain=%d", f.lastCheckpoint(t).Phase, f.cleanup.drainCalls)
		}
	})
}

func TestNomadWorkerQuarantineReconciledOnRecoverInstancesRealCheckpoint(t *testing.T) {
	setup := func(t *testing.T) (*nomadCreateFixture, *realNomadInventory) {
		f := prepareNomadCleanupFixture(t)
		setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
			s.Phase, s.QuarantineState = nomadPhaseCleanupPending, nomad.QuarantinePending
			withReceipt(s)
		})
		inventory := newRealNomadInventory(t, f)
		newNomadFixtureCoordinator(f, inventory.repo)
		return f, inventory
	}
	t.Run("claim held by the cleanup resumes and records absence", func(t *testing.T) {
		f, inventory := setup(t)
		recoverNomadRecord(t, f)
		row := inventory.read(t)
		if row.Phase != nomadPhaseAbsent || row.QuarantineState != nomad.QuarantineMarked || row.DrainReceiptDigest == "" {
			t.Fatalf("durable row phase=%q quarantine=%q receipt=%q", row.Phase, row.QuarantineState, row.DrainReceiptDigest)
		}
		if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 1 {
			t.Fatalf("drain=%d stop=%d", f.cleanup.drainCalls, f.cleanup.stopCalls)
		}
		if held, err := inventory.repo.GetTaskEnvironmentRecoveryClaim(context.Background(), f.request.TaskEnvironmentID); err != nil || held != nil {
			t.Fatalf("cleanup claim not released: %v, %v", held, err)
		}
	})
	t.Run("a foreign claim blocks every external call and durable write", func(t *testing.T) {
		f, inventory := setup(t)
		inventory.holdForeignClaim(t, f)
		recoverNomadRecord(t, f)
		row := inventory.read(t)
		if row.Phase != nomadPhaseCleanupPending || row.QuarantineState != nomad.QuarantinePending {
			t.Fatalf("durable row changed without the claim: phase=%q quarantine=%q", row.Phase, row.QuarantineState)
		}
		if len(f.fake.callLog()) != 0 || f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 {
			t.Fatalf("external calls without the claim: %v drain=%d stop=%d", f.fake.callLog(), f.cleanup.drainCalls, f.cleanup.stopCalls)
		}
	})
	t.Run("a checkpoint without the claim is rejected by the real store", func(t *testing.T) {
		f, inventory := setup(t)
		inventory.holdForeignClaim(t, f)
		err := f.runtime.resumeNomadCleanup(context.Background(), f.request, testCleanupClaim)
		if !errors.Is(err, ErrNomadCleanupRetained) {
			t.Fatalf("resume without the claim = %v, want retained", err)
		}
		if row := inventory.read(t); row.QuarantineState != nomad.QuarantinePending || row.Phase != nomadPhaseCleanupPending {
			t.Fatalf("unclaimed write reached the durable row: %+v", row)
		}
		if f.cleanup.stopCalls != 0 {
			t.Fatalf("job stopped without the claim: %d", f.cleanup.stopCalls)
		}
	})
}

// nomadAPIInterposer fronts the fixture's Nomad API to observe job stops, drop
// the reply of a committed stop, and report extra allocations.
type nomadAPIInterposer struct {
	mu            sync.Mutex
	stopQueries   []string
	loseStopReply bool
	allocations   string
}

func (ip *nomadAPIInterposer) setLoseStopReply(v bool) {
	ip.mu.Lock()
	ip.loseStopReply = v
	ip.mu.Unlock()
}

func (ip *nomadAPIInterposer) queries() []string {
	ip.mu.Lock()
	defer ip.mu.Unlock()
	return append([]string(nil), ip.stopQueries...)
}

func interposeNomadAPI(t *testing.T, f *nomadCreateFixture) *nomadAPIInterposer {
	t.Helper()
	target, err := url.Parse(f.profile.Address)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	ip := &nomadAPIInterposer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isStop := r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/job/")
		ip.mu.Lock()
		lose, allocations := ip.loseStopReply, ip.allocations
		if isStop {
			ip.stopQueries = append(ip.stopQueries, r.URL.RawQuery)
		}
		ip.mu.Unlock()
		switch {
		case isStop && lose:
			forward, _ := http.NewRequestWithContext(r.Context(), r.Method, f.profile.Address+r.URL.RequestURI(), nil)
			forward.Header = r.Header.Clone()
			if resp, err := http.DefaultClient.Do(forward); err == nil {
				_ = resp.Body.Close()
			}
			if conn, _, err := w.(http.Hijacker).Hijack(); err == nil {
				_ = conn.Close()
			}
		case allocations != "" && r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/allocations"):
			_, _ = w.Write([]byte(allocations))
		default:
			proxy.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.request.Metadata["address"] = server.URL
	return ip
}

// stallNomadDrain makes the runtime drain endpoint accept the request and never reply.
func stallNomadDrain(t *testing.T, f *nomadCreateFixture) {
	t.Helper()
	origin := f.sshServer.tcpTarget(0)
	proxy := httputil.NewSingleHostReverseProxy(&url.URL{Scheme: "http", Host: origin})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runtime/seal-and-drain" {
			proxy.ServeHTTP(w, r)
			return
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	f.sshServer.forwardTo(strings.TrimPrefix(server.URL, "http://"))
}

// assertQuarantinedRetained asserts the shared outcome of every quarantine trigger:
// retained cleanup_pending, no false absence, the worker fenced, and no purge.
func assertQuarantinedRetained(t *testing.T, f *nomadCreateFixture, ip *nomadAPIInterposer, err error, reason string) {
	t.Helper()
	if !errors.Is(err, ErrNomadCleanupRetained) || !strings.Contains(err.Error(), reason) {
		t.Fatalf("cleanup = %v, want retained with reason %q", err, reason)
	}
	for _, state := range f.checkpoints {
		if state.Phase == nomadPhaseAbsent {
			t.Fatal("absence was recorded without positive evidence")
		}
	}
	final := f.lastCheckpoint(t)
	if final.Phase != nomadPhaseCleanupPending || final.QuarantineState != nomad.QuarantineMarked {
		t.Fatalf("final phase=%q quarantine=%q, want retained cleanup_pending and marked", final.Phase, final.QuarantineState)
	}
	calls := f.fake.callLog()
	pending, fenced := indexOfCall(calls, "checkpoint:cleanup_pending:pending"), indexOfCall(calls, "node:eligibility")
	if pending < 0 || fenced < pending || f.fake.eligibility != "ineligible" || !strings.Contains(f.fake.marker, reason) {
		t.Fatalf("quarantineWorker was not driven: calls=%v marker=%q", calls, f.fake.marker)
	}
	if indexOfCall(calls, "ssh:fence-release") >= 0 || f.cleanup.releaseCalls != 0 {
		t.Fatalf("fence or inventory released without absence: %v", calls)
	}
	if ip != nil {
		for _, query := range ip.queries() {
			if !strings.Contains(query, "purge=false") {
				t.Fatalf("job stop was not a non-purging stop: %q", query)
			}
		}
	}
}

func TestNomadWorkerQuarantineOnNodeLost(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	ip := interposeNomadAPI(t, f)
	f.fake.status = "down"
	err := runNomadCleanupFixture(f)
	assertQuarantinedRetained(t, f, ip, err, nomadCleanupReasonNodeLost)
	if f.cleanup.stopCalls != 1 || len(f.lastCheckpoint(t).DrainReceiptDigest) != 64 {
		t.Fatalf("stop=%d receipt=%q", f.cleanup.stopCalls, f.lastCheckpoint(t).DrainReceiptDigest)
	}
}

func TestNomadWorkerQuarantineOnUnexpectedAllocation(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	ip := interposeNomadAPI(t, f)
	ip.allocations = `[{"ID":"alloc-1","NodeID":"node-1","Namespace":"default","DesiredStatus":"stop","ClientStatus":"complete","CreateIndex":9},` +
		`{"ID":"alloc-2","NodeID":"node-1","Namespace":"default","DesiredStatus":"run","ClientStatus":"running","CreateIndex":10}]`
	err := runNomadCleanupFixture(f)
	assertQuarantinedRetained(t, f, ip, err, nomadCleanupReasonUnexpectedAllocation)
}

func TestNomadWorkerQuarantineOnLostReplyAfterCheckpoint(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	ip := interposeNomadAPI(t, f)
	ip.setLoseStopReply(true)
	err := runNomadCleanupFixture(f)
	assertQuarantinedRetained(t, f, ip, err, nomadCleanupReasonStopFailed)
	if got := f.lastCheckpoint(t); len(got.DrainReceiptDigest) != 64 || f.cleanup.drainCalls != 1 {
		t.Fatalf("receipt=%q drain=%d, want the receipt persisted before the lost reply", got.DrainReceiptDigest, f.cleanup.drainCalls)
	}
	ip.setLoseStopReply(false)
	if err := f.runtime.resumeNomadCleanup(context.Background(), f.request, testCleanupClaim); err != nil {
		t.Fatalf("resume after the lost reply: %v", err)
	}
	if got := f.lastCheckpoint(t); got.Phase != nomadPhaseAbsent || f.cleanup.drainCalls != 1 {
		t.Fatalf("phase=%q drain=%d after resume", got.Phase, f.cleanup.drainCalls)
	}
	for _, query := range ip.queries() {
		if !strings.Contains(query, "purge=false") {
			t.Fatalf("job was purged: %q", query)
		}
	}
}

func TestNomadWorkerQuarantineOnServerIdentityChange(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	ip := interposeNomadAPI(t, f)
	setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) { s.ServerIdentity = "some-other-agentctl" })
	err := runNomadCleanupFixture(f)
	assertQuarantinedRetained(t, f, ip, err, nomadCleanupReasonIdentityChanged)
	if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 {
		t.Fatalf("drain=%d stop=%d for a changed runtime identity", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
}

func TestNomadWorkerQuarantineOnDrainTimeout(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	ip := interposeNomadAPI(t, f)
	stallNomadDrain(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	err := f.runtime.destroyNomadOperation(ctx, f.request, testCleanupClaim)
	assertQuarantinedRetained(t, f, ip, err, nomadCleanupReasonDrainTimeout)
	if f.cleanup.stopCalls != 0 || len(f.lastCheckpoint(t).DrainReceiptDigest) != 0 {
		t.Fatalf("stop=%d receipt=%q after a drain that never completed", f.cleanup.stopCalls, f.lastCheckpoint(t).DrainReceiptDigest)
	}
}

// realNomadInventory backs the fixture's checkpoint callback with the real
// SQLite inventory, which enforces ownership, revision and claim checks.
type realNomadInventory struct {
	repo    *tasksqlite.Repository
	running *models.ExecutorRunning
}

func newRealNomadInventory(t *testing.T, f *nomadCreateFixture) *realNomadInventory {
	t.Helper()
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	req := f.request
	state, err := nomad.EnvironmentStateFromMetadata(req.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, create := range []error{
		repo.CreateTask(ctx, &models.Task{ID: req.TaskID, Title: "nomad cleanup"}),
		repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
			ID: req.TaskEnvironmentID, TaskID: req.TaskID, ExecutorType: string(models.ExecutorTypeNomad),
			OwnershipGeneration: state.Generation, Status: models.TaskEnvironmentStatusCreating, MaterializationSessionID: req.SessionID,
		}),
		repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: req.SessionID, TaskID: req.TaskID, TaskEnvironmentID: req.TaskEnvironmentID, State: models.TaskSessionStateCreated,
		}),
	} {
		if create != nil {
			t.Fatal(create)
		}
	}
	inventory := &realNomadInventory{repo: repo, running: &models.ExecutorRunning{
		SessionID: req.SessionID, TaskID: req.TaskID, AgentExecutionID: req.InstanceID,
		Runtime: agentruntime.RuntimeNomad, Status: models.ExecutorRunningStatusStarting,
	}}
	inventory.seed(t, state)
	req.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
		return inventory.checkpoint(ctx, metadata)
	}
	return inventory
}

// seed inserts the intent row, then moves it to the scenario state.
func (i *realNomadInventory) seed(t *testing.T, scenario nomad.EnvironmentState) {
	t.Helper()
	intent := scenario
	intent.Phase, intent.QuarantineState = "intent", ""
	intent.DrainReceiptDigest, intent.DrainReceiptGeneration = "", 0
	for _, state := range []nomad.EnvironmentState{intent, scenario} {
		metadata := map[string]interface{}{}
		if err := nomad.PutEnvironmentState(metadata, state); err != nil {
			t.Fatal(err)
		}
		if err := i.checkpoint(context.Background(), metadata); err != nil {
			t.Fatalf("seed %s: %v", state.Phase, err)
		}
	}
}

func (i *realNomadInventory) checkpoint(ctx context.Context, metadata map[string]interface{}) error {
	i.running.Metadata = metadata
	return i.repo.CheckpointNomadExecutorInventory(ctx, i.running)
}

func (i *realNomadInventory) read(t *testing.T) nomad.EnvironmentState {
	t.Helper()
	row, err := i.repo.GetExecutorRunningBySessionID(context.Background(), i.running.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	state, err := nomad.EnvironmentStateFromMetadata(row.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func (i *realNomadInventory) holdForeignClaim(t *testing.T, f *nomadCreateFixture) {
	t.Helper()
	state := i.read(t)
	if _, err := i.repo.AcquireTaskEnvironmentRecoveryClaim(context.Background(), models.TaskEnvironmentRecoveryClaimRequest{
		TaskEnvironmentID: state.EnvironmentID, OwnerTaskID: f.request.TaskID, OwnershipGeneration: state.Generation,
		SessionID: f.request.SessionID, OperationID: "foreign-operation", ExecutorType: string(models.ExecutorTypeNomad),
		AllowCurrentSessionRuntime: true,
	}); err != nil {
		t.Fatalf("acquire foreign claim: %v", err)
	}
}
