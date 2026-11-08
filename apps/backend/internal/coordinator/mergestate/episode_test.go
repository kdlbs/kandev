package mergestate

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 8, 10, 0, 0, 0, time.UTC)

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

func obs(sec int, head string, p Pipeline, c ConflictState, threads int) Observation {
	return Observation{StartedAt: at(sec), HeadSHA: head, Pipeline: p, Conflict: c, UnresolvedThreads: threads}
}

func TestBaselineNeverWakes(t *testing.T) {
	for _, k := range []Kind{KindRed, KindConflict, KindThreads} {
		r := Transition(nil, k, obs(1, "a", PipelineRed, ConflictPresent, 3))
		if !r.Write || r.Wake || !r.Row.Baseline || !r.Row.Active || r.Row.Generation != 1 {
			t.Fatalf("%s: %+v", k, r)
		}
	}
}

func TestFailedFirstReadIsNoBaseline(t *testing.T) {
	r := Transition(nil, KindRed, obs(1, "a", PipelineUnavailable, ConflictUnknown, -1))
	if r.Write || r.Wake {
		t.Fatalf("unavailable first read wrote a row: %+v", r)
	}
	r = Transition(nil, KindThreads, obs(1, "a", PipelineGreen, ConflictClear, -1))
	if r.Write {
		t.Fatalf("unknown thread count wrote a row: %+v", r)
	}
}

func TestWakeEpisodeRecurrence(t *testing.T) {
	row := Transition(nil, KindRed, obs(1, "a", PipelineGreen, ConflictClear, 0)).Row
	r := Transition(&row, KindRed, obs(2, "a", PipelineRed, ConflictClear, 0))
	if !r.Wake || r.Row.Generation != 2 || !r.Row.Active || r.Row.Baseline {
		t.Fatalf("green to red: %+v", r)
	}
	again := Transition(&r.Row, KindRed, obs(3, "a", PipelineRed, ConflictClear, 0))
	if again.Wake || again.Write {
		t.Fatalf("repeat of one episode: %+v", again)
	}
	cleared := Transition(&r.Row, KindRed, obs(4, "a", PipelineGreen, ConflictClear, 0))
	if cleared.Wake || !cleared.Write || cleared.Row.Active {
		t.Fatalf("clear: %+v", cleared)
	}
	recur := Transition(&cleared.Row, KindRed, obs(5, "a", PipelineRed, ConflictClear, 0))
	if !recur.Wake || recur.Row.Generation != 3 {
		t.Fatalf("recurrence: %+v", recur)
	}
}

func TestPendingNoneUnavailableNeverClear(t *testing.T) {
	row := Row{Active: true, Generation: 2, HeadSHA: "a", ObservedAt: at(1)}
	for _, p := range []Pipeline{PipelinePending, PipelineNone, PipelineUnavailable} {
		r := Transition(&row, KindRed, obs(2, "a", p, ConflictUnknown, 0))
		if r.Write || r.Wake {
			t.Fatalf("%s changed the row: %+v", p, r)
		}
	}
}

func TestHeadChangeClearsRed(t *testing.T) {
	row := Row{Active: true, Generation: 2, HeadSHA: "a", ObservedAt: at(1)}
	r := Transition(&row, KindRed, obs(2, "b", PipelinePending, ConflictUnknown, 0))
	if r.Row.Active || !r.Write || r.Wake || r.Row.HeadSHA != "b" {
		t.Fatalf("head change: %+v", r)
	}
}

func TestHeadChangeOntoRedOpensNewEpisode(t *testing.T) {
	row := Row{Active: true, Generation: 2, HeadSHA: "a", ObservedAt: at(1)}
	r := Transition(&row, KindRed, obs(2, "b", PipelineRed, ConflictUnknown, 0))
	if !r.Wake || r.Row.Generation != 3 || r.Row.HeadSHA != "b" {
		t.Fatalf("head onto red: %+v", r)
	}
	rc := Transition(&row, KindConflict, obs(2, "b", PipelineGreen, ConflictPresent, 0))
	if !rc.Wake || rc.Row.Generation != 3 {
		t.Fatalf("head onto conflict: %+v", rc)
	}
}

func TestStaleObservationDiscarded(t *testing.T) {
	row := Row{Active: false, Generation: 2, HeadSHA: "a", ObservedAt: at(10)}
	for _, sec := range []int{5, 10} {
		r := Transition(&row, KindRed, obs(sec, "a", PipelineRed, ConflictUnknown, 0))
		if r.Write || r.Wake {
			t.Fatalf("started at %d: %+v", sec, r)
		}
	}
}

func TestThreadsWakeOnZeroToPositive(t *testing.T) {
	row := Transition(nil, KindThreads, obs(1, "a", PipelineGreen, ConflictClear, 0)).Row
	r := Transition(&row, KindThreads, obs(2, "a", PipelineGreen, ConflictClear, 1))
	if !r.Wake || r.Row.Generation != 2 || !r.Row.Active {
		t.Fatalf("zero to positive: %+v", r)
	}
}

func TestExistingThreadsNoWake(t *testing.T) {
	base := Transition(nil, KindThreads, obs(1, "a", PipelineGreen, ConflictClear, 2)).Row
	r := Transition(&base, KindThreads, obs(2, "a", PipelineGreen, ConflictClear, 5))
	if r.Wake || r.Write {
		t.Fatalf("more threads while some open: %+v", r)
	}
	cleared := Transition(&base, KindThreads, obs(3, "a", PipelineGreen, ConflictClear, 0))
	if cleared.Wake || cleared.Row.Active {
		t.Fatalf("threads cleared: %+v", cleared)
	}
}

func TestThreadsIgnoreHeadChange(t *testing.T) {
	row := Row{Active: true, Generation: 1, HeadSHA: "a", ObservedAt: at(1)}
	r := Transition(&row, KindThreads, obs(2, "b", PipelineGreen, ConflictClear, 2))
	if r.Wake || r.Row.Generation != 1 || !r.Row.Active {
		t.Fatalf("threads head change: %+v", r)
	}
}

func TestEpisodeKey(t *testing.T) {
	if got := EpisodeKey("github|h|1|7", 3); got != "github|h|1|7#3" {
		t.Fatalf("got %q", got)
	}
}
