// Package probe implements the background-workload liveness probe (spec
// docs/specs/disambiguate-waiting/spec.md, §"Background-workload liveness
// probe (agentctl)", D5/D6): given an agent process and a recorded turn
// start, it samples the agent's transitive descendant processes for one
// that started at-or-after the (truncated) turn start and is still alive.
//
// Where the descendant walk finds nothing and the platform can read another
// process's environment, it also attributes a workload that left the
// descendant tree by reparenting back to this session, by matching
// KANDEV_SESSION_ID (see docs/specs/disambiguate-waiting/requirements and
// system-design/orphaned-background-workloads.md).
package probe

import "time"

// Result is the probe's three-way outcome (spec D5/D9). ResultUnknown is
// the fail-closed default for every indeterminate input — a process-table
// read error, an unsupported platform, or an incomplete walk. It must never
// be inferred from a partial or shortened descendant set.
type Result string

const (
	ResultLive    Result = "live"
	ResultSettled Result = "settled"
	ResultUnknown Result = "unknown"
)

// processInfo is one process's identity and lifecycle state as read from
// the OS. D5: a process is identified by (pid, start time), never a bare
// pid — pids are reused.
type processInfo struct {
	PID       int
	PPID      int
	StartTime time.Time
	Zombie    bool
	// StartTimeDatum is the platform's own start-time datum for this
	// process: the value the OS reports at process creation and keeps
	// reporting unchanged for that process's whole life. It is populated
	// only by readers that also implement environmentReader, and it is
	// never derived from the current wall clock, unlike StartTime — using
	// it is the only way to tell a still-running process from a different
	// process that later reused the same pid.
	StartTimeDatum int64
}

// processTableReader captures the whole host process table in one pass —
// the "one snapshot" requirement (D5). A read that cannot complete must
// return an error, never a partial table presented as complete.
type processTableReader interface {
	// Resolution is this platform source's start-time precision. The
	// caller truncates the recorded turn start down to this resolution
	// before comparing against it (round-5 F3, AC-80).
	Resolution() time.Duration
	ReadProcessTable() ([]processInfo, error)
}

// environmentReader is the optional processTableReader capability for
// attributing a workload outside the agent's descendant tree to its
// session, by reading another process's environment. It is implemented
// only by platforms that can read another process's environment; where a
// processTableReader does not implement it, the identity scan is skipped
// entirely and the descendant walk's result stands.
type environmentReader interface {
	// HasSessionID reports whether pid's environment carries
	// KANDEV_SESSION_ID with exactly this value. A false result covers
	// both an absent variable and an environment that could not be read at
	// all — either way the candidate does not contribute, and the caller
	// skips it and keeps scanning rather than treating it as a probe
	// failure.
	HasSessionID(pid int, sessionID string) (bool, error)

	// StartTimeDatum re-reads pid's start-time datum straight from the
	// platform, in the same units as processInfo.StartTimeDatum, for
	// match-only re-validation after a session-id match. Like
	// processInfo.StartTimeDatum, it is never derived from the current
	// wall clock.
	StartTimeDatum(pid int) (int64, error)
}

// ProbeBackgroundWorkloads samples agentPID's transitive descendant
// process set for a member whose start time is at-or-after the truncated
// turnStart and which is not a zombie at snapshot time (D5). It reports
// ResultUnknown, never a shortened descendant set, for any platform or read
// failure that makes a complete snapshot impossible.
//
// When the descendant walk finds nothing, and the platform can read
// another process's environment, and sessionID is non-empty, it also
// attributes a workload that left the descendant tree by reparenting, by
// matching KANDEV_SESSION_ID against sessionID. Where either condition
// does not hold it returns the descendant-only result unchanged, reading no
// process environment.
func ProbeBackgroundWorkloads(agentPID int, turnStart time.Time, sessionID string) (Result, error) {
	reader := platformProcessTableReader()
	if reader == nil {
		return ResultUnknown, nil
	}
	return probeWithReader(reader, agentPID, turnStart, sessionID)
}

func probeWithReader(reader processTableReader, agentPID int, turnStart time.Time, sessionID string) (Result, error) {
	table, err := reader.ReadProcessTable()
	if err != nil {
		return ResultUnknown, err
	}
	if !agentProcessPresent(table, agentPID) {
		// D9: "agent process exited | unknown, never settled". An absent
		// root also means its PPID-linked children are indistinguishable
		// from an unrelated process tree rooted elsewhere on the host (a
		// zero or stale pid is not just a "no children" case).
		return ResultUnknown, nil
	}

	truncatedTurnStart := turnStart.Truncate(reader.Resolution())
	descendants := transitiveDescendants(table, agentPID)
	for _, descendant := range descendants {
		if descendant.Zombie {
			continue
		}
		if !descendant.StartTime.Before(truncatedTurnStart) {
			return ResultLive, nil
		}
	}

	envReader, ok := reader.(environmentReader)
	if !ok || sessionID == "" {
		return ResultSettled, nil
	}
	if orphanCarriesSessionID(envReader, table, descendants, agentPID, truncatedTurnStart, sessionID) {
		return ResultLive, nil
	}
	return ResultSettled, nil
}

// orphanCarriesSessionID scans the snapshot for a non-descendant process
// that started in-turn and carries this session's KANDEV_SESSION_ID. It
// runs only once the descendant walk has already found nothing live.
func orphanCarriesSessionID(
	reader environmentReader,
	table []processInfo,
	descendants []processInfo,
	agentPID int,
	truncatedTurnStart time.Time,
	sessionID string,
) bool {
	excluded := excludedCandidatePIDs(table, descendants, agentPID)
	for _, candidate := range table {
		if !isOrphanCandidate(candidate, excluded, truncatedTurnStart) {
			continue
		}
		matched, err := reader.HasSessionID(candidate.PID, sessionID)
		if err != nil || !matched {
			continue // skip and keep scanning; never turn into unknown
		}
		if candidateStillMatches(reader, candidate) {
			return true
		}
		// The pid was recycled between the snapshot and this read, or the
		// re-validation read failed outright: skip and keep scanning.
	}
	return false
}

func isOrphanCandidate(p processInfo, excluded map[int]bool, truncatedTurnStart time.Time) bool {
	if p.Zombie || excluded[p.PID] {
		return false
	}
	return !p.StartTime.Before(truncatedTurnStart)
}

// candidateStillMatches re-reads candidate's start-time datum and requires
// it to equal the value the snapshot recorded, preserving the legacy (pid,
// start time) identity rule for a bare-pid environment read.
func candidateStillMatches(reader environmentReader, candidate processInfo) bool {
	datum, err := reader.StartTimeDatum(candidate.PID)
	if err != nil {
		return false
	}
	return datum == candidate.StartTimeDatum
}

// excludedCandidatePIDs is the set of pids the identity scan must never
// treat as a candidate: every descendant the walk above already covered,
// plus the agent process and each of its ancestors — they all carry the
// session identity themselves, and none of them is a candidate this scan
// exists to find.
func excludedCandidatePIDs(table []processInfo, descendants []processInfo, agentPID int) map[int]bool {
	excluded := make(map[int]bool, len(descendants)+1)
	for _, d := range descendants {
		excluded[d.PID] = true
	}
	for pid := range ancestorsAndSelf(table, agentPID) {
		excluded[pid] = true
	}
	return excluded
}

// ancestorsAndSelf walks PPID upward from agentPID through one snapshot,
// stopping at a PPID of 0 or below, a PID absent from the snapshot (its
// parent already exited), or a PID reached twice — the same cycle guard
// transitiveDescendants uses against a racy snapshot.
func ancestorsAndSelf(table []processInfo, agentPID int) map[int]bool {
	byPID := make(map[int]processInfo, len(table))
	for _, p := range table {
		byPID[p.PID] = p
	}

	excluded := map[int]bool{agentPID: true}
	current, ok := byPID[agentPID]
	for ok {
		if current.PPID <= 0 {
			break
		}
		if excluded[current.PPID] {
			break // pid reached twice: cycle guard
		}
		parent, found := byPID[current.PPID]
		if !found {
			break // parent already exited; absent from this snapshot
		}
		excluded[current.PPID] = true
		current = parent
	}
	return excluded
}

// agentProcessPresent reports whether agentPID names a real entry in this
// snapshot. A pid of zero or below is never valid — treated the same as
// absent rather than walked, since it would otherwise match the kernel's
// PPID-0-rooted process tree instead of no tree at all.
func agentProcessPresent(table []processInfo, agentPID int) bool {
	if agentPID <= 0 {
		return false
	}
	for _, p := range table {
		if p.PID == agentPID {
			return true
		}
	}
	return false
}

// transitiveDescendants returns every process whose ancestor chain reaches
// rootPID, computed from one process-table snapshot (D5). rootPID itself is
// never included — the agent process is never a member of its own
// descendant set.
func transitiveDescendants(table []processInfo, rootPID int) []processInfo {
	childrenByParent := make(map[int][]processInfo, len(table))
	for _, p := range table {
		childrenByParent[p.PPID] = append(childrenByParent[p.PPID], p)
	}

	visited := map[int]bool{rootPID: true}
	queue := []int{rootPID}
	var descendants []processInfo
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, child := range childrenByParent[pid] {
			if visited[child.PID] {
				continue // guards a pathological ppid cycle in a racy snapshot
			}
			visited[child.PID] = true
			descendants = append(descendants, child)
			queue = append(queue, child.PID)
		}
	}
	return descendants
}
