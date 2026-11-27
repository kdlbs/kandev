package worktree

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/checkoutadoption"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryartifact"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

type workspaceAdoptionArchiveReader interface {
	GetWorkspaceAdoptionForArchive(context.Context, string) (*models.WorkspaceAdoptionReceipt, error)
}

type workspaceAdoptionCleanupCaptureReader interface {
	GetWorkspaceAdoptionForCleanupCapture(context.Context, string) (*models.WorkspaceAdoptionReceipt, error)
}

func (s *SQLiteStore) GetWorkspaceAdoptionForCleanupCapture(ctx context.Context, worktreeID string) (*models.WorkspaceAdoptionReceipt, error) {
	return checkoutadoption.ForWorktreeCleanupCapture(ctx, s.ro, worktreeID)
}

// Cleanup provenance survives the task/environment foreign-key cascade. It is
// read only from the current claimed job, never from caller-supplied worktrees.
func (s *SQLiteStore) GetWorkspaceAdoptionForArchive(ctx context.Context, worktreeID string) (*models.WorkspaceAdoptionReceipt, error) {
	job, cleanup := recoveryclaim.TaskCleanupJobFromContext(ctx)
	if !cleanup {
		return s.GetCurrentWorkspaceAdoption(ctx, worktreeID)
	}
	if job.Attempts < 0 {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	if job.Attempts == 0 {
		return s.workspaceAdoptionForPreparedReset(ctx, worktreeID, job)
	}
	var encoded string
	if err := s.ro.GetContext(ctx, &encoded, s.ro.Rebind(`SELECT resource_snapshot FROM task_resource_cleanup_jobs WHERE id=? AND task_id=? AND state='running' AND attempts=?`), job.ID, job.TaskID, job.Attempts); err != nil {
		return nil, err
	}
	var snapshot struct {
		Worktrees []*Worktree                                 `json:"worktrees"`
		Adoptions map[string]*models.WorkspaceAdoptionReceipt `json:"worktree_adoptions"`
	}
	if err := json.Unmarshal([]byte(encoded), &snapshot); err != nil {
		return nil, err
	}
	receipt := snapshot.Adoptions[worktreeID]
	if receipt == nil {
		return nil, nil
	}
	if receipt.Preview.Selection.Snapshot.EnvironmentOwnerTaskID != job.TaskID {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	for _, wt := range snapshot.Worktrees {
		if wt != nil && wt.ID == worktreeID && adoptionCleanupWorktreeMatches(wt, receipt) {
			if err := s.validateCleanupAdoptionBinding(ctx, worktreeID, receipt); err != nil {
				return nil, err
			}
			return receipt, nil
		}
	}
	return nil, models.ErrWorkspaceAdoptionStale
}

func (s *SQLiteStore) workspaceAdoptionForPreparedReset(ctx context.Context, worktreeID string, job recoveryclaim.TaskCleanupJob) (*models.WorkspaceAdoptionReceipt, error) {
	receipt, err := s.GetCurrentWorkspaceAdoption(ctx, worktreeID)
	if err != nil || receipt == nil {
		return receipt, err
	}
	if receipt.Preview.Selection.Snapshot.EnvironmentOwnerTaskID != job.TaskID {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	var count int
	err = s.ro.GetContext(ctx, &count, s.ro.Rebind(`SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE id=? AND task_id=? AND trigger='reconcile' AND state='prepared' AND attempts=0`), job.ID, job.TaskID)
	if err != nil {
		return nil, err
	}
	if count != 1 {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	return receipt, nil
}

func (s *SQLiteStore) validateCleanupAdoptionBinding(ctx context.Context, worktreeID string, receipt *models.WorkspaceAdoptionReceipt) error {
	current, err := s.GetWorktreeByID(ctx, worktreeID)
	if err != nil && !errors.Is(err, ErrWorktreeNotFound) {
		return err
	}
	if current == nil {
		return nil
	}
	if !adoptionCleanupWorktreeMatches(current, receipt) {
		return models.ErrWorkspaceAdoptionStale
	}
	var count int
	binding := receipt.Preview.Selection.Snapshot
	if err := s.ro.GetContext(ctx, &count, s.ro.Rebind(`SELECT COUNT(*) FROM task_environments WHERE id=? AND task_id=? AND ownership_generation=?`), binding.TaskEnvironmentID, binding.EnvironmentOwnerTaskID, binding.OwnershipGeneration); err != nil {
		return err
	}
	if count != 1 {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}

func adoptionCleanupWorktreeMatches(wt *Worktree, receipt *models.WorkspaceAdoptionReceipt) bool {
	p := receipt.Preview
	slot, err := p.Slot()
	return err == nil && wt.ID == slot.WorktreeID && wt.TaskID == p.Selection.Snapshot.EnvironmentOwnerTaskID &&
		wt.TaskEnvironmentID == p.Selection.Snapshot.TaskEnvironmentID && wt.RepositoryID == slot.RepositoryID &&
		wt.Path == p.Proof.CandidatePath && wt.Branch == p.Proof.Branch && wt.BranchSlug == slot.BranchSlug &&
		wt.RepositoryPath == slot.RepositoryLocalPath
}

// CaptureWorkspaceAdoptionsForCleanup retains backend-validated binding proof
// before task deletion removes the live receipt and inventory rows.
func (m *Manager) CaptureWorkspaceAdoptionsForCleanup(ctx context.Context, worktrees []*Worktree) (map[string]*models.WorkspaceAdoptionReceipt, error) {
	reader, ok := m.store.(workspaceAdoptionCleanupCaptureReader)
	if !ok {
		return nil, nil
	}
	receipts := make(map[string]*models.WorkspaceAdoptionReceipt)
	for _, wt := range worktrees {
		if wt == nil {
			continue
		}
		receipt, err := reader.GetWorkspaceAdoptionForCleanupCapture(ctx, wt.ID)
		if err != nil {
			return nil, err
		}
		if receipt != nil {
			if !adoptionCleanupWorktreeMatches(wt, receipt) {
				return nil, models.ErrWorkspaceAdoptionStale
			}
			receipts[wt.ID] = receipt
		}
	}
	return receipts, nil
}

func (m *Manager) cleanupAdoptionReceipt(ctx context.Context, wt *Worktree) (*models.WorkspaceAdoptionReceipt, error) {
	reader, ok := m.store.(workspaceAdoptionArchiveReader)
	if !ok {
		return nil, nil
	}
	receipt, err := reader.GetWorkspaceAdoptionForArchive(ctx, wt.ID)
	if err != nil || receipt == nil {
		return nil, err
	}
	if !adoptionCleanupWorktreeMatches(wt, receipt) {
		return nil, models.ErrWorkspaceAdoptionStale
	}
	return receipt, nil
}

func (m *Manager) openWorkspaceAdoptionForArchive(ctx context.Context, wt *Worktree) (*WorkspaceAdoptionInspection, error) {
	receipt, err := m.cleanupAdoptionReceipt(ctx, wt)
	if err != nil || receipt == nil {
		return nil, err
	}
	i, err := m.openWorkspaceAdoptionReceiptProof(ctx, receipt.Preview)
	if err != nil {
		return nil, err
	}
	if err := validateArchivePrimarySeparation(i, wt); err != nil {
		i.Close()
		return nil, err
	}
	return i, nil
}

func validateArchivePrimarySeparation(i *WorkspaceAdoptionInspection, wt *Worktree) error {
	source, err := storageworkspaces.PinnedDirectoryInfo(i.handles[i.source])
	if err == nil {
		var candidate os.FileInfo
		candidate, err = storageworkspaces.PinnedDirectoryInfo(i.handles[wt.Path])
		if err == nil && os.SameFile(source, candidate) {
			err = models.ErrWorkspaceAdoptionStale
		}
	}
	if err != nil || filepath.Clean(wt.Path) == filepath.Clean(wt.RepositoryPath) {
		if err == nil {
			err = models.ErrWorkspaceAdoptionStale
		}
		return err
	}
	registered := i.handles[wt.RepositoryPath]
	if registered == nil {
		registered, err = storageworkspaces.OpenDirectoryNoFollow(filepath.Dir(wt.RepositoryPath), wt.RepositoryPath)
		if err != nil {
			return err
		}
		i.handles[wt.RepositoryPath] = registered
		identity, ok := recoveryartifact.FilesystemIdentity(wt.RepositoryPath)
		if !ok {
			return models.ErrWorkspaceAdoptionStale
		}
		i.Proof.Identities[wt.RepositoryPath] = identity
	}
	project, err := storageworkspaces.PinnedDirectoryInfo(registered)
	candidate, candidateErr := storageworkspaces.PinnedDirectoryInfo(i.handles[wt.Path])
	if err != nil || candidateErr != nil || os.SameFile(project, candidate) {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}

func (m *Manager) completeAbsentAdoptionCleanup(ctx context.Context, wt *Worktree) error {
	receipt, err := m.cleanupAdoptionReceipt(ctx, wt)
	if err != nil {
		return err
	}
	if receipt == nil {
		return models.ErrWorkspaceAdoptionStale
	}
	present, err := cleanupPathPresent(wt.Path)
	if err != nil {
		return err
	}
	if present {
		return models.ErrWorkspaceAdoptionStale
	}
	return nil
}
