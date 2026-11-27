package executor

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
)

// ErrPreparedLaunchActorUnavailable means a launch that must run as the
// session's executor actor found no identity after admission. The launch is
// refused before any runtime or secret access instead of running unscoped.
var ErrPreparedLaunchActorUnavailable = errors.New(
	"prepared session start requires an executor actor, but none is available: " +
		"start the session as a user with access to the task, or restore the account that prepared it",
)

// requirePreparedLaunchActor enforces LaunchOptions.RequireExecutorActor. A
// synthetic identity (authentication disabled) satisfies it; an absent
// identity or one without a user does not.
func requirePreparedLaunchActor(ctx context.Context, opts LaunchOptions) error {
	if !opts.RequireExecutorActor {
		return nil
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || (!identity.Synthetic && identity.UserID == "") {
		return ErrPreparedLaunchActorUnavailable
	}
	return nil
}
