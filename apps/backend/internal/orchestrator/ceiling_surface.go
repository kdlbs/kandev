package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ceilingSurfaceRetryBudget bounds AC-49g1's carrier-write retries: at most
// this many consecutive failures before the sweep stops attempting and logs
// once at ERROR, leaving the record deferred and retryable.
const ceilingSurfaceRetryBudget = 5

var errCeilingSurfaceSuperseded = errors.New("ceiling surface belongs to a superseded deferral")

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
	if errors.Is(writeErr, errCeilingSurfaceSuperseded) {
		return true
	}

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
// message row, written through the idempotent message entry point. The
// deferral identity is checked while holding task admission, so replacement
// cannot race the external write with an obsolete card note.
func (s *Service) writeCeilingSurfaceNote(ctx context.Context, taskID, sessionID string, deferral models.CeilingDeferral) error {
	if s.messageCreator == nil {
		return fmt.Errorf("no message creator is configured")
	}
	admissionCtx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	defer release()
	currentRaw, _, err := s.repo.GetTaskDeferredLaunch(admissionCtx, taskID)
	if err != nil {
		return fmt.Errorf("reading deferred launch before surfacing ceiling refusal: %w", err)
	}
	current, readErr := models.ReadCeilingDeferral(currentRaw)
	if readErr != nil {
		return errCeilingSurfaceSuperseded
	}
	identityMatches, compareErr := sameCeilingSurfaceIdentity(current, deferral)
	if compareErr != nil {
		return fmt.Errorf("comparing deferred launch before surfacing ceiling refusal: %w", compareErr)
	}
	if !identityMatches {
		return errCeilingSurfaceSuperseded
	}
	messageID, err := ceilingSurfaceMessageID(taskID, sessionID, deferral)
	if err != nil {
		return fmt.Errorf("building ceiling refusal message identity: %w", err)
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
	return s.messageCreator.CreateSessionMessageIdempotent(
		admissionCtx, messageID, taskID, content, sessionID, string(v1.MessageTypeStatus), "", metadata, false,
	)
}

func sameCeilingSurfaceIdentity(a, b models.CeilingDeferral) (bool, error) {
	if a.ReasonCode != b.ReasonCode || a.Population != b.Population ||
		a.PopulationKnown != b.PopulationKnown || a.Ceiling != b.Ceiling {
		return false, nil
	}
	return sameCeilingDeferralIdentity(a, b)
}

func ceilingSurfaceMessageID(taskID, sessionID string, deferral models.CeilingDeferral) (string, error) {
	identity, err := json.Marshal(struct {
		TaskID          string
		SessionID       string
		Kind            models.CeilingLaunchKind
		Payload         map[string]interface{}
		Origin          string
		ReasonCode      string
		QueuedAt        string
		Population      int
		PopulationKnown bool
		Ceiling         int
	}{
		TaskID:          taskID,
		SessionID:       sessionID,
		Kind:            deferral.Kind,
		Payload:         deferral.Payload,
		Origin:          deferral.Origin,
		ReasonCode:      deferral.ReasonCode,
		QueuedAt:        deferral.QueuedAt.UTC().Format(time.RFC3339),
		Population:      deferral.Population,
		PopulationKnown: deferral.PopulationKnown,
		Ceiling:         deferral.Ceiling,
	})
	if err != nil {
		return "", err
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, append([]byte("ceiling-surface:"), identity...)).String(), nil
}
