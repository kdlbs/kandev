package executor

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

func workspaceInventoryReceiptHasMatchingAttestation(receipt *models.WorkspaceInventoryRecoveryReceipt) bool {
	return receipt.PostRepairVerifiedAt != nil && receipt.PostRepairMatched && receipt.PostRepairEvidence != nil &&
		samePreservationEvidence(
			preservationEvidenceFromModel(receipt.Preservation),
			preservationEvidenceFromModel(*receipt.PostRepairEvidence),
		)
}

// A completed repair attests historical content. Later branch changes require
// current Git ownership proof for the same checkout, without rewriting history.
func validateWorkspaceInventoryBranchContinuity(
	ctx context.Context,
	receipt *models.WorkspaceInventoryRecoveryReceipt,
	env *models.TaskEnvironment,
	info *repoInfo,
	row *models.TaskEnvironmentRepo,
) error {
	if receipt.Preservation.ObservedBranch == row.WorktreeBranch {
		return nil
	}
	if !workspaceInventoryReceiptHasMatchingAttestation(receipt) {
		return fmt.Errorf("%w: branch changed without completed repair attestation", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	if err := validateWorkspaceInventoryRepairCandidate(env, info, row); err != nil {
		return err
	}
	current, err := inspectWorkspaceInventoryCandidate(ctx, info, row)
	if err != nil {
		return fmt.Errorf("%w: cannot verify current checkout branch: %v", models.ErrWorkspaceInventoryRecoveryConflict, err)
	}
	if current.WorktreeID != receipt.Preservation.WorktreeID || current.PathHash != receipt.Preservation.PathHash {
		return fmt.Errorf("%w: current checkout identity differs from completed repair", models.ErrWorkspaceInventoryRecoveryConflict)
	}
	return nil
}
