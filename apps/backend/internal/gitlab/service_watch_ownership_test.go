package gitlab

import (
	"context"
	"errors"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

type barrierCascadeDeleter struct {
	entered chan string
	release chan struct{}
}

func (d *barrierCascadeDeleter) DeleteTaskTree(_ context.Context, id string, cascade bool) (*taskservice.CascadeOutcome, error) {
	if !cascade {
		return nil, errors.New("cascade required")
	}
	d.entered <- id
	<-d.release
	return &taskservice.CascadeOutcome{}, nil
}

type ownershipTaskDeleter struct{ deleted chan string }

func (d *ownershipTaskDeleter) DeleteTask(_ context.Context, id string) error {
	d.deleted <- id
	return nil
}

type watchOwnershipFixture struct {
	store      *Store
	watchID    string
	watchTable string
	generation int64
	reserve    func(context.Context, int64, int) (bool, error)
	assign     func(context.Context, int64, int, string) error
	reset      func(context.Context) (int, error)
	delete     func(context.Context) error
	loadGen    func(context.Context) (int64, bool, error)
}

func newReviewOwnershipFixture(t *testing.T, cascade *barrierCascadeDeleter, taskDeleter *ownershipTaskDeleter) watchOwnershipFixture {
	t.Helper()
	store := newTestStore(t)
	watch := &ReviewWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	if err := store.CreateReviewWatch(t.Context(), watch); err != nil {
		t.Fatal(err)
	}
	service := NewService("", nil, AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)
	service.SetCascadeTaskDeleter(cascade)
	service.SetTaskDeleter(taskDeleter)
	return watchOwnershipFixture{
		store:      store,
		watchID:    watch.ID,
		watchTable: "gitlab_review_watches",
		generation: watch.Generation,
		reserve: func(ctx context.Context, generation int64, iid int) (bool, error) {
			return service.ReserveReviewMRTask(ctx, watch.ID, generation, "group/project", iid, "url")
		},
		assign: func(ctx context.Context, generation int64, iid int, taskID string) error {
			return service.AssignReviewMRTaskID(ctx, watch.ID, generation, "group/project", iid, taskID)
		},
		reset:  func(ctx context.Context) (int, error) { return service.ResetReviewWatch(ctx, watch.ID) },
		delete: func(ctx context.Context) error { return service.DeleteReviewWatch(ctx, watch.ID) },
		loadGen: func(ctx context.Context) (int64, bool, error) {
			loaded, err := store.GetReviewWatch(ctx, watch.ID)
			if loaded == nil {
				return 0, false, err
			}
			return loaded.Generation, true, err
		},
	}
}

func newIssueOwnershipFixture(t *testing.T, cascade *barrierCascadeDeleter, taskDeleter *ownershipTaskDeleter) watchOwnershipFixture {
	t.Helper()
	store := newTestStore(t)
	watch := &IssueWatch{WorkspaceID: "workspace-a", WorkflowID: "wf", WorkflowStepID: "step", AgentProfileID: "agent", ExecutorProfileID: "exec", Enabled: true}
	if err := store.CreateIssueWatch(t.Context(), watch); err != nil {
		t.Fatal(err)
	}
	service := NewService("", nil, AuthMethodNone, nil, newTestLogger(t))
	service.SetStore(store)
	service.SetCascadeTaskDeleter(cascade)
	service.SetTaskDeleter(taskDeleter)
	return watchOwnershipFixture{
		store:      store,
		watchID:    watch.ID,
		watchTable: "gitlab_issue_watches",
		generation: watch.Generation,
		reserve: func(ctx context.Context, generation int64, iid int) (bool, error) {
			return service.ReserveIssueWatchTask(ctx, watch.ID, generation, "group/project", iid, "url")
		},
		assign: func(ctx context.Context, generation int64, iid int, taskID string) error {
			return service.AssignIssueWatchTaskID(ctx, watch.ID, generation, "group/project", iid, taskID)
		},
		reset:  func(ctx context.Context) (int, error) { return service.ResetIssueWatch(ctx, watch.ID) },
		delete: func(ctx context.Context) error { return service.DeleteIssueWatch(ctx, watch.ID) },
		loadGen: func(ctx context.Context) (int64, bool, error) {
			loaded, err := store.GetIssueWatch(ctx, watch.ID)
			if loaded == nil {
				return 0, false, err
			}
			return loaded.Generation, true, err
		},
	}
}

func TestWatchDeleteCompletesAfterCallerCancellation(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func(*testing.T, *barrierCascadeDeleter, *ownershipTaskDeleter) watchOwnershipFixture
	}{{"review", newReviewOwnershipFixture}, {"issue", newIssueOwnershipFixture}} {
		t.Run(test.name, func(t *testing.T) {
			cascade := &barrierCascadeDeleter{entered: make(chan string, 1), release: make(chan struct{})}
			t.Cleanup(func() {
				select {
				case <-cascade.release:
				default:
					close(cascade.release)
				}
			})
			fixture := test.new(t, cascade, &ownershipTaskDeleter{deleted: make(chan string, 1)})
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 1); err != nil || !ok {
				t.Fatalf("reserve: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 1, "task-existing"); err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithCancel(context.Background())
			deleteDone := make(chan error, 1)
			go func() { deleteDone <- fixture.delete(ctx) }()
			if got := <-cascade.entered; got != "task-existing" {
				t.Fatalf("delete cleaned %q", got)
			}
			cancel()
			close(cascade.release)
			if err := <-deleteDone; err != nil {
				t.Fatalf("delete after caller cancellation: %v", err)
			}
			if _, exists, err := fixture.loadGen(t.Context()); err != nil || exists {
				t.Fatalf("watch remains after detached delete: exists=%v err=%v", exists, err)
			}
			if err := fixture.delete(t.Context()); err != nil {
				t.Fatalf("idempotent retry after completed delete: %v", err)
			}
		})
	}
}

func TestWatchDeleteResumesTombstoneAfterFinalDeleteFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func(*testing.T, *barrierCascadeDeleter, *ownershipTaskDeleter) watchOwnershipFixture
	}{{"review", newReviewOwnershipFixture}, {"issue", newIssueOwnershipFixture}} {
		t.Run(test.name, func(t *testing.T) {
			cascade := &barrierCascadeDeleter{entered: make(chan string, 2), release: make(chan struct{}, 2)}
			t.Cleanup(func() {
				for range 2 {
					select {
					case cascade.release <- struct{}{}:
					default:
					}
				}
			})
			fixture := test.new(t, cascade, &ownershipTaskDeleter{deleted: make(chan string, 1)})
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 1); err != nil || !ok {
				t.Fatalf("reserve: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 1, "task-existing"); err != nil {
				t.Fatal(err)
			}
			trigger := "fail_" + test.name + "_watch_delete"
			if _, err := fixture.store.db.Exec("CREATE TRIGGER " + trigger + " BEFORE DELETE ON " + fixture.watchTable +
				" WHEN OLD.id = '" + fixture.watchID + "' BEGIN SELECT RAISE(FAIL, 'forced final delete failure'); END"); err != nil {
				t.Fatalf("create failure trigger: %v", err)
			}

			firstDone := make(chan error, 1)
			go func() { firstDone <- fixture.delete(context.Background()) }()
			<-cascade.entered
			cascade.release <- struct{}{}
			if err := <-firstDone; err == nil {
				t.Fatal("delete unexpectedly succeeded with failure trigger")
			}
			var deleting bool
			var generation int64
			if err := fixture.store.ro.QueryRow("SELECT deleting, generation FROM "+fixture.watchTable+" WHERE id = ?", fixture.watchID).Scan(&deleting, &generation); err != nil {
				t.Fatalf("load tombstone: %v", err)
			}
			if !deleting || generation <= fixture.generation {
				t.Fatalf("tombstone deleting=%v generation=%d", deleting, generation)
			}
			if _, err := fixture.store.db.Exec("DROP TRIGGER " + trigger); err != nil {
				t.Fatalf("drop failure trigger: %v", err)
			}

			retryDone := make(chan error, 1)
			go func() { retryDone <- fixture.delete(context.Background()) }()
			<-cascade.entered
			cascade.release <- struct{}{}
			if err := <-retryDone; err != nil {
				t.Fatalf("retry tombstone delete: %v", err)
			}
			if _, exists, err := fixture.loadGen(t.Context()); err != nil || exists {
				t.Fatalf("watch remains after retry: exists=%v err=%v", exists, err)
			}
		})
	}
}

func TestWatchResetInvalidatesEventReservedBeforeReset(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func(*testing.T, *barrierCascadeDeleter, *ownershipTaskDeleter) watchOwnershipFixture
	}{{"review", newReviewOwnershipFixture}, {"issue", newIssueOwnershipFixture}} {
		t.Run(test.name, func(t *testing.T) {
			cascade := &barrierCascadeDeleter{entered: make(chan string, 1), release: make(chan struct{})}
			t.Cleanup(func() {
				select {
				case <-cascade.release:
				default:
					close(cascade.release)
				}
			})
			deleted := &ownershipTaskDeleter{deleted: make(chan string, 1)}
			fixture := test.new(t, cascade, deleted)
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 1); err != nil || !ok {
				t.Fatalf("reserve existing task: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 1, "task-existing"); err != nil {
				t.Fatal(err)
			}
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 2); err != nil || !ok {
				t.Fatalf("reserve delayed event: ok=%v err=%v", ok, err)
			}

			resetDone := make(chan error, 1)
			go func() {
				_, err := fixture.reset(context.Background())
				resetDone <- err
			}()
			if got := <-cascade.entered; got != "task-existing" {
				t.Fatalf("reset deleted %q first", got)
			}

			if ok, err := fixture.reserve(t.Context(), fixture.generation, 3); err != nil || ok {
				t.Fatalf("stale reserve during reset: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 2, "task-created-late"); !errors.Is(err, ErrWatchOwnershipLost) {
				t.Fatalf("stale attach error=%v", err)
			}
			if got := <-deleted.deleted; got != "task-created-late" {
				t.Fatalf("stale task cleanup=%q", got)
			}

			close(cascade.release)
			if err := <-resetDone; err != nil {
				t.Fatalf("reset: %v", err)
			}
			newGeneration, exists, err := fixture.loadGen(t.Context())
			if err != nil || !exists || newGeneration <= fixture.generation {
				t.Fatalf("generation after reset=%d exists=%v err=%v", newGeneration, exists, err)
			}
			if ok, err := fixture.reserve(t.Context(), newGeneration, 3); err != nil || !ok {
				t.Fatalf("current generation reserve: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestWatchDeleteRejectsDelayedOldEvent(t *testing.T) {
	for _, test := range []struct {
		name string
		new  func(*testing.T, *barrierCascadeDeleter, *ownershipTaskDeleter) watchOwnershipFixture
	}{{"review", newReviewOwnershipFixture}, {"issue", newIssueOwnershipFixture}} {
		t.Run(test.name, func(t *testing.T) {
			cascade := &barrierCascadeDeleter{entered: make(chan string, 1), release: make(chan struct{})}
			t.Cleanup(func() {
				select {
				case <-cascade.release:
				default:
					close(cascade.release)
				}
			})
			deleted := &ownershipTaskDeleter{deleted: make(chan string, 1)}
			fixture := test.new(t, cascade, deleted)
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 1); err != nil || !ok {
				t.Fatalf("reserve existing task: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 1, "task-existing"); err != nil {
				t.Fatal(err)
			}
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 2); err != nil || !ok {
				t.Fatalf("reserve delayed event: ok=%v err=%v", ok, err)
			}

			deleteDone := make(chan error, 1)
			go func() { deleteDone <- fixture.delete(context.Background()) }()
			if got := <-cascade.entered; got != "task-existing" {
				t.Fatalf("delete cleaned %q first", got)
			}
			if ok, err := fixture.reserve(t.Context(), fixture.generation, 3); err != nil || ok {
				t.Fatalf("stale reserve during delete: ok=%v err=%v", ok, err)
			}
			if err := fixture.assign(t.Context(), fixture.generation, 2, "task-created-late"); !errors.Is(err, ErrWatchOwnershipLost) {
				t.Fatalf("stale attach error=%v", err)
			}
			if got := <-deleted.deleted; got != "task-created-late" {
				t.Fatalf("stale task cleanup=%q", got)
			}
			close(cascade.release)
			if err := <-deleteDone; err != nil {
				t.Fatalf("delete: %v", err)
			}
			if _, exists, err := fixture.loadGen(t.Context()); err != nil || exists {
				t.Fatalf("watch remains after delete: exists=%v err=%v", exists, err)
			}
		})
	}
}
