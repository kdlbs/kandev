package orchestrator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func sessionRecoveryBlockResolutionGuidance(
	ctx context.Context,
	store sessionRecoveryBlockStore,
	sessionID, incarnationID string,
	generation int64,
	attempted *models.SessionRecoveryBlock,
) error {
	if attempted == nil || !supportsHistoryContinuationReason(attempted.Reason) {
		return incompleteSessionRecoveryResolution()
	}
	current, err := store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
	if err != nil || current == nil || !sameSessionRecoveryBlockIncident(attempted, current) {
		return incompleteSessionRecoveryResolution()
	}
	return &sessionRecoveryRequiredError{Block: current}
}

func supportsHistoryContinuationReason(reason string) bool {
	switch reason {
	case "native_state_missing", "native_resume_unsupported", "workspace_incompatible",
		durableDeliveryUnresolvedReason, unknownPromptOutcome:
		return true
	default:
		return false
	}
}

func sameSessionRecoveryBlockIncident(attempted, current *models.SessionRecoveryBlock) bool {
	return current.State == models.RecoveryBlockOpen &&
		current.ID == attempted.ID &&
		current.SessionID == attempted.SessionID &&
		current.IncarnationID == attempted.IncarnationID &&
		current.ExpectedGeneration == attempted.ExpectedGeneration &&
		current.Reason == attempted.Reason
}

func incompleteSessionRecoveryResolution() error {
	return fmt.Errorf("resolve session recovery block: block changed or recovery evidence is incomplete")
}
