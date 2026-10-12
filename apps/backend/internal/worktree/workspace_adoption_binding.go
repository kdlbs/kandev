package worktree

import (
	"context"
	"github.com/kandev/kandev/internal/task/checkoutadoption"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"sync"
)

func (s *SQLiteStore) GetCurrentWorkspaceAdoption(ctx context.Context, worktreeID string) (*models.WorkspaceAdoptionReceipt, error) {
	return checkoutadoption.ForWorktree(ctx, s.ro, worktreeID)
}

type workspaceAdoptionReader interface {
	GetCurrentWorkspaceAdoption(context.Context, string) (*models.WorkspaceAdoptionReceipt, error)
}

func (m *Manager) validateWorkspaceAdoptionBinding(ctx context.Context, wt *Worktree) (bool, error) {
	reader, ok := m.store.(workspaceAdoptionReader)
	if !ok {
		return false, nil
	}
	receipt, err := reader.GetCurrentWorkspaceAdoption(ctx, wt.ID)
	if err != nil {
		return false, err
	}
	if receipt == nil {
		return false, nil
	}
	p := receipt.Preview
	slot, err := p.Slot()
	if err != nil {
		return false, err
	}
	if wt.Path != p.Proof.CandidatePath || wt.Branch != p.Proof.Branch || wt.RepositoryID != slot.RepositoryID || wt.TaskID != p.Selection.Snapshot.EnvironmentOwnerTaskID || wt.TaskEnvironmentID != p.Selection.Snapshot.TaskEnvironmentID {
		return false, models.ErrWorkspaceAdoptionStale
	}
	if err := m.ValidateWorkspaceAdoptionProof(ctx, p); err != nil {
		return false, err
	}
	return true, nil
}

// InvalidateWorkspaceAdoptionCache discards all aliases of the published logical slot.
// The durable claim remains held by the service until this returns.
func (m *Manager) InvalidateWorkspaceAdoptionCache(worktreeID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, wt := range m.worktrees {
		if wt.ID == worktreeID {
			delete(m.worktrees, key)
		}
	}
}

// TryLockWorkspaceAdoption prevents simultaneous same-operation borrowers in
// this manager. Durable claims and publication CAS protect independent writers.
func (m *Manager) TryLockWorkspaceAdoption(ctx context.Context, environmentID string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, _ := m.recoveryLocks.LoadOrStore("workspace-adoption:"+environmentID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	if !lock.TryLock() {
		return nil, recoveryclaim.ErrBusy
	}
	return lock.Unlock, nil
}
