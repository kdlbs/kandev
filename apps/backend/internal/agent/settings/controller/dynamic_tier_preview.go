package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

var ErrDynamicTierPreviewDisabled = errors.New("dynamic tier preview is disabled")
var ErrDynamicTierPreviewUnavailable = errors.New("dynamic tier preview is unavailable")

type DynamicTierPreviewer interface {
	PreviewDynamicTiers(context.Context, string, int64, []models.DynamicAgentRoute) (dynamic.TierPreview, error)
}

func (c *Controller) SetDynamicTierPreviewer(previewer DynamicTierPreviewer) {
	c.dynamicTierPreviewer = previewer
}
func (c *Controller) DynamicTiersEnabled() bool {
	return c.dynamicProfileTiersEnabled && c.dynamicAgentRoutingEnabled
}

// PreviewDynamicTiers validates a draft using the existing route codec but never saves it.
func (c *Controller) PreviewDynamicTiers(ctx context.Context, profileID string, document *dto.DynamicAgentProfileDTO) (*dynamic.TierPreview, error) {
	if !c.DynamicTiersEnabled() {
		return nil, ErrDynamicTierPreviewDisabled
	}
	if c.dynamicTierPreviewer == nil {
		return nil, ErrDynamicTierPreviewUnavailable
	}
	if document == nil {
		repo, err := c.dynamicProfileRepository()
		if err != nil {
			return nil, err
		}
		config, routes, err := repo.GetDynamicAgentProfile(ctx, profileID)
		if err != nil {
			return nil, err
		}
		document, err = dynamicProfileDTO(config, routes)
		if err != nil {
			return nil, err
		}
	}
	if document == nil {
		return nil, ErrDynamicProfileCandidatesRequired
	}
	if len(document.Candidates) == 0 {
		return &dynamic.TierPreview{State: "no_candidates", Considered: []dynamic.TierPreviewEntry{}}, nil
	}
	routes, err := c.validateDynamicCandidates(ctx, profileID, document)
	if err != nil {
		return nil, err
	}
	result, err := c.dynamicTierPreviewer.PreviewDynamicTiers(ctx, profileID, document.Version, routes)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDynamicProfileRule, err)
	}
	return &result, nil
}
