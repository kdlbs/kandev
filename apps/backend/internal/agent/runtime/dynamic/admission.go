package dynamic

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agent/usage"
)

// failureProfileForRoute uses the identity pinned when this generation was
// selected, rather than a freshly loaded profile or a later account reading.
func (e *Engine) failureProfileForRoute(ctx context.Context, profile Profile, sessionID string, generation int64, candidateID string) Profile {
	state, ok := e.State(sessionID)
	if !ok || state.Generation != generation || state.ExecutionProfileID != candidateID || state.LogicalProfileID != profile.ID {
		return profile
	}
	profile.Candidates = append([]Candidate(nil), profile.Candidates...)
	for i := range profile.Candidates {
		if profile.Candidates[i].ID == candidateID {
			profile.Candidates[i].UsagePolicy = usage.CloneExecutionUsagePolicy(state.UsagePolicy)
			policy := profile.Candidates[i].UsagePolicy
			if policy != nil && policy.ProfileID == candidateID && usage.MatchesSelectedUsage(ctx, policy) && profile.Candidates[i].ModelID != policy.ExpectedModel {
				profile.Candidates[i].ModelID = policy.ExpectedModel
				profile.Candidates[i].CatalogFree = false
			}
			break
		}
	}
	return profile
}

// LimitVerdict is the mutable account capacity of one candidate at a new turn.
// Empty Reason means the candidate may be admitted.
type LimitVerdict struct {
	Reason  string
	RetryAt time.Time
}

// LimitChecker rechecks configured provider allowances at selection time.
// Implementations must preserve candidate order: a blocked candidate is skipped
// for this decision, never reordered or replaced by another model.
type LimitChecker interface {
	CheckLimit(context.Context, Candidate, string, time.Time) (LimitVerdict, error)
}

// LimitObserver records a classified provider limit against the candidate's
// account. It never changes the routing decision.
type LimitObserver interface {
	ObserveLimit(context.Context, Candidate, *routingerr.Error, time.Time)
}

// LimitSuspension names the scope established by account evidence. Known
// quota scopes also narrow short backoffs without assigning unknown resets;
// other provider-reported reset hints keep the upstream binding-wide behavior.
type LimitSuspension int

const (
	// SuspensionBinding is the upstream binding-wide behavior.
	SuspensionBinding LimitSuspension = iota
	// SuspensionProvider covers one provider on one credential binding.
	SuspensionProvider
	// SuspensionModel covers one model on one credential binding.
	SuspensionModel
)

// LimitFailureResolver refines a classified limit failure with account evidence
// (for example a provider key counter that separates a minute block from the
// daily reset) and reports the suspension scope its hint belongs to. A nil
// result keeps the original failure.
type LimitFailureResolver interface {
	ResolveLimitFailure(context.Context, Candidate, *routingerr.Error) (*routingerr.Error, LimitSuspension)
}

// SetLimitChecker installs the allowance checker. A nil checker disables
// configured allowance admission without changing existing circuits or blocks.
func (e *Engine) SetLimitChecker(checker LimitChecker) { e.limits = checker }

// SetLimitObserver installs classified-limit observation.
func (e *Engine) SetLimitObserver(observer LimitObserver) { e.limitObserver = observer }

// SetLimitFailureResolver installs classified-limit refinement.
func (e *Engine) SetLimitFailureResolver(resolver LimitFailureResolver) { e.limitFailure = resolver }

// blockedByLimit reports the earliest reset when the candidate's configured
// account allowance is exhausted. A remote candidate is never checked against
// the host's account.
func (e *Engine) blockedByLimit(ctx context.Context, candidate Candidate, sessionID string, now time.Time) (time.Time, bool) {
	if e == nil || e.limits == nil || candidate.RemoteExecution {
		return time.Time{}, false
	}
	verdict, err := e.limits.CheckLimit(ctx, candidate, sessionID, now)
	if err != nil || verdict.Reason == "" || !verdict.RetryAt.After(now) {
		return time.Time{}, false
	}
	return verdict.RetryAt, true
}

func (e *Engine) resolveLimitFailure(
	ctx context.Context,
	candidate Candidate,
	failure *routingerr.Error,
) (*routingerr.Error, LimitSuspension) {
	if e == nil || e.limitFailure == nil || failure == nil {
		return failure, SuspensionBinding
	}
	resolved, scope := e.limitFailure.ResolveLimitFailure(ctx, candidate, failure)
	if resolved == nil {
		return failure, scope
	}
	return resolved, scope
}

func (e *Engine) observeLimitFailure(
	ctx context.Context,
	profile Profile,
	candidateID string,
	failure *routingerr.Error,
) {
	if e == nil || e.limitObserver == nil || failure == nil {
		return
	}
	candidate, ok := candidateByID(profile, candidateID)
	if !ok {
		return
	}
	e.limitObserver.ObserveLimit(ctx, candidate, failure, e.now())
}
