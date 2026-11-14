package process

// ProcessFootprint is a read-only measurement of the live processes one
// ownership handle covers. It carries counts and bytes only: never a command
// line, an environment value, a credential, or a resume token, so it is safe to
// project on a diagnostic surface.
//
// The walk is identity-bearing. It starts from the process identifier this
// manager owns and follows parent links, so every measured process is a
// descendant of this instance's own agent process rather than a host-wide
// process-name match.
//
// The byte totals cover only processes the walk could actually measure.
// UnreadableProcesses counts owned processes whose measurement failed, and the
// byte totals are then a lower bound; a caller must never read a nonzero
// UnreadableProcesses as zero memory. A platform that cannot report commit
// charge per process leaves CommittedBytes at zero rather than estimating it
// from resident bytes.
type ProcessFootprint struct {
	// Processes is the number of live processes attributable to this ownership
	// handle, including the leader itself.
	Processes int `json:"processes"`
	// CommittedBytes is the summed commit charge of those processes. Zero on a
	// platform that does not report it per process.
	CommittedBytes uint64 `json:"committed_bytes"`
	// ResidentBytes is the summed resident set size of those processes.
	ResidentBytes uint64 `json:"resident_bytes"`
	// UnreadableProcesses counts owned processes the walk could not measure.
	UnreadableProcesses int `json:"unreadable_processes"`
}

// add accumulates one measured process.
func (f *ProcessFootprint) add(committed, resident uint64) {
	f.Processes++
	f.CommittedBytes += committed
	f.ResidentBytes += resident
}

// unreadable records one owned process whose measurement failed. It is counted
// but never guessed, so the totals stay a lower bound the caller can detect.
func (f *ProcessFootprint) unreadable() {
	f.Processes++
	f.UnreadableProcesses++
}

// ownedFootprint measures the live process tree rooted at the agent process this
// manager owns. It is best-effort and never fails: an unreadable process is
// recorded as unreadable so a caller can distinguish "no memory" from "not
// measurable".
func (m *Manager) ownedFootprint() ProcessFootprint {
	return footprintFromProcessTree(m.agentPID())
}

// OwnedFootprint measures the live process tree this manager owns. It is
// read-only and never becomes a precondition for teardown: a failed or empty
// measurement changes nothing about whether a stop proceeds.
func (m *Manager) OwnedFootprint() ProcessFootprint {
	if !m.startMu.TryLock() {
		return ProcessFootprint{UnreadableProcesses: 1}
	}
	defer m.startMu.Unlock()
	if !m.mu.TryLock() {
		return ProcessFootprint{UnreadableProcesses: 1}
	}
	defer m.mu.Unlock()
	return m.ownedFootprint()
}

// descendsFromProcess walks a parent chain with a bounded hop count so a cycle
// introduced by process-identifier reuse cannot loop forever.
//
// The walk is identical on every platform, so it lives here rather than in a
// tagged file: the Linux build does not include the other-unix file, and a
// platform-specific copy would leave one target without it.
func descendsFromProcess(pid, rootPID int, parentOf map[int]int) bool {
	const maxAncestryHops = 256
	current := pid
	for hop := 0; hop < maxAncestryHops; hop++ {
		if current == rootPID {
			return true
		}
		parent, ok := parentOf[current]
		if !ok || parent == current || parent <= 0 {
			return false
		}
		current = parent
	}
	return false
}
