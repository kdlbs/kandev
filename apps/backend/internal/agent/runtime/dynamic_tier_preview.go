package runtime

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

// PreviewDynamicTiers uses native binding/availability reads, without a route or probe claim.
func (r *ProfileExecutionResolver) PreviewDynamicTiers(ctx context.Context, profileID string, version int64, routes []settingsmodels.DynamicAgentRoute) (dynamic.TierPreview, error) {
	if !r.OrderedTiersEnabled() {
		return dynamic.TierPreview{}, ErrDynamicRoutingDisabled
	}
	if r.engine == nil {
		return dynamic.TierPreview{}, errors.New("dynamic preview unavailable")
	}
	profile, err := r.dynamicProfileFromRoutes(ctx, profileID, version, routes)
	if err != nil {
		return dynamic.TierPreview{}, err
	}
	return r.engine.PreviewTiers(profile)
}
