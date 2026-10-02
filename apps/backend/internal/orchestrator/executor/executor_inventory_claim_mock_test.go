package executor

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

func (m *mockRepository) AcquireTaskEnvironmentRecoveryClaim(_ context.Context, req models.TaskEnvironmentRecoveryClaimRequest) (*models.TaskEnvironmentRecoveryClaim, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inventoryClaims == nil {
		m.inventoryClaims = make(map[string]*models.TaskEnvironmentRecoveryClaim)
	}
	if m.inventoryClaims[req.TaskEnvironmentID] != nil {
		return nil, recoveryclaim.ErrBusy
	}
	for _, session := range m.sessions {
		if session.ID == req.SessionID || session.TaskEnvironmentID != req.TaskEnvironmentID {
			continue
		}
		if session.State != models.TaskSessionStateCompleted && session.State != models.TaskSessionStateFailed && session.State != models.TaskSessionStateCancelled {
			return nil, recoveryclaim.ErrBusy
		}
		if runner := m.executorsRunning[session.ID]; runner != nil && executorRunningIsLiveWriter(runner.Status) {
			return nil, recoveryclaim.ErrBusy
		}
	}
	claim := &models.TaskEnvironmentRecoveryClaim{TaskEnvironmentID: req.TaskEnvironmentID, OwnerTaskID: req.OwnerTaskID,
		OwnershipGeneration: req.OwnershipGeneration, SessionID: req.SessionID, OperationID: req.OperationID, ExecutorType: req.ExecutorType}
	m.inventoryClaims[req.TaskEnvironmentID] = claim
	return claim, nil
}
func (m *mockRepository) ReleaseTaskEnvironmentRecoveryClaim(_ context.Context, claim *models.TaskEnvironmentRecoveryClaim) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.inventoryClaims[claim.TaskEnvironmentID] != claim {
		return recoveryclaim.ErrClaimMismatch
	}
	delete(m.inventoryClaims, claim.TaskEnvironmentID)
	return nil
}
