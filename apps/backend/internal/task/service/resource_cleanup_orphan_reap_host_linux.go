//go:build linux

package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	return snapshotLinuxProc(ctx, "/proc", os.ReadFile, readProcCwdAt)
}

func snapshotLinuxProc(
	ctx context.Context,
	procRoot string,
	readFile func(string) ([]byte, error),
	readCwd func(string, int) (string, error),
) ([]hostProcess, error) {
	entries, err := os.ReadDir(procRoot)
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
		ppid, command, statErr := readProcStatAt(procRoot, pid, readFile)
		if statErr != nil {
			if errors.Is(statErr, fs.ErrNotExist) {
				// The process exited after /proc was enumerated, so it cannot
				// contribute a candidate or an ancestry hop to this snapshot.
				continue
			}
			return nil, fmt.Errorf("read /proc/%d/stat: %w", pid, statErr)
		}
		// A cwd read failure still leaves ancestry (ppid) usable for the
		// ownership walk; leave Cwd empty so this pid never becomes a
		// candidate (attributeOrphanReapCandidates skips empty-cwd entries).
		cwd, _ := readCwd(procRoot, pid)
		procs = append(procs, hostProcess{PID: pid, PPID: ppid, Cwd: cwd, Command: command})
	}
	return procs, nil
}

func (linuxOrphanReapHost) VerifyCwd(ctx context.Context, pid int) (string, error) {
	return readProcCwd(pid)
}

func readProcCwd(pid int) (string, error) {
	return readProcCwdAt("/proc", pid)
}

func readProcCwdAt(procRoot string, pid int) (string, error) {
	target, err := os.Readlink(procRoot + "/" + strconv.Itoa(pid) + "/cwd")
	if err != nil {
		return "", err
	}
	return trimProcCwdDeletedSuffix(target), nil
}

// readProcStat reads /proc/<pid>/stat and parses it via parseProcStatLine.
func readProcStat(pid int) (ppid int, command string, err error) {
	return readProcStatAt("/proc", pid, os.ReadFile)
}

func readProcStatAt(procRoot string, pid int, readFile func(string) ([]byte, error)) (ppid int, command string, err error) {
	data, err := readFile(procRoot + "/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, "", err
	}
	return parseProcStatLine(string(data))
}
