//go:build darwin

package processidentity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func Capture(pid int) (Identity, error) {
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Identity{}, err
	}
	sessionID, err := unix.Getsid(pid)
	if err != nil {
		return Identity{}, err
	}
	start := proc.Proc.P_starttime
	return Identity{
		PID:       pid,
		GroupID:   int(proc.Eproc.Pgid),
		SessionID: sessionID,
		// The final field preserves the stored token format. Kernel session
		// pointers are redacted on macOS; SessionID carries the OS session ID.
		BirthToken: fmt.Sprintf("darwin:%d:%d:0", start.Sec, start.Usec),
	}, nil
}

func Inspect(identity Identity) (State, error) {
	if err := identity.Validate(); err != nil {
		return StateUnknown, err
	}
	if identity.GroupID <= 0 || identity.SessionID <= 0 {
		return StateUnknown, ErrUnverifiableIdentity
	}
	startSec, startUSec, _, err := parseDarwinBirthToken(identity.BirthToken)
	if err != nil {
		return StateUnknown, err
	}
	proc, err := unix.SysctlKinfoProc("kern.proc.pid", identity.PID)
	// x/sys reports EIO for a successful sysctl with an empty process result.
	// Confirm absence independently; an arbitrary I/O error is not death proof.
	if errors.Is(err, syscall.EIO) && errors.Is(syscall.Kill(identity.PID, 0), syscall.ESRCH) {
		return StateExited, nil
	}
	if errors.Is(err, syscall.ESRCH) || errors.Is(err, syscall.ENOENT) {
		return StateExited, nil
	}
	if err != nil {
		return StateUnknown, err
	}
	start := proc.Proc.P_starttime
	sessionID, err := unix.Getsid(identity.PID)
	if errors.Is(err, syscall.ESRCH) {
		return StateExited, nil
	}
	if err != nil {
		return StateUnknown, err
	}
	if start.Sec != startSec || start.Usec != startUSec ||
		int(proc.Eproc.Pgid) != identity.GroupID || sessionID != identity.SessionID {
		return StateReused, nil
	}
	if proc.Proc.P_stat == 5 {
		return StateExited, nil
	}
	return StateAlive, nil
}

func TerminateOwnedSession(ctx context.Context, identity Identity, grace time.Duration) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	if identity.SessionID != identity.PID {
		return fmt.Errorf("process is not an isolated session leader: %w", ErrUnverifiableIdentity)
	}
	state, err := Inspect(identity)
	if err != nil {
		return err
	}
	if state == StateAlive {
		return fmt.Errorf("refusing to contain process group while its owner is alive: %s", identity)
	}
	if state == StateUnknown || state == StateReused {
		return fmt.Errorf("cannot verify owned process %s: %w", identity, ErrUnverifiableIdentity)
	}
	if grace < 0 {
		grace = 0
	}
	groups, err := processGroupsInSession(identity.SessionID, identity.BirthToken)
	if err != nil {
		return err
	}
	for _, groupID := range groups {
		_ = syscall.Kill(-groupID, syscall.SIGTERM)
	}
	graceCtx, cancel := context.WithTimeout(ctx, grace)
	defer cancel()
	if err := waitForSessionExit(graceCtx, identity.SessionID, identity.BirthToken); err == nil {
		return nil
	} else if ctx.Err() != nil {
		return ctx.Err()
	}
	groups, err = processGroupsInSession(identity.SessionID, identity.BirthToken)
	if err != nil {
		return err
	}
	for _, groupID := range groups {
		_ = syscall.Kill(-groupID, syscall.SIGKILL)
	}
	return waitForSessionExit(ctx, identity.SessionID, identity.BirthToken)
}

func parseDarwinBirthToken(token string) (int64, int32, uintptr, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 4 || parts[0] != "darwin" {
		return 0, 0, 0, ErrUnverifiableIdentity
	}
	sec, secErr := strconv.ParseInt(parts[1], 10, 64)
	usec, usecErr := strconv.ParseInt(parts[2], 10, 32)
	session, sessionErr := strconv.ParseUint(parts[3], 16, 64)
	if secErr != nil || usecErr != nil || sessionErr != nil || sec <= 0 || usec < 0 || usec >= 1_000_000 {
		return 0, 0, 0, ErrUnverifiableIdentity
	}
	return sec, int32(usec), uintptr(session), nil
}

func processGroupsInSession(sessionID int, rootBirth string) ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	rootSec, rootUSec, _, err := parseDarwinBirthToken(rootBirth)
	if err != nil {
		return nil, err
	}
	groups := make(map[int]struct{})
	for _, proc := range procs {
		if proc.Proc.P_pid <= 0 || proc.Proc.P_stat == 5 {
			continue
		}
		start := proc.Proc.P_starttime
		if start.Sec < rootSec || (start.Sec == rootSec && start.Usec < rootUSec) {
			continue
		}
		currentSession, err := unix.Getsid(int(proc.Proc.P_pid))
		if errors.Is(err, syscall.ESRCH) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if currentSession == sessionID && proc.Eproc.Pgid > 0 {
			groups[int(proc.Eproc.Pgid)] = struct{}{}
		}
	}
	result := make([]int, 0, len(groups))
	for groupID := range groups {
		result = append(result, groupID)
	}
	return result, nil
}

func waitForSessionExit(ctx context.Context, sessionID int, rootBirth string) error {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		groups, err := processGroupsInSession(sessionID, rootBirth)
		if err != nil {
			return err
		}
		if len(groups) == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// OwnedSessionTerminated verifies process and descendant termination without signaling them.
func OwnedSessionTerminated(identity Identity) (bool, error) {
	state, err := Inspect(identity)
	if err != nil {
		return false, err
	}
	if state == StateAlive {
		return false, nil
	}
	if state != StateExited || identity.SessionID != identity.PID {
		return false, ErrUnverifiableIdentity
	}
	groups, err := processGroupsInSession(identity.SessionID, identity.BirthToken)
	return len(groups) == 0, err
}
