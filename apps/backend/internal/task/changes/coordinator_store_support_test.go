package changes

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (s *coordinatorStore) UpdateTurnChangeSetOverlapsIfRevision(
	ctx context.Context,
	changeSetID string,
	expectedRevision int64,
	overlaps []models.TurnChangeOverlap,
) (bool, error) {
	changeSet := s.changeSets[changeSetID]
	if changeSet == nil {
		return false, nil
	}
	if s.overlapCASConflicts > 0 {
		s.overlapCASConflicts--
		changeSet.Revision++
		return false, nil
	}
	if expectedRevision > 0 && changeSet.Revision != expectedRevision {
		return false, nil
	}
	if err := s.UpdateTurnChangeSetOverlaps(ctx, changeSetID, overlaps); err != nil {
		return false, err
	}
	return true, nil
}

func (s *coordinatorStore) AcceptTurnRepositoryComparison(
	_ context.Context,
	changeSetID, repositoryChangeID string,
	comparison models.TurnRepositoryComparison,
) (bool, error) {
	for _, row := range s.repositoryRows[changeSetID] {
		if row.ID != repositoryChangeID {
			continue
		}
		if row.ComparisonBasis == "" {
			row.ComparisonVersion = comparison.Version
			row.ComparisonBasis = comparison.Basis
			row.ComparisonReason = comparison.Reason
			row.EffectiveStartCommitOID = comparison.EffectiveStartCommitOID
			row.EffectiveStartTreeOID = comparison.EffectiveStartTreeOID
			row.EffectiveEndCommitOID = comparison.EffectiveEndCommitOID
			row.EffectiveEndTreeOID = comparison.EffectiveEndTreeOID
			row.ComparisonRef = comparison.ComparisonRef
			return true, nil
		}
		return row.ComparisonBasis == comparison.Basis &&
			row.EffectiveStartTreeOID == comparison.EffectiveStartTreeOID &&
			row.EffectiveEndTreeOID == comparison.EffectiveEndTreeOID, nil
	}
	return false, nil
}

func (s *coordinatorStore) ListTurnChangeWriterIntervals(
	_ context.Context,
	identity models.TurnChangeCheckoutIdentity,
	after string,
	limit int,
) ([]models.TurnChangeWriterInterval, error) {
	if s.writerIntervalErr != nil {
		return nil, s.writerIntervalErr
	}
	intervals := make([]models.TurnChangeWriterInterval, 0)
	for _, changeSet := range s.changeSets {
		for _, row := range s.repositoryRows[changeSet.ID] {
			if row.StartCapturedAt == nil || !storeCheckoutIdentityMatches(identity, row) {
				continue
			}
			overlaps, _ := json.Marshal(changeSet.OverlapIntervals)
			intervals = append(intervals, models.TurnChangeWriterInterval{
				RepositoryChangeID: row.ID, ChangeSetID: changeSet.ID, CheckoutID: row.CheckoutID,
				EnvironmentRepoID: row.TaskEnvironmentRepoID, WorktreeID: row.WorktreeID,
				StartCapturedAt: *row.StartCapturedAt, TerminalCaptureAt: changeSet.TerminalCaptureStartedAt,
				TurnCompletedAt: s.completedAtByTurn[changeSet.TurnID],
				FinalizedAt:     changeSet.TerminalAt, Revision: changeSet.Revision,
				OverlapIntervalsJSON: string(overlaps),
			})
		}
	}
	sort.Slice(intervals, func(left, right int) bool {
		leftKey := intervals[left].ChangeSetID + "\x00" + intervals[left].CheckoutID
		rightKey := intervals[right].ChangeSetID + "\x00" + intervals[right].CheckoutID
		return leftKey < rightKey
	})
	result := make([]models.TurnChangeWriterInterval, 0, len(intervals))
	for _, interval := range intervals {
		key := interval.ChangeSetID + "\x00" + interval.CheckoutID
		if key <= after {
			continue
		}
		result = append(result, interval)
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result, nil
}

func storeCheckoutIdentityMatches(identity models.TurnChangeCheckoutIdentity, row *models.TurnRepositoryChangeSet) bool {
	if identity.WorktreeID != "" && row.WorktreeID != "" {
		return identity.WorktreeID == row.WorktreeID
	}
	if identity.EnvironmentRepoID != "" && row.TaskEnvironmentRepoID != "" {
		return identity.EnvironmentRepoID == row.TaskEnvironmentRepoID
	}
	return identity.CheckoutID != "" && identity.CheckoutID == row.CheckoutID
}

func (s *coordinatorStore) BeginTurnChangeExportAttempt(
	_ context.Context,
	repositoryChangeID string,
	now time.Time,
	lease time.Duration,
	maxAttempts int64,
) (*models.TurnChangeExportClaim, bool, error) {
	for _, rows := range s.repositoryRows {
		for _, row := range rows {
			if row.ID != repositoryChangeID {
				continue
			}
			if row.ExportAttempts >= maxAttempts || row.ContentComplete ||
				(row.ExportState != "" && row.ExportState != models.TurnChangeExportQueued &&
					row.ExportState != models.TurnChangeExportRetrying) {
				return nil, false, nil
			}
			row.ExportState = models.TurnChangeExportRunning
			row.ExportClaimToken++
			row.ExportAttempts++
			return &models.TurnChangeExportClaim{
				RepositoryChangeID: row.ID, ChangeSetID: row.TurnChangeSetID, CheckoutID: row.CheckoutID,
				Token: row.ExportClaimToken, Attempt: row.ExportAttempts, Cursor: row.ExportCursor,
				ExportBytes: row.ExportBytes, Fingerprint: row.ExportFingerprint, ComparisonVersion: row.ComparisonVersion,
				ComparisonBasis: row.ComparisonBasis, StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
				EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
				EffectiveStartCommitOID: row.EffectiveStartCommitOID, EffectiveStartTreeOID: row.EffectiveStartTreeOID,
				EffectiveEndCommitOID: row.EffectiveEndCommitOID, EffectiveEndTreeOID: row.EffectiveEndTreeOID,
				ComparisonRef: row.ComparisonRef, LeaseUntil: now.Add(lease),
			}, true, nil
		}
	}
	return nil, false, nil
}

func (s *coordinatorStore) StoreTurnChangeExportPage(
	ctx context.Context,
	claim models.TurnChangeExportClaim,
	files []models.TurnChangeFileContent,
	page models.TurnChangeExportPage,
) (models.TurnChangeContentStoreReceipt, bool, error) {
	if s.forceExportClaimLost {
		return models.TurnChangeContentStoreReceipt{}, false, nil
	}
	if s.exportStoreDelay > 0 {
		timer := time.NewTimer(s.exportStoreDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return models.TurnChangeContentStoreReceipt{}, false, ctx.Err()
		case <-timer.C:
		}
	}
	for _, rows := range s.repositoryRows {
		for _, row := range rows {
			if row.ID != claim.RepositoryChangeID || row.ExportClaimToken != claim.Token ||
				row.ExportState != models.TurnChangeExportRunning {
				continue
			}
			if err := s.StoreTurnChangeFiles(ctx, row.ID, files); err != nil {
				return models.TurnChangeContentStoreReceipt{}, false, err
			}
			receipt := s.turnChangeContentReceipt(row.ID)
			row.ExportCursor = page.NextCursor
			row.ExportBytes += receipt.StoredBytes
			row.ExportFingerprint = page.Fingerprint
			row.ExportReason = page.Reason
			switch {
			case page.Failed || (!page.Complete && !receipt.Pending && receipt.Reason != ""):
				row.ExportState, row.ContentComplete = models.TurnChangeExportUnavailable, false
			case page.Retry:
				row.ExportState = models.TurnChangeExportRetrying
				row.ExportClaimOwner, row.ExportClaimExpiresAt = "", nil
			case page.Complete && receipt.Complete:
				row.ExportState, row.ContentComplete = models.TurnChangeExportComplete, true
				row.ExportClaimOwner, row.ExportClaimExpiresAt = "", nil
			case page.KeepClaim:
				row.ExportState = models.TurnChangeExportRunning
			default:
				row.ExportState = models.TurnChangeExportQueued
				row.ExportClaimOwner, row.ExportClaimExpiresAt = "", nil
			}
			return receipt, true, nil
		}
	}
	return models.TurnChangeContentStoreReceipt{}, false, nil
}

func (s *coordinatorStore) turnChangeContentReceipt(repositoryChangeID string) models.TurnChangeContentStoreReceipt {
	pending, failed, storedBytes := 0, 0, int64(0)
	for _, file := range s.files {
		if file.File.RepositoryChangeID != repositoryChangeID {
			continue
		}
		if file.File.ContentAvailability == models.TurnChangeAvailabilityPending {
			pending++
			continue
		}
		if file.File.ContentAvailability != models.TurnChangeAvailabilityReady || file.File.ContentTruncated {
			failed++
			continue
		}
		storedBytes += int64(len(file.CanonicalPatch) + len(file.FilteredPatch) + len(file.OldRendering) + len(file.NewRendering))
	}
	return models.TurnChangeContentStoreReceipt{
		StoredBytes: storedBytes, Pending: pending > 0, Complete: pending == 0 && failed == 0,
	}
}

func (s *coordinatorStore) ReleaseTurnChangeExportClaim(
	_ context.Context,
	claim models.TurnChangeExportClaim,
) error {
	for _, rows := range s.repositoryRows {
		for _, row := range rows {
			if row.ID == claim.RepositoryChangeID && row.ExportClaimToken == claim.Token {
				row.ExportState = models.TurnChangeExportRetrying
			}
		}
	}
	return nil
}

func (s *coordinatorStore) SettleTurnChangeContentProgress(_ context.Context, changeSetID string) error {
	changeSet := s.changeSets[changeSetID]
	if changeSet == nil {
		return nil
	}
	complete := true
	for _, row := range s.repositoryRows[changeSetID] {
		if row.Availability != models.TurnChangeAvailabilityReady || !row.ContentComplete {
			complete = false
		}
	}
	changeSet.ContentComplete = complete
	changeSet.Complete = changeSet.SummaryComplete && complete
	changeSet.Revision++
	return nil
}

func (s *coordinatorStore) SettleTurnRepositoryExportFailure(
	_ context.Context,
	repositoryChangeID string,
	claimToken int64,
	reason models.TurnChangeReason,
) error {
	for _, rows := range s.repositoryRows {
		for _, row := range rows {
			if row.ID == repositoryChangeID && row.ExportClaimToken == claimToken && row.ExportState != models.TurnChangeExportComplete {
				row.ExportState, row.ExportReason, row.ContentComplete = models.TurnChangeExportUnavailable, reason, false
			}
		}
	}
	for index := range s.files {
		file := &s.files[index].File
		if file.RepositoryChangeID == repositoryChangeID && file.ContentAvailability == models.TurnChangeAvailabilityPending {
			file.ContentAvailability, file.ContentReason = models.TurnChangeAvailabilityUnavailable, reason
		}
	}
	return nil
}

func (s *coordinatorStore) ListResumableTurnChangeSets(
	_ context.Context,
	afterID string,
	limit int,
) ([]*models.TurnChangeSet, error) {
	ids := make([]string, 0, len(s.changeSets))
	for id := range s.changeSets {
		if id > afterID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	sets := make([]*models.TurnChangeSet, 0, limit)
	for _, id := range ids {
		changeSet := s.changeSets[id]
		if changeSet == nil || changeSet.TerminalAt != nil {
			continue
		}
		resumable := false
		for _, row := range s.repositoryRows[id] {
			if row.EndCommitOID != "" {
				resumable = true
			}
		}
		if !resumable {
			continue
		}
		sets = append(sets, changeSet)
		if len(sets) >= limit {
			break
		}
	}
	return sets, nil
}

func (s *coordinatorStore) ListTurnChangeExportCandidates(
	_ context.Context,
	_ time.Time,
	_ int,
) ([]*models.TurnRepositoryChangeSet, error) {
	return nil, nil
}

var _ = turnchanges.ExportBatchVersion1

func (s *coordinatorStore) GetTurnRepositoryChange(
	_ context.Context,
	changeSetID, repositoryChangeID string,
) (*models.TurnRepositoryChangeSet, error) {
	for _, row := range s.repositoryRows[changeSetID] {
		if row.ID == repositoryChangeID {
			copy := *row
			return &copy, nil
		}
	}
	return nil, nil
}

func (s *coordinatorStore) ListUnfinishedTurnChangeSetsAfter(
	_ context.Context,
	afterID string,
	limit int,
) ([]*models.TurnChangeSet, error) {
	ids := make([]string, 0, len(s.changeSets))
	for id := range s.changeSets {
		if id > afterID {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	sets := make([]*models.TurnChangeSet, 0, limit)
	for _, id := range ids {
		changeSet := s.changeSets[id]
		if changeSet == nil || changeSet.TerminalAt != nil {
			continue
		}
		sets = append(sets, changeSet)
		if len(sets) >= limit {
			break
		}
	}
	return sets, nil
}

func (s *coordinatorStore) GetTurnChangeSetByID(_ context.Context, id string) (*models.TurnChangeSet, error) {
	changeSet := s.changeSets[id]
	if changeSet == nil {
		return nil, repoerrors.ErrTurnChangeSetNotFound
	}
	copy := *changeSet
	return &copy, nil
}

func (s *coordinatorStore) MarkTurnRepositoryComparisonReady(
	_ context.Context,
	changeSetID, repositoryChangeID string,
	complete bool,
	reason models.TurnChangeReason,
) (bool, error) {
	for _, row := range s.repositoryRows[changeSetID] {
		if row.ID != repositoryChangeID || row.ComparisonBasis == "" || row.EndCommitOID == "" {
			continue
		}
		row.Availability = models.TurnChangeAvailabilityReady
		row.Reason = reason
		row.EnumerationComplete, row.ComparisonComplete = complete, complete
		row.ExportState = models.TurnChangeExportQueued
		return true, nil
	}
	return false, nil
}

func (s *coordinatorStore) ListTurnFileChanges(
	_ context.Context,
	changeSetID, repositoryChangeID string,
	offset, limit int,
) ([]*models.TurnFileChange, int, error) {
	files := make([]*models.TurnFileChange, 0)
	for index := range s.files {
		file := s.files[index].File
		if file.RepositoryChangeID == repositoryChangeID {
			copy := file
			files = append(files, &copy)
		}
	}
	total := len(files)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return files[offset:end], total, nil
}
