//go:build darwin

package service

import (
	"context"
	"os/exec"
	"strconv"
)

// darwinOrphanReapHost implements the macOS detection mechanism:
// `lsof -a -d cwd -F pcn` for pid/command/cwd, merged with
// `ps -Ao pid=,ppid=` for ancestry. Measured at ~0.17s over 1300+ host
// processes, and lsof still reports a process's cwd after the directory
// itself is removed.
type darwinOrphanReapHost struct{}

func defaultOrphanReapHostSnapshotter() orphanReapHostSnapshotter { return darwinOrphanReapHost{} }
func defaultOrphanReapVerifier() orphanReapVerifier               { return darwinOrphanReapHost{} }

func (darwinOrphanReapHost) Snapshot(ctx context.Context) ([]hostProcess, error) {
	lsofOut, err := exec.CommandContext(ctx, "lsof", "-a", "-d", "cwd", "-F", "pcn").Output()
	if err != nil {
		return nil, err
	}
	byPID := parseLsofCwdEntries(lsofOut)

	psOut, err := exec.CommandContext(ctx, "ps", "-Ao", "pid=,ppid=").Output()
	if err != nil {
		return nil, err
	}
	ppidByPID := parsePSAncestry(psOut)

	return combineLsofAndPSSnapshot(byPID, ppidByPID), nil
}

func (darwinOrphanReapHost) VerifyCwd(ctx context.Context, pid int) (string, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-F", "n").Output()
	if err != nil {
		return "", err
	}
	return parseLsofVerifyCwdLine(out)
}
