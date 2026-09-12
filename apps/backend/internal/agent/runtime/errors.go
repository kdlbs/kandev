package runtime

import "github.com/kandev/kandev/internal/agent/runtime/lifecycle"

// BootstrapFailure is the safe, operation-boundary error produced when an
// agent execution fails before it becomes ready.
type BootstrapFailure = lifecycle.BootstrapFailure

// RepositoryPreparationError identifies the repository whose preparation
// prevented a multi-repository launch.
type RepositoryPreparationError = lifecycle.RepositoryPreparationError
