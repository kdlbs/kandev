package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/models"
	sqliterepo "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
)

// HandlePostStartFailure is the post-start fallback hook. Called by the
// office event subscriber when an AgentFailed event arrives for a run
// that was launched via routing. Classifies the error, marks the
// provider degraded if FallbackAllowed, updates the in-flight attempt,
// and re-queues the run so the next dispatch tries the next candidate.
//
// Returns (handled bool, err error). handled=true means the routing
// path took over (caller should NOT run the legacy failure escalation).
func (ss *SchedulerService) HandlePostStartFailure(
	ctx context.Context, run *models.Run, agent *models.AgentInstance, errorMessage string,
) (bool, error) {
	if ss.resolver == nil || run == nil || agent == nil {
		return false, nil
	}
	if run.ResolvedProviderID == nil || *run.ResolvedProviderID == "" {
		return false, nil
	}
	candidate, err := ss.inflightCandidate(ctx, run)
	if err != nil {
		return false, err
	}
	classified := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseStreaming,
		ProviderID: string(candidate.ProviderID),
		Stderr:     errorMessage,
	})
	if !classified.FallbackAllowed {
		return false, nil
	}
	if err := ss.applyPostStartFallback(ctx, run, agent, candidate, classified); err != nil {
		return false, err
	}
	return true, nil
}

// inflightCandidate returns the provider/model/tier for the run's
// CurrentRouteAttemptSeq row.
func (ss *SchedulerService) inflightCandidate(
	ctx context.Context, run *models.Run,
) (routing.Candidate, error) {
	attempts, err := ss.repo.ListRouteAttempts(ctx, run.ID)
	if err != nil {
		return routing.Candidate{}, err
	}
	for _, a := range attempts {
		if a.Seq == run.CurrentRouteAttemptSeq {
			return routing.Candidate{
				ProviderID: routing.ProviderID(a.ProviderID),
				Model:      a.Model,
				Tier:       routing.Tier(a.Tier),
			}, nil
		}
	}
	return routing.Candidate{}, fmt.Errorf("dispatch: no inflight attempt for seq %d",
		run.CurrentRouteAttemptSeq)
}

// applyPostStartFallback updates the attempt + health rows and re-queues
// the run. The next dispatch picks the next candidate via the resolver's
// ExcludeProviders option.
func (ss *SchedulerService) applyPostStartFallback(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, candidate routing.Candidate,
	classified *routingerr.Error,
) error {
	now := time.Now().UTC()
	if err := ss.degradeProviderForCandidate(ctx,
		agent.WorkspaceID, candidate, classified, now); err != nil {
		return err
	}
	if err := ss.finishAttempt(ctx, run.ID, run.CurrentRouteAttemptSeq,
		RouteAttemptOutcomeFailedProviderUnavail, classified, now); err != nil {
		return err
	}
	return ss.RequeueForNextCandidate(ctx, run.ID)
}

// RequeueForNextCandidate flips a claimed run back to queued so the
// next dispatch tick picks the next provider candidate. Preserves
// LogicalProviderOrder / RequestedTier / CurrentRouteAttemptSeq cursor.
func (ss *SchedulerService) RequeueForNextCandidate(
	ctx context.Context, runID string,
) error {
	return ss.repo.RequeueRunForNextCandidate(ctx, runID)
}

// MarkRunSuccessHealth flips the run's resolved provider scopes back to
// healthy. Called from the AgentCompleted subscriber to honour the
// spec's "successful prompt completion → health restored" rule.
func (ss *SchedulerService) MarkRunSuccessHealth(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
) {
	if run == nil || agent == nil {
		return
	}
	if run.ResolvedProviderID == nil || *run.ResolvedProviderID == "" {
		return
	}
	provider := *run.ResolvedProviderID
	resolvedModel := ""
	if run.ResolvedModel != nil {
		resolvedModel = *run.ResolvedModel
	}
	tier := ""
	if run.RequestedTier != nil {
		tier = *run.RequestedTier
	}
	ss.markHealthScopes(ctx, agent.WorkspaceID, routing.Candidate{
		ProviderID: routing.ProviderID(provider),
		Model:      resolvedModel,
		Tier:       routing.Tier(tier),
	})
}

// RetryProvider sets retry_at=now for the (workspace, provider) health
// rows so the next dispatch picks the provider up immediately. Also
// re-evaluates any runs waiting on this provider. Used by the HTTP
// retry endpoint (Phase 5).
func (ss *SchedulerService) RetryProvider(
	ctx context.Context, workspaceID, providerID string,
) error {
	if prober, ok := routingerr.GetProber(providerID); ok {
		if err := ss.runProberAndApply(ctx, prober, workspaceID, providerID); err != nil {
			return err
		}
	} else {
		if err := ss.repo.ForceProviderRetryNow(ctx, workspaceID, providerID); err != nil {
			return err
		}
	}
	return ss.redispatchWaitingRuns(ctx, workspaceID, providerID)
}

// runProberAndApply runs a registered prober and applies its verdict
// to the workspace health rows.
func (ss *SchedulerService) runProberAndApply(
	ctx context.Context, prober routingerr.ProviderProber,
	workspaceID, providerID string,
) error {
	probeErr := prober.Probe(ctx, routingerr.ProbeInput{
		ProviderID:  providerID,
		WorkspaceID: workspaceID,
	})
	if probeErr == nil {
		if err := ss.repo.MarkProviderHealthy(ctx,
			workspaceID, providerID, sqliterepo.HealthScopeProvider, ""); err != nil {
			return err
		}
		ss.publishProviderHealthy(ctx, workspaceID, providerID,
			sqliterepo.HealthScopeProvider, "")
		return nil
	}
	current, _ := ss.repo.GetProviderHealth(ctx,
		workspaceID, providerID, sqliterepo.HealthScopeProvider, "")
	var currentHealth models.ProviderHealth
	if current != nil {
		currentHealth = *current
	}
	now := time.Now().UTC()
	retryAt, step := routing.Schedule(currentHealth, probeErr, now)
	row := models.ProviderHealth{
		WorkspaceID: workspaceID,
		ProviderID:  providerID,
		Scope:       sqliterepo.HealthScopeProvider,
		ScopeValue:  "",
		State:       healthStateFor(probeErr),
		ErrorCode:   string(probeErr.Code),
		RetryAt:     timePtrIfNonZero(retryAt),
		BackoffStep: step,
		LastFailure: &now,
		RawExcerpt:  probeErr.RawExcerpt,
	}
	if err := ss.repo.MarkProviderDegraded(ctx, row); err != nil {
		return err
	}
	ss.publishProviderHealthChanged(ctx, row)
	return nil
}

// redispatchWaitingRuns clears routing-block status on runs that were
// parked because of the now-healthy provider. The next scheduler tick
// will re-dispatch them through dispatchWithRouting. Bumps the run's
// route-cycle baseline so the lifted cycle gets a fresh exclude-set —
// otherwise the resolver would still see every provider that failed
// in the previous cycle and immediately re-park the run.
func (ss *SchedulerService) redispatchWaitingRuns(
	ctx context.Context, workspaceID, providerID string,
) error {
	runs, err := ss.repo.ListRunsWaitingOnProvider(ctx, workspaceID, providerID)
	if err != nil {
		return err
	}
	for _, run := range runs {
		if err := ss.repo.ClearRoutingBlock(ctx, run.ID); err != nil {
			ss.logger.Warn("clear routing block failed",
				zap.String("run_id", run.ID), zap.Error(err))
			continue
		}
		if err := ss.repo.BumpRouteCycleBaseline(ctx, run.ID); err != nil {
			ss.logger.Warn("bump route cycle baseline failed",
				zap.String("run_id", run.ID), zap.Error(err))
		}
	}
	return nil
}

// LiftParkedRuns clears routing-block status on runs whose earliest_retry_at
// has passed so the next scheduler tick re-dispatches them. Called from
// the scheduler tick loop. Bumps each lifted run's route-cycle baseline
// so the previous cycle's exclusion list does not carry over — without
// this the dispatcher would re-see the same failed providers and
// immediately re-park the run.
func (ss *SchedulerService) LiftParkedRuns(ctx context.Context, now time.Time) (int, error) {
	runs, err := ss.repo.ListPendingProviderCapacityRuns(ctx, now)
	if err != nil {
		return 0, err
	}
	lifted := 0
	for _, run := range runs {
		if err := ss.repo.ClearRoutingBlock(ctx, run.ID); err != nil {
			ss.logger.Warn("lift parked run failed",
				zap.String("run_id", run.ID), zap.Error(err))
			continue
		}
		if err := ss.repo.BumpRouteCycleBaseline(ctx, run.ID); err != nil {
			ss.logger.Warn("bump route cycle baseline failed",
				zap.String("run_id", run.ID), zap.Error(err))
		}
		lifted++
	}
	return lifted, nil
}

// ErrInflightNotFound is returned by inflightCandidate when the
// CurrentRouteAttemptSeq does not match any attempt row.
var ErrInflightNotFound = errors.New("dispatch: inflight attempt not found")
