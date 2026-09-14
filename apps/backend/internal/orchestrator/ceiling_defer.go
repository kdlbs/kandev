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
// record's bookkeeping (AC-22a) and is never nested with the payload. sessionID,
// population, populationKnown and ceiling describe this refusal for the AC-49
// card carrier; sessionID is empty for seam 1, which AC-49 excludes entirely.
func (s *Service) deferCeilingRefusal(
	ctx context.Context, taskID, sessionID string, kind models.CeilingLaunchKind, payload map[string]interface{},
	reasonCode string, population int, populationKnown bool, ceiling int,
) error {
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		existingRaw, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return fmt.Errorf("reading deferred launch for task %s: %w", taskID, err)
		}

		deferral := models.CeilingDeferral{
			Kind:            kind,
			Payload:         payload,
			Origin:          string(launchOriginAutomatic),
			ReasonCode:      reasonCode,
			QueuedAt:        time.Now(),
			Population:      population,
			PopulationKnown: populationKnown,
			Ceiling:         ceiling,
		}

		if existingCeiling, readErr := models.ReadCeilingDeferral(existingRaw); readErr == nil {
			// A ceiling_deferred record already exists for this task (AC-12d).
			equivalent, cmpErr := models.CeilingDeferralsEquivalent(existingCeiling, deferral)
			if cmpErr != nil {
				return fmt.Errorf("comparing deferred launch payloads for task %s: %w", taskID, cmpErr)
			}
			if !equivalent {
				// A different launch: retain the one already stored and report
				// the collision rather than losing either payload silently.
				s.logger.Zap().Warn("ceiling refusal superseded by an earlier pending deferral for the same task",
					zap.String("task_id", taskID),
					zap.String("stored_kind", string(existingCeiling.Kind)),
					zap.String("superseded_kind", string(kind)),
					zap.String(ceilingFieldReasonCode, ceilingReasonSuperseded))
				return nil
			}
			if existingCeiling.ReasonCode == reasonCode {
				// AC-12a/AC-49e: the same launch, refused for the same reason,
				// already recorded. Leave it exactly as stored, including its
				// queued_at (AC-32) and its card-surface bookkeeping.
				return nil
			}
			// AC-49f: the same pending launch, but the reason changed (for
			// example a transient population read failure resolved into an
			// ordinary refusal). Update the stored reason and this refusal's
			// population/ceiling reading, preserve queued_at, and reset the
			// surface stamps so the new reason gets its own note (AC-49g1).
			updated := existingCeiling
			updated.ReasonCode = reasonCode
			updated.Population = population
			updated.PopulationKnown = populationKnown
			updated.Ceiling = ceiling
			if s.writeCeilingDeferralUpdate(ctx, taskID, existingRaw, prior, updated) {
				s.attemptCeilingSurfaceWrite(ctx, taskID)
			}
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
			s.attemptCeilingSurfaceWrite(ctx, taskID)
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

// writeCeilingDeferralUpdate writes an updated deferral over the record that
// produced it, clearing the AC-49 surface stamps explicitly: CeilingRecordKeys
// does not emit them at all, so without this a stale ceiling_surface_written_at
// from before the reason changed would otherwise survive the merge untouched
// and permanently suppress the new note. Reports whether the write landed; a
// lost compare is treated as "someone else already moved this record on" and
// is not retried here, since the next admission attempt will see fresh state.
func (s *Service) writeCeilingDeferralUpdate(
	ctx context.Context, taskID string, existingRaw interface{}, prior interface{}, updated models.CeilingDeferral,
) bool {
	record, _, _ := models.MergeCeilingRecord(existingRaw, updated)
	delete(record, models.CeilingSurfaceWrittenAtKey)
	delete(record, models.CeilingSurfaceAttemptCountKey)
	stored, _, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, record)
	if err != nil {
		s.logger.Zap().Warn("could not update a ceiling deferral's reason code",
			zap.String("task_id", taskID), zap.Error(err))
		return false
	}
	return stored
}
