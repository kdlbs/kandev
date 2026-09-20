package probe

import (
	"errors"
	"testing"
	"time"
)

// envCapableReader adds the environmentReader capability on top of
// fakeProcessTableReader, simulating a platform that can read another
// process's environment (Linux). fakeProcessTableReader alone does not
// implement environmentReader, simulating Darwin's missing capability — the
// two fakes are deliberately different types for exactly that reason.
type envCapableReader struct {
	fakeProcessTableReader
	sessionIDs map[int]string // pid -> KANDEV_SESSION_ID value
	sessionErr map[int]error  // pid -> HasSessionID error
	datums     map[int]int64  // pid -> current live start-time datum
	datumErr   map[int]error  // pid -> StartTimeDatum error
	calls      []int          // pids HasSessionID was invoked for
}

func (e *envCapableReader) HasSessionID(pid int, sessionID string) (bool, error) {
	e.calls = append(e.calls, pid)
	if err, ok := e.sessionErr[pid]; ok {
		return false, err
	}
	return e.sessionIDs[pid] == sessionID, nil
}

func (e *envCapableReader) StartTimeDatum(pid int) (int64, error) {
	if err, ok := e.datumErr[pid]; ok {
		return 0, err
	}
	return e.datums[pid], nil
}

func calledFor(calls []int, pid int) bool {
	for _, c := range calls {
		if c == pid {
			return true
		}
	}
	return false
}

// AC-DW-ORPHAN-001.1: a reparented, in-turn, non-zombie candidate carrying
// this session's identity, and whose re-validation datum matches, reads live
// even though it is not a descendant.
func TestOrphanScan_MatchingSessionID_Live(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-DW-ORPHAN-001.2: a process carrying the session identity that started
// before the turn does not contribute, and its environment is never read —
// the turn-start filter runs before any environment access.
func TestOrphanScan_MatchingSessionIDButPreTurn_Settled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart.Add(-time.Hour), StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
	if calledFor(reader.calls, 500) {
		t.Errorf("expected no environment read for a pre-turn candidate, got calls %v", reader.calls)
	}
}

// AC-DW-ORPHAN-001.3: an in-turn, non-descendant process without the
// identity does not contribute.
func TestOrphanScan_NoMatchingSessionID_Settled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "some-other-session"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-DW-ORPHAN-001.4: a zombie is excluded on the identity path too, and its
// environment is never read.
func TestOrphanScan_ZombieCandidate_Settled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, Zombie: true, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
	if calledFor(reader.calls, 500) {
		t.Errorf("expected no environment read for a zombie candidate, got calls %v", reader.calls)
	}
}

// AC-DW-ORPHAN-002.2: a candidate whose environment cannot be read is
// skipped, the scan continues, and a later match is still found.
func TestOrphanScan_EnvironmentReadError_SkipsAndContinues(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 1},
				{PID: 600, PPID: 1, StartTime: turnStart, StartTimeDatum: 7},
			},
		},
		sessionErr: map[int]error{500: errors.New("permission denied")},
		sessionIDs: map[int]string{600: "sess-1"},
		datums:     map[int]int64{600: 7},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
}

// AC-DW-ORPHAN-002.2: an environment-read failure is never reported as
// unknown — it means "does not contribute", not a probe failure.
func TestOrphanScan_EnvironmentReadError_NeverUnknown(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 1},
			},
		},
		sessionErr: map[int]error{500: errors.New("process exited")},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-DW-ORPHAN-002.4: an empty session id on the request skips the identity
// pass entirely and reads no environment at all, even though a candidate
// would otherwise match.
func TestOrphanScan_EmptySessionID_SkipsPassEntirely(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: ""},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no environment read for an empty session id, got calls %v", reader.calls)
	}
}

// AC-DW-ORPHAN-002.5: a descendant hit short-circuits before any
// environment is read, even when a non-descendant candidate also matches.
func TestOrphanScan_DescendantHitSkipsIdentityPass(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 200, PPID: testAgentPID, StartTime: turnStart},
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q", got, ResultLive)
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no environment read once the descendant walk found a live member, got calls %v", reader.calls)
	}
}

// AC-DW-ORPHAN-001.9: the agent process and its ancestors never contribute,
// even when one carries a matching, in-turn session identity, and their
// environments are never read.
func TestOrphanScan_AgentAndAncestorsExcluded(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	agent := processInfo{PID: testAgentPID, PPID: 50, StartTime: time.Unix(0, 0)}
	ancestor := processInfo{PID: 50, PPID: 1, StartTime: turnStart, StartTimeDatum: 99}
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table:      []processInfo{agent, ancestor},
		},
		sessionIDs: map[int]string{50: "sess-1", testAgentPID: "sess-1"},
		datums:     map[int]int64{50: 99},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
	if calledFor(reader.calls, 50) || calledFor(reader.calls, testAgentPID) {
		t.Errorf("expected the agent and its ancestors to never be read, got calls %v", reader.calls)
	}
}

// AC-DW-ORPHAN-001.10: exact equality only — a value that merely shares a
// prefix with the probe's session id must not match.
func TestOrphanScan_SessionIDPrefixDoesNotMatch(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1-extra"},
		datums:     map[int]int64{500: 42},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-DW-ORPHAN-001.11 / legacy (pid, start time) identity: a candidate whose
// start-time datum no longer matches the snapshot at re-read is a recycled
// pid and is skipped rather than matched.
func TestOrphanScan_RevalidationDatumMismatch_Settled(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 42},
			},
		},
		sessionIDs: map[int]string{500: "sess-1"},
		datums:     map[int]int64{500: 999}, // live re-read differs: pid was recycled
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultSettled {
		t.Errorf("got %q, want %q", got, ResultSettled)
	}
}

// AC-DW-ORPHAN-002.2a: a re-validation read that fails outright is skipped,
// the scan continues, and the probe never returns unknown from this step.
func TestOrphanScan_RevalidationReadError_NeverUnknownAndContinues(t *testing.T) {
	turnStart := time.Unix(1000, 0)
	reader := &envCapableReader{
		fakeProcessTableReader: fakeProcessTableReader{
			resolution: time.Millisecond,
			table: []processInfo{
				rootEntry,
				{PID: 500, PPID: 1, StartTime: turnStart, StartTimeDatum: 1},
				{PID: 600, PPID: 1, StartTime: turnStart, StartTimeDatum: 7},
			},
		},
		sessionIDs: map[int]string{500: "sess-1", 600: "sess-1"},
		datumErr:   map[int]error{500: errors.New("process exited before re-read")},
		datums:     map[int]int64{600: 7},
	}

	got, err := probeWithReader(reader, testAgentPID, turnStart, "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != ResultLive {
		t.Errorf("got %q, want %q (scan must continue past a failed re-validation read)", got, ResultLive)
	}
}
