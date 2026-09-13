package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// deferredLaunchCASRetryBudget bounds the read-compare-write retry AC-40a
// requires when a concurrent writer wins the compare-and-set race.
const deferredLaunchCASRetryBudget = 3

// deferCeilingRefusal persists a ceiling_deferred record for an automatic launch
// the admission controller refused, merging it into whatever the task's shared
// deferred_launch value already holds (AC-46/AC-46a) under a row-locked
// compare-and-set (AC-12e/AC-40). The stored origin is always automatic: AC-13c
// states a manual launch is never deferred, so there is nothing else to record.
//
// kind and payload describe the launch to replay; reasonCode is carried onto the
// record's bookkeeping (AC-22a) and is never nested with the payload.
func (s *Service) deferCeilingRefusal(
	ctx context.Context, taskID string, kind models.CeilingLaunchKind, payload map[string]interface{}, reasonCode string,
) error {
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		existingRaw, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return fmt.Errorf("reading deferred launch for task %s: %w", taskID, err)
		}

		deferral := models.CeilingDeferral{
			Kind:       kind,
			Payload:    payload,
			Origin:     string(launchOriginAutomatic),
			ReasonCode: reasonCode,
			QueuedAt:   time.Now(),
		}

		if existingCeiling, readErr := models.ReadCeilingDeferral(existingRaw); readErr == nil {
			// A ceiling_deferred record already exists for this task (AC-12d).
			equivalent, cmpErr := models.CeilingDeferralsEquivalent(existingCeiling, deferral)
			if cmpErr != nil {
				return fmt.Errorf("comparing deferred launch payloads for task %s: %w", taskID, cmpErr)
			}
			if equivalent {
				// AC-12a: the same launch, already recorded. Leave it exactly as
				// stored, including its queued_at (AC-32).
				return nil
			}
			// A different launch: retain the one already stored and report the
			// collision rather than losing either payload silently. AC-49
			// (phase 7) adds the card-visible surface; this WARN plus the
			// reason code on the retained record is the observable available
			// at this phase.
			s.logger.Zap().Warn("ceiling refusal superseded by an earlier pending deferral for the same task",
				zap.String("task_id", taskID),
				zap.String("stored_kind", string(existingCeiling.Kind)),
				zap.String("superseded_kind", string(kind)),
				zap.String("reason_code", ceilingReasonSuperseded))
			return nil
		}

		record, discarded, replaced := models.MergeCeilingRecord(existingRaw, deferral)
		if replaced {
			s.logger.Zap().Warn("deferred_launch held a non-object value; the ceiling refusal replaced it",
				zap.String("task_id", taskID), zap.Any("discarded_value", discarded))
		}

		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, record)
		if err != nil {
			return fmt.Errorf("writing deferred launch for task %s: %w", taskID, err)
		}
		if stored {
			return nil
		}
		if lostCompare {
			continue
		}
		return fmt.Errorf("writing deferred launch for task %s: repository reported neither stored nor a lost compare", taskID)
	}
	return fmt.Errorf("%s: could not persist deferred launch for task %s after %d attempts",
		ceilingReasonDeferWriteFailed, taskID, deferredLaunchCASRetryBudget)
}
