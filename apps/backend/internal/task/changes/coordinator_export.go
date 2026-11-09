package changes

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
)

func (c *Coordinator) preserveTerminalRepositoryContent(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, error) {
	archiveClient, archiveOK := client.(ArchiveClient)
	if !archiveOK || c.content == nil || !c.content.HistoryEnabled() {
		return c.preserveTerminalRepositoryExport(ctx, changeSet, row, client)
	}
	if c.exportLimiter != nil {
		if !c.exportLimiter.Acquire(ctx) {
			return row, ctx.Err()
		}
		defer c.exportLimiter.Release()
	}
	return c.preserveHistoricalRepositoryContent(ctx, changeSet, row, archiveClient)
}

func (c *Coordinator) preserveTerminalRepositoryExport(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, error) {
	if client == nil || c.content == nil {
		return c.failTurnRepositoryExport(ctx, row, row.ExportClaimToken, models.TurnChangeReasonContentUnavailable)
	}
	if c.exportLimiter != nil {
		if !c.exportLimiter.Acquire(ctx) {
			return row, ctx.Err()
		}
		defer c.exportLimiter.Release()
	}
	lease := c.acquirePreservationLease(ctx, changeSet.ID)
	if lease != nil {
		defer c.releasePreservationLease(ctx, lease)
	}
	lastReason := models.TurnChangeReasonExportTimeout
	lastToken := row.ExportClaimToken
	for attempt := 0; attempt < turnChangeExportAttempts; attempt++ {
		if ctx.Err() != nil {
			lastReason = models.TurnChangeReasonExportInterrupted
			break
		}
		claim, ok, err := c.beginTurnChangeExportAttempt(ctx, row.ID)
		if err != nil {
			return row, err
		}
		if !ok {
			return c.resolveSkippedExportAttempt(ctx, changeSet.ID, row)
		}
		lastToken = claim.Token
		attemptCtx, cancel := context.WithTimeout(ctx, turnChangeExportAttemptTime)
		outcome := c.runTurnChangeExportAttempt(attemptCtx, changeSet, row, *claim, client)
		cancel()
		if outcome.done {
			return c.reloadTurnRepositoryRow(ctx, changeSet.ID, row.ID)
		}
		if outcome.reason != "" {
			lastReason = outcome.reason
		}
		if !outcome.retry {
			break
		}
		if err := waitTurnChangeExportRetry(ctx, attempt); err != nil {
			lastReason = models.TurnChangeReasonExportInterrupted
			break
		}
	}
	return c.failTurnRepositoryExport(ctx, row, lastToken, lastReason)
}

type turnChangeExportAttemptOutcome struct {
	done   bool
	retry  bool
	reason models.TurnChangeReason
}

func (c *Coordinator) runTurnChangeExportAttempt(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	claim models.TurnChangeExportClaim,
	client CheckpointClient,
) turnChangeExportAttemptOutcome {
	for pages := 0; pages < turnChangeExportMaxPages; pages++ {
		if ctx.Err() != nil {
			_ = c.recordTurnChangeExportRetry(ctx, claim, models.TurnChangeReasonExportTimeout)
			return turnChangeExportAttemptOutcome{retry: true, reason: models.TurnChangeReasonExportTimeout}
		}
		export, err := c.exportTurnRepositoryPage(ctx, changeSet, row, claim, client)
		if err != nil {
			reason := turnChangeExportFailureReason(err)
			_ = c.recordTurnChangeExportRetry(ctx, claim, reason)
			return turnChangeExportAttemptOutcome{retry: turnChangeExportRetryable(err), reason: reason}
		}
		files, receipt := prepareTurnChangeExportFiles(export, row.CheckoutID, row.ID)
		page := turnChangeExportPageFor(export, receipt, claim)
		page.KeepClaim = !page.Complete && !page.Failed && !page.Retry
		writeCtx, cancel := turnChangePersistenceContextWith(ctx)
		storedReceipt, stored, storeErr := c.repository.StoreTurnChangeExportPage(writeCtx, claim, files, page)
		cancel()
		if storeErr != nil {
			return turnChangeExportAttemptOutcome{retry: false, reason: turnChangeExportFailureReason(storeErr)}
		}
		if !stored {
			return turnChangeExportAttemptOutcome{retry: false, reason: models.TurnChangeReasonExportInterrupted}
		}
		claim.Cursor = page.NextCursor
		claim.ExportBytes += storedReceipt.StoredBytes
		claim.Fingerprint = page.Fingerprint
		if page.Complete {
			if !storedReceipt.Complete {
				return turnChangeExportAttemptOutcome{retry: false, reason: turnChangePageFailureReason(page, storedReceipt)}
			}
			return turnChangeExportAttemptOutcome{done: true}
		}
		if page.Failed {
			return turnChangeExportAttemptOutcome{retry: false, reason: page.Reason}
		}
		if page.Retry {
			return turnChangeExportAttemptOutcome{retry: true, reason: page.Reason}
		}
	}
	return turnChangeExportAttemptOutcome{retry: false, reason: models.TurnChangeReasonSizeLimit}
}

func (c *Coordinator) acquirePreservationLease(ctx context.Context, changeSetID string) *models.TurnChangeContentLease {
	leaseCtx, cancel := context.WithTimeout(ctx, turnChangePersistenceTimeout)
	defer cancel()
	lease, err := c.repository.AcquireTurnChangeContentLease(leaseCtx, changeSetID, 5*time.Minute)
	if err != nil {
		return nil
	}
	return lease
}

func (c *Coordinator) releasePreservationLease(ctx context.Context, lease *models.TurnChangeContentLease) {
	if lease == nil {
		return
	}
	releaseCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	_ = c.repository.ReleaseTurnChangeContentLease(releaseCtx, lease.ID)
}

func (c *Coordinator) beginTurnChangeExportAttempt(
	ctx context.Context,
	repositoryChangeID string,
) (*models.TurnChangeExportClaim, bool, error) {
	claimCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	return c.repository.BeginTurnChangeExportAttempt(claimCtx, repositoryChangeID, c.now().UTC(), turnChangeExportLease, turnChangeExportAttempts)
}

func (c *Coordinator) recordTurnChangeExportRetry(
	ctx context.Context,
	claim models.TurnChangeExportClaim,
	reason models.TurnChangeReason,
) error {
	page := models.TurnChangeExportPage{
		NextCursor: claim.Cursor, Retry: true, Reason: reason,
		Fingerprint: claim.Fingerprint, NextAttemptAt: timePtr(c.now().UTC()),
	}
	writeCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	_, stored, err := c.repository.StoreTurnChangeExportPage(writeCtx, claim, nil, page)
	if err != nil || !stored {
		releaseCtx, releaseCancel := turnChangePersistenceContextWith(ctx)
		defer releaseCancel()
		return c.repository.ReleaseTurnChangeExportClaim(releaseCtx, claim)
	}
	return nil
}

func turnChangePageFailureReason(page models.TurnChangeExportPage, receipt models.TurnChangeContentStoreReceipt) models.TurnChangeReason {
	if page.Reason != "" {
		return page.Reason
	}
	if receipt.Reason != "" {
		return receipt.Reason
	}
	return models.TurnChangeReasonExportInterrupted
}

func turnChangeExportPageFor(
	export *turnchanges.CheckpointExport,
	receipt ExportReceipt,
	claim models.TurnChangeExportClaim,
) models.TurnChangeExportPage {
	page := models.TurnChangeExportPage{
		NextCursor: export.NextCursor, Complete: export.Complete, BatchBytes: export.ExportBytes,
		Fingerprint: export.InventoryFingerprint, TotalFiles: export.TotalFiles,
	}
	if export.ExportBatchVersion != turnchanges.ExportBatchVersion1 {
		page.NextCursor = claim.Cursor + int64(len(export.Files))
		page.Complete = export.Complete
		page.Fingerprint = ""
	}
	if !page.Complete && export.Reason != "" {
		switch export.Reason {
		case turnchanges.ReasonExportTimeout, turnchanges.ReasonContentUnavailable:
			page.Retry = true
			page.Reason = export.Reason
			page.NextAttemptAt = timePtr(time.Now().UTC())
		default:
			page.Failed = true
			page.Reason = export.Reason
		}
	}
	if !page.Complete && !page.Failed && page.NextCursor <= claim.Cursor {
		page.Retry = true
		page.Reason = models.TurnChangeReasonExportTimeout
		page.NextAttemptAt = timePtr(time.Now().UTC())
	}
	_ = receipt
	return page
}

func (c *Coordinator) failTurnRepositoryExport(
	ctx context.Context,
	row models.TurnRepositoryChangeSet,
	claimToken int64,
	reason models.TurnChangeReason,
) (models.TurnRepositoryChangeSet, error) {
	if reason == "" || !reason.Valid() {
		reason = models.TurnChangeReasonContentUnavailable
	}
	writeCtx, cancel := turnChangePersistenceContextWith(ctx)
	err := c.repository.SettleTurnRepositoryExportFailure(writeCtx, row.ID, claimToken, reason)
	cancel()
	if err != nil {
		return row, err
	}
	row.ExportState, row.ExportReason, row.ContentComplete = models.TurnChangeExportUnavailable, reason, false
	return row, nil
}

func (c *Coordinator) reloadTurnRepositoryRow(
	ctx context.Context,
	changeSetID, repositoryChangeID string,
) (models.TurnRepositoryChangeSet, error) {
	readCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	row, err := c.repository.GetTurnRepositoryChange(readCtx, changeSetID, repositoryChangeID)
	if err != nil {
		return models.TurnRepositoryChangeSet{}, err
	}
	return *row, nil
}

func (c *Coordinator) exportTurnRepositoryPage(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	claim models.TurnChangeExportClaim,
	client CheckpointClient,
) (*turnchanges.CheckpointExport, error) {
	request := exportRequestForTurnRepository(changeSet.ID, row, claim)
	pageCtx, cancel := context.WithTimeout(ctx, turnChangeExportPageCeiling)
	defer cancel()
	export, err := client.ExportTurnCheckpoint(pageCtx, request)
	if err != nil {
		return nil, err
	}
	if !matchesTurnChangeExportEndpoints(export, request) {
		return nil, turnChangeComparisonError{reason: models.TurnChangeReasonHistoryContextChanged}
	}
	if err := validateTurnChangeExportPage(export, request, row); err != nil {
		return nil, err
	}
	return export, nil
}

type turnChangeComparisonError struct{ reason models.TurnChangeReason }

func (e turnChangeComparisonError) Error() string { return string(e.reason) }

func validateTurnChangeExportPage(
	export *turnchanges.CheckpointExport,
	request turnchanges.ExportRequest,
	row models.TurnRepositoryChangeSet,
) error {
	if export == nil {
		return turnChangeComparisonError{reason: models.TurnChangeReasonContentUnavailable}
	}
	if len(export.Files) > turnchanges.MaxExportBatchEntries && export.ExportBatchVersion == turnchanges.ExportBatchVersion1 {
		return turnChangeComparisonError{reason: models.TurnChangeReasonEntryLimit}
	}
	if request.BudgetBytes > 0 && export.ExportBytes > request.BudgetBytes {
		return turnChangeComparisonError{reason: models.TurnChangeReasonSizeLimit}
	}
	if request.InventoryFingerprint != "" && export.InventoryFingerprint != "" &&
		export.InventoryFingerprint != request.InventoryFingerprint {
		return turnChangeComparisonError{reason: models.TurnChangeReasonHistoryContextChanged}
	}
	if request.ComparisonVersion != turnchanges.ComparisonVersion2 {
		if export.ComparisonVersion == turnchanges.ComparisonVersion2 {
			return turnChangeComparisonError{reason: models.TurnChangeReasonHistoryContextChanged}
		}
		return nil
	}
	if export.ComparisonVersion != turnchanges.ComparisonVersion2 {
		return turnChangeComparisonError{reason: models.TurnChangeReasonHistoryContextChanged}
	}
	if export.ComparisonBasis != row.ComparisonBasis ||
		export.EffectiveStartTreeOID != row.EffectiveStartTreeOID ||
		export.EffectiveEndTreeOID != row.EffectiveEndTreeOID {
		return turnChangeComparisonError{reason: models.TurnChangeReasonHistoryContextChanged}
	}
	return nil
}

func exportRequestForTurnRepository(
	changeSetID string,
	row models.TurnRepositoryChangeSet,
	claim models.TurnChangeExportClaim,
) turnchanges.ExportRequest {
	version := turnchanges.ComparisonVersionLegacy
	if row.ComparisonVersion == int64(turnchanges.ComparisonVersion2) {
		version = turnchanges.ComparisonVersion2
	}
	budget := int64(maxTurnExportContentBytes) - claim.ExportBytes
	if budget < 0 {
		budget = 0
	}
	return turnchanges.ExportRequest{
		ChangeSetID: changeSetID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
		HashAlgorithm: row.HashAlgorithm, StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
		ComparisonVersion: version, ComparisonBasis: row.ComparisonBasis,
		EffectiveStartCommitOID: row.EffectiveStartCommitOID, EffectiveStartTreeOID: row.EffectiveStartTreeOID,
		EffectiveEndCommitOID: row.EffectiveEndCommitOID, EffectiveEndTreeOID: row.EffectiveEndTreeOID,
		ComparisonRef: row.ComparisonRef, ExportBatchVersion: turnchanges.ExportBatchVersion1,
		Cursor: claim.Cursor, MaxEntries: turnchanges.MaxExportBatchEntries,
		InventoryFingerprint: claim.Fingerprint, BudgetBytes: budget,
	}
}

func turnChangeExportFailureReason(err error) models.TurnChangeReason {
	var comparisonErr turnChangeComparisonError
	if errors.As(err, &comparisonErr) {
		return comparisonErr.reason
	}
	var checkpointErr interface{ CaptureReason() turnchanges.ReasonCode }
	if errors.As(err, &checkpointErr) {
		reason := checkpointErr.CaptureReason()
		if reason.Valid() && reason != "" {
			return reason
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return models.TurnChangeReasonExportTimeout
	}
	if errors.Is(err, context.Canceled) {
		return models.TurnChangeReasonExportInterrupted
	}
	return models.TurnChangeReasonExportTimeout
}

func turnChangeExportRetryable(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var comparisonErr turnChangeComparisonError
	if errors.As(err, &comparisonErr) {
		return comparisonErr.reason == models.TurnChangeReasonExportTimeout
	}
	var checkpointErr interface{ CaptureReason() turnchanges.ReasonCode }
	if errors.As(err, &checkpointErr) {
		reason := checkpointErr.CaptureReason()
		return reason == turnchanges.ReasonExportTimeout || reason == turnchanges.ReasonContentUnavailable
	}
	return true
}

func waitTurnChangeExportRetry(ctx context.Context, attempt int) error {
	delay := 75 * time.Millisecond
	if attempt > 0 {
		delay = 150 * time.Millisecond
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Coordinator) resolveSkippedExportAttempt(
	ctx context.Context,
	changeSetID string,
	row models.TurnRepositoryChangeSet,
) (models.TurnRepositoryChangeSet, error) {
	latest, err := c.reloadTurnRepositoryRow(ctx, changeSetID, row.ID)
	if err != nil {
		return row, err
	}
	if latest.ContentComplete || latest.ExportState == models.TurnChangeExportComplete ||
		latest.ExportState == models.TurnChangeExportUnavailable {
		return latest, nil
	}
	now := c.now().UTC()
	if latest.ExportState == models.TurnChangeExportRunning &&
		latest.ExportClaimExpiresAt != nil && latest.ExportClaimExpiresAt.After(now) {
		return latest, nil
	}
	if latest.ExportNextAttemptAt != nil && latest.ExportNextAttemptAt.After(now) {
		return latest, nil
	}
	return c.failTurnRepositoryExport(ctx, latest, latest.ExportClaimToken, models.TurnChangeReasonExportTimeout)
}
