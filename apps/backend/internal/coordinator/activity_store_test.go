package coordinator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func seedActivity(t *testing.T, store *Store, coordinatorID, id string, class Action, outcome ActivityOutcome, at time.Time) {
	t.Helper()
	row := validRow(coordinatorID)
	row.ID = id
	row.ActionClass = class
	row.Outcome = outcome
	row.CreatedAt = at
	row.UpdatedAt = at
	if err := store.InsertActivity(context.Background(), store.db, row); err != nil {
		t.Fatal(err)
	}
}

func TestListActivityRows_OrderCursorAndClass(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	other := newTestCoordinator(t, store, "ws-1")
	base := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	seedActivity(t, store, c.ID, "a", ActionCreateTask, ActivityProposed, base)
	seedActivity(t, store, c.ID, "b", ActionMove, ActivityProposed, base.Add(time.Second))
	seedActivity(t, store, c.ID, "c", ActionCreateTask, ActivityApproved, base.Add(time.Second))
	seedActivity(t, store, c.ID, "d", ActionUnknown, ActivityRefused, base.Add(2*time.Second+500*time.Millisecond))
	seedActivity(t, store, other.ID, "x", ActionCreateTask, ActivityProposed, base.Add(time.Hour))

	rows, err := store.ListActivityRows(ctx, c.ID, "", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := func(rs []ActivityRow) string {
		out := ""
		for _, r := range rs {
			out += r.ID
		}
		return out
	}
	if got := ids(rows); got != "dcba" {
		t.Fatalf("order = %q, want dcba (created_at desc, id desc)", got)
	}
	page, err := store.ListActivityRows(ctx, c.ID, "", &ActivityCursor{CreatedAt: rows[1].CreatedAt, ID: rows[1].ID}, 10)
	if err != nil || ids(page) != "ba" {
		t.Fatalf("after c = %q, %v; want ba (same-instant tiebreak by id)", ids(page), err)
	}
	limited, err := store.ListActivityRows(ctx, c.ID, "", nil, 2)
	if err != nil || ids(limited) != "dc" {
		t.Fatalf("limit 2 = %q, %v", ids(limited), err)
	}
	filtered, err := store.ListActivityRows(ctx, c.ID, ActionCreateTask, nil, 10)
	if err != nil || ids(filtered) != "ca" {
		t.Fatalf("class filter = %q, %v", ids(filtered), err)
	}
	unknown, err := store.ListActivityRows(ctx, c.ID, ActionUnknown, nil, 10)
	if err != nil || ids(unknown) != "d" {
		t.Fatalf("unknown filter = %q, %v", ids(unknown), err)
	}
}

func TestGetActivityRow_ScopedToCoordinator(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	other := newTestCoordinator(t, store, "ws-1")
	seedActivity(t, store, c.ID, "a", ActionCreateTask, ActivityApproved, time.Now().UTC())
	if got, err := store.GetActivityRow(ctx, store.db, c.ID, "a"); err != nil || got.ID != "a" {
		t.Fatalf("own row = %+v, %v", got, err)
	}
	if _, err := store.GetActivityRow(ctx, store.db, other.ID, "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign row err = %v, want ErrNotFound", err)
	}
	if _, err := store.GetActivityRow(ctx, store.db, c.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing row err = %v, want ErrNotFound", err)
	}
}

func TestActivityCounts_GroupsAndWindow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Now().UTC()
	seedActivity(t, store, c.ID, "old", ActionCreateTask, ActivityApproved, now.Add(-48*time.Hour))
	seedActivity(t, store, c.ID, "p1", ActionCreateTask, ActivityProposed, now.Add(-time.Hour))
	seedActivity(t, store, c.ID, "a1", ActionCreateTask, ActivityApproved, now.Add(-time.Hour))
	edited := validRow(c.ID)
	edited.ID, edited.Outcome, edited.Edited = "a2", ActivityApproved, true
	if err := store.InsertActivity(ctx, store.db, edited); err != nil {
		t.Fatal(err)
	}
	refused := validRow(c.ID)
	refused.ID, refused.Outcome, refused.ActionClass, refused.RefusalCount = "r1", ActivityRefused, ActionMove, 3
	if err := store.InsertActivity(ctx, store.db, refused); err != nil {
		t.Fatal(err)
	}
	got, err := store.ActivityCounts(ctx, c.ID, now.Add(-24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	tally := map[string]int64{}
	for _, g := range got {
		tally[fmt.Sprintf("%s/%s/%v", g.Class, g.Outcome, g.Edited)] += g.Rows
		if g.Outcome == ActivityRefused {
			tally["refusals"] += g.Refusal
		}
	}
	want := map[string]int64{"create_task/proposed/false": 1, "create_task/approved/false": 1, "create_task/approved/true": 1, "move/refused/false": 1, "refusals": 3}
	for k, v := range want {
		if tally[k] != v {
			t.Errorf("%s = %d, want %d (all: %v)", k, tally[k], v, tally)
		}
	}
	if len(tally) != len(want) {
		t.Errorf("unexpected buckets: %v", tally)
	}
	first, err := store.EarliestActivityAt(ctx, c.ID)
	if err != nil || first == nil || !first.Equal(now.Add(-48*time.Hour)) {
		t.Fatalf("earliest = %v, %v", first, err)
	}
	empty := newTestCoordinator(t, store, "ws-1")
	if none, err := store.EarliestActivityAt(ctx, empty.ID); err != nil || none != nil {
		t.Fatalf("earliest of empty = %v, %v", none, err)
	}
}

func TestDeleteActivityBatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		seedActivity(t, store, c.ID, fmt.Sprintf("old-%d", i), ActionCreateTask, ActivityProposed, now.Add(-401*24*time.Hour+time.Duration(i)*time.Second))
	}
	seedActivity(t, store, c.ID, "keep", ActionCreateTask, ActivityProposed, now.Add(-399*24*time.Hour))
	cutoff := now.Add(-400 * 24 * time.Hour)
	n, err := store.DeleteActivityBatch(ctx, cutoff, 3)
	if err != nil || n != 3 {
		t.Fatalf("first batch = %d, %v", n, err)
	}
	n, err = store.DeleteActivityBatch(ctx, cutoff, 3)
	if err != nil || n != 2 {
		t.Fatalf("second batch = %d, %v", n, err)
	}
	if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].ID != "keep" {
		t.Fatalf("survivors = %+v", rows)
	}
}

func TestMarkUndone_EmptyUndoerStoresNull(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	seedActivity(t, store, c.ID, "a", ActionCreateTask, ActivityApproved, time.Now().UTC())
	if changed, err := store.MarkUndone(ctx, store.db, "a", "", time.Now()); err != nil || !changed {
		t.Fatalf("mark = %v, %v", changed, err)
	}
	if got := listActivity(t, store, c.ID)[0]; got.UndoneBy != nil || got.UndoneAt == nil {
		t.Fatalf("undone_by = %v, undone_at = %v; want NULL undoer with the time set", got.UndoneBy, got.UndoneAt)
	}
}
