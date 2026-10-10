//go:build linux

package processidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

type procStat struct {
	pid       int
	state     byte
	groupID   int
	sessionID int
	startTime uint64
}

type ownedProcess struct {
	pid       int
	groupID   int
	sessionID int
	startTime uint64
}

func Capture(pid int) (Identity, error) {
	stat, err := readProcStat(pid)
	if err != nil {
		return Identity{}, err
	}
	bootID, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return Identity{}, fmt.Errorf("read kernel boot identity: %w", err)
	}
	return Identity{
		PID:        stat.pid,
		GroupID:    stat.groupID,
		SessionID:  stat.sessionID,
		BirthToken: fmt.Sprintf("linux:%s:%d", strings.TrimSpace(string(bootID)), stat.startTime),
	}, nil
}

func Inspect(identity Identity) (State, error) {
	if err := identity.Validate(); err != nil {
		return StateUnknown, err
	}
	if identity.GroupID <= 0 || identity.SessionID <= 0 {
		return StateUnknown, ErrUnverifiableIdentity
	}
	stat, err := readProcStat(identity.PID)
	if errors.Is(err, os.ErrNotExist) {
		return StateExited, nil
	}
	if err != nil {
		return StateUnknown, err
	}
	wantBoot, wantStart, err := parseBirthToken(identity.BirthToken)
	if err != nil {
		return StateUnknown, err
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return StateUnknown, fmt.Errorf("read kernel boot identity: %w", err)
	}
	if stat.startTime != wantStart || strings.TrimSpace(string(boot)) != wantBoot ||
		stat.groupID != identity.GroupID || stat.sessionID != identity.SessionID {
		return StateReused, nil
	}
	if stat.state == 'Z' || stat.state == 'X' {
		return StateExited, nil
	}
	return StateAlive, nil
}

func TerminateOwnedSession(ctx context.Context, identity Identity, grace time.Duration) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	if identity.SessionID != identity.PID || identity.GroupID != identity.PID {
		return fmt.Errorf("process is not an isolated session leader: %w", ErrUnverifiableIdentity)
	}
	state, err := Inspect(identity)
	if err != nil {
		return err
	}
	if state == StateUnknown || state == StateReused {
		return fmt.Errorf("cannot verify owned process %s: %w", identity, ErrUnverifiableIdentity)
	}
	if state == StateAlive {
		return fmt.Errorf("refusing to contain process group while its owner is alive: %s", identity)
	}
	_, startTime, err := parseBirthToken(identity.BirthToken)
	if err != nil {
		return err
	}
	if grace < 0 {
		grace = 0
	}
	return containOwnedSession(ctx, identity.SessionID, startTime, grace)
}

func containOwnedSession(ctx context.Context, sessionID int, startTime uint64, grace time.Duration) error {
	processes, err := processesInOwnedSession(sessionID, startTime)
	if err != nil {
		return err
	}
	if err := signalOwnedProcesses(processes, sessionID, syscall.SIGTERM); err != nil {
		return err
	}

	graceCtx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if err := waitForOwnedSessionExit(graceCtx, sessionID, startTime); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return ctx.Err()
	}
	return killOwnedSession(ctx, sessionID, startTime)
}

func killOwnedSession(ctx context.Context, sessionID int, startTime uint64) error {
	processes, err := processesInOwnedSession(sessionID, startTime)
	if err != nil {
		return err
	}
	if err := signalOwnedProcesses(processes, sessionID, syscall.SIGKILL); err != nil {
		return err
	}
	return waitForOwnedSessionExit(ctx, sessionID, startTime)
}

func parseBirthToken(token string) (string, uint64, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 3 || parts[0] != "linux" || parts[1] == "" {
		return "", 0, ErrUnverifiableIdentity
	}
	start, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil || start == 0 {
		return "", 0, ErrUnverifiableIdentity
	}
	return parts[1], start, nil
}

func readProcStat(pid int) (procStat, error) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return procStat{}, err
	}
	line := string(data)
	end := strings.LastIndexByte(line, ')')
	if end < 0 || end+1 >= len(line) {
		return procStat{}, fmt.Errorf("parse proc stat for pid %d: malformed command field", pid)
	}
	fields := strings.Fields(line[end+1:])
	if len(fields) < 20 {
		return procStat{}, fmt.Errorf("parse proc stat for pid %d: incomplete fields", pid)
	}
	state := fields[0]
	groupID, err := strconv.Atoi(fields[2])
	if err != nil {
		return procStat{}, fmt.Errorf("parse proc stat group for pid %d: %w", pid, err)
	}
	sessionID, err := strconv.Atoi(fields[3])
	if err != nil {
		return procStat{}, fmt.Errorf("parse proc stat session for pid %d: %w", pid, err)
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return procStat{}, fmt.Errorf("parse proc stat birth time for pid %d: %w", pid, err)
	}
	return procStat{pid: pid, state: state[0], groupID: groupID, sessionID: sessionID, startTime: startTime}, nil
}

func processesInOwnedSession(sessionID int, rootStartTime uint64) ([]ownedProcess, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("enumerate processes: %w", err)
	}
	processes := make([]ownedProcess, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || !entry.IsDir() {
			continue
		}
		stat, err := readProcStat(pid)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if stat.sessionID != sessionID || stat.startTime < rootStartTime || stat.state == 'Z' || stat.state == 'X' {
			continue
		}
		processes = append(processes, ownedProcess{
			pid: stat.pid, groupID: stat.groupID, sessionID: stat.sessionID, startTime: stat.startTime,
		})
	}
	return processes, nil
}

func signalOwnedProcesses(processes []ownedProcess, sessionID int, signal syscall.Signal) error {
	for _, process := range processes {
		pidfd, err := unix.PidfdOpen(process.pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return fmt.Errorf("open process identity for pid %d: %w", process.pid, err)
		}
		stat, statErr := readProcStat(process.pid)
		if errors.Is(statErr, os.ErrNotExist) {
			_ = unix.Close(pidfd)
			continue
		}
		if statErr != nil {
			_ = unix.Close(pidfd)
			return fmt.Errorf("recheck owned process %d: %w", process.pid, statErr)
		}
		if stat.sessionID != sessionID || stat.groupID != process.groupID || stat.startTime != process.startTime {
			_ = unix.Close(pidfd)
			continue
		}
		err = unix.PidfdSendSignal(pidfd, signal, nil, 0)
		_ = unix.Close(pidfd)
		if err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("signal owned process %d: %w", process.pid, err)
		}
	}
	return nil
}

func waitForOwnedSessionExit(ctx context.Context, sessionID int, rootStartTime uint64) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		processes, err := processesInOwnedSession(sessionID, rootStartTime)
		if err != nil {
			return err
		}
		if len(processes) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
