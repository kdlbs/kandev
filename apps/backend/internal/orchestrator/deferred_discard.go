package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/automation"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

// ceilingClaimOwnerDiscard labels the short-lived claim an explicit discard
// takes while it settles. It reuses the same claim key and lease the replay and
// Send Now use, so neither can start dispatching the launch while the user
// decision is being applied.
const ceilingClaimOwnerDiscard = "user_discard"

// deferredLaunchOwnershipClearer is the atomic clear-and-release capability the
// discard needs. It is asserted on the repository, following the
// RemoveTaskMetadataKeyIfValue idiom, so the shared task repository interface
// and its many existing fakes do not grow a method only this path uses. A
// repository that cannot provide it fails closed: the discard retains the
// record rather than separating the two effects.
type deferredLaunchOwnershipClearer interface {
	ClearTaskDeferredLaunchAndReleaseOwnershipIfUnchanged(
		ctx context.Context, taskID string, prior interface{}, value map[string]interface{}, expectedOwner string,
	) (stored bool, lostCompare bool, ownershipReleased bool, err error)
}

// discardOwnershipPlan is the read-only decision about the automation start
// ownership a discarded launch carries.
type discardOwnershipPlan struct {
	// owner is the marker value to release inside the same task transaction as
	// the record clear. Empty means this discard has nothing to release.
	owner string
	// retain means ownership could not be proven, so the discard must apply
	// neither the clear nor the release.
	retain bool
}

// DiscardDeferredCeilingLaunch removes a pending ceiling-deferred launch at an
// explicit user's request. It is the only user-facing entry point into the
// ceiling record, and it never dispatches anything: no session is started or
// stopped and no manual launch is performed.
//
// The three outcomes are distinct by construction (see
// models.DeferredLaunchDiscardOutcome). Failures detected before settlement
// retain the record and its ownership without changing the automation run. If
// the final record clear fails after settlement, the record remains for a retry
// or the next sweep; retained then describes the stored record, not a run that
// can still launch.
//
// Only the ceiling half of the shared deferred_launch record is removed, and
// only for the record this call observed. A different owner's keys (a
// coexisting WIP or dependency intent) and a successor generation survive.
func (s *Service) DiscardDeferredCeilingLaunch(
	ctx context.Context, taskID string,
) (models.DeferredLaunchDiscardResult, error) {
	if s == nil || s.repo == nil {
		return models.DeferredLaunchDiscardResult{}, fmt.Errorf("deferred launch discard is unavailable")
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return models.DeferredLaunchDiscardResult{}, err
	}
	if task == nil {
		return models.DeferredLaunchDiscardResult{}, fmt.Errorf("task %s: %w", taskID, repository.ErrTaskNotFound)
	}
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return models.DeferredLaunchDiscardResult{}, err
	}
	if flag, _ := record[models.CeilingDeferredKey].(bool); !flag {
		return models.DeferredLaunchDiscardResult{Outcome: models.DeferredLaunchDiscardAlreadyClear}, nil
	}
	deferral, err := models.ReadCeilingDeferral(record)
	if err != nil {
		// The record claims the ceiling half, so it is not "already clear"; its
		// identity cannot be compared, so it is not safe to remove either.
		s.logger.Zap().Warn("could not decode the ceiling deferred record for an explicit discard",
			zap.String("task_id", taskID), zap.Error(err))
		return models.DeferredLaunchDiscardResult{
			Outcome: models.DeferredLaunchDiscardRetained,
			Reason:  models.DeferredLaunchDiscardReasonRecordUnreadable,
		}, nil
	}
	// Take the existing dispatch claim for this exact record before any
	// settlement side effect runs. Binding the claim to the observed identity
	// and taking it under the entry-admission lock is what makes the later
	// clear safe: no replay or Send Now can start dispatching in between, and a
	// successor record that replaced this one is never settled under the old
	// decision.
	claim, matches, err := s.claimCeilingDeferralForDiscard(ctx, taskID, deferral)
	if err != nil {
		return models.DeferredLaunchDiscardResult{}, err
	}
	if !matches {
		return s.classifyUnclaimedDiscard(ctx, taskID)
	}
	if claim == nil {
		return models.DeferredLaunchDiscardResult{
			Outcome: models.DeferredLaunchDiscardRetained,
			Reason:  models.DeferredLaunchDiscardReasonClaimInFlight,
		}, nil
	}
	defer claim.releaseIfHeld(ctx)
	outcome, reason := s.discardCeilingDeferral(
		withCeilingDispatchClaim(ctx, claim), task, claim.deferral,
		"the queued launch was discarded by the user", nil, true,
	)
	return models.DeferredLaunchDiscardResult{Outcome: outcome, Reason: reason}, nil
}

// claimCeilingDeferralForDiscard takes the dispatch claim for the exact record
// the caller observed, using the replay/Send Now claim key and lease. The first
// return value is nil with matches true when another dispatcher already owns
// the record; matches is false when the record changed or disappeared first, in
// which case nothing was claimed and nothing may be settled.
func (s *Service) claimCeilingDeferralForDiscard(
	ctx context.Context, taskID string, expected models.CeilingDeferral,
) (*ceilingDeferredLaunchClaim, bool, error) {
	if s == nil || s.repo == nil || taskID == "" {
		return nil, false, fmt.Errorf("deferred launch discard is unavailable")
	}
	ctx, release := s.lockCeilingEntryAdmission(ctx, taskID)
	defer release()
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		record, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			return nil, false, fmt.Errorf("read ceiling launch claim: %w", err)
		}
		current, err := models.ReadCeilingDeferral(record)
		if err != nil {
			return nil, false, nil
		}
		equivalent, err := sameCeilingDeferralIdentity(current, expected)
		if err != nil {
			return nil, false, err
		}
		if !equivalent {
			return nil, false, nil
		}
		if existing, claimed := models.ReadCeilingLaunchClaimDetails(record); claimed && !existing.Expired(time.Now().UTC()) {
			return nil, true, nil
		}
		updated := cloneCeilingRecord(record)
		claimID := uuid.NewString()
		updated[models.CeilingLaunchClaimKey] = map[string]interface{}{
			"id":                                  claimID,
			"owner":                               ceilingClaimOwnerDiscard,
			models.CeilingLaunchClaimExpiresAtKey: time.Now().UTC().Add(ceilingClaimLeaseDuration).Format(time.RFC3339Nano),
		}
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, updated)
		if err != nil {
			return nil, false, fmt.Errorf("claim ceiling launch: %w", err)
		}
		if stored {
			return &ceilingDeferredLaunchClaim{
				svc: s, taskID: taskID, deferral: current,
				id: claimID, owner: ceilingClaimOwnerDiscard, held: true,
			}, true, nil
		}
		if !lostCompare {
			return nil, false, fmt.Errorf("claim ceiling launch: repository reported no write")
		}
	}
	return nil, false, fmt.Errorf("claim ceiling launch: compare-and-set retries exhausted")
}

// classifyUnclaimedDiscard answers a discard whose exact record was replaced or
// consumed before it could be claimed. Nothing was settled, so the answer must
// not claim otherwise.
func (s *Service) classifyUnclaimedDiscard(
	ctx context.Context, taskID string,
) (models.DeferredLaunchDiscardResult, error) {
	record, _, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
	if err != nil {
		return models.DeferredLaunchDiscardResult{}, err
	}
	if flag, _ := record[models.CeilingDeferredKey].(bool); !flag {
		return models.DeferredLaunchDiscardResult{Outcome: models.DeferredLaunchDiscardAlreadyClear}, nil
	}
	return models.DeferredLaunchDiscardResult{
		Outcome: models.DeferredLaunchDiscardRetained,
		Reason:  models.DeferredLaunchDiscardReasonRecordSuperseded,
	}, nil
}

// discardCeilingDeferral is the one settlement path for a ceiling-deferred
// launch that will never run. The automatic replay drop and the explicit user
// discard both use it, so the two cannot drift apart on ordering or ownership.
//
// Explicit discards first prove that task ownership and session inventory can
// be read. The run is then settled, its release eligibility is read again, and
// the deferred record is cleared. When atomicOwnership is set, the exact
// automation start marker is removed inside the same task transaction as the
// record clear. If that final write fails, the retained record can be cleared by
// a retry or the next sweep; the already-settled run is not made launchable
// again. onSettled runs after settlement and before the durable write. Its
// error keeps the record when a caller-owned settlement is incomplete.
func (s *Service) discardCeilingDeferral(
	ctx context.Context, task *models.Task, deferral models.CeilingDeferral,
	settlementReason string, onSettled func() error, atomicOwnership bool,
) (models.DeferredLaunchDiscardOutcome, models.DeferredLaunchDiscardReason) {
	if task == nil || task.ID == "" {
		return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonClear
	}
	run := automationRunFromCeilingPayload(deferral.Payload)
	if reason := s.preflightDiscardRunOwnership(ctx, task, run, atomicOwnership); reason != "" {
		return models.DeferredLaunchDiscardRetained, reason
	}
	if !s.failQueuedAutomationRun(ctx, deferral.Payload, settlementReason) {
		return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonSettlement
	}
	// Eligibility is evaluated on the state settlement just produced: a queued
	// or triggered run only becomes a failed, unbound run after it settles, and
	// deciding before that would leave its marker behind on a successful
	// discard.
	releaseOwner := ""
	if atomicOwnership && run != nil {
		plan := s.discardOwnershipPlan(ctx, task, run.RunID)
		if plan.retain {
			return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonOwnership
		}
		releaseOwner = plan.owner
	}
	if onSettled != nil {
		if err := onSettled(); err != nil {
			return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonSettlement
		}
	}
	result, _ := s.clearCeilingDeferredRecordOutcome(ctx, task.ID, &deferral, true, releaseOwner)
	switch result {
	case ceilingClearStored:
		if !atomicOwnership && run != nil {
			s.releaseDroppedAutomationStartOwnership(ctx, task, run.RunID)
		}
		s.publishTaskUpdatedByID(ctx, task.ID)
		return models.DeferredLaunchDiscardDiscarded, ""
	case ceilingClearClaimed:
		return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonClaimInFlight
	default:
		return models.DeferredLaunchDiscardRetained, models.DeferredLaunchDiscardReasonClear
	}
}

func (s *Service) preflightDiscardRunOwnership(
	ctx context.Context, task *models.Task, run *automationRunLaunch, atomicOwnership bool,
) models.DeferredLaunchDiscardReason {
	if !atomicOwnership || run == nil {
		return ""
	}
	return s.preflightDiscardOwnership(ctx, task, run.RunID)
}

// preflightDiscardOwnership checks the matching task's inventory and the
// repository capability before an explicit discard settles its automation
// run. The post-settlement ownership plan reloads run evidence and inventory to
// decide whether the exact marker is safe to release.
func (s *Service) preflightDiscardOwnership(
	ctx context.Context, task *models.Task, runID string,
) models.DeferredLaunchDiscardReason {
	if task == nil || task.ID == "" || runID == "" {
		return ""
	}
	current, err := s.repo.GetTask(ctx, task.ID)
	if err != nil || current == nil {
		s.logger.Zap().Warn("could not read task ownership before discarding a deferred launch",
			zap.String("task_id", task.ID), zap.String("run_id", runID), zap.Error(err))
		return models.DeferredLaunchDiscardReasonOwnership
	}
	owner, _ := current.Metadata[models.MetaKeyAutomationStartOwned].(string)
	if owner != runID {
		return ""
	}
	if _, ok := s.repo.(deferredLaunchOwnershipClearer); !ok {
		s.logger.Zap().Warn("task repository cannot atomically clear a discarded launch and its ownership",
			zap.String("task_id", task.ID), zap.String("run_id", runID))
		return models.DeferredLaunchDiscardReasonClear
	}
	if _, err := s.repo.ListTaskSessions(ctx, task.ID); err != nil {
		s.logger.Zap().Warn("could not read session inventory before discarding a deferred launch",
			zap.String("task_id", task.ID), zap.String("run_id", runID), zap.Error(err))
		return models.DeferredLaunchDiscardReasonOwnership
	}
	return ""
}

// discardOwnershipPlan decides, from read-only evidence taken after settlement,
// whether this discard must release the automation start ownership marker. A
// failed, unbound ordinary run with no persisted session leaves a marker that
// is still the settled start's to release. A retry run can release its marker
// only when its exact retry ledger proves the reserved launch stopped. A bound
// or live run keeps its marker; every release happens in the same transaction
// as the clear.
func (s *Service) discardOwnershipPlan(
	ctx context.Context, task *models.Task, runID string,
) discardOwnershipPlan {
	binding, ok := s.automationService.(automationRunBinding)
	owner, _ := task.Metadata[models.MetaKeyAutomationStartOwned].(string)
	if !ok || owner == "" || owner != runID {
		return discardOwnershipPlan{}
	}
	run, err := binding.GetRun(ctx, runID)
	if err != nil {
		// Without the run row this request cannot tell whether the marker is
		// still its own to release, so it must not change anything.
		s.logger.Zap().Warn("could not read the automation run of a discarded launch",
			zap.String("task_id", task.ID), zap.String("run_id", runID), zap.Error(err))
		return discardOwnershipPlan{retain: true}
	}
	if run == nil || run.TaskID != task.ID {
		return discardOwnershipPlan{}
	}
	if run.RetryGroupID != "" {
		return s.discardRetryOwnershipPlan(ctx, task, runID, owner, run)
	}
	if run.Status != automation.RunStatusFailed || run.SessionID != "" || run.TurnID != "" {
		return discardOwnershipPlan{}
	}
	sessions, err := s.repo.ListTaskSessions(ctx, task.ID)
	if err != nil {
		s.logger.Zap().Warn("could not list sessions before releasing discarded launch ownership",
			zap.String("task_id", task.ID), zap.String("run_id", runID), zap.Error(err))
		return discardOwnershipPlan{retain: true}
	}
	if len(sessions) != 0 {
		return discardOwnershipPlan{}
	}
	return discardOwnershipPlan{owner: owner}
}

func (s *Service) discardRetryOwnershipPlan(
	ctx context.Context, task *models.Task, runID, owner string, run *automation.AutomationRun,
) discardOwnershipPlan {
	ledger, ok := s.automationService.(automationRetryLaunchOwner)
	if !ok {
		return discardOwnershipPlan{retain: true}
	}
	launch, err := ledger.GetRetryLaunch(ctx, runID)
	if err != nil {
		s.logger.Zap().Warn("could not read the retry launch of a discarded launch",
			zap.String("task_id", task.ID), zap.String("run_id", runID), zap.Error(err))
		return discardOwnershipPlan{retain: true}
	}
	if automationInitialStartResolved(task, run, launch) {
		return discardOwnershipPlan{owner: owner}
	}
	return discardOwnershipPlan{}
}

// ceilingDeferralClaimed reports whether the record carries a dispatch claim
// that has not expired. A record written before leases existed has no claim and
// is therefore not claimed.
func (s *Service) ceilingDeferralClaimed(record map[string]interface{}) bool {
	claim, ok := models.ReadCeilingLaunchClaimDetails(record)
	return ok && !claim.Expired(time.Now().UTC())
}

// ceilingClaimOwnedByContext reports whether the live claim on the record is
// the one this call already holds, so a dispatcher settling its own claim is
// never treated as a second owner.
func ceilingClaimOwnedByContext(ctx context.Context, taskID string, record map[string]interface{}) bool {
	own, ok := ctx.Value(ceilingDispatchClaimContextKey{}).(*ceilingDeferredLaunchClaim)
	if !ok || own == nil || own.taskID != taskID {
		return false
	}
	claim, held := models.ReadCeilingLaunchClaimDetails(record)
	return held && claim.ID == own.id
}
