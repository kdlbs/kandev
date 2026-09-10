//go:build linux

package service

import (
	"context"
	"os"
	"strconv"
)

// linuxOrphanReapHost implements the Linux detection mechanism:
// /proc/<pid>/cwd (whose target carries a " (deleted)" suffix after the
// directory is removed, stripped before comparison) plus /proc/<pid>/stat
// for ancestry and command name.
type linuxOrphanReapHost struct{}

func defaultOrphanReapHostSnapshotter() orphanReapHostSnapshotter { return linuxOrphanReapHost{} }
func defaultOrphanReapVerifier() orphanReapVerifier               { return linuxOrphanReapHost{} }

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
		ppid, command, statErr := readProcStat(pid)
		if statErr != nil {
			// Gone since the directory listing, or unreadable: no ancestry
			// or cwd is obtainable at all, so this pid can be neither a
			// candidate nor an ancestry hop.
			continue
		}
		// A cwd read failure still leaves ancestry (ppid) usable for the
		// ownership walk; leave Cwd empty so this pid never becomes a
		// candidate (attributeOrphanReapCandidates skips empty-cwd entries).
		cwd, _ := readProcCwd(pid)
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
	return trimProcCwdDeletedSuffix(target), nil
}

// readProcStat reads /proc/<pid>/stat and parses it via parseProcStatLine.
func readProcStat(pid int) (ppid int, command string, err error) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", err
	}
	return parseProcStatLine(string(data))
}
