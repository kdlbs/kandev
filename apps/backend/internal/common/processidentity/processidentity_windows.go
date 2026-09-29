//go:build windows

package processidentity

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

func Capture(pid int) (Identity, error) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Identity{}, err
	}
	defer windows.CloseHandle(process)
	creationTime, err := processCreationTime(process)
	if err != nil {
		return Identity{}, err
	}
	return Identity{PID: pid, BirthToken: fmt.Sprintf("windows:%d", creationTime)}, nil
}

func Inspect(identity Identity) (State, error) {
	if err := identity.Validate(); err != nil {
		return StateUnknown, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(identity.PID))
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return StateExited, nil
		}
		return StateUnknown, err
	}
	defer windows.CloseHandle(process)
	creationTime, err := processCreationTime(process)
	if err != nil {
		return StateUnknown, err
	}
	want, err := parseWindowsBirthToken(identity.BirthToken)
	if err != nil {
		return StateUnknown, err
	}
	if creationTime != want {
		return StateReused, nil
	}
	return StateAlive, nil
}

func TerminateOwnedSession(_ context.Context, identity Identity, _ time.Duration) error {
	if err := identity.Validate(); err != nil {
		return err
	}
	return fmt.Errorf("Windows descendant cleanup requires the launcher's owned Job Object: %w", ErrUnverifiableIdentity)
}

func processCreationTime(process windows.Handle) (uint64, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	return uint64(creation.Nanoseconds()), nil
}

func parseWindowsBirthToken(token string) (uint64, error) {
	parts := strings.Split(token, ":")
	if len(parts) != 2 || parts[0] != "windows" {
		return 0, ErrUnverifiableIdentity
	}
	created, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil || created == 0 {
		return 0, ErrUnverifiableIdentity
	}
	return created, nil
}
