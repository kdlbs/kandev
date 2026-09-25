package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type ceilingDispatchClaimContextKey struct{}

func withCeilingDispatchClaim(ctx context.Context, claim *ceilingDeferredLaunchClaim) context.Context {
	return context.WithValue(ctx, ceilingDispatchClaimContextKey{}, claim)
}

// admitCeilingDispatch orders final route validation and renewal of the exact
// dispatch claim with route mutation. The renewal is the local admission
// boundary; later route changes use lifecycle cancellation. No admission lock
// or lock-bearing context escapes into runtime I/O or callback publication.
func (s *Service) admitCeilingDispatch(ctx context.Context, taskID string) error {
	claim, _ := ctx.Value(ceilingDispatchClaimContextKey{}).(*ceilingDeferredLaunchClaim)
	binding := ceilingEntryBindingFromContext(ctx)
	if claim == nil && binding == nil {
		return nil
	}
	ctx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	defer release()
	if claim != nil {
		if claim.taskID != taskID {
			return ErrCeilingLaunchSuperseded
		}
		bound, present, err := models.ReadCeilingWorkflowEntryBinding(claim.deferral.Payload)
		if err != nil {
			return err
		}
		if present {
			binding = &bound
		}
		ctx = withCeilingEntryKind(ctx, claim.deferral.Kind)
	}
	if err := s.validateClaimedCeilingBinding(ctx, taskID, binding); err != nil {
		return err
	}
	if claim == nil {
		return nil
	}
	return s.renewCeilingDispatchClaim(ctx, claim)
}

func (s *Service) renewCeilingDispatchClaim(ctx context.Context, claim *ceilingDeferredLaunchClaim) error {
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, claim.taskID)
		if err != nil {
			return err
		}
		current, err := models.ReadCeilingDeferral(record)
		if err != nil {
			return ErrCeilingLaunchClaimed
		}
		equivalent, err := sameCeilingDeferralIdentity(current, claim.deferral)
		if err != nil {
			return err
		}
		details, exists := models.ReadCeilingLaunchClaimDetails(record)
		now := time.Now().UTC()
		if !equivalent || !exists || details.ID != claim.id || details.Expired(now) {
			return ErrCeilingLaunchClaimed
		}
		updated := cloneCeilingRecord(record)
		updated[models.CeilingLaunchClaimKey] = map[string]interface{}{
			"id": claim.id, "owner": claim.owner,
			models.CeilingLaunchClaimExpiresAtKey: now.Add(ceilingClaimLeaseDuration).Format(time.RFC3339Nano),
		}
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, claim.taskID, prior, updated)
		if err != nil {
			return err
		}
		if stored {
			return nil
		}
		if !lostCompare {
			return fmt.Errorf("admit ceiling dispatch: repository reported no write")
		}
	}
	return fmt.Errorf("admit ceiling dispatch: compare-and-set retries exhausted")
}
