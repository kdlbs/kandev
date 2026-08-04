package sleepinhibition

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func TestServiceReconcilesOneSharedLeaseAcrossWorkingSessions(t *testing.T) {
	reader := &fakeSessionReader{}
	inhibitor := &fakeInhibitor{platform: PlatformLinux, supported: true}
	service := newTestService(t, reader, inhibitor)

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer service.Stop()
	if _, err := service.Update(context.Background(), Settings{Enabled: true}); err != nil {
		t.Fatalf("enable: %v", err)
	}

	reader.set(session(models.TaskSessionStateStarting))
	publishSessionEvent(t, service)
	waitFor(t, func() bool { return inhibitor.acquireCount() == 1 })

	reader.set(
		session(models.TaskSessionStateStarting),
		session(models.TaskSessionStateRunning),
	)
	publishSessionEvent(t, service)
	time.Sleep(10 * time.Millisecond)
	if got := inhibitor.acquireCount(); got != 1 {
		t.Fatalf("acquire count with two working sessions = %d, want 1", got)
	}

	reader.set(session(models.TaskSessionStateWaitingForInput))
	publishSessionEvent(t, service)
	waitFor(t, func() bool { return inhibitor.releaseCount() == 1 })

	response, err := service.Update(context.Background(), Settings{Enabled: false})
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if response.Settings.Enabled {
		t.Fatal("disabled response retained enabled setting")
	}
	if response.Status.Active {
		t.Fatal("disabled response retained active lease")
	}
}

func TestServiceStartupAndUpdateAcquireOnlyWhenWorking(t *testing.T) {
	reader := &fakeSessionReader{}
	reader.set(session(models.TaskSessionStateRunning))
	inhibitor := &fakeInhibitor{platform: PlatformLinux, supported: true}
	service := newTestService(t, reader, inhibitor)

	if _, err := service.Update(context.Background(), Settings{Enabled: true}); err != nil {
		t.Fatalf("enable before start: %v", err)
	}
	if inhibitor.acquireCount() != 1 {
		t.Fatalf("synchronous update acquire count = %d, want 1", inhibitor.acquireCount())
	}
	service.Stop()
	if inhibitor.releaseCount() != 1 {
		t.Fatalf("shutdown release count = %d, want 1", inhibitor.releaseCount())
	}
}

func TestServiceRetriesAfterUnexpectedLeaseExitAndAcquireFailure(t *testing.T) {
	reader := &fakeSessionReader{}
	reader.set(session(models.TaskSessionStateRunning))
	inhibitor := &fakeInhibitor{
		platform:   PlatformLinux,
		supported:  true,
		acquireErr: NewIssueError(IssueRequestFailed, errors.New("temporary")),
	}
	service := newTestService(t, reader, inhibitor, WithReconcileInterval(5*time.Millisecond))
	if _, err := service.Update(context.Background(), Settings{Enabled: true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	response, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get failed state: %v", err)
	}
	if response.Status.Issue != IssueRequestFailed {
		t.Fatalf("failed acquire issue = %q, want %q", response.Status.Issue, IssueRequestFailed)
	}

	inhibitor.setAcquireErr(nil)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer service.Stop()
	waitFor(t, func() bool { return inhibitor.acquireCount() >= 2 })
	lease := inhibitor.lastLease()
	lease.fail(errors.New("helper exited"))
	waitFor(t, func() bool { return inhibitor.acquireCount() >= 3 })
	response, err = service.Get(context.Background())
	if err != nil {
		t.Fatalf("get after lease retry: %v", err)
	}
	if !response.Status.Active {
		t.Fatalf("lease was not active after retry: %#v", response.Status)
	}
}

func TestServiceKeepsLeaseWhenRepositoryReadFails(t *testing.T) {
	reader := &fakeSessionReader{}
	reader.set(session(models.TaskSessionStateRunning))
	inhibitor := &fakeInhibitor{platform: PlatformLinux, supported: true}
	service := newTestService(t, reader, inhibitor)
	if _, err := service.Update(context.Background(), Settings{Enabled: true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !inhibitor.hasLease() {
		t.Fatal("expected lease after enabling with working session")
	}

	reader.setError(errors.New("database unavailable"))
	if _, err := service.Update(context.Background(), Settings{Enabled: true}); err != nil {
		t.Fatalf("update with read failure: %v", err)
	}
	response, err := service.Get(context.Background())
	if err != nil {
		t.Fatalf("get after read failure: %v", err)
	}
	if !response.Status.Active || inhibitor.releaseCount() != 0 {
		t.Fatalf("read failure changed active lease: response=%#v releases=%d", response, inhibitor.releaseCount())
	}
}

func TestServiceReportsCapabilityFailureBeforeAWorkingSessionExists(t *testing.T) {
	reader := &fakeSessionReader{}
	inhibitor := &fakeInhibitor{
		platform:  PlatformLinux,
		supported: true,
		probeErr:  NewIssueError(IssueSystemServiceUnavailable, errors.New("logind unavailable")),
	}
	service := newTestService(t, reader, inhibitor)

	response, err := service.Update(context.Background(), Settings{Enabled: true})
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if response.Status.Active {
		t.Fatal("capability failure reported an active lease")
	}
	if response.Status.Issue != IssueSystemServiceUnavailable {
		t.Fatalf("capability issue = %q, want %q", response.Status.Issue, IssueSystemServiceUnavailable)
	}
}

func newTestService(t *testing.T, reader *fakeSessionReader, inhibitor *fakeInhibitor, options ...Option) *Service {
	t.Helper()
	eventBus := bus.NewMemoryEventBus(logger.Default())
	service := NewService(NewStore(&memoryRawStore{}), reader, inhibitor, eventBus, nil, options...)
	t.Cleanup(service.Stop)
	return service
}

func publishSessionEvent(t *testing.T, service *Service) {
	t.Helper()
	if err := service.eventBus.Publish(
		context.Background(),
		events.TaskSessionStateChanged,
		bus.NewEvent(events.TaskSessionStateChanged, "sleep-inhibition-test", nil),
	); err != nil {
		t.Fatalf("publish session event: %v", err)
	}
}

func waitFor(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for condition")
}

func session(state models.TaskSessionState) *models.TaskSession {
	return &models.TaskSession{State: state}
}

type memoryRawStore struct {
	mu    sync.Mutex
	raw   []byte
	found bool
}

func (s *memoryRawStore) Get(context.Context, string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.raw...), s.found, nil
}

func (s *memoryRawStore) Save(_ context.Context, _ string, raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append([]byte(nil), raw...)
	s.found = true
	return nil
}

type fakeSessionReader struct {
	mu       sync.Mutex
	sessions []*models.TaskSession
	err      error
}

func (r *fakeSessionReader) ListActiveTaskSessions(context.Context) ([]*models.TaskSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return nil, r.err
	}
	return append([]*models.TaskSession(nil), r.sessions...), nil
}

func (r *fakeSessionReader) set(sessions ...*models.TaskSession) {
	r.mu.Lock()
	r.sessions = sessions
	r.err = nil
	r.mu.Unlock()
}

func (r *fakeSessionReader) setError(err error) {
	r.mu.Lock()
	r.err = err
	r.mu.Unlock()
}

type fakeInhibitor struct {
	mu         sync.Mutex
	platform   Platform
	supported  bool
	probeErr   error
	acquireErr error
	leases     []*fakeLease
	acquires   int
	releases   int
}

func (i *fakeInhibitor) Platform() Platform { return i.platform }
func (i *fakeInhibitor) Supported() bool    { return i.supported }
func (i *fakeInhibitor) Probe(context.Context) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.probeErr
}

func (i *fakeInhibitor) Acquire(context.Context) (Lease, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.acquires++
	if i.acquireErr != nil {
		return nil, i.acquireErr
	}
	lease := newFakeLease(i)
	i.leases = append(i.leases, lease)
	return lease, nil
}

func (i *fakeInhibitor) setAcquireErr(err error) {
	i.mu.Lock()
	i.acquireErr = err
	i.mu.Unlock()
}

func (i *fakeInhibitor) acquireCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.acquires
}

func (i *fakeInhibitor) releaseCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.releases
}

func (i *fakeInhibitor) hasLease() bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.leases) > 0
}

func (i *fakeInhibitor) lastLease() *fakeLease {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.leases[len(i.leases)-1]
}

type fakeLease struct {
	inhibitor *fakeInhibitor
	done      chan error
	once      sync.Once
}

func newFakeLease(inhibitor *fakeInhibitor) *fakeLease {
	return &fakeLease{inhibitor: inhibitor, done: make(chan error, 1)}
}

func (l *fakeLease) Release() error {
	l.once.Do(func() {
		l.inhibitor.mu.Lock()
		l.inhibitor.releases++
		l.inhibitor.mu.Unlock()
		close(l.done)
	})
	return nil
}

func (l *fakeLease) Done() <-chan error { return l.done }

func (l *fakeLease) fail(err error) {
	l.once.Do(func() {
		l.done <- err
		close(l.done)
	})
}
