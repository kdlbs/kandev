package changes

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

type terminalProgress struct {
	finalization       models.TurnChangeSetFinalization
	anyCompared        bool
	allSummaryComplete bool
	allContentComplete bool
	fileCount          int64
	binaryCount        int64
	unknownCount       int64
	contentBytes       int64
	added              int64
	deleted            int64
	addedKnown         bool
	deletedKnown       bool
}

func (c *Coordinator) Finish(ctx context.Context, terminal Terminal, client CheckpointClient) error {
	if c == nil || c.repository == nil {
		return errors.New("turn change coordinator is not configured")
	}
	if terminal.TaskID == "" || terminal.SessionID == "" || terminal.TurnID == "" || terminal.PromptGeneration == 0 {
		return nil
	}
	changeSet, terminal, done, err := c.claimTerminalInterval(terminal, client)
	if err != nil || done {
		return err
	}
	rows, err := c.loadTerminalRepositoryRows(ctx, changeSet, terminal, client)
	if err != nil {
		return err
	}
	progress := newTerminalProgress(changeSet, terminal, len(rows))
	for _, stored := range rows {
		row, comparison, contentBytes := c.finishTerminalRepository(ctx, changeSet, *stored, client)
		progress.add(row, comparison, contentBytes)
	}
	finalization := progress.result()
	if err := c.persistTerminalFinalization(changeSet, terminal, finalization, client); err != nil {
		return err
	}
	_ = c.cleanupRepositoryCheckpointRefs(finalization.Repositories, client)
	return nil
}

func (c *Coordinator) claimTerminalInterval(terminal Terminal, client CheckpointClient) (*models.TurnChangeSet, Terminal, bool, error) {
	changeSetID := turnChangeSetID(terminal.TurnID)
	changeSet, terminal, done, err := c.loadTerminalTurnChangeSet(terminal, changeSetID)
	if err != nil || done {
		return changeSet, terminal, done, err
	}
	if !sameTurnChangeTerminalOwner(changeSet, terminal.Admission) {
		terminal = terminalWithAcceptedOwner(terminal, changeSet)
		return nil, terminal, true, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCheckoutUnavailable, client)
	}
	claimed, latest, done, err := c.claimTerminalCapture(changeSet, terminal, client)
	if err != nil || done {
		return latest, terminal, done, err
	}
	if !claimed {
		return nil, terminal, true, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCaptureFailed, client)
	}
	changeSet, err = c.reloadTerminalCapture(terminal, changeSetID)
	if err != nil {
		return nil, terminal, false, err
	}
	terminal.At = *changeSet.TerminalCaptureStartedAt
	terminal.Outcome = changeSet.TerminalCaptureOutcome
	terminal.FinalAssistantMessageID = changeSet.TerminalCaptureFinalMessageID
	if !changeSet.CaptureEnabled || !changeSet.StartAccepted {
		return nil, terminal, true, c.finalizeUnavailable(context.Background(), changeSet, terminal, changeSet.Reason, client)
	}
	return changeSet, terminal, false, nil
}

func (c *Coordinator) loadTerminalTurnChangeSet(terminal Terminal, changeSetID string) (*models.TurnChangeSet, Terminal, bool, error) {
	readCtx, cancel := turnChangePersistenceContext()
	changeSet, err := c.repository.GetTurnChangeSet(readCtx, terminal.TaskID, terminal.SessionID, changeSetID)
	cancel()
	if errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
		return nil, terminal, true, nil
	}
	if err != nil {
		return nil, terminal, false, fmt.Errorf("load terminal turn change set: %w", err)
	}
	if changeSet.TerminalAt != nil {
		return changeSet, terminal, true, nil
	}
	if terminal.At.IsZero() {
		terminal.At = c.now().UTC()
	} else {
		terminal.At = terminal.At.UTC()
	}
	return changeSet, terminal, false, nil
}

func terminalWithAcceptedOwner(terminal Terminal, changeSet *models.TurnChangeSet) Terminal {
	terminal.Admission = Admission{
		TaskID: changeSet.TaskID, SessionID: changeSet.TaskSessionID,
		TaskEnvironmentID: changeSet.TaskEnvironmentID, TurnID: changeSet.TurnID,
		ExecutionID: changeSet.RuntimeExecutionID, StartupAttemptID: changeSet.StartupAttemptID,
		PromptGeneration: uint64(changeSet.PromptGeneration),
	}
	terminal.Outcome = "boundary_lost"
	return terminal
}

func (c *Coordinator) claimTerminalCapture(
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	client CheckpointClient,
) (bool, *models.TurnChangeSet, bool, error) {
	claim := models.TurnChangeSetTerminalClaim{
		At: terminal.At, ExecutionID: terminal.ExecutionID, StartupAttemptID: terminal.StartupAttemptID,
		PromptGeneration: int64(terminal.PromptGeneration), TaskEnvironmentID: terminal.TaskEnvironmentID,
		Outcome: terminal.Outcome, FinalAssistantMessageID: terminal.FinalAssistantMessageID,
	}
	claimCtx, cancel := turnChangePersistenceContext()
	claimed, err := c.repository.ClaimTurnChangeSetTerminal(claimCtx, changeSet.ID, changeSet.Revision, claim)
	cancel()
	if err != nil {
		return false, nil, true, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCaptureFailed, client)
	}
	if claimed {
		return true, changeSet, false, nil
	}
	return c.resolveExistingTerminalClaim(changeSet, terminal, claim, client)
}

func (c *Coordinator) resolveExistingTerminalClaim(
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	claim models.TurnChangeSetTerminalClaim,
	client CheckpointClient,
) (bool, *models.TurnChangeSet, bool, error) {
	latest, err := c.readTerminalTurnChangeSet(terminal)
	if err != nil {
		return false, nil, false, fmt.Errorf("read terminal turn change claim: %w", err)
	}
	if latest.TerminalAt != nil {
		return false, latest, true, nil
	}
	if !terminalChangeClaimMatches(latest, claim) {
		err := c.finalizeUnavailable(context.Background(), latest, terminal, models.TurnChangeReasonCheckoutUnavailable, client)
		return false, latest, true, err
	}
	return true, latest, false, nil
}

func (c *Coordinator) readTerminalTurnChangeSet(terminal Terminal) (*models.TurnChangeSet, error) {
	ctx, cancel := turnChangePersistenceContext()
	defer cancel()
	return c.repository.GetTurnChangeSet(ctx, terminal.TaskID, terminal.SessionID, turnChangeSetID(terminal.TurnID))
}

func (c *Coordinator) reloadTerminalCapture(terminal Terminal, changeSetID string) (*models.TurnChangeSet, error) {
	changeSet, err := c.readTerminalTurnChangeSet(terminal)
	if err != nil {
		return nil, fmt.Errorf("reload claimed terminal turn change set: %w", err)
	}
	if changeSet.ID != changeSetID || changeSet.TerminalCaptureStartedAt == nil {
		return nil, errors.New("claimed terminal turn change set has no durable capture timestamp")
	}
	return changeSet, nil
}

func (c *Coordinator) loadTerminalRepositoryRows(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	client CheckpointClient,
) ([]*models.TurnRepositoryChangeSet, error) {
	rowsCtx, cancel := turnChangePersistenceContext()
	rows, err := c.repository.ListTurnRepositoryChanges(rowsCtx, changeSet.ID)
	cancel()
	if err != nil {
		return nil, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCheckoutUnavailable, client)
	}
	var overlapErr error
	changeSet.OverlapIntervals, overlapErr = c.closeAdmittedOverlaps(changeSet.ID, terminal.At)
	if overlapErr != nil {
		c.logger.Warn("failed to close admitted turn-change overlap intervals",
			zap.String("change_set_id", changeSet.ID), zap.Error(overlapErr))
	}
	if client == nil {
		return nil, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCheckoutUnavailable, client)
	}
	scopes, scopeErr := client.TurnCheckpointRepositoryScopes(ctx)
	if scopeErr != nil || !sameTurnChangeCheckoutsWithScopes(rows, terminal.Checkouts, scopes) {
		return nil, c.finalizeUnavailable(context.Background(), changeSet, terminal, models.TurnChangeReasonCheckoutUnavailable, client)
	}
	return rows, nil
}

func newTerminalProgress(changeSet *models.TurnChangeSet, terminal Terminal, repositoryCount int) *terminalProgress {
	return &terminalProgress{
		finalization: models.TurnChangeSetFinalization{
			Availability: models.TurnChangeAvailabilityReady,
			TerminalAt:   terminal.At, TerminalOutcome: terminal.Outcome,
			FinalAssistantMessageID: terminal.FinalAssistantMessageID,
			RetainUntil:             timePtr(terminal.At.Add(DefaultContentRetention)),
			OverlapIntervals:        changeSet.OverlapIntervals,
			Repositories:            make([]models.TurnRepositoryChangeSet, 0, repositoryCount),
		},
		allSummaryComplete: repositoryCount > 0,
		allContentComplete: repositoryCount > 0,
		addedKnown:         true,
		deletedKnown:       true,
	}
}

func (p *terminalProgress) add(
	row models.TurnRepositoryChangeSet,
	comparison *turnchanges.CheckpointComparison,
	contentBytes int64,
) {
	if row.Availability != models.TurnChangeAvailabilityReady {
		p.allSummaryComplete, p.allContentComplete = false, false
	}
	if comparison != nil {
		p.addComparison(comparison)
		if !comparison.Complete {
			p.allSummaryComplete = false
		}
	}
	if !row.ContentComplete {
		p.allContentComplete = false
	}
	p.contentBytes += contentBytes
	p.finalization.Repositories = append(p.finalization.Repositories, row)
}

func (p *terminalProgress) addComparison(comparison *turnchanges.CheckpointComparison) {
	p.anyCompared = true
	p.fileCount += comparison.FileCount
	p.binaryCount += comparison.BinaryCount
	p.unknownCount += comparison.UnknownCount
	if comparison.AddedLines == nil {
		p.addedKnown = false
	} else {
		p.added += *comparison.AddedLines
	}
	if comparison.DeletedLines == nil {
		p.deletedKnown = false
	} else {
		p.deleted += *comparison.DeletedLines
	}
}

func (p *terminalProgress) result() models.TurnChangeSetFinalization {
	finalization := p.finalization
	if !p.anyCompared {
		finalization.Availability = models.TurnChangeAvailabilityUnavailable
		finalization.Reason = models.TurnChangeReasonCheckoutUnavailable
	}
	finalization.SummaryComplete = p.anyCompared && p.allSummaryComplete
	finalization.ContentComplete = p.anyCompared && p.allContentComplete
	finalization.Complete = finalization.SummaryComplete && finalization.ContentComplete
	finalization.FileCount, finalization.BinaryFileCount = p.fileCount, p.binaryCount
	finalization.UnknownCountFileCount = p.unknownCount
	finalization.RepositoryCount = int64(len(finalization.Repositories))
	finalization.ContentBytes = p.contentBytes
	if p.addedKnown {
		finalization.AddedLines = &p.added
	}
	if p.deletedKnown {
		finalization.DeletedLines = &p.deleted
	}
	if !finalization.Complete && finalization.Reason == "" {
		finalization.Reason = models.TurnChangeReasonContentUnavailable
	}
	return finalization
}

func (c *Coordinator) finishTerminalRepository(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, *turnchanges.CheckpointComparison, int64) {
	if row.Availability != models.TurnChangeAvailabilityPending || row.StartTreeOID == "" {
		return unavailableTurnRepository(row, models.TurnChangeReasonCheckoutUnavailable), nil, 0
	}
	if row.EndCommitOID == "" {
		var err error
		row, err = c.captureAndAcceptTurnRepositoryEnd(ctx, changeSet, row, client)
		if err != nil {
			return unavailableTurnRepository(row, checkpointReason(err, models.TurnChangeReasonCaptureFailed)), nil, 0
		}
	}
	comparison, err := client.CompareTurnCheckpoints(ctx, compareRequestForTurnRepository(changeSet.ID, row))
	if err != nil || !validComparison(comparison, changeSet.ID, row) {
		if err == nil {
			err = errors.New("comparison response did not match the accepted endpoint pair")
		}
		return unavailableTurnRepository(row, checkpointReason(err, turnchanges.ReasonComparisonFailed)), nil, 0
	}
	row.EnumerationComplete, row.ComparisonComplete = comparison.Complete, comparison.Complete
	row.Availability, row.Reason = models.TurnChangeAvailabilityReady, comparison.Reason
	if err := c.storeTerminalComparisonFiles(row, comparison.Files); err != nil {
		row.ContentComplete = false
		row.Reason = models.TurnChangeReasonContentUnavailable
		return row, comparison, 0
	}
	return c.exportTerminalRepositoryContent(ctx, changeSet, row, comparison, client)
}

func unavailableTurnRepository(row models.TurnRepositoryChangeSet, reason models.TurnChangeReason) models.TurnRepositoryChangeSet {
	row.Availability, row.Reason = models.TurnChangeAvailabilityUnavailable, reason
	row.ContentComplete, row.ComparisonComplete, row.EnumerationComplete = false, false, false
	return row
}

func (c *Coordinator) captureAndAcceptTurnRepositoryEnd(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, error) {
	end, err := client.CaptureTurnCheckpoint(ctx, turnchanges.CheckpointRequest{
		ChangeSetID: changeSet.ID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath, Boundary: turnchanges.CheckpointEnd,
	})
	if err != nil || !validCheckpoint(end, changeSet.ID, row.CheckoutID, turnchanges.CheckpointEnd) {
		if err == nil {
			err = errors.New("checkpoint response did not match the admitted end boundary")
		}
		return row, err
	}
	if end.HashAlgorithm != row.HashAlgorithm {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cleanupErr := client.DeleteTurnCheckpoint(cleanupCtx, turnchanges.CheckpointDeleteRequest{
			ChangeSetID: changeSet.ID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
			Boundary: turnchanges.CheckpointEnd, CommitOID: end.CommitOID,
		})
		cancel()
		return row, errors.Join(fmt.Errorf("capture end for checkout %q: hash algorithm changed from %q to %q", row.CheckoutID, row.HashAlgorithm, end.HashAlgorithm), cleanupErr)
	}
	row.EndCommitOID, row.EndTreeOID, row.EndReachabilityRef = end.CommitOID, end.TreeOID, end.ReachabilityRef
	row.EndCapturedAt = timePtr(end.CapturedAt.UTC())
	row.CleanupPending = row.CleanupPending || row.EndReachabilityRef != ""
	acceptCtx, cancel := turnChangePersistenceContext()
	accepted, err := c.repository.AcceptTurnRepositoryEnd(acceptCtx, changeSet.ID, row.ID, row.StartCommitOID, row.StartTreeOID, row)
	cancel()
	if err != nil {
		return row, err
	}
	if !accepted {
		return row, errors.New("end endpoint was not accepted for the stored start")
	}
	return row, nil
}

func compareRequestForTurnRepository(changeSetID string, row models.TurnRepositoryChangeSet) turnchanges.CompareRequest {
	return turnchanges.CompareRequest{
		ChangeSetID: changeSetID, CheckoutID: row.CheckoutID, Repo: row.RepositorySubpath,
		HashAlgorithm: row.HashAlgorithm, StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
	}
}

func (c *Coordinator) storeTerminalComparisonFiles(row models.TurnRepositoryChangeSet, files []turnchanges.CheckpointFile) error {
	summaries := make([]models.TurnChangeFileContent, 0, len(files))
	for _, file := range files {
		summaries = append(summaries, models.TurnChangeFileContent{File: checkpointFileSummary(row.ID, row.CheckoutID, file)})
	}
	ctx, cancel := turnChangePersistenceContext()
	defer cancel()
	return c.repository.StoreTurnChangeFiles(ctx, row.ID, summaries)
}

func (c *Coordinator) exportTerminalRepositoryContent(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	comparison *turnchanges.CheckpointComparison,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, *turnchanges.CheckpointComparison, int64) {
	if c.content == nil {
		row.ContentComplete, row.Reason = false, models.TurnChangeReasonContentUnavailable
		return row, comparison, 0
	}
	receipt, err := c.content.ExportAndStore(ctx, client, turnchanges.ExportRequest(compareRequestForTurnRepository(changeSet.ID, row)), row.ID)
	if err != nil {
		row.ContentComplete, row.Reason = false, models.TurnChangeReasonContentUnavailable
		return row, comparison, 0
	}
	row.ContentComplete = receipt.Complete
	if !receipt.Complete {
		row.Reason = receipt.Reason
	}
	return row, comparison, receipt.ExportBytes
}

func (c *Coordinator) persistTerminalFinalization(
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	finalization models.TurnChangeSetFinalization,
	client CheckpointClient,
) error {
	ctx, cancel := turnChangePersistenceContext()
	finalized, err := c.repository.FinalizeTurnChangeSet(ctx, changeSet.ID, changeSet.Revision, finalization)
	cancel()
	if err == nil && finalized {
		return nil
	}
	if err == nil {
		err = errors.New("terminal finalization lost its ownership revision")
	}
	latestCtx, latestCancel := turnChangePersistenceContext()
	latest, readErr := c.repository.GetTurnChangeSet(latestCtx, terminal.TaskID, terminal.SessionID, changeSet.ID)
	latestCancel()
	if readErr == nil && latest.TerminalAt != nil {
		return nil
	}
	if readErr == nil {
		if settleErr := c.finalizeUnavailable(context.Background(), latest, terminal, models.TurnChangeReasonContentUnavailable, client); settleErr != nil {
			return errors.Join(err, settleErr)
		}
		return nil
	}
	return fmt.Errorf("finalize turn change set: %w", errors.Join(err, readErr))
}
