//go:build darwin

package service

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
)

// darwinOrphanReapHost implements AC-TASKS-ORPHAN-REAP-002.1's macOS
// mechanism: `lsof -a -d cwd -F pcn` for pid/command/cwd, merged with
// `ps -Ao pid=,ppid=` for ancestry. Measured against the input-inventory
// receipts recorded in the task plan: ~0.17s over 1300+ host processes, and
// lsof still reports a process's cwd after the directory itself is removed.
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

	procs := make([]hostProcess, 0, len(byPID))
	for pid, proc := range byPID {
		proc.PPID = ppidByPID[pid]
		procs = append(procs, proc)
	}
	return procs, nil
}

func (darwinOrphanReapHost) VerifyCwd(ctx context.Context, pid int) (string, error) {
	out, err := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-F", "n").Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "n") {
			return line[1:], nil
		}
	}
	return "", errors.New("orphan reap: no cwd entry for pid")
}

// parseLsofCwdEntries parses `lsof -F pcn` output. A record missing a parsed
// pid or cwd is dropped rather than surfaced (AC-TASKS-ORPHAN-REAP-002.1: "A
// process whose entry cannot be parsed is not a candidate").
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
				current.Command = line[1:]
			}
		case 'n':
			if haveCurrent {
				current.Cwd = line[1:]
			}
		}
	}
	flush()
	return byPID
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
