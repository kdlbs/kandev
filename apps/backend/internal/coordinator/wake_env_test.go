package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// fakeWakeSources is a WakeSources over maps, with per-method failures.
type fakeWakeSources struct {
	mu       sync.Mutex
	primary  map[string]string
	question map[string]string
	perms    map[string][]string
	errStamp map[string]string
	state    map[string]string
	last     map[string]*time.Time
	fail     map[string]error
}

func newFakeWakeSources() *fakeWakeSources {
	return &fakeWakeSources{
		primary: map[string]string{}, question: map[string]string{}, perms: map[string][]string{},
		errStamp: map[string]string{}, state: map[string]string{}, last: map[string]*time.Time{},
		fail: map[string]error{},
	}
}

func (f *fakeWakeSources) failWith(method string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.fail, method)
		return
	}
	f.fail[method] = err
}

func (f *fakeWakeSources) PrimarySessionID(_ context.Context, taskID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.primary[taskID], f.fail["PrimarySessionID"]
}

func (f *fakeWakeSources) PendingQuestionID(_ context.Context, sessionID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.question[sessionID], f.fail["PendingQuestionID"]
}

func (f *fakeWakeSources) PendingPermissionIDs(_ context.Context, sessionID string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.perms[sessionID], f.fail["PendingPermissionIDs"]
}

func (f *fakeWakeSources) ActiveErrorStamp(_ context.Context, sessionID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.errStamp[sessionID], f.fail["ActiveErrorStamp"]
}

func (f *fakeWakeSources) TaskState(_ context.Context, taskID string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state[taskID], f.fail["TaskState"]
}

func (f *fakeWakeSources) LastActivityAt(_ context.Context, taskID string) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last[taskID], f.fail["LastActivityAt"]
}

// wakeEnv is a phase 3 service over a store with the task tables the own-task
// predicates read, an in-memory bus, an autonomous coordinator watching every
// workflow, and a fake WakeSources.
type wakeEnv struct {
	svc     *Service
	store   *Store
	fix     *wakeFixture
	bus     *bus.MemoryEventBus
	sources *fakeWakeSources

	mu       sync.Mutex
	sequence []string
}

func (e *wakeEnv) note(s string) {
	e.mu.Lock()
	e.sequence = append(e.sequence, s)
	e.mu.Unlock()
}

func (e *wakeEnv) seq() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.sequence...)
}

func newWakeEnv(t *testing.T) *wakeEnv {
	t.Helper()
	store := newTestStore(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	f := newWakeFixture(t, store)
	mustExec(t, store, `UPDATE coordinators SET watch_scope = 'all' WHERE id = ?`, f.c.ID)
	svc := NewService(store, newValidatorForTest(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t), WithPhase2(true), WithPhase3(true))
	memBus := bus.NewMemoryEventBus(newTestLogger(t))
	t.Cleanup(memBus.Close)
	env := &wakeEnv{svc: svc, store: store, fix: f, bus: memBus, sources: newFakeWakeSources()}
	svc.SetDecisionDeps(nil, nil, memBus)
	if _, err := memBus.Subscribe(events.CoordinatorUpdated, func(_ context.Context, ev *bus.Event) error {
		if p, ok := ev.Data.(CoordinatorUpdatedPayload); ok && p.AutonomyChanged {
			env.note("updated")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	svc.SetKick(func(_ context.Context, id string) error {
		env.note("kick:" + id)
		return nil
	})
	return env
}

// ownTask adds an own task in wf-a with a primary session "s-<id>".
func (e *wakeEnv) ownTask(t *testing.T, id string) string {
	t.Helper()
	e.fix.addOwn(t, id, "wf-a")
	sid := "s-" + id
	e.sources.mu.Lock()
	e.sources.primary[id] = sid
	e.sources.mu.Unlock()
	return sid
}

func (e *wakeEnv) start(t *testing.T) {
	t.Helper()
	e.svc.StartWakeRecorder(e.bus, e.sources)
	t.Cleanup(e.svc.StopWakeRecorder)
}

func (e *wakeEnv) publish(t *testing.T, subject string, data map[string]any) {
	t.Helper()
	if err := e.bus.Publish(context.Background(), subject, bus.NewEvent(subject, "test", data)); err != nil {
		t.Fatal(err)
	}
}

type storedWake struct{ TaskID, Kind, Key, Status string }

func (e *wakeEnv) wakes(t *testing.T) []storedWake {
	t.Helper()
	rows, err := e.store.db.Query(`SELECT task_id, kind, episode_key, status FROM coordinator_wakes ORDER BY task_id, kind, episode_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []storedWake
	for rows.Next() {
		var w storedWake
		if err := rows.Scan(&w.TaskID, &w.Kind, &w.Key, &w.Status); err != nil {
			t.Fatal(err)
		}
		out = append(out, w)
	}
	return out
}

var errBoom = errors.New("boom")
