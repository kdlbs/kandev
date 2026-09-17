package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// ErrCeilingLaunchClaimed reports that another dispatcher currently owns the
// exact durable ceiling launch. The caller must leave its own work queued and
// let the owner settle the record.
var ErrCeilingLaunchClaimed = errors.New("ceiling deferred launch is already claimed")

const (
	ceilingClaimOwnerSendNow = "send_now"
	ceilingClaimOwnerReplay  = "ceiling_replay"
	// The lease is long enough for a normal launch, but finite so a process
	// exit cannot make a durable queue entry undispatchable forever.
	ceilingClaimLeaseDuration = 5 * time.Minute
)

// ceilingDeferredLaunchClaim is the ownership token for one exact ceiling
// record. It is persisted in the shared deferred_launch row while the runtime
// dispatch is in flight, which serializes replay with an explicit Send Now.
type ceilingDeferredLaunchClaim struct {
	svc      *Service
	taskID   string
	deferral models.CeilingDeferral
	id       string
	owner    string
	held     bool
}

// claimCeilingDeferredLaunch claims only a matching ceiling record. The bool
// reports that a matching record exists; a nil claim with true means another
// dispatcher owns it. A non-matching record is not an error because a task may
// receive a successor launch while an older queue snapshot is still visible.
func (s *Service) claimCeilingDeferredLaunch(
	ctx context.Context,
	taskID, expectedSessionID, owner string,
) (*ceilingDeferredLaunchClaim, bool, error) {
	if s == nil || s.repo == nil || taskID == "" || owner == "" {
		return nil, false, nil
	}
	ctx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	defer release()
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return nil, false, fmt.Errorf("read ceiling launch claim: %w", err)
		}
		deferral, err := models.ReadCeilingDeferral(record)
		if err != nil {
			return nil, false, nil
		}
		targetsSession, targetErr := s.ceilingDeferralTargetsSession(
			ctx, taskID, deferral, expectedSessionID,
		)
		if targetErr != nil {
			return nil, false, fmt.Errorf("resolve ceiling launch recipient: %w", targetErr)
		}
		if !targetsSession {
			return nil, false, nil
		}

		if existingClaim, claimed := models.ReadCeilingLaunchClaimDetails(record); claimed &&
			!existingClaim.Expired(time.Now().UTC()) {
			// The owner label describes the dispatcher class, not a re-entrant
			// invocation. Two replay ticks (or two Send Now requests) can use the
			// same label concurrently, so adopting a same-owner claim would let
			// both callers dispatch the exact record.
			return nil, true, nil
		}

		updated := cloneCeilingRecord(record)
		claimID := uuid.NewString()
		updated[models.CeilingLaunchClaimKey] = map[string]interface{}{
			"id":                                  claimID,
			"owner":                               owner,
			models.CeilingLaunchClaimExpiresAtKey: time.Now().UTC().Add(ceilingClaimLeaseDuration).Format(time.RFC3339Nano),
		}
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil {
			return nil, false, fmt.Errorf("claim ceiling launch: %w", err)
		}
		if stored {
			return &ceilingDeferredLaunchClaim{
				svc: s, taskID: taskID, deferral: deferral,
				id: claimID, owner: owner, held: true,
			}, true, nil
		}
		if !lostCompare {
			return nil, false, fmt.Errorf("claim ceiling launch: repository reported no write")
		}
	}
	return nil, false, fmt.Errorf("claim ceiling launch: compare-and-set retries exhausted")
}

func cloneCeilingRecord(record map[string]interface{}) map[string]interface{} {
	if record == nil {
		return map[string]interface{}{}
	}
	clone := make(map[string]interface{}, len(record)+1)
	for key, value := range record {
		clone[key] = value
	}
	return clone
}

// releaseIfHeld removes only this claim marker. The deferred launch remains
// intact so a pre-dispatch failure can be retried by the sweeper.
func (c *ceilingDeferredLaunchClaim) releaseIfHeld(ctx context.Context) {
	if c == nil || !c.held || c.svc == nil {
		return
	}
	if err := c.svc.mutateCeilingClaim(context.WithoutCancel(ctx), c, false); err != nil {
		c.svc.logger.Zap().Warn("could not release ceiling launch claim",
			zap.String("task_id", c.taskID), zap.Error(err))
		return
	}
	c.held = false
}

// settle consumes the exact deferred record after dispatch was accepted. WIP
// keys that share the row remain untouched.
func (c *ceilingDeferredLaunchClaim) settle(ctx context.Context) {
	if c == nil || !c.held || c.svc == nil {
		return
	}
	if err := c.svc.mutateCeilingClaim(context.WithoutCancel(ctx), c, true); err != nil {
		c.svc.logger.Zap().Warn("could not settle ceiling launch claim",
			zap.String("task_id", c.taskID), zap.Error(err))
		// The dispatch was accepted. Keep the marker until its lease expires so
		// a retry cannot immediately replay the same prompt. A later dispatcher
		// can reclaim the marker and settle the exact record if the CAS failed.
		c.held = false
		return
	}
	c.held = false
}

// mutateCeilingClaim performs the compare-and-set settlement for one claim.
// A changed deferral or claim is never modified by this older owner.
func (s *Service) mutateCeilingClaim(ctx context.Context, claim *ceilingDeferredLaunchClaim, settle bool) error {
	// Claim mutation is another task-admission operation. Keep the lock around
	// the compare-and-set only; settling publishes after release because task
	// event subscribers may re-enter runtime reconciliation.
	admissionCtx, release := s.lockCeilingEntryAdmission(ctx, claim.taskID)
	var publish bool
	var result error
	func() {
		defer release()
		for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
			record, prior, err := s.repo.GetTaskDeferredLaunch(admissionCtx, claim.taskID)
			if err != nil {
				result = err
				return
			}
			current, err := models.ReadCeilingDeferral(record)
			if err != nil {
				return
			}
			equivalent, err := sameCeilingDeferralIdentity(current, claim.deferral)
			if err != nil {
				result = err
				return
			}
			if !equivalent {
				// Capacity observations are mutable bookkeeping. The claim id below
				// still binds the mutation to this exact in-flight owner, while the
				// stable deferral identity prevents a successor from being touched.
				return
			}
			claimID, _, ok := models.ReadCeilingLaunchClaim(record)
			if !ok || claimID != claim.id {
				return
			}

			updated := cloneCeilingRecord(record)
			if settle {
				updated = stripCeilingRecordKeys(updated)
			} else {
				delete(updated, models.CeilingLaunchClaimKey)
			}
			stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(admissionCtx, claim.taskID, prior, updated)
			if err != nil {
				result = err
				return
			}
			if stored {
				publish = settle
				return
			}
			if !lostCompare {
				result = fmt.Errorf("repository reported no claim settlement")
				return
			}
		}
		result = fmt.Errorf("claim settlement compare-and-set retries exhausted")
	}()
	if result != nil {
		return result
	}
	if publish {
		s.publishTaskUpdatedByID(context.WithoutCancel(ctx), claim.taskID)
	}
	return nil
}
