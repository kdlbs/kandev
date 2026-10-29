package changes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"go.uber.org/zap"
)

type terminalProgress struct {
	finalization       models.TurnChangeSetFinalization
	anyCompared        bool
	anyAvailable       bool
	unavailableReason  models.TurnChangeReason
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

const (
	turnChangeExportAttempts    = 3
	turnChangeExportMaxPages    = 2000
	turnChangeExportAttemptTime = 20 * time.Second
	turnChangeExportLease       = 45 * time.Second
)

func (c *Coordinator) Finish(ctx context.Context, terminal Terminal, client CheckpointClient) error {
	if err := c.CaptureTerminalEndpoints(ctx, terminal, client); err != nil {
		return err
	}
	return c.ProcessTerminal(ctx, terminal, client)
}

// CaptureTerminalEndpoints durably claims a turn's terminal identity and
// accepts each end checkpoint before any comparison or content export begins.
// Callers may release the prompt-generation fence after this method succeeds.
func (c *Coordinator) CaptureTerminalEndpoints(ctx context.Context, terminal Terminal, client CheckpointClient) error {
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
	return c.captureAndAcceptTerminalRepositoryEndpoints(ctx, changeSet, terminal, rows, client)
}

func (c *Coordinator) captureAndAcceptTerminalRepositoryEndpoints(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	rows []*models.TurnRepositoryChangeSet,
	client CheckpointClient,
) error {
	if len(rows) == 0 {
		return nil
	}
	scopes, scopeErr := client.TurnCheckpointRepositoryScopes(ctx)
	for _, stored := range rows {
		row := *stored
		if row.Availability != models.TurnChangeAvailabilityPending || row.StartTreeOID == "" ||
			(row.EndCommitOID != "" && row.EndTreeOID != "") {
			continue
		}
		reason := models.TurnChangeReasonCheckoutUnavailable
		if scopeErr == nil && sameTurnChangeCheckoutWithScopes(&row, terminal.Checkouts, scopes) {
			if _, captureErr := c.captureAndAcceptTurnRepositoryEnd(ctx, changeSet, row, client); captureErr == nil {
				continue
			} else {
				reason = checkpointReason(captureErr, models.TurnChangeReasonCaptureFailed)
			}
		}
		if err := c.settleUnavailableTurnRepositoryEnd(changeSet.ID, row, reason); err != nil {
			return err
		}
	}
	return nil
}

// ProcessTerminal compares and exports only endpoints already accepted by
// CaptureTerminalEndpoints. It never asks an executor to create a new end.
func (c *Coordinator) ProcessTerminal(ctx context.Context, terminal Terminal, client CheckpointClient) error {
	if err := c.ProcessTerminalComparison(ctx, terminal, client); err != nil {
		return err
	}
	return c.ProcessTerminalContent(ctx, terminal, client)
}

// ProcessTerminalComparison accepts the comparison and summary while content
// preservation may still be pending.
func (c *Coordinator) ProcessTerminalComparison(ctx context.Context, terminal Terminal, client CheckpointClient) error {
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
		if stored.EndCommitOID == "" || stored.EndTreeOID == "" {
			reason := stored.Reason
			if reason == "" {
				reason = models.TurnChangeReasonCaptureFailed
			}
			progress.add(unavailableTurnRepository(*stored, reason), nil)
			continue
		}
		row, comparison := c.compareTerminalRepository(ctx, changeSet, *stored, client)
		progress.add(row, comparison)
	}
	finalization := progress.summaryResult()
	return c.persistTerminalFinalization(changeSet, terminal, finalization, client)
}

// ProcessTerminalContent preserves acknowledged content in bounded pages and
// settles completeness from every stored file.
func (c *Coordinator) ProcessTerminalContent(ctx context.Context, terminal Terminal, client CheckpointClient) error {
	if turnChangeOutcomeIsStop(terminal.Outcome) {
		stopCtx, stopCancel := context.WithTimeout(ctx, 2*time.Second)
		defer stopCancel()
		ctx = stopCtx
	}
	changeSet, err := c.readTerminalTurnChangeSetWith(ctx, terminal)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTurnChangeSetNotFound) {
			return nil
		}
		return err
	}
	if changeSet.TerminalAt == nil {
		return nil
	}
	rows, err := c.repository.ListTurnRepositoryChanges(ctx, changeSet.ID)
	if err != nil {
		return err
	}
	var failures []error
	for _, row := range rows {
		if row == nil || row.Availability != models.TurnChangeAvailabilityReady ||
			row.EndCommitOID == "" || row.ContentComplete {
			continue
		}
		if _, err := c.preserveTerminalRepositoryContent(ctx, changeSet, *row, client); err != nil {
			failures = append(failures, err)
		}
	}
	if err := c.settleContentProgress(ctx, changeSet.ID); err != nil {
		failures = append(failures, err)
	}
	_ = c.cleanupSettledCheckpointRefs(ctx, changeSet.ID, client)
	return errors.Join(failures...)
}

func turnChangeOutcomeIsStop(outcome string) bool {
	lower := strings.ToLower(outcome)
	return strings.Contains(lower, "stop") || strings.Contains(lower, "cancel")
}

func (c *Coordinator) settleContentProgress(ctx context.Context, changeSetID string) error {
	settleCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	return c.repository.SettleTurnChangeContentProgress(settleCtx, changeSetID)
}

func (c *Coordinator) cleanupSettledCheckpointRefs(ctx context.Context, changeSetID string, client CheckpointClient) error {
	readCtx, cancel := turnChangePersistenceContextWith(ctx)
	rows, err := c.repository.ListTurnRepositoryChanges(readCtx, changeSetID)
	cancel()
	if err != nil {
		return err
	}
	settled := make([]models.TurnRepositoryChangeSet, 0, len(rows))
	for _, row := range rows {
		if row == nil || !row.CleanupPending {
			continue
		}
		if row.ExportState != models.TurnChangeExportComplete && row.ExportState != models.TurnChangeExportUnavailable {
			continue
		}
		settled = append(settled, *row)
	}
	return c.cleanupRepositoryCheckpointRefs(ctx, settled, client)
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
	return c.readTerminalTurnChangeSetWith(context.Background(), terminal)
}

func (c *Coordinator) readTerminalTurnChangeSetWith(ctx context.Context, terminal Terminal) (*models.TurnChangeSet, error) {
	readCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	return c.repository.GetTurnChangeSet(readCtx, terminal.TaskID, terminal.SessionID, turnChangeSetID(terminal.TurnID))
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
	if overlapErr := c.reconcileWriterOverlaps(ctx, changeSet); overlapErr != nil {
		c.logger.Warn("failed to reconcile turn-change overlap intervals",
			zap.String("change_set_id", changeSet.ID), zap.Error(overlapErr))
	} else if latest, readErr := c.readTerminalTurnChangeSet(terminal); readErr == nil {
		changeSet.OverlapIntervals = latest.OverlapIntervals
		changeSet.Revision = latest.Revision
	}
	if client == nil {
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

func (p *terminalProgress) add(row models.TurnRepositoryChangeSet, comparison *turnchanges.CheckpointComparison) {
	if row.Availability == models.TurnChangeAvailabilityReady {
		p.anyAvailable = true
	} else {
		p.allSummaryComplete, p.allContentComplete = false, false
		if p.unavailableReason == "" {
			p.unavailableReason = turnChangeUnavailableReason(row)
		}
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

// summaryResult publishes comparison availability before content finishes.
// Content pending is not a failure and adds no reason.
func (p *terminalProgress) summaryResult() models.TurnChangeSetFinalization {
	finalization := p.finalization
	if !p.anyCompared {
		finalization.Availability = models.TurnChangeAvailabilityUnavailable
		finalization.Reason = p.unavailableReason
		if finalization.Reason == "" {
			finalization.Reason = models.TurnChangeReasonCheckoutUnavailable
		}
	} else if !p.anyAvailable {
		finalization.Availability = models.TurnChangeAvailabilityUnavailable
		finalization.Reason = p.unavailableReason
		if finalization.Reason == "" {
			finalization.Reason = models.TurnChangeReasonHistoryProvenanceMissing
		}
	}
	finalization.SummaryComplete = p.anyCompared && p.allSummaryComplete
	finalization.ContentComplete = false
	finalization.Complete = false
	finalization.FileCount, finalization.BinaryFileCount = p.fileCount, p.binaryCount
	finalization.UnknownCountFileCount = p.unknownCount
	finalization.RepositoryCount = int64(len(finalization.Repositories))
	if p.addedKnown {
		finalization.AddedLines = &p.added
	}
	if p.deletedKnown {
		finalization.DeletedLines = &p.deleted
	}
	if !finalization.SummaryComplete && finalization.Reason == "" {
		finalization.Reason = models.TurnChangeReasonContentUnavailable
	}
	return finalization
}

func turnChangeEffectiveEndMatchesRow(expectation models.TurnRepositoryComparison, row models.TurnRepositoryChangeSet) bool {
	if expectation.Version != int64(turnchanges.ComparisonVersion2) ||
		expectation.Basis == turnchanges.ComparisonHistoryUnavailable {
		return true
	}
	return expectation.EffectiveEndCommitOID == row.EndCommitOID &&
		expectation.EffectiveEndTreeOID == row.EndTreeOID
}

func turnChangeUnavailableReason(row models.TurnRepositoryChangeSet) models.TurnChangeReason {
	if row.ComparisonReason != "" {
		return row.ComparisonReason
	}
	return row.Reason
}

func (c *Coordinator) compareTerminalRepository(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) (models.TurnRepositoryChangeSet, *turnchanges.CheckpointComparison) {
	if row.Availability != models.TurnChangeAvailabilityPending || row.StartTreeOID == "" ||
		row.EndCommitOID == "" || row.EndTreeOID == "" {
		return unavailableTurnRepository(row, models.TurnChangeReasonCheckoutUnavailable), nil
	}
	request := compareRequestForTurnRepository(changeSet.ID, row)
	request.ComparisonVersion = turnChangeComparisonVersion(row)
	comparison, err := client.CompareTurnCheckpoints(ctx, request)
	if err != nil || !validComparison(comparison, changeSet.ID, row) {
		if err == nil {
			err = errors.New("comparison response did not match the accepted endpoint pair")
		}
		return unavailableTurnRepository(row, checkpointReason(err, turnchanges.ReasonComparisonFailed)), nil
	}
	expectation, comparisonReason, ok := comparisonExpectationForRow(comparison, row, request.ComparisonVersion)
	if !ok || !turnChangeEffectiveEndMatchesRow(expectation, row) {
		return unavailableTurnRepository(row, models.TurnChangeReasonHistoryContextChanged), nil
	}
	accepted, err := c.acceptTurnRepositoryComparison(ctx, changeSet.ID, row, expectation)
	if err != nil || !accepted {
		return unavailableTurnRepository(row, models.TurnChangeReasonHistoryContextChanged), nil
	}
	return c.storeAcceptedTerminalComparison(ctx, changeSet, row, comparison, expectation, comparisonReason)
}

func comparisonExpectationForRow(
	comparison *turnchanges.CheckpointComparison,
	row models.TurnRepositoryChangeSet,
	requestedVersion int,
) (models.TurnRepositoryComparison, models.TurnChangeReason, bool) {
	expectation, reason, ok := turnChangeComparisonExpectation(comparison, requestedVersion)
	if expectation.Basis == turnchanges.ComparisonLegacyRaw {
		expectation.EffectiveStartCommitOID = row.StartCommitOID
		expectation.EffectiveStartTreeOID = row.StartTreeOID
		expectation.EffectiveEndCommitOID = row.EndCommitOID
		expectation.EffectiveEndTreeOID = row.EndTreeOID
	}
	return expectation, reason, ok
}

func (c *Coordinator) storeAcceptedTerminalComparison(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	comparison *turnchanges.CheckpointComparison,
	expectation models.TurnRepositoryComparison,
	comparisonReason models.TurnChangeReason,
) (models.TurnRepositoryChangeSet, *turnchanges.CheckpointComparison) {
	row.ComparisonVersion, row.ComparisonBasis = expectation.Version, expectation.Basis
	row.ComparisonReason = expectation.Reason
	row.EffectiveStartCommitOID, row.EffectiveStartTreeOID = expectation.EffectiveStartCommitOID, expectation.EffectiveStartTreeOID
	row.EffectiveEndCommitOID, row.EffectiveEndTreeOID = expectation.EffectiveEndCommitOID, expectation.EffectiveEndTreeOID
	row.ComparisonRef = expectation.ComparisonRef
	if comparisonReason != "" {
		row.Availability, row.Reason = models.TurnChangeAvailabilityUnavailable, comparisonReason
		row.ComparisonReason = comparisonReason
		row.ExportState, row.ExportReason = models.TurnChangeExportUnavailable, comparisonReason
		row.EnumerationComplete, row.ComparisonComplete = false, false
		return row, comparison
	}
	row.EnumerationComplete, row.ComparisonComplete = comparison.Complete, comparison.Complete
	row.Availability, row.Reason = models.TurnChangeAvailabilityReady, comparison.Reason
	row.ExportState = models.TurnChangeExportQueued
	if err := c.storeTerminalComparisonFiles(ctx, row, comparison.Files); err != nil {
		row.ContentComplete = false
		row.ExportState, row.ExportReason = models.TurnChangeExportUnavailable, models.TurnChangeReasonContentUnavailable
		return row, comparison
	}
	markCtx, markCancel := turnChangePersistenceContextWith(ctx)
	marked, markErr := c.repository.MarkTurnRepositoryComparisonReady(markCtx, changeSet.ID, row.ID, comparison.Complete, comparison.Reason)
	markCancel()
	if markErr != nil || !marked {
		return unavailableTurnRepository(row, models.TurnChangeReasonContentUnavailable), comparison
	}
	row.EnumerationComplete, row.ComparisonComplete = comparison.Complete, comparison.Complete
	row.Availability = models.TurnChangeAvailabilityReady
	return row, comparison
}

func (c *Coordinator) acceptTurnRepositoryComparison(
	ctx context.Context,
	changeSetID string,
	row models.TurnRepositoryChangeSet,
	expectation models.TurnRepositoryComparison,
) (bool, error) {
	acceptCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	return c.repository.AcceptTurnRepositoryComparison(acceptCtx, changeSetID, row.ID, expectation)
}

// preserveHistoricalRepositoryContent retains the accepted interval in a
// backend-owned Git source instead of exporting every file into SQL.
func (c *Coordinator) preserveHistoricalRepositoryContent(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client ArchiveClient,
) (models.TurnRepositoryChangeSet, error) {
	row.ContentSource = models.TurnChangeSourceGitArchive
	receipt, err := c.content.Preserve(ctx, client, row)
	if err != nil {
		c.logger.Warn("turn change content preservation failed",
			zap.String("change_set_id", changeSet.ID), zap.String("checkout_id", row.CheckoutID),
			zap.Error(err))
		row.ContentComplete, row.Reason = false, models.TurnChangeReasonContentUnavailable
		row.PreservationState = models.TurnChangePreservationPending
		return row, err
	}
	row.PreservationAttempts = receipt.Attempts
	row.PreservationBytes = receipt.Bytes
	row.PreservationReceipt = receipt.Receipt
	if receipt.Source != "" {
		row.ContentSource = receipt.Source
	}
	if !receipt.Complete && receipt.Cause != nil {
		c.logger.Warn("turn change content preservation did not settle",
			zap.String("change_set_id", changeSet.ID), zap.String("checkout_id", row.CheckoutID),
			zap.Bool("retryable", receipt.Retryable), zap.Int64("attempts", receipt.Attempts),
			zap.String("reason", string(receipt.Reason)), zap.Error(receipt.Cause))
	}
	if receipt.Complete {
		row.ContentComplete = true
		row.PreservationState = models.TurnChangePreservationReady
		row.Reason = ""
		return row, nil
	}
	row.ContentComplete = false
	row.Reason = receipt.Reason
	if row.Reason == "" {
		row.Reason = models.TurnChangeReasonContentUnavailable
	}
	if receipt.Retryable {
		row.PreservationState = models.TurnChangePreservationPending
	} else {
		row.PreservationState = models.TurnChangePreservationUnavailable
	}
	return row, nil
}

func unavailableTurnRepository(row models.TurnRepositoryChangeSet, reason models.TurnChangeReason) models.TurnRepositoryChangeSet {
	row.Availability, row.Reason = models.TurnChangeAvailabilityUnavailable, reason
	row.ContentComplete, row.ComparisonComplete, row.EnumerationComplete = false, false, false
	row.ExportState, row.ExportReason = models.TurnChangeExportUnavailable, reason
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
	if end.CommonGitDir != "" {
		row.SourceGitDir = end.CommonGitDir
	}
	row.EndCapturedAt = timePtr(end.CapturedAt.UTC())
	row.EndProvenanceJSON = encodeTurnChangeProvenance(end.Provenance)
	row.CleanupPending = row.CleanupPending || row.EndReachabilityRef != ""
	acceptCtx, cancel := turnChangePersistenceContextWith(ctx)
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

func (c *Coordinator) storeTerminalComparisonFiles(ctx context.Context, row models.TurnRepositoryChangeSet, files []turnchanges.CheckpointFile) error {
	summaries := make([]models.TurnChangeFileContent, 0, len(files))
	for _, file := range files {
		summaries = append(summaries, models.TurnChangeFileContent{File: checkpointFileSummary(row.ID, row.CheckoutID, file)})
	}
	storeCtx, cancel := turnChangePersistenceContextWith(ctx)
	defer cancel()
	return c.repository.StoreTurnChangeFiles(storeCtx, row.ID, summaries)
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
	if readErr == nil && latest.TerminalAt == nil && latest.Revision != changeSet.Revision {
		retryCtx, retryCancel := turnChangePersistenceContext()
		retried, retryErr := c.repository.FinalizeTurnChangeSet(retryCtx, latest.ID, latest.Revision, finalization)
		retryCancel()
		if retryErr == nil && retried {
			return nil
		}
	}
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
