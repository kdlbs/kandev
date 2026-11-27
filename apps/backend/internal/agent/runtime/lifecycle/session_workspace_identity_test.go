package lifecycle

import "testing"

func TestResumeChecksDirectoryAndNativeState(t *testing.T) {
	tests := []struct {
		name   string
		input  WorkspaceRestoreInput
		reason RestoreReason
	}{
		{
			name:   "same directory",
			input:  WorkspaceRestoreInput{OriginalAgentCWD: "/task", TargetAgentCWD: "/task", NativeStatePresent: true},
			reason: RestoreReasonNone,
		},
		{
			name:   "stable mount with changed host path",
			input:  WorkspaceRestoreInput{OriginalAgentCWD: "/task-host", TargetAgentCWD: "/task-runtime", NativeStatePresent: true, SupportsDirectoryMove: true},
			reason: RestoreReasonNone,
		},
		{
			name:   "missing state",
			input:  WorkspaceRestoreInput{OriginalAgentCWD: "/task", TargetAgentCWD: "/task", NativeStatePresent: false},
			reason: RestoreReasonNativeStateMissing,
		},
		{
			name:   "unsupported changed directory",
			input:  WorkspaceRestoreInput{OriginalAgentCWD: "/task", TargetAgentCWD: "/other", NativeStatePresent: true},
			reason: RestoreReasonWorkspaceIncompatible,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := EvaluateWorkspaceRestore(test.input).Reason; got != test.reason {
				t.Fatalf("reason = %q, want %q", got, test.reason)
			}
		})
	}
}

func TestBranchRecoveryNeverFallsBackToNewConversation(t *testing.T) {
	decision := DecideRestore(RestoreRequest{
		Failure: RestoreFailure{Reason: RestoreReasonNativeStateMissing},
		Action:  RestoreActionResumeNewBranch,
	})
	if decision.ActionAuthorized || decision.Outcome != RestoreOutcomeBlocked || decision.AllowsContextContinuation {
		t.Fatalf("branch recovery decision = %+v, want native-only blocked decision", decision)
	}
}
