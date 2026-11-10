package executor

import "context"

// ExecutorTypeCapabilities provides behavioral queries about executor types.
// Implemented by the lifecycle manager using its backend registry.
type ExecutorTypeCapabilities interface {
	RequiresCloneURL(executorType string) bool
	ShouldApplyPreferredShell(executorType string) bool
	// ValidateExecutorRuntime reports whether the concrete selection's mapped
	// backend is registered. It must fail closed for every nonempty selection.
	ValidateExecutorRuntime(executorType string) error
}

// ExecutorExecutionRuntimeAdmission is implemented by the lifecycle manager to
// compare a retained execution's recorded runtime with the admitted selection.
// It is read-only and never migrates, stops, or replaces a worker.
type ExecutorExecutionRuntimeAdmission interface {
	ValidateExecutionRuntime(ctx context.Context, sessionID, taskEnvironmentID, executorType string) error
}
