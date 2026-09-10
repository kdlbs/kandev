package service

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
)

// parseLsofCwdEntries parses `lsof -F pcn` output. A record missing a parsed
// pid or cwd is dropped rather than surfaced (AC-TASKS-ORPHAN-REAP-002.1: "A
// process whose entry cannot be parsed is not a candidate"). Pure string
// parsing, so it carries no build tag even though only darwin's snapshotter
// calls it — see apps/backend/AGENTS.md's platform-untagged-helper rule.
func parseLsofCwdEntries(out []byte) map[int]hostProcess {
	byPID := make(map[int]hostProcess)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	var current hostProcess
	haveCurrent := false
	flush := func() {
		if haveCurrent && current.PID != 0 && current.Cwd != "" {
			byPID[current.PID] = current
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case 'p':
			flush()
			pid, err := strconv.Atoi(line[1:])
			current = hostProcess{}
			haveCurrent = err == nil
			if haveCurrent {
				current.PID = pid
			}
		case 'c':
			if haveCurrent {
				if value := line[1:]; containsOrphanReapControlByte(value) {
					haveCurrent = false
				} else {
					current.Command = value
				}
			}
		case 'n':
			if haveCurrent {
				if value := line[1:]; containsOrphanReapControlByte(value) {
					haveCurrent = false
				} else {
					current.Cwd = value
				}
			}
		}
	}
	flush()
	return byPID
}

// containsOrphanReapControlByte reports whether s carries a raw ASCII
// control byte. Genuine lsof -F output escapes such bytes to a printable
// caret form, so a raw one in a parsed field is not a real path or command --
// the record it belongs to is treated as unparseable rather than trusted with
// a partial or corrupted value.
func containsOrphanReapControlByte(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return true
		}
	}
	return false
}

// parsePSAncestry parses `ps -Ao pid=,ppid=` output into a pid->ppid map.
func parsePSAncestry(out []byte) map[int]int {
	ppidByPID := make(map[int]int)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		ppidByPID[pid] = ppid
	}
	return ppidByPID
}

// combineLsofAndPSSnapshot merges lsof's per-pid cwd/command data with ps's
// per-pid ancestry into one host snapshot. A pid lsof could not resolve a cwd
// for still contributes its ancestry (ppid) with an empty Cwd: candidate
// attribution already requires a non-empty Cwd (attributeOrphanReapCandidates
// skips empty-cwd entries), but the ownership walk
// (AC-TASKS-ORPHAN-REAP-003.3) needs every pid's ancestry to be resolvable,
// including one whose cwd is unreadable.
func combineLsofAndPSSnapshot(byPID map[int]hostProcess, ppidByPID map[int]int) []hostProcess {
	procs := make([]hostProcess, 0, len(byPID)+len(ppidByPID))
	seen := make(map[int]struct{}, len(byPID))
	for pid, proc := range byPID {
		proc.PPID = ppidByPID[pid]
		procs = append(procs, proc)
		seen[pid] = struct{}{}
	}
	for pid, ppid := range ppidByPID {
		if _, ok := seen[pid]; ok {
			continue
		}
		procs = append(procs, hostProcess{PID: pid, PPID: ppid})
	}
	return procs
}
