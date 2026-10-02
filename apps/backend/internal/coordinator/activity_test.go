package coordinator

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func newPhase2Service(t *testing.T, store *Store, on bool) *Service {
	t.Helper()
	return NewService(store, NewValidator(nil, nil), &fakeWorkspaceAuthorizer{}, newTestLogger(t), WithPhase2(on))
}

func validRow(coordinatorID string) ActivityRow {
	return ActivityRow{
		CoordinatorID: coordinatorID,
		WorkspaceID:   "ws-1",
		ActionClass:   ActionCreateTask,
		Outcome:       ActivityProposed,
		Authorization: AuthRequiresApproval,
		Detail:        "d",
	}
}

func listActivity(t *testing.T, store *Store, coordinatorID string) []ActivityRow {
	t.Helper()
	var rows []ActivityRow
	err := store.db.SelectContext(context.Background(), &rows, store.db.Rebind(`SELECT `+activityColumns+` FROM coordinator_activity WHERE coordinator_id = ? ORDER BY created_at, id`), coordinatorID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestInsertActivity_AssignsDefaults(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	if err := store.InsertActivity(ctx, store.db, validRow(c.ID)); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	if r.ID == "" || r.CreatedAt.IsZero() || !r.UpdatedAt.Equal(r.CreatedAt) || r.RefusalCount != 1 {
		t.Fatalf("defaults not assigned: %+v", r)
	}
}

func TestInsertActivity_Invalid(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	mutations := map[string]func(*ActivityRow){
		"class":         func(r *ActivityRow) { r.ActionClass = Action("bogus") },
		"outcome":       func(r *ActivityRow) { r.Outcome = ActivityOutcome("bogus") },
		"authorization": func(r *ActivityRow) { r.Authorization = ActivityAuthorization("bogus") },
		"coordinator":   func(r *ActivityRow) { r.CoordinatorID = "" },
		"workspace":     func(r *ActivityRow) { r.WorkspaceID = "" },
	}
	for name, mutate := range mutations {
		row := validRow(c.ID)
		mutate(&row)
		if err := store.InsertActivity(ctx, store.db, row); !errors.Is(err, ErrInvalidActivity) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if n := len(listActivity(t, store, c.ID)); n != 0 {
		t.Fatalf("invalid rows wrote %d rows", n)
	}
	row := validRow(c.ID)
	row.ActionClass = ActionUnknown
	if err := store.InsertActivity(ctx, store.db, row); err != nil {
		t.Fatalf("unknown class must be accepted: %v", err)
	}
}

func TestInsertActivity_TruncatesDetailToRunes(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	row := validRow(c.ID)
	row.Detail = strings.Repeat("é", 1500)
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
	got := listActivity(t, store, c.ID)[0].Detail
	if len([]rune(got)) != 1000 || strings.HasSuffix(got, "...") {
		t.Fatalf("detail has %d runes", len([]rune(got)))
	}
}

func TestInsertActivity_CountsByOutcome(t *testing.T) {
	store := newTestStore(t)
	c := newTestCoordinator(t, store, "ws-1")
	before := activityRowsCounter(ActivityRefused)
	row := validRow(c.ID)
	row.Outcome = ActivityRefused
	row.Authorization = AuthDenied
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
	if got := activityRowsCounter(ActivityRefused); got != before+1 {
		t.Fatalf("counter = %d, want %d", got, before+1)
	}
	bad := validRow(c.ID)
	bad.Outcome = "bogus"
	_ = store.InsertActivity(context.Background(), store.db, bad)
	if got := activityRowsCounter(ActivityRefused); got != before+1 {
		t.Fatal("failed insert must not count")
	}
}

func TestMarkUndone(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	row := validRow(c.ID)
	row.ID = "act-1"
	if err := store.InsertActivity(ctx, store.db, row); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	changed, err := store.MarkUndone(ctx, store.db, "act-1", "user-1", at)
	if err != nil || !changed {
		t.Fatalf("first undo = %v, %v", changed, err)
	}
	changed, err = store.MarkUndone(ctx, store.db, "act-1", "user-2", at.Add(time.Hour))
	if err != nil || changed {
		t.Fatalf("second undo = %v, %v", changed, err)
	}
	changed, err = store.MarkUndone(ctx, store.db, "missing", "user-1", at)
	if err != nil || changed {
		t.Fatalf("missing undo = %v, %v", changed, err)
	}
	got := listActivity(t, store, c.ID)[0]
	if got.UndoneBy == nil || *got.UndoneBy != "user-1" || got.UndoneAt == nil || !got.UndoneAt.Equal(at) {
		t.Fatalf("undone marker = %+v", got)
	}
}

func TestRecord_GatedOnPhase2(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	off := newPhase2Service(t, store, false)
	if err := off.Record(ctx, store.db, validRow(c.ID)); err != nil {
		t.Fatal(err)
	}
	if err := off.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, "denied"); err != nil {
		t.Fatal(err)
	}
	if n := len(listActivity(t, store, c.ID)); n != 0 {
		t.Fatalf("flag off wrote %d rows", n)
	}
	on := newPhase2Service(t, store, true)
	if err := on.Record(ctx, store.db, validRow(c.ID)); err != nil {
		t.Fatal(err)
	}
	if n := len(listActivity(t, store, c.ID)); n != 1 {
		t.Fatalf("flag on wrote %d rows", n)
	}
}

func TestRecordRefusal_CoalescesWithinWindow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	svc := newPhase2Service(t, store, true)

	record := func() {
		t.Helper()
		if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, "policy_denied"); err != nil {
			t.Fatal(err)
		}
	}
	record()
	now = now.Add(30 * time.Second)
	record()
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].RefusalCount != 2 || rows[0].Outcome != ActivityRefused || rows[0].Authorization != AuthDenied {
		t.Fatalf("rows = %+v", rows)
	}
	if !rows[0].UpdatedAt.Equal(now) {
		t.Fatalf("updated_at = %v, want %v", rows[0].UpdatedAt, now)
	}
	// Past the window a new row starts.
	now = now.Add(2 * time.Minute)
	record()
	if rows = listActivity(t, store, c.ID); len(rows) != 2 {
		t.Fatalf("rows after window = %d", len(rows))
	}
	// A different reason or class never coalesces.
	if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, "other"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMessage, "policy_denied"); err != nil {
		t.Fatal(err)
	}
	if rows = listActivity(t, store, c.ID); len(rows) != 4 {
		t.Fatalf("rows = %d", len(rows))
	}
}

func TestRecordRefusal_WindowBoundaryIsInclusive(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	svc := newPhase2Service(t, store, true)
	_ = svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, "r")
	now = now.Add(60 * time.Second)
	_ = svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, "r")
	if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].RefusalCount != 2 {
		t.Fatalf("rows at exactly 60s = %+v", rows)
	}
}

func TestRecordRefusal_ConcurrentMakesOneRow(t *testing.T) {
	assertConcurrentRefusalsCoalesce(t, newTestStore(t))
}

func assertConcurrentRefusalsCoalesce(t *testing.T, store *Store) {
	t.Helper()
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	svc := newPhase2Service(t, store, true)
	const n = 10
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionResume, "r"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].RefusalCount != n {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRecordRefusal_InvalidAndMissing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	svc := newPhase2Service(t, store, true)
	if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionMove, ""); !errors.Is(err, ErrInvalidActivity) {
		t.Fatalf("empty reason err = %v", err)
	}
	if err := svc.RecordRefusal(ctx, c.ID, "ws-1", Action("bogus"), "r"); !errors.Is(err, ErrInvalidActivity) {
		t.Fatalf("bad class err = %v", err)
	}
	if err := svc.RecordRefusal(ctx, c.ID, "ws-1", ActionUnknown, "r"); err != nil {
		t.Fatalf("unknown class: %v", err)
	}
	if err := svc.RecordRefusal(ctx, "gone", "ws-1", ActionMove, "r"); err != nil {
		t.Fatalf("deleted coordinator must not error: %v", err)
	}
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM coordinator_activity WHERE coordinator_id = 'gone'`); err != nil || n != 0 {
		t.Fatalf("rows for missing coordinator = %d, %v", n, err)
	}
}

func TestRecord_AgainstDeleteLeavesNoRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	svc := newPhase2Service(t, store, true)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_ = store.withCoordinatorLock(ctx, c.ID, func(tx coordinatorExec) error {
			return svc.Record(ctx, tx, validRow(c.ID))
		})
	}()
	go func() {
		defer wg.Done()
		_ = store.DeleteCoordinator(ctx, "ws-1", c.ID)
	}()
	wg.Wait()
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM coordinator_activity WHERE coordinator_id = ?`, c.ID); err != nil || n != 0 {
		t.Fatalf("surviving rows = %d, %v", n, err)
	}
}

// Activity rows are append-only apart from the two documented updates.
func TestActivitySourceScan_WriteStatements(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	update := regexp.MustCompile("(?s)UPDATE coordinator_activity(.*?)WHERE")
	matches := 0
	defer func() {
		if matches < 2 {
			t.Errorf("source scan matched %d UPDATE coordinator_activity statements, want at least 2", matches)
		}
	}()
	allowedSets := []string{"undone_at = ?, undone_by = ?, updated_at = ?", "refusal_count = refusal_count + 1, updated_at = ?"}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		for _, m := range update.FindAllStringSubmatch(src, -1) {
			matches++
			set := strings.Join(strings.Fields(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m[1]), "SET"))), " ")
			ok := false
			for _, a := range allowedSets {
				ok = ok || set == a
			}
			if !ok {
				t.Errorf("%s: UPDATE coordinator_activity SET %q is not an allowed write", name, set)
			}
		}
		if strings.Contains(src, "DELETE FROM coordinator_activity") &&
			name != "store.go" && name != "store_workspace_delete.go" && name != "retention.go" {
			t.Errorf("%s deletes activity rows outside retention and the deletion transactions", name)
		}
	}
}

func TestInsertActivity_Phase3Values(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	cases := map[string]func(*ActivityRow){
		"returned":    func(r *ActivityRow) { r.Outcome = ActivityReturned },
		"automatic":   func(r *ActivityRow) { r.Authorization = AuthAutomatic },
		"improvement": func(r *ActivityRow) { r.ActionClass = ActionImprovement },
	}
	for name, mutate := range cases {
		row := validRow(c.ID)
		mutate(&row)
		if err := store.InsertActivity(ctx, store.db, row); err != nil {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestActionImprovementIsActivityOnly(t *testing.T) {
	for _, a := range AllActions {
		if a == ActionImprovement {
			t.Fatal("improvement must not be a policy action")
		}
	}
	if isPolicyAction(ActionImprovement) {
		t.Fatal("improvement must not be a policy action")
	}
}
