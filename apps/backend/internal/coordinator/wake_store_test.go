package coordinator

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

// wakeFixture is a coordinator with autonomy on, over a tasks table shaped
// like the production one for the columns the own-task predicates read.
type wakeFixture struct {
	store *Store
	c     *Coordinator
}

func newWakeFixture(t *testing.T, store *Store) *wakeFixture {
	t.Helper()
	tsType := "DATETIME"
	if store.db.DriverName() != "sqlite3" {
		tsType = "TIMESTAMP"
	}
	mustExec(t, store, `CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, state TEXT,
		archived_at `+tsType+`, is_ephemeral INTEGER NOT NULL DEFAULT 0)`)
	c := newTestCoordinator(t, store, "ws-1")
	setAutonomy(t, store, c.ID, true)
	return &wakeFixture{store: store, c: c}
}

func setAutonomy(t *testing.T, store *Store, coordinatorID string, on bool) {
	t.Helper()
	v := 0
	if on {
		v = 1
	}
	mustExec(t, store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, v, coordinatorID)
}

func (f *wakeFixture) addTask(t *testing.T, id, workflowID string) {
	t.Helper()
	var wf any
	if workflowID != "" {
		wf = workflowID
	}
	mustExec(t, f.store, `INSERT INTO tasks (id, workspace_id, workflow_id, state) VALUES (?, 'ws-1', ?, 'IN_PROGRESS')`, id, wf)
}

func (f *wakeFixture) addProposal(t *testing.T, coordinatorID, taskID, kind, status string) {
	t.Helper()
	now := time.Now().UTC()
	mustExec(t, f.store, `INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, task_id, kind, created_at, updated_at)
		VALUES (?, ?, 'ws-1', ?, '{}', ?, ?, ?, ?)`, uuid.NewString(), coordinatorID, status, taskID, kind, now, now)
}

// addOwn creates a task the coordinator owns.
func (f *wakeFixture) addOwn(t *testing.T, id, workflowID string) {
	t.Helper()
	f.addTask(t, id, workflowID)
	f.addProposal(t, f.c.ID, id, "create_task", "approved")
}

func ownTaskIDs(t *testing.T, store *Store, coordinatorID string) []string {
	t.Helper()
	own, err := store.ListOwnTasks(context.Background(), coordinatorID)
	if err != nil {
		t.Fatalf("ListOwnTasks: %v", err)
	}
	ids := []string{}
	for _, o := range own {
		ids = append(ids, o.TaskID)
	}
	return ids
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func runOwnTasks(t *testing.T, store *Store) {
	f := newWakeFixture(t, store)
	other := newTestCoordinator(t, store, "ws-1")
	f.addOwn(t, "t-b", "wf-a")
	f.addOwn(t, "t-a", "")
	f.addProposal(t, f.c.ID, "t-a", "create_task", "approved") // a second approval names the same task
	f.addTask(t, "t-archived", "wf-a")
	f.addProposal(t, f.c.ID, "t-archived", "create_task", "approved")
	mustExec(t, store, `UPDATE tasks SET archived_at = ? WHERE id = 't-archived'`, time.Now().UTC())
	f.addTask(t, "t-ephemeral", "wf-a")
	f.addProposal(t, f.c.ID, "t-ephemeral", "create_task", "approved")
	mustExec(t, store, `UPDATE tasks SET is_ephemeral = 1 WHERE id = 't-ephemeral'`)
	f.addTask(t, "t-conv", "wf-a")
	f.addProposal(t, f.c.ID, "t-conv", "create_task", "approved")
	mustExec(t, store, `UPDATE coordinators SET conversation_task_id = 't-conv' WHERE id = ?`, f.c.ID)
	for _, kind := range []string{"message", "move", "resume"} {
		f.addTask(t, "t-"+kind, "wf-a")
		f.addProposal(t, f.c.ID, "t-"+kind, kind, "approved")
	}
	f.addTask(t, "t-pending", "wf-a")
	f.addProposal(t, f.c.ID, "t-pending", "create_task", "pending")
	f.addTask(t, "t-other", "wf-a")
	f.addProposal(t, other.ID, "t-other", "create_task", "approved")
	f.addProposal(t, f.c.ID, "t-gone", "create_task", "approved") // no tasks row

	own, err := store.ListOwnTasks(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []OwnTask{{"t-a", "ws-1", ""}, {"t-b", "ws-1", "wf-a"}}
	if len(own) != len(want) || own[0] != want[0] || own[1] != want[1] {
		t.Fatalf("ListOwnTasks = %+v, want %+v", own, want)
	}
	if ids := ownTaskIDs(t, store, other.ID); len(ids) != 1 || ids[0] != "t-other" {
		t.Fatalf("other coordinator own = %v", ids)
	}
	if ids := ownTaskIDs(t, store, "nobody"); ids == nil || len(ids) != 0 {
		t.Fatalf("unknown coordinator own = %v, want empty non-nil", ids)
	}
}

func TestListOwnTasks(t *testing.T) { runOwnTasks(t, newTestStore(t)) }

func TestListOwnTasks_Postgres(t *testing.T) { runOwnTasks(t, newTestStorePostgres(t)) }

func runCoordinatorsOwningTask(t *testing.T, store *Store) {
	f := newWakeFixture(t, store)
	second := newTestCoordinator(t, store, "ws-1")
	off := newTestCoordinator(t, store, "ws-1")
	setAutonomy(t, store, second.ID, true)
	f.addOwn(t, "t-1", "wf-a")
	f.addProposal(t, second.ID, "t-1", "create_task", "approved")
	f.addProposal(t, off.ID, "t-1", "create_task", "approved")
	f.addTask(t, "t-msg", "wf-a")
	f.addProposal(t, f.c.ID, "t-msg", "message", "approved")

	got, err := store.CoordinatorsOwningTask(context.Background(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{f.c.ID, second.ID}
	if want[0] > want[1] {
		want[0], want[1] = want[1], want[0]
	}
	if !sameStrings(got, want) {
		t.Fatalf("CoordinatorsOwningTask = %v, want %v (autonomy-off coordinator excluded, ordered by id)", got, want)
	}
	if got, err := store.CoordinatorsOwningTask(context.Background(), "t-msg"); err != nil || len(got) != 0 {
		t.Fatalf("non-create_task task owners = %v, %v", got, err)
	}
	mustExec(t, store, `UPDATE coordinators SET conversation_task_id = 't-1' WHERE id = ?`, f.c.ID)
	got, _ = store.CoordinatorsOwningTask(context.Background(), "t-1")
	if !sameStrings(got, []string{second.ID}) {
		t.Fatalf("conversation task must not be owned by its coordinator: %v", got)
	}
}

func TestCoordinatorsOwningTask(t *testing.T) { runCoordinatorsOwningTask(t, newTestStore(t)) }

func TestCoordinatorsOwningTask_Postgres(t *testing.T) {
	runCoordinatorsOwningTask(t, newTestStorePostgres(t))
}

func wakeRows(t *testing.T, store *Store, coordinatorID string) int {
	t.Helper()
	var n int
	if err := store.db.QueryRow(store.db.Rebind(`SELECT COUNT(*) FROM coordinator_wakes WHERE coordinator_id = ?`), coordinatorID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func mustRecord(t *testing.T, store *Store, coordinatorID, taskID string, kind WakeKind, key string) RecordWakeResult {
	t.Helper()
	res, err := store.RecordWake(context.Background(), coordinatorID, taskID, kind, key)
	if err != nil {
		t.Fatalf("RecordWake: %v", err)
	}
	return res
}

func runRecordWake(t *testing.T, store *Store) {
	ctx := context.Background()
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")

	res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "q1")
	if res.Outcome != WakeInserted || res.WorkspaceID != "ws-1" {
		t.Fatalf("first = %+v", res)
	}
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "q1"); res.Outcome != WakeExists {
		t.Fatalf("repeat = %+v", res)
	}
	var status string
	if err := store.db.QueryRow(`SELECT status FROM coordinator_wakes`).Scan(&status); err != nil || status != "pending" {
		t.Fatalf("stored status = %q, %v", status, err)
	}
	// A superseded or delivered row is never re-opened.
	mustExec(t, store, `UPDATE coordinator_wakes SET status = 'superseded'`)
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "q1"); res.Outcome != WakeExists {
		t.Fatalf("after supersede = %+v", res)
	}
	if got := count(t, store, "coordinator_wakes"); got != 1 {
		t.Fatalf("wakes = %d", got)
	}
	// A different kind or key is a different episode.
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindPermission, "q1"); res.Outcome != WakeInserted {
		t.Fatalf("other kind = %+v", res)
	}
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "q2"); res.Outcome != WakeInserted {
		t.Fatalf("other key = %+v", res)
	}

	f.addTask(t, "t-msg", "wf-a")
	f.addProposal(t, f.c.ID, "t-msg", "message", "approved")
	if res := mustRecord(t, store, f.c.ID, "t-msg", WakeKindCompleted, "completed"); res.Outcome != WakeNotOwn {
		t.Fatalf("not own = %+v", res)
	}
	if res := mustRecord(t, store, f.c.ID, "t-nope", WakeKindCompleted, "completed"); res.Outcome != WakeNotOwn {
		t.Fatalf("missing task = %+v", res)
	}
	before := count(t, store, "coordinator_wakes")
	setAutonomy(t, store, f.c.ID, false)
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindError, "e1"); res.Outcome != WakeAutonomyOff || res.WorkspaceID != "ws-1" {
		t.Fatalf("autonomy off = %+v", res)
	}
	if got := count(t, store, "coordinator_wakes"); got != before {
		t.Fatalf("wakes stored while autonomy off: %d -> %d", before, got)
	}
	if _, err := store.RecordWake(ctx, "missing", "t-1", WakeKindError, "e1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing coordinator err = %v", err)
	}
}

func TestRecordWake(t *testing.T) { runRecordWake(t, newTestStore(t)) }

func TestRecordWake_Postgres(t *testing.T) { runRecordWake(t, newTestStorePostgres(t)) }

func TestRecordWake_RejectsInvalidInput(t *testing.T) {
	store := newTestStore(t)
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")
	for name, args := range map[string][4]string{
		"empty coordinator": {"", "t-1", "question", "k"},
		"empty task":        {f.c.ID, "", "question", "k"},
		"unknown kind":      {f.c.ID, "t-1", "stalled", "k"},
		"empty key":         {f.c.ID, "t-1", "question", ""},
		"blank key":         {f.c.ID, "t-1", "question", " \t"},
	} {
		if _, err := store.RecordWake(context.Background(), args[0], args[1], WakeKind(args[2]), args[3]); err == nil {
			t.Fatalf("%s: want an error", name)
		}
	}
	if got := count(t, store, "coordinator_wakes"); got != 0 {
		t.Fatalf("invalid input stored %d wakes", got)
	}
}

func seedPendingWakes(t *testing.T, f *wakeFixture, n int) {
	t.Helper()
	for i := range n {
		insertWakeAt(t, f.store, f.c, fmt.Sprintf("seed-%03d", i), "pending", time.Now().UTC())
	}
}

func runRecordWakeCap(t *testing.T, store *Store) {
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")
	seedPendingWakes(t, f, 199)
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "last"); res.Outcome != WakeInserted {
		t.Fatalf("199th slot = %+v", res)
	}
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "over"); res.Outcome != WakeCapped {
		t.Fatalf("at 200 = %+v", res)
	}
	// An existing episode reports exists, not capped, at the cap.
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "last"); res.Outcome != WakeExists {
		t.Fatalf("existing at cap = %+v", res)
	}
	// Delivered wakes do not count against the cap.
	mustExec(t, store, `UPDATE coordinator_wakes SET status = 'delivered' WHERE id = 'seed-000'`)
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "again"); res.Outcome != WakeInserted {
		t.Fatalf("after a slot freed = %+v", res)
	}
}

func TestRecordWake_CapAndFreedSlot(t *testing.T) { runRecordWakeCap(t, newTestStore(t)) }

func TestRecordWake_Postgres_CapAndFreedSlot(t *testing.T) {
	runRecordWakeCap(t, newTestStorePostgres(t))
}

func runConcurrentCap(t *testing.T, store *Store) {
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")
	seedPendingWakes(t, f, 190)
	var wg sync.WaitGroup
	outcomes := make([]WakeOutcome, 20)
	errs := make([]error, 20)
	for i := range outcomes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := store.RecordWake(context.Background(), f.c.ID, "t-1", WakeKindQuestion, fmt.Sprintf("q-%02d", i))
			outcomes[i], errs[i] = res.Outcome, err
		}()
	}
	wg.Wait()
	inserted := 0
	for i, err := range errs {
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if outcomes[i] == WakeInserted {
			inserted++
		} else if outcomes[i] != WakeCapped {
			t.Fatalf("call %d outcome = %s", i, outcomes[i])
		}
	}
	var pending int
	if err := store.db.QueryRow(store.db.Rebind(`SELECT COUNT(*) FROM coordinator_wakes WHERE coordinator_id = ? AND status = 'pending'`), f.c.ID).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 200 || inserted != 10 {
		t.Fatalf("pending = %d inserted = %d, want 200 and 10", pending, inserted)
	}
}

func TestRecordWake_ConcurrentAtCapLeavesExactly200(t *testing.T) {
	runConcurrentCap(t, newTestStore(t))
}

func TestRecordWake_Postgres_ConcurrentAtCapLeavesExactly200(t *testing.T) {
	runConcurrentCap(t, newMultiConnStorePostgres(t))
}

func TestRecordWake_ConcurrentSameEpisodeInsertsOnce(t *testing.T) {
	store := newTestStore(t)
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")
	var wg sync.WaitGroup
	var mu sync.Mutex
	inserted := 0
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := store.RecordWake(context.Background(), f.c.ID, "t-1", WakeKindCompleted, "completed")
			if err != nil {
				t.Error(err)
				return
			}
			if res.Outcome == WakeInserted {
				mu.Lock()
				inserted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if inserted != 1 || count(t, store, "coordinator_wakes") != 1 {
		t.Fatalf("inserted = %d rows = %d, want 1 and 1", inserted, count(t, store, "coordinator_wakes"))
	}
}

func TestRecordWake_AutonomyOffCommittedFirstStoresNothing(t *testing.T) {
	store := newTestStore(t)
	f := newWakeFixture(t, store)
	f.addOwn(t, "t-1", "wf-a")
	off := false
	if _, _, err := store.PatchCoordinator(context.Background(), "ws-1", f.c.ID, CoordinatorPatch{AutonomyEnabled: &off}, nil); err != nil {
		t.Fatal(err)
	}
	if res := mustRecord(t, store, f.c.ID, "t-1", WakeKindQuestion, "q"); res.Outcome != WakeAutonomyOff {
		t.Fatalf("outcome = %+v", res)
	}
	if got := wakeRows(t, store, f.c.ID); got != 0 {
		t.Fatalf("wakes = %d", got)
	}
}

func TestExistingWakeKeys(t *testing.T) {
	store := newTestStore(t)
	f := newWakeFixture(t, store)
	other := newTestCoordinator(t, store, "ws-1")
	ids := make([]string, 0, 900)
	for i := range 900 {
		ids = append(ids, fmt.Sprintf("t-%04d", i))
	}
	now := time.Now().UTC()
	insert := func(coordinatorID, taskID, kind, key, status string) {
		mustExec(t, store, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
			VALUES (?, ?, 'ws-1', ?, ?, ?, ?, ?, ?)`, uuid.NewString(), coordinatorID, taskID, kind, key, status, now, now)
	}
	insert(f.c.ID, "t-0000", "question", "q", "pending")
	insert(f.c.ID, "t-0450", "completed", "completed", "delivered")
	insert(f.c.ID, "t-0899", "stall", "s", "superseded")
	insert(f.c.ID, "t-other", "question", "q", "pending")
	insert(other.ID, "t-0001", "question", "q", "pending")

	got, err := store.ExistingWakeKeys(context.Background(), f.c.ID, ids)
	if err != nil {
		t.Fatal(err)
	}
	want := map[WakeKey]struct{}{
		{"t-0000", WakeKindQuestion, "q"}:          {},
		{"t-0450", WakeKindCompleted, "completed"}: {},
		{"t-0899", WakeKindStall, "s"}:             {},
	}
	if len(got) != len(want) {
		t.Fatalf("keys = %v", got)
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			t.Fatalf("missing %v in %v", k, got)
		}
	}
	empty, err := store.ExistingWakeKeys(context.Background(), f.c.ID, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty ids = %v, %v", empty, err)
	}
}

func TestExistingWakeKeys_ReadErrorIsAnError(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.ExistingWakeKeys(ctx, "c", []string{"t"}); err == nil {
		t.Fatal("cancelled read must be an error")
	}
}

func runPruneKeepsOwnTaskWakes(t *testing.T, store *Store) {
	ctx := context.Background()
	f := newWakeFixture(t, store)
	now := time.Now().UTC()
	old := now.Add(-31 * 24 * time.Hour)
	insertOld := func(id, taskID string) {
		mustExec(t, store, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
			VALUES (?, ?, 'ws-1', ?, 'completed', 'completed', 'delivered', ?, ?)`, id, f.c.ID, taskID, old, old)
	}
	f.addOwn(t, "t-live", "wf-a")
	f.addOwn(t, "t-archived", "wf-a")
	f.addOwn(t, "t-ephemeral", "wf-a")
	mustExec(t, store, `UPDATE tasks SET is_ephemeral = 1 WHERE id = 't-ephemeral'`)
	f.addTask(t, "t-noproposal", "wf-a")
	f.addOwn(t, "t-gone", "wf-a")
	mustExec(t, store, `DELETE FROM tasks WHERE id = 't-gone'`)
	insertOld("w-live", "t-live")
	insertOld("w-archived", "t-archived")
	insertOld("w-ephemeral", "t-ephemeral")
	insertOld("w-noproposal", "t-noproposal")
	insertOld("w-gone", "t-gone")
	mustExec(t, store, `UPDATE tasks SET archived_at = ? WHERE id = 't-archived'`, now)

	if _, wakes, err := store.PruneWakeState(ctx, now); err != nil || wakes != 3 {
		t.Fatalf("prune = %d, %v; want 3", wakes, err)
	}
	for _, id := range []string{"w-live", "w-archived"} {
		wakeStatus(t, store, id)
	}
	for _, id := range []string{"w-ephemeral", "w-noproposal", "w-gone"} {
		var n int
		_ = store.db.QueryRow(store.db.Rebind(`SELECT COUNT(*) FROM coordinator_wakes WHERE id = ?`), id).Scan(&n)
		if n != 0 {
			t.Fatalf("%s should have been pruned", id)
		}
	}
	// 31 days on, the archived task's wake is still kept.
	if _, wakes, err := store.PruneWakeState(ctx, now.Add(31*24*time.Hour)); err != nil || wakes != 0 {
		t.Fatalf("second prune = %d, %v; want 0", wakes, err)
	}
}

func TestPruneWakeState_KeepsWakesOfTasksThatCanBeOwnTasks(t *testing.T) {
	runPruneKeepsOwnTaskWakes(t, newTestStore(t))
}

func TestPruneWakeState_Postgres_KeepsWakesOfTasksThatCanBeOwnTasks(t *testing.T) {
	runPruneKeepsOwnTaskWakes(t, newTestStorePostgres(t))
}
