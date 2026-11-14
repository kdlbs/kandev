package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

const testChangeKey = "github|github.com|r1|42"

func mergeObs(sec int, head string, p mergestate.Pipeline, c mergestate.ConflictState, threads int) mergestate.Observation {
	base := time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)
	return mergestate.Observation{StartedAt: base.Add(time.Duration(sec) * time.Second), HeadSHA: head, Pipeline: p, Conflict: c, UnresolvedThreads: threads}
}

func pendingWakeKinds(t *testing.T, store *Store, coordinatorID string) []string {
	t.Helper()
	var kinds []string
	err := store.db.Select(&kinds, store.db.Rebind(`SELECT kind FROM coordinator_wakes WHERE coordinator_id = ? ORDER BY kind, task_id`), coordinatorID)
	if err != nil {
		t.Fatal(err)
	}
	return kinds
}

func runChangeObservation(t *testing.T, store *Store) {
	ctx := context.Background()
	a := newTestCoordinator(t, store, "ws-1")
	b := newTestCoordinator(t, store, "ws-1")
	off := newTestCoordinator(t, store, "ws-1")
	setAutonomy(t, store, a.ID, true)
	setAutonomy(t, store, b.ID, true)
	targets := []WakeTarget{{a.ID, "t-1"}, {b.ID, "t-2"}, {off.ID, "t-1"}}
	apply := func(o mergestate.Observation) ChangeObservationResult {
		t.Helper()
		res, err := store.ApplyChangeObservation(ctx, ChangeObservationInput{WorkspaceID: "ws-1", ChangeKey: testChangeKey, Obs: o, Targets: targets})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	apply(mergeObs(1, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0))
	if got := pendingWakeKinds(t, store, a.ID); len(got) != 0 {
		t.Fatalf("baseline woke: %v", got)
	}
	apply(mergeObs(2, "h1", mergestate.PipelineGreen, mergestate.ConflictClear, 0))
	res := apply(mergeObs(3, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0))
	if got := pendingWakeKinds(t, store, a.ID); len(got) != 1 || got[0] != "change_red" {
		t.Fatalf("coordinator a wakes: %v", got)
	}
	if got := pendingWakeKinds(t, store, b.ID); len(got) != 1 {
		t.Fatalf("coordinator b wakes: %v", got)
	}
	if got := pendingWakeKinds(t, store, off.ID); len(got) != 0 {
		t.Fatalf("autonomy-off coordinator woke: %v", got)
	}
	if !res.Transitions[mergestate.KindRed].Wake {
		t.Fatalf("transition: %+v", res.Transitions)
	}

	apply(mergeObs(4, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0))
	apply(mergeObs(2, "h1", mergestate.PipelineGreen, mergestate.ConflictClear, 0)) // started before the stored read
	if got := pendingWakeKinds(t, store, a.ID); len(got) != 1 {
		t.Fatalf("repeat or stale read woke again: %v", got)
	}
}

func TestApplyChangeObservation(t *testing.T) { runChangeObservation(t, newTestStore(t)) }

func TestApplyChangeObservation_Postgres(t *testing.T) {
	runChangeObservation(t, newTestStorePostgres(t))
}

func TestApplyChangeObservationCappedStillStoresRow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	c := newTestCoordinator(t, store, "ws-1")
	setAutonomy(t, store, c.ID, true)
	now := time.Now().UTC()
	for i := 0; i < pendingWakeCap; i++ {
		mustExec(t, store, `INSERT INTO coordinator_wakes (id, coordinator_id, workspace_id, task_id, kind, episode_key, status, created_at, updated_at)
			VALUES (?, ?, 'ws-1', ?, 'question', ?, 'pending', ?, ?)`, uuid.NewString(), c.ID, "t-x", uuid.NewString(), now, now)
	}
	in := ChangeObservationInput{WorkspaceID: "ws-1", ChangeKey: testChangeKey, Targets: []WakeTarget{{c.ID, "t-1"}}}
	in.Obs = mergeObs(1, "h1", mergestate.PipelineGreen, mergestate.ConflictClear, 0)
	if _, err := store.ApplyChangeObservation(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Obs = mergeObs(2, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0)
	res, err := store.ApplyChangeObservation(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Wakes) != 1 || res.Wakes[0].Outcome != WakeCapped {
		t.Fatalf("outcomes: %+v", res.Wakes)
	}
	open, err := store.OpenChangeEpisodes(ctx, "ws-1", testChangeKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].Kind != mergestate.KindRed || open[0].Generation != 2 {
		t.Fatalf("open episodes: %+v", open)
	}
}

func TestOpenChangeEpisodesSkipsBaselineAndClear(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	in := ChangeObservationInput{WorkspaceID: "ws-1", ChangeKey: testChangeKey}
	in.Obs = mergeObs(1, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0)
	if _, err := store.ApplyChangeObservation(ctx, in); err != nil {
		t.Fatal(err)
	}
	open, err := store.OpenChangeEpisodes(ctx, "ws-1", testChangeKey)
	if err != nil || len(open) != 0 {
		t.Fatalf("baseline listed as open: %+v %v", open, err)
	}
}
