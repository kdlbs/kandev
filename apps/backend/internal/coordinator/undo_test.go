package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
)

func authedContext(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID})
}

func assertUndoRefusal(t *testing.T, err error, code, reason string) {
	t.Helper()
	var r *UndoRefusal
	if !errors.As(err, &r) {
		t.Fatalf("err = %v, want *UndoRefusal", err)
	}
	if r.Code != code || r.Reason != reason {
		t.Fatalf("refusal = %s/%s, want %s/%s", r.Code, r.Reason, code, reason)
	}
}

func undoneRows(t *testing.T, store *Store, coordinatorID string) []ActivityRow {
	t.Helper()
	var out []ActivityRow
	for _, r := range listActivity(t, store, coordinatorID) {
		if r.Outcome == ActivityUndone {
			out = append(out, r)
		}
	}
	return out
}

func seedCreateRow(t *testing.T, store *Store, c *Coordinator, id, taskID string) {
	t.Helper()
	seedApproved(t, store, c, id, ActionCreateTask, time.Now().UTC(), func(r *ActivityRow) {
		r.TargetTaskID = &taskID
		r.Detail = "make it"
	})
}

func seedMoveRow(t *testing.T, store *Store, c *Coordinator, id, taskID, outcome string) {
	t.Helper()
	pid := "p-" + id
	seedMoveProposal(t, store, c, pid, outcome)
	seedApproved(t, store, c, id, ActionMove, time.Now().UTC(), func(r *ActivityRow) {
		r.TargetTaskID = &taskID
		r.ProposalID = &pid
	})
}

const moveOutcomeS1toS2 = `{"from_step_id":"s1","to_step_id":"s2"}`

func moveFixture(t *testing.T) (*Service, *Store, *Coordinator, *fakeUndoTasks) {
	svc, store, c, _, fake := newActivityService(t, true)
	fake.tasks["t1"] = &UndoTask{Identifier: "KAN-1", WorkflowID: "wf", WorkflowStepID: "s2"}
	fake.steps["s1"] = &UndoStep{Name: "Todo", WorkflowID: "wf"}
	seedMoveRow(t, store, c, "m1", "t1", moveOutcomeS1toS2)
	return svc, store, c, fake
}

func TestUndoActivity_CreateArchivesAndMarks(t *testing.T) {
	svc, store, c, az, fake := newActivityService(t, true)
	ctx := authedContext("user-7")
	seedCreateRow(t, store, c, "r1", "t1")
	item, err := svc.UndoActivity(ctx, "ws-1", c.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.archived) != 1 || fake.archived[0] != "t1" {
		t.Fatalf("archived = %v", fake.archived)
	}
	if item.UndoneAt == nil || item.UndoneBy == nil || *item.UndoneBy != "user-7" || item.Undoable {
		t.Fatalf("item = %+v", item)
	}
	rows := undoneRows(t, store, c.ID)
	if len(rows) != 1 {
		t.Fatalf("undone rows = %d", len(rows))
	}
	u := rows[0]
	if u.UndoOfID == nil || *u.UndoOfID != "r1" || u.ActionClass != ActionCreateTask || u.Detail != "make it" ||
		u.Authorization != AuthRequiresApproval || u.ActorUserID == nil || *u.ActorUserID != "user-7" ||
		u.TargetTaskID == nil || *u.TargetTaskID != "t1" {
		t.Fatalf("undone row = %+v", u)
	}
	if az.scopes[0] != authz.ScopeWorkspaceManage {
		t.Fatalf("scopes = %v", az.scopes)
	}
}

func TestUndoActivity_NoAuthStoresNullUsers(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	item, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if item.UndoneBy != nil {
		t.Fatalf("undone_by = %v", *item.UndoneBy)
	}
	if u := undoneRows(t, store, c.ID)[0]; u.ActorUserID != nil {
		t.Fatalf("actor = %v", *u.ActorUserID)
	}
}

func TestUndoActivity_CreateGoneTaskCountsAsDone(t *testing.T) {
	for _, archErr := range []error{ErrTaskAlreadyArchived, ErrTaskNotFound} {
		svc, store, c, _, fake := newActivityService(t, true)
		seedCreateRow(t, store, c, "r1", "t1")
		fake.archErr = archErr
		if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1"); err != nil {
			t.Fatalf("%v: %v", archErr, err)
		}
		if len(undoneRows(t, store, c.ID)) != 1 {
			t.Fatalf("%v: no undone row", archErr)
		}
	}
}

func TestUndoActivity_CreateArchiveFailureWritesNothing(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	fake.archErr = errors.New("boom")
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1"); err == nil || errors.As(err, new(*UndoRefusal)) {
		t.Fatalf("err = %v", err)
	}
	if len(undoneRows(t, store, c.ID)) != 0 {
		t.Fatal("undone row written")
	}
	row, _ := store.GetActivityRow(context.Background(), store.db, c.ID, "r1")
	if row.UndoneAt != nil {
		t.Fatal("row marked")
	}
}

func TestUndoActivity_Refusals(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	ctx := context.Background()
	seedCreateRow(t, store, c, "done", "t1")
	if _, err := svc.UndoActivity(ctx, "ws-1", c.ID, "done"); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UndoActivity(ctx, "ws-1", c.ID, "done")
	assertUndoRefusal(t, err, UndoAlreadyUndone, "")
	if len(fake.archived) != 1 {
		t.Fatalf("second undo called the task service: %v", fake.archived)
	}
	seedApproved(t, store, c, "msg", ActionMessage, time.Now().UTC(), nil)
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "msg")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
	seedActivity(t, store, c.ID, "prop", ActionCreateTask, ActivityProposed, time.Now().UTC())
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "prop")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
	seedApproved(t, store, c, "notask", ActionCreateTask, time.Now().UTC(), nil)
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "notask")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
	seedMoveRow(t, store, c, "noop", "t1", `{"from_step_id":"s2","to_step_id":"s2","noop":true}`)
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "noop")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
	pid := "p-notarget"
	seedMoveProposal(t, store, c, pid, moveOutcomeS1toS2)
	seedApproved(t, store, c, "notarget", ActionMove, time.Now().UTC(), func(r *ActivityRow) { r.ProposalID = &pid })
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "notarget")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
	seedMoveRow(t, store, c, "badout", "t1", `{"to_step_id":"s2"}`)
	_, err = svc.UndoActivity(ctx, "ws-1", c.ID, "badout")
	assertUndoRefusal(t, err, UndoNotUndoable, "")
}

func TestUndoActivity_NotFoundForeignAndPhaseOff(t *testing.T) {
	svc, store, c, _, _ := newActivityService(t, true)
	other := newTestCoordinator(t, store, "ws-1")
	seedCreateRow(t, store, other, "theirs", "t9")
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "theirs"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign row err = %v", err)
	}
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("absent row err = %v", err)
	}
	off, _, oc, _, _ := newActivityService(t, false)
	if _, err := off.UndoActivity(context.Background(), "ws-1", oc.ID, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("phase off err = %v", err)
	}
}

func TestUndoActivity_MoveBack(t *testing.T) {
	for _, autoStart := range []bool{false, true} {
		svc, store, c, fake := moveFixture(t)
		fake.steps["s1"].AutoStart = autoStart
		if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "m1"); err != nil {
			t.Fatal(err)
		}
		if len(fake.moves) != 1 {
			t.Fatalf("moves = %+v", fake.moves)
		}
		m := fake.moves[0]
		if m.ID != "t1" || m.WorkflowID != "wf" || m.StepID != "s1" || m.Opts.ExpectedWorkflowID != "wf" || m.Opts.SkipStepPrompt != autoStart {
			t.Fatalf("autoStart=%v move = %+v", autoStart, m)
		}
		if len(undoneRows(t, store, c.ID)) != 1 {
			t.Fatal("no undone row")
		}
	}
}

func TestUndoActivity_MoveRefusalsWriteNothing(t *testing.T) {
	archived := time.Now().UTC()
	cases := []struct {
		name   string
		mutate func(f *fakeUndoTasks)
		reason string
	}{
		{"archived", func(f *fakeUndoTasks) { f.tasks["t1"].ArchivedAt = &archived }, "archived"},
		{"task gone", func(f *fakeUndoTasks) { delete(f.tasks, "t1") }, "archived"},
		{"elsewhere", func(f *fakeUndoTasks) { f.tasks["t1"].WorkflowStepID = "s9" }, "moved"},
		{"step deleted", func(f *fakeUndoTasks) { delete(f.steps, "s1") }, "step_deleted"},
		{"step done", func(f *fakeUndoTasks) { f.steps["s1"].CompletesOnEnter = true }, "step_done"},
		{"agent running", func(f *fakeUndoTasks) { f.sessions = true }, "agent_running"},
		{"step full", func(f *fakeUndoTasks) { f.moveErr = ErrWIPLimitExceeded }, "step_full"},
		{"move conflict", func(f *fakeUndoTasks) { f.moveErr = ErrMoveConflict }, "moved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, store, c, fake := moveFixture(t)
			tc.mutate(fake)
			_, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "m1")
			assertUndoRefusal(t, err, UndoConflict, tc.reason)
			if len(undoneRows(t, store, c.ID)) != 0 {
				t.Fatal("undone row written")
			}
			row, _ := store.GetActivityRow(context.Background(), store.db, c.ID, "m1")
			if row.UndoneAt != nil {
				t.Fatal("row marked")
			}
		})
	}
}

func TestUndoActivity_MoveAlreadyBackSkipsTheCall(t *testing.T) {
	svc, store, c, fake := moveFixture(t)
	fake.tasks["t1"].WorkflowStepID = "s1"
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	if len(fake.moves) != 0 || len(undoneRows(t, store, c.ID)) != 1 {
		t.Fatalf("moves = %v", fake.moves)
	}
}

func TestUndoActivity_MoveQueuedBehindLimitStillMarks(t *testing.T) {
	svc, store, c, fake := moveFixture(t)
	fake.admitted = false
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	if len(undoneRows(t, store, c.ID)) != 1 {
		t.Fatal("no undone row")
	}
}

func TestUndoActivity_MoveReadErrorsAre500NotRefusals(t *testing.T) {
	boom := errors.New("boom")
	cases := map[string]func(f *fakeUndoTasks){
		"task read":    func(f *fakeUndoTasks) { f.taskErr = boom },
		"step read":    func(f *fakeUndoTasks) { f.stepErr = boom },
		"session read": func(f *fakeUndoTasks) { f.sessErr = boom },
		"move failure": func(f *fakeUndoTasks) { f.moveErr = boom },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			svc, store, c, fake := moveFixture(t)
			mutate(fake)
			_, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "m1")
			if err == nil || errors.As(err, new(*UndoRefusal)) || errors.Is(err, ErrNotFound) {
				t.Fatalf("err = %v", err)
			}
			if len(undoneRows(t, store, c.ID)) != 0 {
				t.Fatal("undone row written")
			}
		})
	}
}

func TestUndoActivity_ConcurrentUndosReverseOnce(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1")
		}()
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range errs {
		var r *UndoRefusal
		switch {
		case err == nil:
			ok++
		case errors.As(err, &r) && r.Code == UndoAlreadyUndone:
			refused++
		default:
			t.Fatalf("err = %v", err)
		}
	}
	if ok != 1 || refused != 1 || len(fake.archived) != 1 || len(undoneRows(t, store, c.ID)) != 1 {
		t.Fatalf("ok=%d refused=%d archived=%d", ok, refused, len(fake.archived))
	}
}

func TestUndoActivity_WaiterHonoursContext(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	entered := make(chan struct{})
	release := make(chan struct{})
	fake.onArchive = func() {
		close(entered)
		<-release
	}
	first := make(chan error, 1)
	go func() {
		_, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1")
		first <- err
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := svc.UndoActivity(ctx, "ws-1", c.ID, "r1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiter err = %v", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if len(undoneRows(t, store, c.ID)) != 1 {
		t.Fatal("holder did not finish")
	}
	if n := svc.undoLocks.size(); n != 0 {
		t.Fatalf("lock entries left = %d", n)
	}
}

func TestUndoActivity_RowRetainedAwayMidUndoIs404(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	fake.onArchive = func() {
		if _, err := store.DeleteActivityBatch(context.Background(), time.Now().UTC().Add(time.Hour), 10); err != nil {
			t.Error(err)
		}
	}
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestUndoActivity_CoordinatorDeletedMidUndoIs404AndWritesNothing(t *testing.T) {
	svc, store, c, _, fake := newActivityService(t, true)
	seedCreateRow(t, store, c, "r1", "t1")
	fake.onArchive = func() {
		if err := store.DeleteCoordinator(context.Background(), "ws-1", c.ID); err != nil {
			t.Error(err)
		}
	}
	if _, err := svc.UndoActivity(context.Background(), "ws-1", c.ID, "r1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(fake.archived) != 1 {
		t.Fatal("reversal did not run")
	}
}
