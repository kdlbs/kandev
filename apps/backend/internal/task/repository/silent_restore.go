package repository

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

var ErrSilentRestoreAttemptConflict = repoerrors.ErrSilentRestoreAttemptConflict

// SilentRestoreRepository is the optional persistence capability used by the
// bounded startup worker. It stays separate from the general task repository
// because most task consumers do not perform restart recovery.
type SilentRestoreRepository interface {
	ListSilentRestoreCandidates(ctx context.Context, afterSessionID string, limit int) ([]models.SilentRestoreCandidate, error)
	GetRestoreAttempt(ctx context.Context, id string) (*models.RestoreAttempt, error)
	EnsureRestoreAttempt(ctx context.Context, attempt *models.RestoreAttempt) (*models.RestoreAttempt, bool, error)
	CompareAndSwapSilentRestoreCheckpoint(ctx context.Context, id string, expected, next models.SilentRestoreCheckpoint) (bool, error)
	CommitSilentRestore(ctx context.Context, commit *models.SilentRestoreCommit) (bool, error)
}
