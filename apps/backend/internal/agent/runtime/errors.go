package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// BootstrapFailure is the safe, operation-boundary error produced when an
// agent execution fails before it becomes ready.
type BootstrapFailure = lifecycle.BootstrapFailure

// RepositoryPreparationError identifies the repository whose preparation
// prevented a multi-repository launch.
type RepositoryPreparationError = lifecycle.RepositoryPreparationError

// ErrCancelEscalated reports that cancellation released local admission after
// the provider failed to acknowledge the cancellation within its bound.
var ErrCancelEscalated = lifecycle.ErrCancelEscalated

// ErrVirtualProfile marks a logical routing profile that has no concrete execution profile.
var ErrVirtualProfile = lifecycle.ErrVirtualProfile
