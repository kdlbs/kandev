// Package probe implements the background-workload liveness probe (spec
// docs/specs/disambiguate-waiting/spec.md, §"Background-workload liveness
// probe (agentctl)", D5/D6): given an agent process and a recorded turn
// start, it samples the agent's transitive descendant processes for one
// that started at-or-after the (truncated) turn start and is still alive.
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

// ProbeBackgroundWorkloads samples agentPID's transitive descendant
// process set for a member whose start time is at-or-after the truncated
// turnStart and which is not a zombie at snapshot time (D5). It reports
// ResultUnknown, never a shortened descendant set, for any platform or read
// failure that makes a complete snapshot impossible.
func ProbeBackgroundWorkloads(agentPID int, turnStart time.Time) (Result, error) {
	reader := platformProcessTableReader()
	if reader == nil {
		return ResultUnknown, nil
	}
	return probeWithReader(reader, agentPID, turnStart)
}

func probeWithReader(reader processTableReader, agentPID int, turnStart time.Time) (Result, error) {
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
	for _, descendant := range transitiveDescendants(table, agentPID) {
		if descendant.Zombie {
			continue
		}
		if !descendant.StartTime.Before(truncatedTurnStart) {
			return ResultLive, nil
		}
	}
	return ResultSettled, nil
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
