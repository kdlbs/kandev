//go:build !linux && !darwin && !windows

package processidentity

import (
	"context"
	"time"
)

func Capture(int) (Identity, error) { return Identity{}, ErrUnverifiableIdentity }

func Inspect(Identity) (State, error) { return StateUnknown, ErrUnverifiableIdentity }

// InspectProcStat fails closed where no procfs handle exists.
func InspectProcStat(Identity, string, []byte) (State, error) {
	return StateUnknown, ErrUnverifiableIdentity
}

func TerminateOwnedSession(context.Context, Identity, time.Duration) error {
	return ErrUnverifiableIdentity
}

// BirthBootMatches fails closed where no kernel boot identity is available.
func BirthBootMatches(Identity) (bool, error) { return false, ErrUnverifiableIdentity }

// SessionMembersRemain fails closed when session membership cannot be inspected.
func SessionMembersRemain(Identity) (bool, error) { return false, ErrUnverifiableIdentity }

// OwnedSessionTerminated fails closed when descendant ownership cannot be inspected.
func OwnedSessionTerminated(Identity) (bool, error) { return false, ErrUnverifiableIdentity }

func OwnedGroupTerminated(Identity) (bool, error) { return false, ErrUnverifiableIdentity }
