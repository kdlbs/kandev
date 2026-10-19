package controller

import (
	"context"
	"errors"
)

// ErrAccountBlockReleaseUnavailable means the runtime that holds dynamic
// routing circuits is not wired, so no hold can be read or released.
var ErrAccountBlockReleaseUnavailable = errors.New("account-block release is unavailable")

// AccountBlockReleaser closes the hold that a provider account restriction
// places on one execution profile. The runtime owns the circuits, so the
// settings controller depends on this narrow seam rather than on the engine.
type AccountBlockReleaser interface {
	ReleaseAccountBlock(ctx context.Context, executionProfileID string) (bool, error)
}

// SetAccountBlockReleaser injects the runtime release seam.
func (c *Controller) SetAccountBlockReleaser(releaser AccountBlockReleaser) {
	c.accountBlockReleaser = releaser
}

// ReleaseAccountBlock is the owner's release of an account-blocked execution
// profile. It reports whether a hold was closed; a profile without a hold is a
// successful no-op. It changes no profile setting, credential, or candidate
// order, and it is the only path that lifts the hold.
func (c *Controller) ReleaseAccountBlock(ctx context.Context, profileID string) (bool, error) {
	profile, err := c.repo.GetAgentProfile(ctx, profileID)
	if err != nil {
		if isProfileNotFoundErr(err) {
			return false, ErrAgentProfileNotFound
		}
		return false, err
	}
	// Office-scoped profiles belong to the workspace-scoped office surface;
	// 404 keeps their existence hidden here, as the other profile routes do.
	if profile == nil || profile.WorkspaceID != "" {
		return false, ErrAgentProfileNotFound
	}
	if c.accountBlockReleaser == nil {
		return false, ErrAccountBlockReleaseUnavailable
	}
	return c.accountBlockReleaser.ReleaseAccountBlock(ctx, profile.ID)
}
