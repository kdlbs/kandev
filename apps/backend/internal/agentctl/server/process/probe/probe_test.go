package probe

import (
	"errors"
	"testing"
	"time"
)

type fakeProcessTableReader struct {
	resolution time.Duration
	table      []processInfo
	err        error
}

func (f fakeProcessTableReader) Resolution() time.Duration { return f.resolution }

func (f fakeProcessTableReader) ReadProcessTable() ([]processInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.table, nil
}

const testAgentPID = 100

// AC-70: every descendant started strictly before the recorded turn start.
func TestProbeWithReader_AllDescendantsPreTurn_Settled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := fakeProcessTableReader{
		resolution: time.Millisecond,
		table: []processInfo{
			{PID: 200, PPID: testAgentPID, StartTime: turnStart.Add(-time.Hour)},
		},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-70a: one descendant started at-or-after the (truncated) turn start.
func TestProbeWithReader_DescendantAtOrAfterTurnStart_Live(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := fakeProcessTableReader{
		resolution: time.Millisecond,
		table: []processInfo{
			{PID: 200, PPID: testAgentPID, StartTime: turnStart},
		},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-71: a descendant reached only through a grandchild ppid edge (as a
// detached background shell in its own process group would be) is still
// found — the probe walks the ppid chain, not process-group membership,
// per §L's measurement that group membership is unusable as the predicate.
func TestProbeWithReader_TransitiveGrandchildStillCounted(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := fakeProcessTableReader{
		resolution: time.Millisecond,
		table: []processInfo{
			{PID: 200, PPID: testAgentPID, StartTime: turnStart.Add(-time.Hour)},
			{PID: 300, PPID: 200, StartTime: turnStart}, // grandchild, own pgrp in reality
		},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-72: pre-turn-only descendants settle; a newly observed post-turn-start
// descendant flips the same session back to live.
func TestProbeWithReader_PreTurnThenPostTurnDescendant(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	preTurnOnly := []processInfo{
		{PID: 200, PPID: testAgentPID, StartTime: turnStart.Add(-time.Minute)},
	}

	settledReader := fakeProcessTableReader{resolution: time.Millisecond, table: preTurnOnly}
	got, err := probeWithReader(settledReader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("pre-turn only: got %q, want %q", got, ResultSettled)
	}

	withNewDescendant := append(append([]processInfo{}, preTurnOnly...), processInfo{
		PID: 201, PPID: testAgentPID, StartTime: turnStart.Add(time.Second),
	})
	liveReader := fakeProcessTableReader{resolution: time.Millisecond, table: withNewDescendant}
	got, err = probeWithReader(liveReader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("with a new post-turn-start descendant: got %q, want %q", got, ResultLive)
	}
}

// AC-80: a descendant that started before the recorded turn start but
// within the same truncated-resolution bucket still counts as live — the
// comparison is always against the truncated turn start (round-5 F3), never
// the raw one. An implementation using whole-second (ps-lstart-style)
// truncation is what this guards against.
func TestProbeWithReader_TruncationBoundary(t *testing.T) {
	resolution := 10 * time.Millisecond
	turnStart := time.Unix(1000, 4*int64(time.Millisecond)) // 4ms into a 10ms bucket
	reader := fakeProcessTableReader{
		resolution: resolution,
		table: []processInfo{
			{PID: 200, PPID: testAgentPID, StartTime: turnStart.Truncate(resolution)},
		},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-27a: a zombie descendant is excluded from the live predicate even
// though its recorded start time is after the turn start.
func TestProbeWithReader_ZombieDescendantYieldsSettled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := fakeProcessTableReader{
		resolution: time.Millisecond,
		table: []processInfo{
			{PID: 200, PPID: testAgentPID, StartTime: turnStart.Add(time.Second), Zombie: true},
		},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// D5: a process-table read failure yields unknown, never a shortened set
// mistaken for a complete one.
func TestProbeWithReader_ReadErrorYieldsUnknown(t *testing.T) {
	reader := fakeProcessTableReader{resolution: time.Millisecond, err: errors.New("boom")}

	got, err := probeWithReader(reader, testAgentPID, time.Unix(1000, 0))
	if err == nil {
		t.Fatalf("expected the read error to be surfaced")
	}
	if got != ResultUnknown {
		t.Errorf("got %q, want %q", got, ResultUnknown)
	}
}

func TestProbeWithReader_NoDescendantsYieldsSettled(t *testing.T) {
	reader := fakeProcessTableReader{resolution: time.Millisecond}

	got, err := probeWithReader(reader, testAgentPID, time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// The agent process itself must never count as its own descendant, even
// when the table (incorrectly) contains a self-referential ppid entry.
func TestTransitiveDescendants_ExcludesRootProcessItself(t *testing.T) {
	table := []processInfo{
		{PID: testAgentPID, PPID: testAgentPID, StartTime: time.Now()},
		{PID: 200, PPID: testAgentPID, StartTime: time.Now()},
	}

	descendants := transitiveDescendants(table, testAgentPID)
	for _, d := range descendants {
		if d.PID == testAgentPID {
			t.Fatalf("expected the root process to never appear in its own descendant set")
		}
	}
	if len(descendants) != 1 || descendants[0].PID != 200 {
		t.Fatalf("expected exactly one descendant (200), got %+v", descendants)
	}
}

// ProbeBackgroundWorkloads must never panic, on any platform, for an
// arbitrary pid.
func TestProbeBackgroundWorkloads_DoesNotPanicForAnArbitraryPID(t *testing.T) {
	got, err := ProbeBackgroundWorkloads(1, time.Now())
	if err != nil {
		return
	}
	switch got {
	case ResultLive, ResultSettled, ResultUnknown:
	default:
		t.Errorf("unexpected result %q", got)
	}
}
