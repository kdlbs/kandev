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
	startSec, startUSec, err := parseDarwinBirthToken(identity.BirthToken)
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

// TerminateOwnedSession never signals orphaned Darwin session members: a numeric
// session ID cannot bind them to a dead leader's birth identity after ID reuse.
func TerminateOwnedSession(ctx context.Context, identity Identity, _ time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	terminated, err := OwnedSessionTerminated(identity)
	if err != nil {
		return err
	}
	if !terminated {
		return fmt.Errorf("cannot verify orphan ownership for %s: %w", identity, ErrUnverifiableIdentity)
	}
	return nil
}

func parseDarwinBirthToken(token string) (int64, int32, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 4 || parts[0] != "darwin" {
		return 0, 0, ErrUnverifiableIdentity
	}
	sec, secErr := strconv.ParseInt(parts[1], 10, 64)
	usec, usecErr := strconv.ParseInt(parts[2], 10, 32)
	_, sessionErr := strconv.ParseUint(parts[3], 16, 64)
	if secErr != nil || usecErr != nil || sessionErr != nil || sec <= 0 || usec < 0 || usec >= 1_000_000 {
		return 0, 0, ErrUnverifiableIdentity
	}
	return sec, int32(usec), nil
}

func processGroupsInSession(sessionID int, rootBirth string) ([]int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return nil, err
	}
	rootSec, rootUSec, err := parseDarwinBirthToken(rootBirth)
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
		groupID, err := darwinSessionMemberGroup(proc, sessionID)
		if err != nil {
			return nil, err
		}
		if groupID > 0 {
			groups[groupID] = struct{}{}
		}
	}
	result := make([]int, 0, len(groups))
	for groupID := range groups {
		result = append(result, groupID)
	}
	return result, nil
}

func darwinSessionMemberGroup(proc unix.KinfoProc, sessionID int) (int, error) {
	currentSession, err := unix.Getsid(int(proc.Proc.P_pid))
	if errors.Is(err, syscall.ESRCH) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if currentSession != sessionID {
		return 0, nil
	}
	start := proc.Proc.P_starttime
	state, err := Inspect(Identity{
		PID: int(proc.Proc.P_pid), GroupID: int(proc.Eproc.Pgid), SessionID: sessionID,
		BirthToken: fmt.Sprintf("darwin:%d:%d:0", start.Sec, start.Usec),
	})
	if err != nil {
		return 0, err
	}
	if state == StateExited {
		return 0, nil
	}
	if state != StateAlive {
		return 0, ErrUnverifiableIdentity
	}
	return int(proc.Eproc.Pgid), nil
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
