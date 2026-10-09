package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

type recoveryTestClock struct {
	mu            sync.Mutex
	now           time.Time
	sleeps        []time.Duration
	stableStarted chan struct{}
	stableRelease chan struct{}
}

func newRecoveryTestClock() *recoveryTestClock {
	return &recoveryTestClock{
		now:           time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC),
		stableStarted: make(chan struct{}, 1),
		stableRelease: make(chan struct{}),
	}
}

func (clock *recoveryTestClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *recoveryTestClock) Sleep(ctx context.Context, duration time.Duration) error {
	if duration == 5*time.Minute {
		select {
		case clock.stableStarted <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-clock.stableRelease:
			clock.mu.Lock()
			clock.now = clock.now.Add(duration)
			clock.mu.Unlock()
			return nil
		}
	}
	clock.mu.Lock()
	clock.sleeps = append(clock.sleeps, duration)
	clock.now = clock.now.Add(duration)
	clock.mu.Unlock()
	return nil
}

func (clock *recoveryTestClock) Backoffs() []time.Duration {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return append([]time.Duration(nil), clock.sleeps...)
}

func newPublishedRecoveryOwner(t *testing.T) (*RuntimeOwner, *RuntimeBindingCandidate) {
	t.Helper()
	owner := NewRuntimeOwner(nil, nil, "backend-boot-stable")
	t.Cleanup(owner.Stop)
	candidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatalf("prepare initial runtime: %v", err)
	}
	if err := candidate.Configure("127.0.0.1", 39429, "initial-secret", 1, nil); err != nil {
		t.Fatalf("configure initial runtime: %v", err)
	}
	if err := candidate.Commit(); err != nil {
		t.Fatalf("publish initial runtime: %v", err)
	}
	return owner, candidate
}

func waitRecoverySnapshot(t *testing.T, owner *RuntimeOwner, wantStatus string) AvailabilitySnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snapshot, ok := owner.Snapshot()
		if ok && snapshot.Status == wantStatus {
			return snapshot
		}
		time.Sleep(time.Millisecond)
	}
	snapshot, _ := owner.Snapshot()
	t.Fatalf("runtime status = %q, want %q (snapshot=%+v)", snapshot.Status, wantStatus, snapshot)
	return AvailabilitySnapshot{}
}

func waitRecoveryAttempt(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime recovery attempt did not start")
	}
}

const recoveryChildReadyEnv = "KANDEV_TEST_RECOVERY_CHILD_READY"

func TestRecoveryControlProcessHelper(t *testing.T) {
	readyPath := os.Getenv(recoveryChildReadyEnv)
	if readyPath == "" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for recovery test child: %v", err)
	}
	if err := os.WriteFile(readyPath, []byte(listener.Addr().String()), 0o600); err != nil {
		t.Fatalf("write recovery test child endpoint: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve recovery test child: %v", err)
	}
}

func startRecoveryControlProcess(t *testing.T) (int, string, int, func()) {
	t.Helper()
	readyPath := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRecoveryControlProcessHelper$")
	cmd.Env = append(os.Environ(), recoveryChildReadyEnv+"="+readyPath)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start recovery test child: %v", err)
	}
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		})
	}
	t.Cleanup(stop)
	deadline := time.Now().Add(3 * time.Second)
	var endpoint string
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(readyPath)
		if err == nil {
			endpoint = string(data)
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if endpoint == "" {
		stop()
		t.Fatal("recovery test child did not publish its endpoint")
	}
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		stop()
		t.Fatalf("parse recovery test child endpoint %q: %v", endpoint, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		stop()
		t.Fatalf("parse recovery test child port %q: %v", portText, err)
	}
	return cmd.Process.Pid, host, port, stop
}

func TestRuntimeReplacementKeepsBackendBoot(t *testing.T) {
	backendPID := os.Getpid()
	oldPID, oldHost, oldPort, stopOld := startRecoveryControlProcess(t)
	owner := NewRuntimeOwner(nil, nil, "backend-boot-stable")
	t.Cleanup(owner.Stop)
	current, err := owner.PrepareBinding()
	if err != nil {
		t.Fatalf("prepare initial runtime: %v", err)
	}
	if err := current.Configure(oldHost, oldPort, "initial-secret", oldPID, func() error { stopOld(); return nil }); err != nil {
		t.Fatalf("configure initial runtime: %v", err)
	}
	if err := current.Commit(); err != nil {
		t.Fatalf("publish initial runtime: %v", err)
	}
	clock := newRecoveryTestClock()
	started := make(chan struct{}, 1)
	var replacementPID int
	var replacementHost string
	var replacementPort int
	coordinator := NewRecoveryCoordinator(owner, func(_ context.Context, _ RuntimeLossHandler) error {
		pid, host, port, stop := startRecoveryControlProcess(t)
		replacement, err := owner.PrepareBinding()
		if err != nil {
			stop()
			return err
		}
		if err := replacement.Configure(host, port, "replacement-secret", pid, func() error { stop(); return nil }); err != nil {
			stop()
			return err
		}
		if err := replacement.Commit(); err != nil {
			return err
		}
		replacementPID = pid
		replacementHost = host
		replacementPort = port
		started <- struct{}{}
		return nil
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)

	stopOld()
	if !current.MarkUnexpectedExit() {
		t.Fatal("initial runtime exit was not accepted")
	}
	if !coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true) {
		t.Fatal("runtime loss was not accepted by coordinator")
	}
	coordinator.Start(context.Background())
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("replacement was not started")
	}

	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	if snapshot.BootID != "backend-boot-stable" {
		t.Fatalf("replacement changed backend boot id to %q", snapshot.BootID)
	}
	if snapshot.RuntimeEpoch <= current.Epoch() {
		t.Fatalf("runtime epoch = %d, want successor after %d", snapshot.RuntimeEpoch, current.Epoch())
	}
	if snapshot.RecoveryID == "" {
		t.Fatal("successful recovery did not retain its correlation id")
	}
	if replacementPID == oldPID {
		t.Fatalf("replacement process id = %d, want a new child after %d", replacementPID, oldPID)
	}
	if os.Getpid() != backendPID {
		t.Fatalf("backend process changed from %d to %d", backendPID, os.Getpid())
	}
	response, err := http.Get(fmt.Sprintf("http://%s:%d/health", replacementHost, replacementPort))
	if err != nil {
		t.Fatalf("replacement control process health: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("replacement health status = %d, want 200", response.StatusCode)
	}
	if err := current.Commit(); !errors.Is(err, ErrRuntimeCandidateState) {
		t.Fatalf("retired candidate commit error = %v, want candidate-state error", err)
	}
}

func TestRuntimeReplacementBudget(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	attempts := 0
	coordinator := NewRecoveryCoordinator(owner, func(context.Context, RuntimeLossHandler) error {
		attempts++
		return errors.New("candidate startup failed")
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())

	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusUnavailable)
	if attempts != 3 {
		t.Fatalf("replacement starts = %d, want 3", attempts)
	}
	if snapshot.Reason != AvailabilityReasonRecoveryExhausted || !snapshot.RetryAllowed {
		t.Fatalf("exhausted snapshot = %+v", snapshot)
	}
	if got := clock.Backoffs(); len(got) != 2 || got[0] != time.Second || got[1] != 2*time.Second {
		t.Fatalf("backoffs = %v, want [1s 2s]", got)
	}
}

func TestRuntimeRecoverIdempotentRequest(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	attempts := 0
	coordinator := NewRecoveryCoordinator(owner, func(context.Context, RuntimeLossHandler) error {
		attempts++
		if attempts <= 3 {
			return errors.New("candidate startup failed")
		}
		replacement, err := owner.PrepareBinding()
		if err != nil {
			return err
		}
		if err := replacement.Configure("127.0.0.1", 39430, "manual-retry-secret", 4, nil); err != nil {
			return err
		}
		return replacement.Commit()
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	coordinator.Start(context.Background())
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	exhausted := waitRecoverySnapshot(t, owner, AvailabilityStatusUnavailable)
	deadline := time.Now().Add(2 * time.Second)
	for (!exhausted.RetryAllowed || exhausted.Reason != AvailabilityReasonRecoveryExhausted) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		exhausted, _ = owner.Snapshot()
	}
	if exhausted.Reason != AvailabilityReasonRecoveryExhausted || !exhausted.RetryAllowed {
		t.Fatalf("automatic recovery did not become retryable: %+v", exhausted)
	}
	if _, err := coordinator.Retry(context.Background(), exhausted.BootID, exhausted.RuntimeEpoch+1, "stale-request"); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatalf("stale manual retry error = %v, want conflict", err)
	}
	if _, err := coordinator.RetryAtRevision(
		context.Background(), exhausted.BootID, exhausted.RuntimeEpoch, exhausted.Revision+1, "stale-revision",
	); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatalf("stale revision retry error = %v, want conflict", err)
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	manual, err := coordinator.RetryAtRevision(
		requestCtx, exhausted.BootID, exhausted.RuntimeEpoch, exhausted.Revision, "manual-request",
	)
	if err != nil {
		t.Fatalf("accepted retry was canceled with its caller: %v", err)
	}
	if manual.Status != AvailabilityStatusRecovering || manual.RecoveryID == exhausted.RecoveryID {
		t.Fatalf("manual retry snapshot = %+v; prior = %+v", manual, exhausted)
	}
	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	if snapshot.BootID != exhausted.BootID || snapshot.RuntimeEpoch <= exhausted.RuntimeEpoch {
		t.Fatalf("manual replacement did not preserve boot and advance epoch: %+v", snapshot)
	}
	if attempts != 4 {
		t.Fatalf("replacement starts including manual retry = %d, want 4", attempts)
	}
	duplicate, err := coordinator.RetryAtRevision(
		context.Background(), exhausted.BootID, exhausted.RuntimeEpoch, exhausted.Revision, "manual-request",
	)
	if err != nil {
		t.Fatalf("duplicate retry request: %v", err)
	}
	if duplicate.RuntimeEpoch != snapshot.RuntimeEpoch || attempts != 4 {
		t.Fatalf("duplicate retry started another episode: snapshot=%+v attempts=%d", duplicate, attempts)
	}
}

func TestRuntimeRecoverStaleEpoch(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	coordinator := NewRecoveryCoordinator(owner, func(ctx context.Context, _ RuntimeLossHandler) error {
		<-ctx.Done()
		return ctx.Err()
	}, RecoveryCoordinatorOptions{})
	t.Cleanup(coordinator.Stop)
	coordinator.Start(context.Background())
	if !current.MarkUnexpectedExit() {
		t.Fatal("runtime exit did not retire the current epoch")
	}
	if !coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true) {
		t.Fatal("runtime exit was not accepted by the coordinator")
	}
	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusRecovering)
	if _, err := coordinator.RetryAtRevision(
		context.Background(), snapshot.BootID, snapshot.RuntimeEpoch+1, snapshot.Revision, "stale-epoch",
	); !errors.Is(err, ErrRecoveryConflict) {
		t.Fatalf("stale runtime epoch retry error = %v, want conflict", err)
	}
}

func TestRuntimeReplacementStableWindowResetsBudget(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	started := make(chan struct{}, 5)
	attempts := 0
	var latest *RuntimeBindingCandidate
	var notify RuntimeLossHandler
	coordinator := NewRecoveryCoordinator(owner, func(_ context.Context, onRuntimeLoss RuntimeLossHandler) error {
		attempts++
		candidate, err := owner.PrepareBinding()
		if err != nil {
			return err
		}
		if err := candidate.Configure("127.0.0.1", 39429+attempts, fmt.Sprintf("secret-%d", attempts), attempts, nil); err != nil {
			return err
		}
		if err := candidate.Commit(); err != nil {
			return err
		}
		latest = candidate
		notify = onRuntimeLoss
		started <- struct{}{}
		return nil
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())
	waitRecoveryAttempt(t, started)
	waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	<-clock.stableStarted

	latest.MarkUnexpectedExit()
	notify(latest.Epoch(), AvailabilityReasonAgentctlExited, true)
	waitRecoveryAttempt(t, started)
	waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	<-clock.stableStarted
	close(clock.stableRelease)
	deadline := time.Now().Add(2 * time.Second)
	for coordinator.currentStarts() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := coordinator.currentStarts(); got != 0 {
		t.Fatalf("starts since stable health = %d, want 0", got)
	}

	latest.MarkUnexpectedExit()
	notify(latest.Epoch(), AvailabilityReasonAgentctlExited, true)
	waitRecoveryAttempt(t, started)
	waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	latest.MarkUnexpectedExit()
	notify(latest.Epoch(), AvailabilityReasonAgentctlExited, true)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		snapshot, _ := owner.Snapshot()
		coordinator.mu.Lock()
		active := coordinator.active != nil
		pending := coordinator.pending != nil
		coordinator.mu.Unlock()
		t.Fatalf("stable health did not restore the replacement start budget: snapshot=%+v starts=%d active=%t pending=%t attempts=%d",
			snapshot, coordinator.currentStarts(), active, pending, attempts)
	}
	if attempts != 4 {
		t.Fatalf("replacement starts after stable reset = %d, want 4", attempts)
	}
}

func TestRuntimeReplacementSingleFlight(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	attempts := 0
	var mu sync.Mutex
	coordinator := NewRecoveryCoordinator(owner, func(ctx context.Context, _ RuntimeLossHandler) error {
		mu.Lock()
		attempts++
		attempt := attempts
		mu.Unlock()
		if attempt == 1 {
			entered <- struct{}{}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-release:
			return errors.New("single failed attempt")
		}
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery attempt did not start")
	}
	for i := 0; i < 20; i++ {
		if coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true) {
			t.Fatal("duplicate loss signal started another recovery")
		}
	}
	joined, err := coordinator.Retry(context.Background(), "backend-boot-stable", current.Epoch(), "retry-duplicate")
	if err != nil {
		t.Fatalf("retry should join active episode: %v", err)
	}
	if joined.RecoveryID == "" || joined.Status != AvailabilityStatusRecovering {
		t.Fatalf("joined snapshot = %+v", joined)
	}
	close(release)
	waitRecoverySnapshot(t, owner, AvailabilityStatusUnavailable)
	mu.Lock()
	gotAttempts := attempts
	mu.Unlock()
	if gotAttempts != 3 {
		t.Fatalf("replacement starts = %d, want budget-limited 3", gotAttempts)
	}
}

func TestRuntimeReplacementCrashBeforePublish(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	attempts := 0
	coordinator := NewRecoveryCoordinator(owner, func(_ context.Context, _ RuntimeLossHandler) error {
		attempts++
		candidate, err := owner.PrepareBinding()
		if err != nil {
			return err
		}
		if err := candidate.Configure("127.0.0.1", 39430, fmt.Sprintf("candidate-%d", attempts), 2, nil); err != nil {
			return err
		}
		if attempts == 1 {
			candidate.MarkUnexpectedExit()
			_ = candidate.Abort()
			return errors.New("candidate exited before publish")
		}
		return candidate.Commit()
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())

	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusAvailable)
	if attempts != 2 {
		t.Fatalf("attempts = %d, want one failed candidate then one published candidate", attempts)
	}
	if snapshot.RuntimeEpoch <= current.Epoch() || snapshot.Reason != "" {
		t.Fatalf("snapshot after candidate recovery = %+v", snapshot)
	}
}

func TestRuntimeReplacementBlocksUncontainedCandidate(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	attempts := 0
	coordinator := NewRecoveryCoordinator(owner, func(
		_ context.Context,
		onRuntimeLoss RuntimeLossHandler,
	) error {
		attempts++
		candidate, err := owner.PrepareBinding()
		if err != nil {
			return err
		}
		if err := candidate.Configure("127.0.0.1", 39430, "candidate-secret", 2, nil); err != nil {
			return err
		}
		candidate.MarkUnexpectedExit()
		onRuntimeLoss(candidate.Epoch(), AvailabilityReasonOwnershipUnverified, false)
		_ = candidate.Abort()
		return errors.New("candidate descendant ownership is unknown")
	}, RecoveryCoordinatorOptions{Clock: clock})
	t.Cleanup(coordinator.Stop)
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())

	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusUnavailable)
	if attempts != 1 {
		t.Fatalf("replacement starts = %d, want one safe candidate attempt", attempts)
	}
	if snapshot.Reason != AvailabilityReasonOwnershipUnverified || snapshot.RetryAllowed {
		t.Fatalf("uncontained candidate snapshot = %+v", snapshot)
	}
}

func TestRuntimeReplacementShutdownRace(t *testing.T) {
	owner, current := newPublishedRecoveryOwner(t)
	clock := newRecoveryTestClock()
	entered := make(chan struct{}, 1)
	coordinator := NewRecoveryCoordinator(owner, func(ctx context.Context, _ RuntimeLossHandler) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}, RecoveryCoordinatorOptions{Clock: clock})
	current.MarkUnexpectedExit()
	coordinator.NotifyRuntimeLoss(current.Epoch(), AvailabilityReasonAgentctlExited, true)
	coordinator.Start(context.Background())
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery attempt did not start")
	}
	coordinator.Stop()
	snapshot := waitRecoverySnapshot(t, owner, AvailabilityStatusRecovering)
	if snapshot.Status == AvailabilityStatusAvailable {
		t.Fatalf("shutdown race published a replacement: %+v", snapshot)
	}
}
