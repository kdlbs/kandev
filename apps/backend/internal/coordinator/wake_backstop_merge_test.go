package coordinator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

type fakeChangeLinks struct{ byChange map[string][]string }

func (f fakeChangeLinks) TasksLinkedToChange(_ context.Context, _, changeKey string) ([]string, error) {
	return f.byChange[changeKey], nil
}

func (e *wakeEnv) openEpisode(t *testing.T, changeKey string, p mergestate.Pipeline) {
	t.Helper()
	in := ChangeObservationInput{WorkspaceID: "ws-1", ChangeKey: changeKey}
	in.Obs = mergeObs(1, "h1", mergestate.PipelineGreen, mergestate.ConflictClear, 0)
	if _, err := e.store.ApplyChangeObservation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	in.Obs = mergeObs(2, "h1", p, mergestate.ConflictClear, 0)
	if _, err := e.store.ApplyChangeObservation(context.Background(), in); err != nil {
		t.Fatal(err)
	}
}

func TestBackstopDerivesMissingMergeWakes(t *testing.T) {
	e := newWakeEnv(t)
	e.fix.addTask(t, "tm", "wf-a")
	e.svc.merge = true
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm"}}})
	e.openEpisode(t, testChangeKey, mergestate.PipelineRed)

	e.pass(t)
	got := pendingWakeKinds(t, e.store, e.fix.c.ID)
	if len(got) != 1 || got[0] != "change_red" {
		t.Fatalf("derived wakes = %v", got)
	}
	e.pass(t)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 1 {
		t.Fatalf("second pass duplicated: %v", got)
	}
}

func TestBackstopSkipsBaselineAndUnlinkedAndAutonomyOff(t *testing.T) {
	e := newWakeEnv(t)
	e.fix.addTask(t, "tm", "wf-a")
	e.svc.merge = true
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm"}}})
	baseline := ChangeObservationInput{WorkspaceID: "ws-1", ChangeKey: "github|github.com|r1|7"}
	baseline.Obs = mergeObs(1, "h1", mergestate.PipelineRed, mergestate.ConflictClear, 0)
	if _, err := e.store.ApplyChangeObservation(context.Background(), baseline); err != nil {
		t.Fatal(err)
	}
	e.openEpisode(t, "github|github.com|r1|8", mergestate.PipelineRed) // no task linked
	e.pass(t)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 0 {
		t.Fatalf("wakes for baseline or unlinked change: %v", got)
	}
	e.openEpisode(t, testChangeKey, mergestate.PipelineRed)
	setAutonomy(t, e.store, e.fix.c.ID, false)
	e.pass(t)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 0 {
		t.Fatalf("autonomy-off coordinator got wakes: %v", got)
	}
}

func TestBackstopMergeWakeRespectsWatchSet(t *testing.T) {
	e := newWakeEnv(t)
	e.fix.addTask(t, "tm", "wf-elsewhere")
	mustExec(t, e.store, `UPDATE coordinators SET watch_scope = 'selected' WHERE id = ?`, e.fix.c.ID)
	mustExec(t, e.store, `INSERT INTO coordinator_watches (coordinator_id, workflow_id, workspace_id, created_at) VALUES (?, 'wf-a', 'ws-1', ?)`, e.fix.c.ID, mergeObs(0, "", "", 0, 0).StartedAt)
	e.svc.merge = true
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm"}}})
	e.openEpisode(t, testChangeKey, mergestate.PipelineRed)
	e.pass(t)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 0 {
		t.Fatalf("wake for a card outside the watch set: %v", got)
	}
}

func TestBackstopMergeWakesOffWithFlagOff(t *testing.T) {
	e := newWakeEnv(t)
	e.fix.addTask(t, "tm", "wf-a")
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm"}}})
	e.openEpisode(t, testChangeKey, mergestate.PipelineRed)
	e.pass(t)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 0 {
		t.Fatalf("merge wake derived with the flag off: %v", got)
	}
}
