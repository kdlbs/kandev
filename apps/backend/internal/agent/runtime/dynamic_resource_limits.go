package runtime

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	agentusage "github.com/kandev/kandev/internal/agent/usage"
)

// ProviderLimitReader supplies the operator-entered provider limit settings.
type ProviderLimitReader interface {
	ListProviderLimits(ctx context.Context) ([]dynamic.ProviderLimit, error)
}

// SetProviderLimitReader wires the operator-entered provider blocks into
// candidate resolution.
func (r *ProfileExecutionResolver) SetProviderLimitReader(reader ProviderLimitReader) {
	r.providerLimits = reader
}

// SetLimitChecker installs configured provider allowance admission.
func (r *ProfileExecutionResolver) SetLimitChecker(checker dynamic.LimitChecker) {
	if r != nil && r.engine != nil {
		r.engine.SetLimitChecker(checker)
	}
}

// SetLimitObserver installs classified-limit observation.
func (r *ProfileExecutionResolver) SetLimitObserver(observer dynamic.LimitObserver) {
	if r != nil && r.engine != nil {
		r.engine.SetLimitObserver(observer)
	}
}

// SetLimitFailureResolver installs classified-limit refinement.
func (r *ProfileExecutionResolver) SetLimitFailureResolver(resolver dynamic.LimitFailureResolver) {
	if r != nil && r.engine != nil {
		r.engine.SetLimitFailureResolver(resolver)
	}
}

// providerLimitMap loads the provider settings keyed by provider. A read
// failure answers no settings: an operator block is an addition to failure
// suspensions, so losing it never makes a suspended candidate selectable.
func (r *ProfileExecutionResolver) providerLimitMap(ctx context.Context) map[string]dynamic.ProviderLimit {
	if r.providerLimits == nil {
		return nil
	}
	limits, err := r.providerLimits.ListProviderLimits(ctx)
	if err != nil {
		return nil
	}
	byProvider := make(map[string]dynamic.ProviderLimit, len(limits))
	for _, limit := range limits {
		byProvider[dynamic.NormalizeProvider(limit.Provider)] = limit
	}
	return byProvider
}

// applyResourceLimits gives each enabled candidate its model circuit key and
// the operator block covering its provider.
func (r *ProfileExecutionResolver) applyResourceLimits(ctx context.Context, candidates []dynamic.Candidate) {
	limits := r.providerLimitMap(ctx)
	now := time.Now()
	for index := range candidates {
		candidate := &candidates[index]
		if !candidate.Enabled || candidate.ModelID == "" {
			continue
		}
		if free, known := agentusage.CatalogFreeModel(candidate.ModelID); known && free {
			candidate.CatalogFree = true
		}
		if limit, ok := limits[dynamic.ProviderOf(candidate.ModelID)]; ok {
			if until, blocked := limit.BlockedUntilCandidate(*candidate, now); blocked {
				candidate.SuspendedUntil = until
			}
		}
	}
}
