package repository

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// TurnChangesRepository owns immutable turn interval records and their
// compare-and-set admission and finalization boundaries.
type TurnChangesRepository interface {
	CreateTurnChangeSet(ctx context.Context, changeSet *models.TurnChangeSet) error
	GetTurnChangeSet(ctx context.Context, taskID, sessionID, changeSetID string) (*models.TurnChangeSet, error)
	ListTurnChangeSets(ctx context.Context, taskID, sessionID string, offset, limit int) ([]*models.TurnChangeSet, int, error)
	ListUnfinishedTurnChangeSets(ctx context.Context, limit int) ([]*models.TurnChangeSet, error)
	ListTurnRepositoryChanges(ctx context.Context, changeSetID string) ([]*models.TurnRepositoryChangeSet, error)
	GetTurnRepositoryChange(ctx context.Context, changeSetID, repositoryChangeID string) (*models.TurnRepositoryChangeSet, error)
	ListTurnFileChanges(ctx context.Context, changeSetID, repositoryChangeID string, offset, limit int) ([]*models.TurnFileChange, int, error)
	AcceptTurnChangeSetStart(ctx context.Context, changeSetID string, expectedRevision int64, repositories []models.TurnRepositoryChangeSet) (bool, error)
	AdvanceTurnChangeSetPromptGeneration(ctx context.Context, changeSetID, executionID, startupAttemptID, taskEnvironmentID string, expectedRevision, generation int64) (bool, error)
	ClaimTurnChangeSetTerminal(ctx context.Context, changeSetID string, expectedRevision int64, claim models.TurnChangeSetTerminalClaim) (bool, error)
	AcceptTurnRepositoryEnd(ctx context.Context, changeSetID, repositoryChangeID string, expectedStartCommitOID, expectedStartTreeOID string, end models.TurnRepositoryChangeSet) (bool, error)
	MarkTurnRepositoryCheckpointRefsCleaned(ctx context.Context, repositoryChangeID string) error
	UpdateTurnChangeSetOverlaps(ctx context.Context, changeSetID string, overlaps []models.TurnChangeOverlap) error
	FinalizeTurnChangeSet(ctx context.Context, changeSetID string, expectedRevision int64, finalization models.TurnChangeSetFinalization) (bool, error)
	StoreTurnChangeFiles(ctx context.Context, repositoryChangeID string, files []models.TurnChangeFileContent) error
	StoreTurnChangeFilesWithReceipt(ctx context.Context, repositoryChangeID string, files []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error)
	ReadTurnChangeContent(ctx context.Context, changeSetID, fileChangeID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error)
	SetTurnRepositoryContentStatus(ctx context.Context, repositoryChangeID string, complete bool, reason models.TurnChangeReason) error
	AcquireTurnChangeContentLease(ctx context.Context, changeSetID string, duration time.Duration) (*models.TurnChangeContentLease, error)
	ReleaseTurnChangeContentLease(ctx context.Context, leaseID string) error
	ApplyTurnChangeRetention(ctx context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error)
}
