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
	GetTurnChangeSetByID(ctx context.Context, changeSetID string) (*models.TurnChangeSet, error)
	ListTurnChangeSets(ctx context.Context, taskID, sessionID string, offset, limit int) ([]*models.TurnChangeSet, int, error)
	ListUnfinishedTurnChangeSets(ctx context.Context, limit int) ([]*models.TurnChangeSet, error)
	ListUnfinishedTurnChangeSetsAfter(ctx context.Context, afterID string, limit int) ([]*models.TurnChangeSet, error)
	ListResumableTurnChangeSets(ctx context.Context, afterID string, limit int) ([]*models.TurnChangeSet, error)
	ListTurnRepositoryChanges(ctx context.Context, changeSetID string) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryChangesForSets(ctx context.Context, changeSetIDs []string) (map[string][]*models.TurnRepositoryChangeSet, error)
	GetTurnRepositoryChange(ctx context.Context, changeSetID, repositoryChangeID string) (*models.TurnRepositoryChangeSet, error)
	ListTurnFileChanges(ctx context.Context, changeSetID, repositoryChangeID string, offset, limit int) ([]*models.TurnFileChange, int, error)
	AcceptTurnChangeSetStart(ctx context.Context, changeSetID string, expectedRevision int64, repositories []models.TurnRepositoryChangeSet) (bool, error)
	AdvanceTurnChangeSetPromptGeneration(ctx context.Context, changeSetID, executionID, startupAttemptID, taskEnvironmentID string, expectedRevision, generation int64) (bool, error)
	ClaimTurnChangeSetTerminal(ctx context.Context, changeSetID string, expectedRevision int64, claim models.TurnChangeSetTerminalClaim) (bool, error)
	AcceptTurnRepositoryEnd(ctx context.Context, changeSetID, repositoryChangeID string, expectedStartCommitOID, expectedStartTreeOID string, end models.TurnRepositoryChangeSet) (bool, error)
	SetTurnRepositoryEndUnavailable(ctx context.Context, changeSetID, repositoryChangeID, startCommitOID, startTreeOID string, reason models.TurnChangeReason) (bool, error)
	MarkTurnRepositoryCheckpointRefsCleaned(ctx context.Context, repositoryChangeID string) error
	UpdateTurnRepositoryPreservation(ctx context.Context, repositoryChangeID string, source models.TurnChangeContentSource, state models.TurnChangePreservation, reason models.TurnChangeReason, attempts, bytes int64, receipt string, contentComplete bool) error
	MarkTurnChangeFilesGitBacked(ctx context.Context, repositoryChangeID string) (bool, error)
	GetTurnFileContentSource(ctx context.Context, changeSetID, fileChangeID string) (*models.TurnFileContentSource, error)
	ListTurnRepositoryPreservationRows(ctx context.Context, states []models.TurnChangePreservation, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnPreservationJobs(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	SetTurnRepositoryPreservationDeadline(ctx context.Context, repositoryChangeID string, deadline time.Time) error
	UpdateTurnChangeSetContentStatus(ctx context.Context, changeSetID string, contentComplete bool, reason models.TurnChangeReason) error
	TaskPreservationBytes(ctx context.Context, changeSetID string) (int64, error)
	ListTurnHistoryRetentionCandidates(ctx context.Context, cutoff, now time.Time, limit int) ([]*models.TurnHistoryRetentionCandidate, error)
	ListTurnHistoryRetentionCandidatesPage(ctx context.Context, cutoff, now time.Time, afterTaskID string, limit int) ([]*models.TurnHistoryRetentionCandidate, string, error)
	ExpireTurnHistory(ctx context.Context, changeSetIDs []string, cutoff time.Time, reason models.TurnChangeReason, now time.Time) (models.TurnHistoryExpiryResult, error)
	ListExpiredTurnHistoryRepositories(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryPreservationReady(ctx context.Context, limit int) ([]*models.TurnRepositoryChangeSet, error)
	ListTurnRepositoryPreservationReadyPage(ctx context.Context, afterID string, limit int) ([]*models.TurnRepositoryChangeSet, string, error)
	ClearTurnRepositoryPreservationReceipt(ctx context.Context, repositoryChangeID string) error
	UpdateTurnChangeSetOverlaps(ctx context.Context, changeSetID string, overlaps []models.TurnChangeOverlap) error
	UpdateTurnChangeSetOverlapsIfRevision(ctx context.Context, changeSetID string, expectedRevision int64, overlaps []models.TurnChangeOverlap) (bool, error)
	AcceptTurnRepositoryComparison(ctx context.Context, changeSetID, repositoryChangeID string, comparison models.TurnRepositoryComparison) (bool, error)
	MarkTurnRepositoryComparisonReady(ctx context.Context, changeSetID, repositoryChangeID string, complete bool, reason models.TurnChangeReason) (bool, error)
	ListTurnChangeWriterIntervals(ctx context.Context, identity models.TurnChangeCheckoutIdentity, after string, limit int) ([]models.TurnChangeWriterInterval, error)
	BeginTurnChangeExportAttempt(ctx context.Context, repositoryChangeID string, now time.Time, lease time.Duration, maxAttempts int64) (*models.TurnChangeExportClaim, bool, error)
	StoreTurnChangeExportPage(ctx context.Context, claim models.TurnChangeExportClaim, files []models.TurnChangeFileContent, page models.TurnChangeExportPage) (models.TurnChangeContentStoreReceipt, bool, error)
	ReleaseTurnChangeExportClaim(ctx context.Context, claim models.TurnChangeExportClaim) error
	SettleTurnChangeContentProgress(ctx context.Context, changeSetID string) error
	SettleTurnRepositoryExportFailure(ctx context.Context, repositoryChangeID string, claimToken int64, reason models.TurnChangeReason) error
	ListTurnChangeExportCandidates(ctx context.Context, now time.Time, limit int) ([]*models.TurnRepositoryChangeSet, error)
	FinalizeTurnChangeSet(ctx context.Context, changeSetID string, expectedRevision int64, finalization models.TurnChangeSetFinalization) (bool, error)
	StoreTurnChangeFiles(ctx context.Context, repositoryChangeID string, files []models.TurnChangeFileContent) error
	StoreTurnChangeFilesWithReceipt(ctx context.Context, repositoryChangeID string, files []models.TurnChangeFileContent) (models.TurnChangeContentStoreReceipt, error)
	ReadTurnChangeContent(ctx context.Context, changeSetID, fileChangeID string, variant models.TurnChangeContentVariant) (*models.TurnChangeContentPayload, error)
	SetTurnRepositoryContentStatus(ctx context.Context, repositoryChangeID string, complete bool, reason models.TurnChangeReason) error
	AcquireTurnChangeContentLease(ctx context.Context, changeSetID string, duration time.Duration) (*models.TurnChangeContentLease, error)
	ReleaseTurnChangeContentLease(ctx context.Context, leaseID string) error
	ApplyTurnChangeRetention(ctx context.Context, policy models.TurnChangeRetentionPolicy, now time.Time) (models.TurnChangeRetentionResult, error)
}
