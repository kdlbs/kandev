package service

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
)

// procDeletedSuffix is the suffix Linux appends to /proc/<pid>/cwd's link
// target once the directory it points to has been removed.
const procDeletedSuffix = " (deleted)"

// trimProcCwdDeletedSuffix strips /proc/<pid>/cwd's " (deleted)" suffix.
// Pure string logic, so it carries no build tag even though only Linux's
// snapshotter calls it -- see apps/backend/AGENTS.md's
// platform-untagged-helper rule.
func trimProcCwdDeletedSuffix(target string) string {
	return strings.TrimSuffix(target, procDeletedSuffix)
}

// parseProcStatLine parses the content of /proc/<pid>/stat: "pid (comm)
// state ppid ...". comm is located between the first '(' and the last ')'
// so an embedded space or paren in the command name cannot desynchronize
// the field count. Pure string logic, so it carries no build tag even
// though only Linux's snapshotter calls it.
func parseProcStatLine(line string) (ppid int, command string, err error) {
	line = strings.TrimSpace(line)
	open := strings.IndexByte(line, '(')
	closeParen := strings.LastIndexByte(line, ')')
	if open < 0 || closeParen < open {
		return 0, "", os.ErrInvalid
	}
	command = line[open+1 : closeParen]
	rest := strings.Fields(line[closeParen+1:])
	if len(rest) < 2 {
		return 0, "", os.ErrInvalid
	}
	ppid, err = strconv.Atoi(rest[1])
	if err != nil {
		return 0, "", err
	}
	return ppid, command, nil
}

// parseLsofCwdEntries parses `lsof -F pcn` output. A record missing a parsed
// pid or cwd is dropped rather than surfaced as a candidate with a partial
// identity. Pure string parsing, so it carries no build tag even though only
// darwin's snapshotter calls it — see apps/backend/AGENTS.md's
// platform-untagged-helper rule.
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

// parseLsofVerifyCwdLine parses `lsof -a -p <pid> -d cwd -F n` output, the
// single-pid pre-signal re-verification read. A raw control byte in the cwd
// line is rejected the same way parseLsofCwdEntries rejects one in the
// whole-host snapshot: this read gates an irreversible signal, so it must
// not trust a corrupted or desynchronized line any more than that one does.
func parseLsofVerifyCwdLine(out []byte) (string, error) {
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.HasPrefix(line, "n") {
			continue
		}
		value := line[1:]
		if containsOrphanReapControlByte(value) {
			return "", errors.New("orphan reap: cwd entry contains a raw control byte")
		}
		return value, nil
	}
	return "", errors.New("orphan reap: no cwd entry for pid")
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
// skips empty-cwd entries), but the ownership ancestry walk needs every pid's
// ancestry to be resolvable, including one whose cwd is unreadable.
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
