package orchestrator

import "context"

type silentRestoreStreamRecoverer interface {
	RecoverSilentRestoreCandidateStream(context.Context, string, string, string) error
}
