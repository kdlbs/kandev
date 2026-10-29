package orchestrator

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// Authorization is performed by loadSessionForRecovery before this branch.
func (s *Service) recoverInventoryMetadataOnly(ctx context.Context, taskID, sessionID, action, key string) (*LaunchSessionResponse, error) {
	if action != recoveryActionRepairWorkspaceInventory || strings.TrimSpace(key) == "" || s.executor == nil {
		return nil, models.ErrWorkspaceInventoryRecoveryInvalid
	}
	receipt, err := s.executor.RepairWorkspaceInventoryOnly(ctx, taskID, sessionID, strings.TrimSpace(key))
	if err != nil {
		return nil, err
	}
	return &LaunchSessionResponse{Success: true, TaskID: taskID, SessionID: sessionID, State: "workspace_repaired",
		WorkspaceInventoryRecoveryReceipt: receipt, ActivationDisposition: "not_started", ActivationReason: "metadata_only"}, nil
}
