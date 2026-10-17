package scheduler

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	runmodels "github.com/kandev/kandev/internal/runs/models"
)

// Capacity waits are not provider attempts. Task-owned ceiling replay retains
// its existing cap/disposition; Office-owned capacity waits do not spend it.
func providerAttemptCount(run *runmodels.Run, prior []models.RouteAttempt, commonOwned ...bool) int {
	if extractRunTaskID(run.Payload) != "" && (len(commonOwned) == 0 || !commonOwned[0]) {
		return len(prior)
	}
	count := 0
	for _, attempt := range prior {
		if attempt.Outcome != RouteAttemptOutcomeDeferredCapacity {
			count++
		}
	}
	return count
}

// The original Office queue or still-live attempt owns continuation. Do not
// create provider backoff, fallback, or an operator-only parked state here.
func (ss *SchedulerService) deferRunSessionAdmission(ctx context.Context, run *runmodels.Run, seq int) (bool, *routing.BlockReason, error) {
	now := time.Now().UTC()
	attempt := models.RouteAttempt{RunID: run.ID, Seq: seq, Outcome: RouteAttemptOutcomeDeferredCapacity, FinishedAt: &now}
	if err := ss.repo.UpdateRouteAttemptOutcome(ctx, &attempt); err != nil {
		return false, nil, err
	}
	ss.publishRouteAttemptAppended(ctx, run.ID, ss.hydrateAttempt(ctx, run.ID, seq, attempt))
	return false, nil, nil
}

// Keep an unchanged, not-yet-launched Office admission as one attempt. Existing
// wait history is retained, while repeated scheduler ticks add no new rows.
func (ss *SchedulerService) recordDispatchAttempt(
	ctx context.Context, run *runmodels.Run, candidate routing.Candidate,
	tier routing.Tier, tierSource string, prior []models.RouteAttempt,
) (int, error) {
	if (extractRunTaskID(run.Payload) == "" || runmodels.MatchesTaskCapacityRun(ctx, run)) && len(prior) > 0 {
		last := prior[len(prior)-1]
		if sameWaitingCandidate(last, candidate, tier, tierSource) {
			run.CurrentRouteAttemptSeq = last.Seq
			attempt := models.RouteAttempt{RunID: run.ID, Seq: last.Seq, Outcome: RouteAttemptOutcomeLaunched}
			if err := ss.repo.UpdateRouteAttemptOutcome(ctx, &attempt); err != nil {
				return 0, err
			}
			ss.publishRouteAttemptAppended(ctx, run.ID, ss.hydrateAttempt(ctx, run.ID, last.Seq, attempt))
			return last.Seq, nil
		}
	}
	return ss.recordAttemptStart(ctx, run, candidate, tier, tierSource)
}

func sameWaitingCandidate(attempt models.RouteAttempt, candidate routing.Candidate, tier routing.Tier, tierSource string) bool {
	return attempt.Outcome == RouteAttemptOutcomeDeferredCapacity &&
		attempt.ExecutionProfileID == candidate.ExecutionProfileID && attempt.ProviderID == string(candidate.ProviderID) &&
		attempt.Model == candidate.Model && attempt.Tier == string(tier) && attempt.TierSource == tierSource
}
