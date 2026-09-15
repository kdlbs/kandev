package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ceilingSurfaceRetryBudget bounds AC-49g1's carrier-write retries: at most
// this many consecutive failures before the sweep stops attempting and logs
// once at ERROR, leaving the record deferred and retryable.
const ceilingSurfaceRetryBudget = 5

// attemptCeilingSurfaceWrite is AC-49's single surfacing actor for the
// ordinary refusal path (seams 2-5; seam 1 has no session and is excluded).
// It writes the card note for a task's currently-stored ceiling deferral,
// unless one was already written for the current reason (AC-49e) or the
// write has already failed ceilingSurfaceRetryBudget consecutive times for
// this reason (AC-49g1). It is called both inline by deferCeilingRefusal,
// right after a fresh or reason-changed write, and by the sweep's drain pass
// for a launch still deferred this tick (AC-49g).
//
// It always re-reads the current record rather than trusting a caller-held
// snapshot, so a write racing with this one is never overwritten blind.
func (s *Service) attemptCeilingSurfaceWrite(ctx context.Context, taskID string) {
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			s.logger.Zap().Warn("could not read deferred launch to surface a ceiling refusal",
				zap.String("task_id", taskID), zap.Error(err))
			return
		}
		deferral, readErr := models.ReadCeilingDeferral(record)
		if readErr != nil {
			return
		}
		sessionID := sessionIDFromCeilingPayload(deferral)
		if sessionID == "" {
			return
		}
		if _, written := record[models.CeilingSurfaceWrittenAtKey]; written {
			return
		}
		if ceilingRecordAttemptCount(record) >= ceilingSurfaceRetryBudget {
			return
		}

		if s.writeCeilingSurfaceNoteAndStamp(ctx, taskID, sessionID, deferral, record, prior) {
			return
		}
		// A lost compare-and-set race: re-read and try again, up to the CAS
		// retry budget, mirroring deferCeilingRefusal's own loop.
	}
}

// ceilingRecordAttemptCount reads the AC-49g1 failure counter off a raw
// deferred_launch record, defaulting to zero for an absent or malformed value.
func ceilingRecordAttemptCount(record map[string]interface{}) int {
	raw, ok := record[models.CeilingSurfaceAttemptCountKey]
	if !ok {
		return 0
	}
	return models.CeilingRecordInt(raw)
}

// writeCeilingSurfaceNoteAndStamp attempts the card note write, then stores
// either the AC-49e success stamp or the AC-49g1 failure counter under the
// same compare-and-set the caller read prior from. It reports whether the
// write landed (stored or not, but not a lost compare), so the caller knows
// whether to retry the read-modify-write cycle.
func (s *Service) writeCeilingSurfaceNoteAndStamp(
	ctx context.Context, taskID, sessionID string, deferral models.CeilingDeferral,
	record map[string]interface{}, prior interface{},
) bool {
	writeErr := s.writeCeilingSurfaceNote(ctx, taskID, sessionID, deferral)

	updated := make(map[string]interface{}, len(record)+1)
	for key, value := range record {
		updated[key] = value
	}
	if writeErr == nil {
		updated[models.CeilingSurfaceWrittenAtKey] = time.Now().UTC().Format(time.RFC3339)
		delete(updated, models.CeilingSurfaceAttemptCountKey)
	} else {
		count := ceilingRecordAttemptCount(record) + 1
		updated[models.CeilingSurfaceAttemptCountKey] = count
		s.logger.Zap().Warn("could not write the ceiling refusal card note; a later sweep will retry",
			zap.String("task_id", taskID), zap.String("session_id", sessionID),
			zap.String(ceilingFieldReasonCode, ceilingReasonSurfaceWriteFailed), zap.Error(writeErr))
		if count >= ceilingSurfaceRetryBudget {
			s.logger.Zap().Error("ceiling refusal card note failed repeatedly; giving up until the reason changes",
				zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Int("attempts", count))
		}
	}

	stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
	if err != nil {
		s.logger.Zap().Warn("could not persist the ceiling card-surface bookkeeping",
			zap.String("task_id", taskID), zap.Error(err))
		return true
	}
	if lostCompare {
		return false
	}
	_ = stored
	return true
}

// writeCeilingSurfaceNote is AC-49's carrier: one session-anchored status
// message row, written through the existing CreateSessionMessage entry point
// createRecoveryStatusMessage already uses for a different variant. It
// carries no recovery actions (AC-49d): a ceiling deferral clears itself on
// its own.
func (s *Service) writeCeilingSurfaceNote(ctx context.Context, taskID, sessionID string, deferral models.CeilingDeferral) error {
	if s.messageCreator == nil {
		return fmt.Errorf("no message creator is configured")
	}
	metadata := map[string]interface{}{
		metaKeyVariant:         metaVariantCeiling,
		ceilingFieldReasonCode: deferral.ReasonCode,
		ceilingFieldCeiling:    deferral.Ceiling,
		metaKeyTaskID:          taskID,
		metaKeySessionID:       sessionID,
	}
	content := fmt.Sprintf("This session's launch is queued: the instance is at its session ceiling (%s).", deferral.ReasonCode)
	if deferral.PopulationKnown {
		metadata["population"] = deferral.Population
		content = fmt.Sprintf("This session's launch is queued: %d of %d concurrent sessions are in use.",
			deferral.Population, deferral.Ceiling)
	}
	return s.messageCreator.CreateSessionMessage(
		ctx, taskID, content, sessionID, string(v1.MessageTypeStatus), "", metadata, false,
	)
}
