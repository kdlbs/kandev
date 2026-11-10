package changes

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

const turnChangeWriterPageSize = 100

// reconcileWriterOverlaps derives shared-checkout attribution from canonical
// half-open writing intervals. Export progress never participates.
func (c *Coordinator) reconcileWriterOverlaps(ctx context.Context, changeSet *models.TurnChangeSet) error {
	if c == nil || c.repository == nil || changeSet == nil {
		return nil
	}
	rows, err := c.repository.ListTurnRepositoryChanges(ctx, changeSet.ID)
	if err != nil {
		return err
	}
	turnCompletedAt := c.turnChangeCanonicalCompletion(ctx, changeSet.TurnID)
	accumulator := newOverlapAccumulator(changeSet)
	for _, row := range rows {
		if row == nil || row.StartCapturedAt == nil {
			continue
		}
		if err := c.accumulateWriterRowOverlaps(ctx, changeSet, row, turnCompletedAt, accumulator); err != nil {
			return err
		}
	}
	return c.persistTurnChangeOverlaps(ctx, accumulator)
}

func (c *Coordinator) turnChangeCanonicalCompletion(ctx context.Context, turnID string) *time.Time {
	if c.turns == nil || turnID == "" {
		return nil
	}
	turn, err := c.turns.GetTurn(ctx, turnID)
	if err != nil || turn == nil || turn.CompletedAt == nil {
		return nil
	}
	at := turn.CompletedAt.UTC()
	return &at
}

type turnChangeOverlapAccumulator struct {
	original map[string][]models.TurnChangeOverlap
	updated  map[string][]models.TurnChangeOverlap
	revision map[string]int64
	touched  map[string]map[string]struct{}
}

func newOverlapAccumulator(changeSet *models.TurnChangeSet) *turnChangeOverlapAccumulator {
	return &turnChangeOverlapAccumulator{
		original: map[string][]models.TurnChangeOverlap{changeSet.ID: changeSet.OverlapIntervals},
		updated:  map[string][]models.TurnChangeOverlap{changeSet.ID: changeSet.OverlapIntervals},
		revision: map[string]int64{changeSet.ID: changeSet.Revision},
		touched:  map[string]map[string]struct{}{},
	}
}

func (a *turnChangeOverlapAccumulator) ensure(
	changeSetID string,
	overlaps []models.TurnChangeOverlap,
	revision int64,
) {
	if _, seen := a.original[changeSetID]; seen {
		return
	}
	a.original[changeSetID] = overlaps
	a.updated[changeSetID] = overlaps
	a.revision[changeSetID] = revision
}

func (a *turnChangeOverlapAccumulator) touch(changeSetID, key string) {
	if a.touched[changeSetID] == nil {
		a.touched[changeSetID] = map[string]struct{}{}
	}
	a.touched[changeSetID][key] = struct{}{}
}

func (c *Coordinator) accumulateWriterRowOverlaps(
	ctx context.Context,
	changeSet *models.TurnChangeSet,
	row *models.TurnRepositoryChangeSet,
	turnCompletedAt *time.Time,
	accumulator *turnChangeOverlapAccumulator,
) error {
	identity := models.TurnChangeCheckoutIdentity{
		CheckoutID: row.CheckoutID, EnvironmentRepoID: row.TaskEnvironmentRepoID, WorktreeID: row.WorktreeID,
	}
	current := turnChangeWriterIntervalFor(changeSet, row, turnCompletedAt)
	cursor := ""
	for {
		intervals, err := c.repository.ListTurnChangeWriterIntervals(ctx, identity, cursor, turnChangeWriterPageSize)
		if err != nil {
			return err
		}
		for index := range intervals {
			candidate := &intervals[index]
			accumulator.ensure(candidate.ChangeSetID,
				decodeTurnChangeOverlapsForReconcile(candidate.OverlapIntervalsJSON), candidate.Revision)
			if candidate.ChangeSetID == changeSet.ID {
				continue
			}
			desired, intersects := turnChangeWriterIntersection(current, candidate)
			key := candidate.ChangeSetID + "\x00" + row.CheckoutID
			accumulator.updated[changeSet.ID] = setTurnChangeOverlap(
				accumulator.updated[changeSet.ID], candidate.ChangeSetID, row.CheckoutID, desired, intersects)
			accumulator.touch(changeSet.ID, key)
			peerKey := changeSet.ID + "\x00" + candidate.CheckoutID
			accumulator.updated[candidate.ChangeSetID] = setTurnChangeOverlap(
				accumulator.updated[candidate.ChangeSetID], changeSet.ID, candidate.CheckoutID, desired, intersects)
			accumulator.touch(candidate.ChangeSetID, peerKey)
		}
		if len(intervals) < turnChangeWriterPageSize {
			break
		}
		last := intervals[len(intervals)-1]
		cursor = last.ChangeSetID + "\x00" + last.CheckoutID
	}
	return nil
}

func turnChangeWriterIntervalFor(
	changeSet *models.TurnChangeSet,
	row *models.TurnRepositoryChangeSet,
	turnCompletedAt *time.Time,
) models.TurnChangeWriterInterval {
	return models.TurnChangeWriterInterval{
		RepositoryChangeID: row.ID, ChangeSetID: changeSet.ID, CheckoutID: row.CheckoutID,
		EnvironmentRepoID: row.TaskEnvironmentRepoID, WorktreeID: row.WorktreeID,
		StartCapturedAt: *row.StartCapturedAt, TerminalCaptureAt: changeSet.TerminalCaptureStartedAt,
		TurnCompletedAt: turnCompletedAt, FinalizedAt: changeSet.TerminalAt, Revision: changeSet.Revision,
		OverlapIntervalsJSON: changeSet.OverlapIntervalsJSON,
	}
}

func turnChangeWriterIntersection(
	current models.TurnChangeWriterInterval,
	candidate *models.TurnChangeWriterInterval,
) (models.TurnChangeOverlap, bool) {
	start := current.StartCapturedAt.UTC()
	if candidate.StartCapturedAt.After(start) {
		start = candidate.StartCapturedAt.UTC()
	}
	end := earliestTurnChangeWriterEnd(current, *candidate)
	if end != nil && !end.After(start) {
		return models.TurnChangeOverlap{}, false
	}
	return models.TurnChangeOverlap{CheckoutID: current.CheckoutID, StartedAt: start, EndedAt: end}, true
}

func earliestTurnChangeWriterEnd(
	current models.TurnChangeWriterInterval,
	candidate models.TurnChangeWriterInterval,
) *time.Time {
	var end *time.Time
	for _, value := range []*time.Time{
		turnChangeWriterEndOf(current), turnChangeWriterEndOf(candidate),
	} {
		if value == nil {
			continue
		}
		if end == nil || value.Before(*end) {
			at := value.UTC()
			end = &at
		}
	}
	return end
}

func turnChangeWriterEndOf(interval models.TurnChangeWriterInterval) *time.Time {
	var end *time.Time
	for _, candidate := range []*time.Time{interval.TerminalCaptureAt, interval.TurnCompletedAt} {
		if candidate == nil {
			continue
		}
		if end == nil || candidate.Before(*end) {
			at := candidate.UTC()
			end = &at
		}
	}
	return end
}

func decodeTurnChangeOverlapsForReconcile(raw string) []models.TurnChangeOverlap {
	if raw == "" {
		return nil
	}
	var overlaps []models.TurnChangeOverlap
	if err := json.Unmarshal([]byte(raw), &overlaps); err != nil {
		return nil
	}
	return overlaps
}

func setTurnChangeOverlap(
	overlaps []models.TurnChangeOverlap,
	changeSetID, checkoutID string,
	desired models.TurnChangeOverlap,
	intersects bool,
) []models.TurnChangeOverlap {
	result := make([]models.TurnChangeOverlap, 0, len(overlaps)+1)
	for _, overlap := range overlaps {
		if overlap.ChangeSetID == changeSetID && overlap.CheckoutID == checkoutID {
			continue
		}
		result = append(result, overlap)
	}
	if !intersects {
		return result
	}
	desired.ChangeSetID, desired.CheckoutID = changeSetID, checkoutID
	return append(result, desired)
}

func (c *Coordinator) persistTurnChangeOverlaps(
	ctx context.Context,
	accumulator *turnChangeOverlapAccumulator,
) error {
	for changeSetID, values := range accumulator.updated {
		if sameTurnChangeOverlaps(accumulator.original[changeSetID], values) {
			continue
		}
		if err := c.persistTurnChangeOverlapList(ctx, changeSetID, accumulator, values); err != nil {
			return err
		}
		c.publishOverlapSummary(ctx, changeSetID)
	}
	return nil
}

func (c *Coordinator) persistTurnChangeOverlapList(
	ctx context.Context,
	changeSetID string,
	accumulator *turnChangeOverlapAccumulator,
	values []models.TurnChangeOverlap,
) error {
	revision := accumulator.revision[changeSetID]
	for attempt := 0; attempt < 3; attempt++ {
		updated, err := c.repository.UpdateTurnChangeSetOverlapsIfRevision(ctx, changeSetID, revision, values)
		if err != nil || updated {
			return err
		}
		latest, err := c.repository.GetTurnChangeSetByID(ctx, changeSetID)
		if err != nil {
			return err
		}
		values = mergeTouchedTurnChangeOverlaps(latest.OverlapIntervals, values, accumulator.touched[changeSetID])
		revision = latest.Revision
	}
	return errors.New("turn change overlap reconciliation lost repeated revision conflicts")
}

func mergeTouchedTurnChangeOverlaps(
	latest, proposed []models.TurnChangeOverlap,
	touched map[string]struct{},
) []models.TurnChangeOverlap {
	merged := latest
	for key := range touched {
		changeSetKey, checkoutKey, found := splitTurnChangeOverlapKey(key)
		if !found {
			continue
		}
		var desired models.TurnChangeOverlap
		intersects := false
		for _, value := range proposed {
			if value.ChangeSetID == changeSetKey && value.CheckoutID == checkoutKey {
				desired, intersects = value, true
			}
		}
		merged = setTurnChangeOverlap(merged, changeSetKey, checkoutKey, desired, intersects)
	}
	return merged
}

func splitTurnChangeOverlapKey(key string) (string, string, bool) {
	for index := 0; index < len(key); index++ {
		if key[index] == 0 {
			return key[:index], key[index+1:], true
		}
	}
	return "", "", false
}

func (c *Coordinator) publishOverlapSummary(ctx context.Context, changeSetID string) {
	if c.summaryPublisher == nil {
		return
	}
	changeSet, err := c.repository.GetTurnChangeSetByID(ctx, changeSetID)
	if err != nil {
		return
	}
	if err := c.summaryPublisher.PublishTurnChangeSummary(ctx, changeSet.TaskID, changeSet.TaskSessionID, changeSet.ID); err != nil {
		c.logger.Warn("failed to publish reconciled turn-change overlaps",
			zap.String("change_set_id", changeSetID), zap.Error(err))
	}
}

func sameTurnChangeOverlaps(left, right []models.TurnChangeOverlap) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
