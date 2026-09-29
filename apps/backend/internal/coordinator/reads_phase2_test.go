package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestLoadWatchSet(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	set, err := store.LoadWatchSet(ctx, store.db, c.ID)
	if err != nil || !set.All || set.WorkflowIDs == nil {
		t.Fatalf("default set = %+v, %v", set, err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	if set, err = store.LoadWatchSet(ctx, store.db, c.ID); err != nil {
		t.Fatal(err)
	}
	if set.All || set.Contains("wf") || set.WorkflowIDs == nil {
		t.Fatalf("default set = %+v", set)
	}
	now := time.Now().UTC()
	for _, wf := range []string{"wf-b", "wf-a"} {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, ?, 'ws-1', ?)`, c.ID, wf, now); err != nil {
			t.Fatal(err)
		}
	}
	set, _ = store.LoadWatchSet(ctx, store.db, c.ID)
	if !reflect.DeepEqual(set.WorkflowIDs, []string{"wf-a", "wf-b"}) || !set.Contains("wf-a") || set.Contains("") {
		t.Fatalf("selected set = %+v", set)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET watch_scope = 'all' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	set, _ = store.LoadWatchSet(ctx, store.db, c.ID)
	if !set.All || !set.Contains("anything") || set.Contains("") {
		t.Fatalf("all set = %+v", set)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET watch_scope = 'weird' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	if set, _ = store.LoadWatchSet(ctx, store.db, c.ID); set.All {
		t.Fatal("unknown scope must read as selected")
	}
	if _, err := store.LoadWatchSet(ctx, store.db, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing coordinator err = %v", err)
	}
}

func TestActiveStandingOrders(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	orders, err := store.ActiveStandingOrders(ctx, store.db, c.ID)
	if err != nil || orders == nil || len(orders) != 0 {
		t.Fatalf("empty = %v, %v", orders, err)
	}
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ins := func(id string, at time.Time, retired bool) {
		var ra any
		if retired {
			ra = at
		}
		if _, err := store.db.ExecContext(ctx, `INSERT INTO coordinator_standing_orders (id, coordinator_id, workspace_id, text, created_by, created_at, retired_at) VALUES (?, ?, 'ws-1', 't', 'u', ?, ?)`, id, c.ID, at, ra); err != nil {
			t.Fatal(err)
		}
	}
	ins("b", t0, false)
	ins("a", t0, false)
	ins("c", t0.Add(time.Hour), false)
	ins("r", t0, true)
	orders, err = store.ActiveStandingOrders(ctx, store.db, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, o := range orders {
		ids = append(ids, o.ID)
	}
	if !reflect.DeepEqual(ids, []string{"a", "b", "c"}) {
		t.Fatalf("order = %v", ids)
	}
}

func TestGoalReads(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	if g, err := store.ActiveGoal(ctx, store.db, c.ID); err != nil || g != nil {
		t.Fatalf("no goal = %v, %v", g, err)
	}
	if g, err := store.LastMetGoal(ctx, store.db, c.ID); err != nil || g != nil {
		t.Fatalf("no met goal = %v, %v", g, err)
	}
	t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ins := func(id, status string, metAt *time.Time) {
		if _, err := store.db.ExecContext(ctx, `INSERT INTO coordinator_goals (id, coordinator_id, workspace_id, name, status, criteria_json, baseline_json, set_at, met_at, created_at, updated_at) VALUES (?, ?, 'ws-1', 'n', ?, '[{"id":"c1","text":"x","done":true}]', '{}', ?, ?, ?, ?)`, id, c.ID, status, t0, metAt, t0, t0); err != nil {
			t.Fatal(err)
		}
	}
	m1, m2 := t0.Add(time.Hour), t0.Add(2*time.Hour)
	ins("m1", "met", &m1)
	ins("m2", "met", &m2)
	ins("act", "active", nil)
	g, err := store.ActiveGoal(ctx, store.db, c.ID)
	if err != nil || g == nil || g.ID != "act" || len(g.Criteria) != 1 || !g.Criteria[0].Done {
		t.Fatalf("active = %+v, %v", g, err)
	}
	g, err = store.LastMetGoal(ctx, store.db, c.ID)
	if err != nil || g == nil || g.ID != "m2" {
		t.Fatalf("last met = %+v, %v", g, err)
	}
	b, _ := json.Marshal(g)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	for _, k := range []string{"id", "coordinator_id", "name", "due_on", "status", "criteria", "baseline", "set_at", "met_at", "met_by", "created_at", "updated_at"} {
		if _, ok := m[k]; !ok {
			t.Errorf("goal JSON missing %q", k)
		}
	}
}

func TestPolicyView(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	off := newPhase2Service(t, store, false)
	v, err := off.Policy(ctx, c.ID)
	if err != nil || v.PolicyRevision != 0 || v.WatchScope != "all" || v.WorkflowIDs == nil || len(v.Actions) != 6 || v.Actions[ActionCreateTask] != SettingRequiresApproval {
		t.Fatalf("phase1 view = %+v, %v", v, err)
	}
	if v.Actions[ActionMove] != SettingDenied {
		t.Fatalf("phase1 move = %v", v.Actions[ActionMove])
	}
	on := newPhase2Service(t, store, true)
	if _, err := on.Policy(ctx, "gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE coordinators SET policy_json = 'not json', policy_revision = 4 WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	v, err = on.Policy(ctx, c.ID)
	if err != nil || v.PolicyRevision != 4 {
		t.Fatalf("unreadable view = %+v, %v", v, err)
	}
	for _, a := range AllActions {
		if v.Actions[a] != SettingDenied {
			t.Fatalf("unreadable policy %s = %v, want denied", a, v.Actions[a])
		}
	}
	// A second read of the same revision must not error either.
	if _, err := on.Policy(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
}
