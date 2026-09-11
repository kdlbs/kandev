package lifecycle

import "strings"

// WorkspaceRestoreInput contains the two identities that must be checked
// before native restore. Host paths can change while an executor mount stays
// stable, so the agent-visible path is the policy input.
type WorkspaceRestoreInput struct {
	OriginalAgentCWD      string
	TargetAgentCWD        string
	NativeStatePresent    bool
	SupportsDirectoryMove bool
}

// EvaluateWorkspaceRestore returns a typed failure only when the workspace or
// native state prevents a safe native restore.
func EvaluateWorkspaceRestore(input WorkspaceRestoreInput) RestoreFailure {
	if !input.NativeStatePresent {
		return RestoreFailure{Reason: RestoreReasonNativeStateMissing}
	}
	if strings.TrimSpace(input.OriginalAgentCWD) == strings.TrimSpace(input.TargetAgentCWD) ||
		input.SupportsDirectoryMove {
		return RestoreFailure{Reason: RestoreReasonNone}
	}
	return RestoreFailure{Reason: RestoreReasonWorkspaceIncompatible}
}
