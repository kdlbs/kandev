package orchestrator

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ceilingManualOverrideMetadataKey is AC-53's session metadata key: an
// audit record of a manual launch admitted while the instance was at or
// above its session ceiling.
const ceilingManualOverrideMetadataKey = "ceiling_manual_override"

// recordManualOverrideIfAdmitted implements AC-14/AC-53: once a manually
// admitted reservation is settled onto a real session, it stamps a
// once-per-session audit record on the session and writes the AC-14 card
// warning. It is a no-op unless this launch was an actual override (an
// ordinary admission never reaches here with manualOverride set) and a
// session exists to attach to — called from the point each seam's
// reservation is bound to that session id, independent of whether the
// launch or resume attempt that follows later succeeds or fails, so a
// failure between admission and agent start can never erase the only
// evidence an override happened. AC-14b's "the launch failed before a
// session existed" case (seam 1's pre-session-creation window) still never
// reaches this at all.
func (s *Service) recordManualOverrideIfAdmitted(
	ctx context.Context, taskID, sessionID string, manualOverride bool, population int, populationKnown bool, ceiling int,
) {
	if !manualOverride || sessionID == "" {
		return
	}

	reasonCodes := []string{ceilingReasonManualOverride}
	if !populationKnown {
		// AC-33a: "unknown" is an absent key, not a rendered word — population
		// is omitted entirely below, and the reason why is added alongside
		// the override code rather than replacing it.
		reasonCodes = append(reasonCodes, ceilingReasonUnknownPopulation)
	}
	record := map[string]interface{}{
		"reason_codes":      reasonCodes,
		ceilingFieldCeiling: ceiling,
		"recorded_at":       time.Now().UTC().Format(time.RFC3339),
	}
	if populationKnown {
		record["population"] = population
	}

	stored, err := s.repo.SetSessionMetadataKeyIfAbsent(ctx, sessionID, ceilingManualOverrideMetadataKey, record)
	if err != nil {
		// AC-53b: the launch is already admitted and proceeding; losing the
		// audit record is a reporting gap, not a lost launch. Never fail,
		// delay, retry or roll back for this.
		s.logger.Zap().Warn("could not record the ceiling manual-override audit metadata",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
	if err == nil && !stored {
		// Idempotent: a prior call for this session already recorded (and,
		// per this same rule, already warned) the override that created it.
		return
	}

	s.writeCeilingManualOverrideWarning(ctx, taskID, sessionID, population, populationKnown, ceiling)
}

// writeCeilingManualOverrideWarning is AC-14's card warning, naming the
// population and ceiling the override was admitted against.
func (s *Service) writeCeilingManualOverrideWarning(
	ctx context.Context, taskID, sessionID string, population int, populationKnown bool, ceiling int,
) {
	metadata := map[string]interface{}{
		metaKeyVariant:         metaVariantCeiling,
		ceilingFieldReasonCode: ceilingReasonManualOverride,
		ceilingFieldCeiling:    ceiling,
		metaKeyTaskID:          taskID,
	}
	content := fmt.Sprintf("This launch was started manually while the instance was at its session ceiling of %d.", ceiling)
	if populationKnown {
		metadata["population"] = population
		content = fmt.Sprintf("This launch was started manually while the instance was at its session ceiling (%d of %d).",
			population, ceiling)
	}
	if s.messageCreator == nil {
		return
	}
	if err := s.messageCreator.CreateSessionMessage(
		ctx, taskID, content, sessionID, string(v1.MessageTypeStatus), "", metadata, false,
	); err != nil {
		s.logger.Zap().Warn("could not write the ceiling manual-override card warning",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
	}
}
