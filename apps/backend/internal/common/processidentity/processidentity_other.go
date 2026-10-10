//go:build !linux && !darwin && !windows

package processidentity

import (
	"context"
	"time"
)

func Capture(int) (Identity, error) { return Identity{}, ErrUnverifiableIdentity }

func Inspect(Identity) (State, error) { return StateUnknown, ErrUnverifiableIdentity }

func TerminateOwnedSession(context.Context, Identity, time.Duration) error {
	return ErrUnverifiableIdentity
}

// OwnedSessionTerminated fails closed when descendant ownership cannot be inspected.
func OwnedSessionTerminated(Identity) (bool, error) { return false, ErrUnverifiableIdentity }
