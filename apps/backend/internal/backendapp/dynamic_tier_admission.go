package backendapp

import (
	"context"
	"time"

	"github.com/google/uuid"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// tierLimitAdapter maps the provider tier service onto the dynamic routing
// allowance seams.
type tierLimitAdapter struct {
	usage    *usageProviderAdapter
	calendar *dynamicLimitCalendar
}

func newTierLimitAdapter(usage *usageProviderAdapter, calendar *dynamicLimitCalendar) *tierLimitAdapter {
	return &tierLimitAdapter{usage: usage, calendar: calendar}
}

func installTierLimitAdapter(resolver *agentruntime.ProfileExecutionResolver, usage *usageProviderAdapter, calendar *dynamicLimitCalendar, reader agentruntime.ProviderLimitReader) {
	if resolver == nil {
		return
	}
	limiter := newTierLimitAdapter(usage, calendar)
	resolver.SetLimitChecker(limiter)
	resolver.SetLimitObserver(limiter)
	resolver.SetLimitFailureResolver(limiter)
	resolver.SetProviderLimitReader(reader)
}

// CheckLimit reports the first exhausted configured allowance.
func (a *tierLimitAdapter) CheckLimit(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
	sessionID string,
	now time.Time,
) (dynamicruntime.LimitVerdict, error) {
	if a == nil || a.usage == nil || a.usage.tierLimits == nil || candidate.RemoteExecution {
		return dynamicruntime.LimitVerdict{}, nil
	}
	verdict, err := a.usage.tierLimits.CheckCandidate(ctx, candidate, sessionID, now)
	return dynamicruntime.LimitVerdict{Reason: verdict.Reason, RetryAt: verdict.RetryAt}, err
}

// ResolveLimitFailure refines a classified limit with account evidence.
func (a *tierLimitAdapter) ResolveLimitFailure(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
	failure *routingerr.Error,
) (*routingerr.Error, dynamicruntime.LimitSuspension) {
	if a == nil || a.usage == nil || a.usage.tierLimits == nil {
		return failure, dynamicruntime.SuspensionBinding
	}
	if candidate.UsagePolicy != nil && candidate.UsagePolicy.ProfileID == candidate.ID &&
		agentusage.MatchesSelectedUsage(ctx, candidate.UsagePolicy) && candidate.ModelID != candidate.UsagePolicy.ExpectedModel {
		candidate.ModelID = candidate.UsagePolicy.ExpectedModel
		candidate.CatalogFree = false
		if catalog := a.usage.tierLimits.freeModels; catalog != nil {
			free, known := catalog.IsFreeModel(ctx, candidate.ModelID)
			candidate.CatalogFree = known && free
		}
	}
	if !a.usage.tierLimits.matchesSelectedAccount(ctx, candidate) {
		return failure, dynamicruntime.SuspensionBinding
	}
	resolved := a.usage.tierLimits.resolveFailure(ctx, candidate, failure)
	if resolved == nil {
		resolved = failure
	}
	scope := dynamicruntime.SuspensionBinding
	if resolved != failure {
		// The OpenRouter key counter separates a minute block from the
		// account's daily free allowance. A minute block is account scope; a
		// free daily exhaustion suspends only the failing model.
		scope = dynamicruntime.SuspensionProvider
		if resolved.ClassifierRule == "account_limit.daily" &&
			(dynamicruntime.IsFreeModel(candidate.ModelID) || candidate.CatalogFree) {
			scope = dynamicruntime.SuspensionModel
		}
	} else if dynamicruntime.FailureCauseFor(candidate, failure) == dynamicruntime.CauseFreeDailyLimit &&
		(dynamicruntime.IsFreeModel(candidate.ModelID) || dynamicruntime.ModelScoped(candidate.ModelID)) {
		scope = dynamicruntime.SuspensionModel
	}
	// A classified quota limit on a paid model without the provider's own
	// reset uses the operator-entered monthly reset. The monthly allowance is
	// provider-scoped.
	if reset, ok := a.monthlyReset(ctx, candidate, resolved); ok {
		copy := *resolved
		copy.ResetHint = &reset
		return &copy, dynamicruntime.SuspensionProvider
	}
	return resolved, scope
}

// monthlyReset returns the operator-entered monthly reset for a paid-model
// quota failure that did not carry the provider's own reset hint.
func (a *tierLimitAdapter) monthlyReset(ctx context.Context, candidate dynamicruntime.Candidate, failure *routingerr.Error) (time.Time, bool) {
	if failure.Code != routingerr.CodeQuotaLimited || failure.ResetHint != nil || a.calendar == nil {
		return time.Time{}, false
	}
	if dynamicruntime.IsFreeModel(candidate.ModelID) || candidate.CatalogFree {
		return time.Time{}, false
	}
	now := time.Now()
	reset, ok := a.calendar.MonthlyReset(ctx, candidate, now)
	if !ok || !reset.After(now) {
		return time.Time{}, false
	}
	return reset, true
}

// ObserveLimit records the classified limit and learns an undisclosed
// allowance. It runs off the routing path.
func (a *tierLimitAdapter) ObserveLimit(
	ctx context.Context,
	candidate dynamicruntime.Candidate,
	failure *routingerr.Error,
	at time.Time,
) {
	if a == nil || a.usage == nil || a.usage.tierLimits == nil || failure == nil {
		return
	}
	if failure.Code != routingerr.CodeRateLimited && failure.Code != routingerr.CodeQuotaLimited {
		return
	}
	ctx = agentusage.SnapshotCandidateUsage(context.WithoutCancel(ctx), candidate.ID)
	if candidate.UsagePolicy != nil {
		policy := *candidate.UsagePolicy
		candidate.UsagePolicy = &policy
	}
	// Keep the request's selected account/model evidence while allowing the
	// detached observer to finish after the originating request is canceled.
	go func() {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		observation := &sqliterepo.UsageLimitObservation{
			ID: uuid.NewString(), ExecutionProfileID: candidate.ID,
			AccountKey: a.accountKey(ctx, candidate.ID), Code: string(failure.Code), ObservedAt: at,
		}
		_ = a.usage.tierLimits.recordObservation(ctx, candidate, failure, observation)
	}()
}

func (a *tierLimitAdapter) accountKey(ctx context.Context, profileID string) string {
	if a != nil && a.usage != nil {
		if profile, agentType, ok := a.usage.loadProfile(ctx, profileID); ok {
			if binding, bound := a.usage.bindingFor(profile, agentType); bound && binding.accountKey != "" {
				return binding.accountKey
			}
		}
	}
	return "profile:" + profileID
}
