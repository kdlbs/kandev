//go:build linux

package service

import (
	"context"
	"os"
	"strconv"
	"strings"
)

// linuxOrphanReapHost implements AC-TASKS-ORPHAN-REAP-002.1's Linux
// mechanism: /proc/<pid>/cwd (whose target carries a " (deleted)" suffix
// after the directory is removed, stripped before comparison) plus
// /proc/<pid>/stat for ancestry and command name.
type linuxOrphanReapHost struct{}

func defaultOrphanReapHostSnapshotter() orphanReapHostSnapshotter { return linuxOrphanReapHost{} }
func defaultOrphanReapVerifier() orphanReapVerifier               { return linuxOrphanReapHost{} }

const procDeletedSuffix = " (deleted)"

func (linuxOrphanReapHost) Snapshot(ctx context.Context) ([]hostProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	procs := make([]hostProcess, 0, len(entries))
	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		cwd, cwdErr := readProcCwd(pid)
		if cwdErr != nil {
			// Gone since the directory listing, or unreadable: not a
			// candidate (AC-TASKS-ORPHAN-REAP-002.1).
			continue
		}
		ppid, command, statErr := readProcStat(pid)
		if statErr != nil {
			continue
		}
		procs = append(procs, hostProcess{PID: pid, PPID: ppid, Cwd: cwd, Command: command})
	}
	return procs, nil
}

func (linuxOrphanReapHost) VerifyCwd(ctx context.Context, pid int) (string, error) {
	return readProcCwd(pid)
}

func readProcCwd(pid int) (string, error) {
	target, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(target, procDeletedSuffix), nil
}

// readProcStat parses /proc/<pid>/stat: "pid (comm) state ppid ...". comm is
// located between the first '(' and the last ')' so an embedded space or
// paren in the command name cannot desynchronize the field count.
func readProcStat(pid int) (ppid int, command string, err error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", err
	}
	line := strings.TrimSpace(string(data))
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
