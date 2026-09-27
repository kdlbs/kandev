package lifecycle

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// schedulerTestStore is a fake sshOrphanSweepExecutorStore +
// sshOrphanSweepStore. executor is returned for every GetExecutor call
// (regardless of id) and never appears in the interval listing, so tests
// exercise only the event-triggered path unless they opt into the interval
// path explicitly.
type schedulerTestStore struct {
	executor       *models.Executor
	getExecutorHit chan string
}

func (s *schedulerTestStore) GetExecutor(_ context.Context, id string) (*models.Executor, error) {
	if s.getExecutorHit != nil {
		select {
		case s.getExecutorHit <- id:
		default:
		}
	}
	return s.executor, nil
}

func (s *schedulerTestStore) ListSSHExecutorsForReachability(context.Context) ([]*models.Executor, error) {
	return nil, nil
}

func (s *schedulerTestStore) GetExecutorReachability(context.Context, string) (*models.ExecutorReachability, error) {
	return nil, models.ErrExecutorReachabilityNotFound
}

func (s *schedulerTestStore) GetTask(context.Context, string) (*models.Task, error) {
	return nil, repoerrors.ErrTaskNotFound
}

func (s *schedulerTestStore) ListTaskSessions(context.Context, string) ([]*models.TaskSession, error) {
	return nil, nil
}

func (s *schedulerTestStore) ListExecutorsRunningByTaskID(context.Context, string) ([]*models.ExecutorRunning, error) {
	return nil, nil
}

// hangingSSHListener accepts TCP connections but never writes an SSH version
// banner, so any dial against it blocks in the transport handshake until its
// context is cancelled. acceptCount lets a test prove how many dial attempts
// actually reached the network, independent of how many times the store's
// GetExecutor was called.
func newHangingSSHListener(t *testing.T) (addr string, acceptCount *int32) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var count int32
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			atomic.AddInt32(&count, 1)
			t.Cleanup(func() { _ = conn.Close() })
			// Never write an SSH version banner back. Keep draining
			// whatever the client sends (its own version string, key
			// exchange bytes, ...) so the connection stays open and the
			// handshake blocks forever, instead of the client reading EOF
			// or a reset the moment it writes anything.
			go func() {
				buf := make([]byte, 4096)
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
				}
			}()
		}
	}()
	return listener.Addr().String(), &count
}

func slowDialExecutor(id, addr string) *models.Executor {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		panic(err)
	}
	return &models.Executor{
		ID:   id,
		Type: models.ExecutorTypeSSH,
		Config: map[string]string{
			"ssh_host":             host,
			"ssh_port":             port,
			"ssh_host_fingerprint": "SHA256:deadbeef",
			"ssh_user":             "kandev",
			"ssh_identity_source":  string(SSHIdentitySourceAgent),
		},
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestOrphanSweepSchedulerCoalescesConcurrentTriggersForSameExecutor(t *testing.T) {
	addr, acceptCount := newHangingSSHListener(t)
	executor := slowDialExecutor("executor-1", addr)
	store := &schedulerTestStore{executor: executor}

	eventBus := bus.NewMemoryEventBus(logger.Default())
	scheduler := NewOrphanSweepScheduler(store, store, 3600, logger.Default())
	scheduler.Start(context.Background(), eventBus)
	defer scheduler.Stop()

	publishReachable := func() {
		event := bus.NewEvent(events.ExecutorReachabilityChanged, "test",
			sshOrphanReachabilityEvent{ExecutorID: executor.ID, State: "reachable"})
		if err := eventBus.Publish(context.Background(), events.ExecutorReachabilityChanged, event); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	publishReachable()

	deadline := time.Now().Add(2 * time.Second)
	for atomic.LoadInt32(acceptCount) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the first sweep to reach the network")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// A second trigger for the same executor while the first sweep is still
	// blocked in its SSH dial must be dropped, not queued — AC.13's "one
	// sweep per executor at a time; a trigger that arrives during a sweep is
	// coalesced."
	publishReachable()
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(acceptCount); got != 1 {
		t.Fatalf("dial attempts observed by the listener = %d, want exactly 1 (coalesced)", got)
	}
}

// @covers AC-EXECUTORS-SSH-EXECUTOR-001.13
func TestOrphanSweepSchedulerIgnoresNonReachableState(t *testing.T) {
	store := &schedulerTestStore{
		executor:       &models.Executor{ID: "executor-1", Type: models.ExecutorTypeSSH},
		getExecutorHit: make(chan string, 1),
	}
	eventBus := bus.NewMemoryEventBus(logger.Default())
	scheduler := NewOrphanSweepScheduler(store, store, 3600, logger.Default())
	scheduler.Start(context.Background(), eventBus)
	defer scheduler.Stop()

	for _, state := range []string{"unreachable", "unknown", ""} {
		event := bus.NewEvent(events.ExecutorReachabilityChanged, "test",
			sshOrphanReachabilityEvent{ExecutorID: "executor-1", State: state})
		if err := eventBus.Publish(context.Background(), events.ExecutorReachabilityChanged, event); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	select {
	case id := <-store.getExecutorHit:
		t.Fatalf("GetExecutor was called for id %q on a non-reachable state event", id)
	case <-time.After(100 * time.Millisecond):
	}
}
