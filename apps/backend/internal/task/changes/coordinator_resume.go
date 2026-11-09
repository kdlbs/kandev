package changes

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/common/turnchanges"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const (
	turnChangeResumePageSize = 20
	turnChangeResumePages    = 5
)

// ResumeUnfinished completes summary and content processing from already
// accepted immutable evidence. It never captures a new endpoint or relaunches
// an executor.
func (c *Coordinator) ResumeUnfinished(ctx context.Context, provider TurnChangeExportClientProvider) error {
	if c == nil || c.repository == nil || provider == nil {
		return nil
	}
	var failures []error
	afterID := ""
	for page := 0; page < turnChangeResumePages; page++ {
		changeSets, err := c.repository.ListResumableTurnChangeSets(ctx, afterID, turnChangeResumePageSize)
		if err != nil {
			return err
		}
		for _, changeSet := range changeSets {
			if changeSet == nil {
				continue
			}
			afterID = changeSet.ID
			if err := c.resumeChangeSet(ctx, changeSet, provider); err != nil {
				failures = append(failures, err)
			}
		}
		if len(changeSets) < turnChangeResumePageSize {
			break
		}
	}
	return errors.Join(failures...)
}

func (c *Coordinator) resumeChangeSet(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	provider TurnChangeExportClientProvider,
) error {
	setup, ready, err := c.prepareResume(ctx, changeSet, provider)
	if err != nil || !ready {
		return err
	}
	if setup.release != nil {
		defer setup.release()
	}
	changeSet, err = c.resumeTerminalClaim(ctx, changeSet, setup.terminal)
	if err != nil || changeSet == nil {
		return err
	}
	rows, err := c.repository.ListTurnRepositoryChanges(ctx, changeSet.ID)
	if err != nil {
		return err
	}
	if !setup.hasClient {
		if turnChangeRestoreInProgress(provider, changeSet.TaskSessionID) {
			return nil
		}
		return c.resumeWithoutProvider(ctx, changeSet, setup.terminal, rows)
	}
	return c.resumeWithClient(ctx, changeSet, setup.terminal, rows, setup.client)
}

// turnChangeRestoreInProgress reports concrete evidence that an executor is
// still starting. Without that evidence, a completed turn must not stay
// silently pending forever.
func turnChangeRestoreInProgress(provider TurnChangeExportClientProvider, sessionID string) bool {
	restorer, ok := provider.(TurnChangeRestoreStateProvider)
	return ok && restorer.TurnChangeRestoreInProgress(sessionID)
}

type resumeSetup struct {
	terminal  Terminal
	client    CheckpointClient
	release   func()
	hasClient bool
}

func (c *Coordinator) prepareResume(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	provider TurnChangeExportClientProvider,
) (resumeSetup, bool, error) {
	if c.boundaryGuard != nil {
		state := c.boundaryGuard.TurnChangeCaptureOperation(ctx, changeSet.RuntimeExecutionID, changeSet.PromptGeneration)
		if state != TurnChangeOperationInactive {
			return resumeSetup{}, false, nil
		}
	}
	turn, err := c.turns.GetTurn(ctx, changeSet.TurnID)
	if err != nil || turn == nil || turn.CompletedAt == nil {
		return resumeSetup{}, false, err
	}
	setup := resumeSetup{terminal: Terminal{Admission: Admission{
		TaskID: changeSet.TaskID, SessionID: changeSet.TaskSessionID, TaskEnvironmentID: changeSet.TaskEnvironmentID,
		TurnID: changeSet.TurnID, ExecutionID: changeSet.RuntimeExecutionID, StartupAttemptID: changeSet.StartupAttemptID,
		PromptGeneration: uint64(changeSet.PromptGeneration),
	}, At: turn.CompletedAt.UTC(), Outcome: "restart_resume"}}
	setup.client, setup.release, setup.hasClient = provider.AcquireTurnChangeExporter(ctx, changeSet.TaskSessionID)
	return setup, true, nil
}

func (c *Coordinator) resumeWithClient(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	rows []*models.TurnRepositoryChangeSet,
	client CheckpointClient,
) error {
	progress := newTerminalProgress(changeSet, terminal, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		resumed := c.resumeRepositoryRow(ctx, changeSet, *row, client)
		progress.add(resumed.row, resumed.comparison)
	}
	if err := c.reconcileWriterOverlaps(ctx, changeSet); err != nil {
		c.logger.Warn("failed to reconcile resumed turn overlaps", zap.String("change_set_id", changeSet.ID), zap.Error(err))
	} else if latest, readErr := c.readTerminalTurnChangeSet(terminal); readErr == nil {
		changeSet.Revision = latest.Revision
		changeSet.OverlapIntervals = latest.OverlapIntervals
		progress.finalization.OverlapIntervals = latest.OverlapIntervals
	}
	finalization := progress.summaryResult()
	if err := c.persistTerminalFinalization(changeSet, terminal, finalization, client); err != nil {
		return err
	}
	if err := c.ProcessTerminalContent(ctx, terminal, client); err != nil {
		return err
	}
	c.publishResumedSummary(ctx, changeSet)
	return nil
}

// resumeWithoutProvider rebuilds the durable summary from SQL and settles
// content that no retained executor can still provide. It never captures or
// relaunches anything.
func (c *Coordinator) resumeWithoutProvider(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
	rows []*models.TurnRepositoryChangeSet,
) error {
	progress := newTerminalProgress(changeSet, terminal, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		resumed, comparison, ok, err := c.resumeStoredRepositoryRow(ctx, changeSet.ID, *row)
		if err != nil {
			return err
		}
		if ok {
			progress.add(resumed, comparison)
			continue
		}
		progress.add(unavailableTurnRepository(*row, models.TurnChangeReasonExportMissingObject), nil)
	}
	if err := c.reconcileWriterOverlaps(ctx, changeSet); err != nil {
		c.logger.Warn("failed to reconcile recovered turn overlaps", zap.String("change_set_id", changeSet.ID), zap.Error(err))
	} else if latest, readErr := c.readTerminalTurnChangeSetWith(ctx, terminal); readErr == nil {
		changeSet.Revision = latest.Revision
		changeSet.OverlapIntervals = latest.OverlapIntervals
		progress.finalization.OverlapIntervals = latest.OverlapIntervals
	}
	finalization := progress.summaryResult()
	if err := c.persistTerminalFinalization(changeSet, terminal, finalization, nil); err != nil {
		return err
	}
	if err := c.settleContentProgress(ctx, changeSet.ID); err != nil {
		return err
	}
	c.publishResumedSummary(ctx, changeSet)
	return nil
}

func (c *Coordinator) resumeStoredRepositoryRow(
	ctx context.Context,
	changeSetID string,
	row models.TurnRepositoryChangeSet,
) (models.TurnRepositoryChangeSet, *turnchanges.CheckpointComparison, bool, error) {
	if row.ComparisonBasis == "" || !row.ComparisonComplete {
		return row, nil, false, nil
	}
	comparison, err := c.storedTurnRepositoryComparison(ctx, changeSetID, row)
	if err != nil {
		return row, nil, false, nil
	}
	resumed := row
	resumed.Availability = models.TurnChangeAvailabilityReady
	if !resumed.ContentComplete {
		settleCtx, cancel := turnChangePersistenceContextWith(ctx)
		err := c.repository.SettleTurnRepositoryExportFailure(
			settleCtx, resumed.ID, resumed.ExportClaimToken, models.TurnChangeReasonExportMissingObject,
		)
		cancel()
		if err != nil {
			return row, nil, false, err
		}
		resumed.ExportState = models.TurnChangeExportUnavailable
		resumed.ExportReason = models.TurnChangeReasonExportMissingObject
		resumed.Reason = models.TurnChangeReasonExportMissingObject
	}
	return resumed, comparison, true, nil
}

func (c *Coordinator) resumeTerminalClaim(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	terminal Terminal,
) (*models.TurnChangeSet, error) {
	claim := models.TurnChangeSetTerminalClaim{
		At: terminal.At, ExecutionID: terminal.ExecutionID, StartupAttemptID: terminal.StartupAttemptID,
		PromptGeneration: int64(terminal.PromptGeneration), TaskEnvironmentID: terminal.TaskEnvironmentID,
		Outcome: terminal.Outcome,
	}
	writeCtx, cancel := turnChangePersistenceContext()
	claimed, err := c.repository.ClaimTurnChangeSetTerminal(writeCtx, changeSet.ID, changeSet.Revision, claim)
	cancel()
	if err != nil {
		return nil, err
	}
	if !claimed {
		latest, readErr := c.readTerminalTurnChangeSet(terminal)
		if readErr != nil {
			return nil, readErr
		}
		if latest.TerminalAt != nil {
			return nil, nil
		}
		if !terminalChangeClaimMatches(latest, claim) {
			return nil, errors.New("resumed terminal claim lost its immutable ownership")
		}
		return latest, nil
	}
	return c.reloadTerminalCapture(terminal, changeSet.ID)
}

type resumedTurnRepository struct {
	row        models.TurnRepositoryChangeSet
	comparison *turnchanges.CheckpointComparison
}

func (c *Coordinator) resumeRepositoryRow(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row models.TurnRepositoryChangeSet,
	client CheckpointClient,
) resumedTurnRepository {
	if row.EndCommitOID == "" || row.EndTreeOID == "" {
		return resumedTurnRepository{row: unavailableTurnRepository(row, models.TurnChangeReasonCheckoutUnavailable)}
	}
	if row.ComparisonBasis != "" && row.ComparisonComplete {
		comparison, err := c.storedTurnRepositoryComparison(ctx, changeSet.ID, row)
		if err != nil {
			return resumedTurnRepository{row: unavailableTurnRepository(row, models.TurnChangeReasonHistoryProvenanceMissing)}
		}
		row.Availability = models.TurnChangeAvailabilityReady
		row.Reason = comparison.Reason
		row.ComparisonComplete, row.EnumerationComplete = comparison.Complete, comparison.Complete
		row.ExportState = models.TurnChangeExportQueued
		return resumedTurnRepository{row: row, comparison: comparison}
	}
	row, comparison := c.compareTerminalRepository(ctx, changeSet, row, client)
	return resumedTurnRepository{row: row, comparison: comparison}
}

func (c *Coordinator) storedTurnRepositoryComparison(
	ctx context.Context,
	changeSetID string,
	row models.TurnRepositoryChangeSet,
) (*turnchanges.CheckpointComparison, error) {
	comparison := &turnchanges.CheckpointComparison{
		ChangeSetID: changeSetID, CheckoutID: row.CheckoutID, HashAlgorithm: row.HashAlgorithm,
		StartCommitOID: row.StartCommitOID, StartTreeOID: row.StartTreeOID,
		EndCommitOID: row.EndCommitOID, EndTreeOID: row.EndTreeOID,
		ComparisonVersion: int(row.ComparisonVersion), ComparisonBasis: row.ComparisonBasis,
		ComparisonReason: row.ComparisonReason, ComparisonRef: row.ComparisonRef,
		EffectiveStartCommitOID: row.EffectiveStartCommitOID, EffectiveStartTreeOID: row.EffectiveStartTreeOID,
		EffectiveEndCommitOID: row.EffectiveEndCommitOID, EffectiveEndTreeOID: row.EffectiveEndTreeOID,
		Complete: row.ComparisonComplete, Reason: row.Reason,
	}
	added, deleted := int64(0), int64(0)
	addedKnown, deletedKnown := true, true
	for offset := 0; ; {
		files, total, err := c.repository.ListTurnFileChanges(ctx, changeSetID, row.ID, offset, 100)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if file == nil {
				continue
			}
			comparison.FileCount++
			if file.Binary {
				comparison.BinaryCount++
				continue
			}
			if file.AddedLines == nil || file.DeletedLines == nil {
				comparison.UnknownCount++
				addedKnown, deletedKnown = false, false
				continue
			}
			added += *file.AddedLines
			deleted += *file.DeletedLines
		}
		offset += len(files)
		if offset >= total || len(files) == 0 {
			break
		}
	}
	if addedKnown {
		comparison.AddedLines = &added
	}
	if deletedKnown {
		comparison.DeletedLines = &deleted
	}
	return comparison, nil
}

func (c *Coordinator) publishResumedSummary(ctx context.Context, changeSet *models.TurnChangeSet) {
	if c.summaryPublisher == nil {
		return
	}
	_ = c.summaryPublisher.PublishTurnChangeSummary(ctx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID)
}
