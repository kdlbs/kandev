package coordinator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/coordinator/mergestate"
)

func newObserveEnv(t *testing.T) (*wakeEnv, *fakeChangeReader) {
	t.Helper()
	e := newWakeEnv(t)
	e.fix.addTask(t, "tm", "wf-a")
	e.svc.merge = true
	e.svc.SetChangeLinks(fakeChangeLinks{byChange: map[string][]string{testChangeKey: {"tm"}}})
	r := &fakeChangeReader{}
	obs := NewChangeObserver(r)
	e.svc.SetChangeObserver(obs)
	return e, r
}

var testKey = mergestate.ChangeKey{Provider: "github", Host: "github.com", RepoID: "r1", Number: 42}

func TestObserveChangeRecordsWakeForWatchingCoordinators(t *testing.T) {
	e, r := newObserveEnv(t)
	r.snap = mergestate.Snapshot{Key: testKey, HeadSHA: "h1", Pipeline: mergestate.PipelineGreen, Conflict: mergestate.ConflictClear, UnresolvedThreads: 0}
	if _, err := e.svc.ObserveChange(context.Background(), "ws-1", testKey, true); err != nil {
		t.Fatal(err)
	}
	r.snap.Pipeline = mergestate.PipelineRed
	if _, err := e.svc.ObserveChange(context.Background(), "ws-1", testKey, true); err != nil {
		t.Fatal(err)
	}
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 1 || got[0] != "change_red" {
		t.Fatalf("wakes = %v", got)
	}
}

func TestObserveChangeFailedReadChangesNothing(t *testing.T) {
	e, r := newObserveEnv(t)
	r.snap = mergestate.Snapshot{Key: testKey, HeadSHA: "h1", Pipeline: mergestate.PipelineGreen, Conflict: mergestate.ConflictClear}
	_, _ = e.svc.ObserveChange(context.Background(), "ws-1", testKey, true)
	r.err = context.DeadlineExceeded
	if _, err := e.svc.ObserveChange(context.Background(), "ws-1", testKey, true); err == nil {
		t.Fatal("want the read error")
	}
	open, _ := e.store.ListOpenEpisodes(context.Background(), "ws-1")
	if len(open) != 0 {
		t.Fatalf("a failed read opened an episode: %+v", open)
	}
}

func TestObserveChangeFlagOffReadsNothing(t *testing.T) {
	e, r := newObserveEnv(t)
	e.svc.merge = false
	if _, err := e.svc.ObserveChange(context.Background(), "ws-1", testKey, true); err != nil {
		t.Fatal(err)
	}
	if r.calls.Load() != 0 {
		t.Fatal("provider read with the flag off")
	}
}

func TestObserveChangeSkipsAutonomyOffCoordinator(t *testing.T) {
	e, r := newObserveEnv(t)
	setAutonomy(t, e.store, e.fix.c.ID, false)
	r.snap = mergestate.Snapshot{Key: testKey, HeadSHA: "h1", Pipeline: mergestate.PipelineGreen, Conflict: mergestate.ConflictClear}
	_, _ = e.svc.ObserveChange(context.Background(), "ws-1", testKey, true)
	r.snap.Pipeline = mergestate.PipelineRed
	_, _ = e.svc.ObserveChange(context.Background(), "ws-1", testKey, true)
	if got := pendingWakeKinds(t, e.store, e.fix.c.ID); len(got) != 0 {
		t.Fatalf("autonomy-off coordinator woke: %v", got)
	}
}
