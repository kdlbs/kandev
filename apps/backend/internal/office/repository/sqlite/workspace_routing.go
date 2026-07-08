package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/routing"
)

// RoutingTierProfileReference identifies one workspace routing tier that was
// authored from a source agent profile.
type RoutingTierProfileReference struct {
	WorkspaceID string
	ProviderID  routing.ProviderID
	Tier        routing.Tier
}

// GetWorkspaceRouting returns the workspace's routing config. When no
// row exists yet, the returned config carries the spec defaults:
// Enabled=false, DefaultTier=balanced, empty order and profile map.
// This keeps callers (resolver, HTTP) from having to special-case the
// "never configured" workspace.
func (r *Repository) GetWorkspaceRouting(
	ctx context.Context, workspaceID string,
) (*routing.WorkspaceConfig, error) {
	var (
		enabled    int
		tier       string
		orderRaw   string
		profsRaw   string
		reasonsRaw string
		updatedAt  time.Time
	)
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT enabled, default_tier, provider_order, provider_profiles,
			COALESCE(tier_per_reason, '{}'), updated_at
		FROM office_workspace_routing
		WHERE workspace_id = ?
	`), workspaceID).Scan(&enabled, &tier, &orderRaw, &profsRaw, &reasonsRaw, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultWorkspaceRouting(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("routing: get workspace %q: %w", workspaceID, err)
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:     enabled != 0,
		DefaultTier: routing.Tier(tier),
	}
	if err := json.Unmarshal([]byte(orderRaw), &cfg.ProviderOrder); err != nil {
		return nil, fmt.Errorf("routing: decode order: %w", err)
	}
	if err := json.Unmarshal([]byte(profsRaw), &cfg.ProviderProfiles); err != nil {
		return nil, fmt.Errorf("routing: decode profiles: %w", err)
	}
	if err := json.Unmarshal([]byte(reasonsRaw), &cfg.TierPerReason); err != nil {
		return nil, fmt.Errorf("routing: decode tier_per_reason: %w", err)
	}
	if cfg.ProviderProfiles == nil {
		cfg.ProviderProfiles = map[routing.ProviderID]routing.ProviderProfile{}
	}
	return cfg, nil
}

// UpsertWorkspaceRouting persists cfg for workspaceID, replacing any
// previously stored row. JSON marshaling errors are surfaced before
// touching the DB so a malformed config can never leave a half-written
// row behind.
func (r *Repository) UpsertWorkspaceRouting(
	ctx context.Context, workspaceID string, cfg *routing.WorkspaceConfig,
) error {
	if cfg == nil {
		return fmt.Errorf("routing: nil config for workspace %q", workspaceID)
	}
	orderRaw, err := marshalOrder(cfg.ProviderOrder)
	if err != nil {
		return err
	}
	profsRaw, err := marshalProfiles(cfg.ProviderProfiles)
	if err != nil {
		return err
	}
	reasonsRaw, err := marshalTierPerReason(cfg.TierPerReason)
	if err != nil {
		return err
	}
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	tier := string(cfg.DefaultTier)
	if tier == "" {
		tier = string(routing.TierBalanced)
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_workspace_routing
			(workspace_id, enabled, default_tier, provider_order, provider_profiles, tier_per_reason, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			enabled = excluded.enabled,
			default_tier = excluded.default_tier,
			provider_order = excluded.provider_order,
			provider_profiles = excluded.provider_profiles,
			tier_per_reason = excluded.tier_per_reason,
			updated_at = excluded.updated_at
	`), workspaceID, enabled, tier, orderRaw, profsRaw, reasonsRaw, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("routing: upsert workspace %q: %w", workspaceID, err)
	}
	return nil
}

// defaultWorkspaceRouting returns the spec-mandated empty config used
// when no row exists yet.
func defaultWorkspaceRouting() *routing.WorkspaceConfig {
	return &routing.WorkspaceConfig{
		Enabled:          false,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    []routing.ProviderID{},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{},
		TierPerReason:    routing.TierPerReason{},
	}
}

// ListRoutingTierReferencesByAgentProfile returns every workspace tier mapping
// that records profileID as its source. Used by profile deletion to avoid
// orphaning a tier/profile association.
func (r *Repository) ListRoutingTierReferencesByAgentProfile(
	ctx context.Context, profileID string,
) ([]RoutingTierProfileReference, error) {
	if profileID == "" {
		return nil, nil
	}
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`
		SELECT workspace_id, provider_order, provider_profiles
		FROM office_workspace_routing
		WHERE provider_profiles LIKE ?
	`), "%"+profileID+"%")
	if err != nil {
		return nil, fmt.Errorf("routing: list tier profile refs: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var refs []RoutingTierProfileReference
	for rows.Next() {
		var workspaceID, orderRaw, profsRaw string
		if err := rows.Scan(&workspaceID, &orderRaw, &profsRaw); err != nil {
			return nil, fmt.Errorf("routing: scan tier profile refs: %w", err)
		}
		var order []routing.ProviderID
		if err := json.Unmarshal([]byte(orderRaw), &order); err != nil {
			return nil, fmt.Errorf("routing: decode tier profile order refs: %w", err)
		}
		profiles := map[routing.ProviderID]routing.ProviderProfile{}
		if err := json.Unmarshal([]byte(profsRaw), &profiles); err != nil {
			return nil, fmt.Errorf("routing: decode tier profile refs: %w", err)
		}
		for _, providerID := range order {
			profile, ok := profiles[providerID]
			if !ok {
				continue
			}
			refs = appendTierProfileReference(refs, workspaceID, providerID, routing.TierFrontier, profile.TierProfileIDs.Frontier, profileID)
			refs = appendTierProfileReference(refs, workspaceID, providerID, routing.TierBalanced, profile.TierProfileIDs.Balanced, profileID)
			refs = appendTierProfileReference(refs, workspaceID, providerID, routing.TierEconomy, profile.TierProfileIDs.Economy, profileID)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routing: iterate tier profile refs: %w", err)
	}
	return refs, nil
}

func appendTierProfileReference(
	refs []RoutingTierProfileReference,
	workspaceID string,
	providerID routing.ProviderID,
	tier routing.Tier,
	gotProfileID string,
	wantProfileID string,
) []RoutingTierProfileReference {
	if gotProfileID != wantProfileID {
		return refs
	}
	return append(refs, RoutingTierProfileReference{
		WorkspaceID: workspaceID,
		ProviderID:  providerID,
		Tier:        tier,
	})
}

// marshalTierPerReason marshals the wake-reason tier policy, normalising
// nil to "{}" to satisfy the column's NOT NULL invariant.
func marshalTierPerReason(m routing.TierPerReason) (string, error) {
	if m == nil {
		m = routing.TierPerReason{}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("routing: marshal tier_per_reason: %w", err)
	}
	return string(b), nil
}

// marshalOrder marshals the provider order, normalising nil to "[]"
// so the column's NOT NULL default invariant is preserved.
func marshalOrder(order []routing.ProviderID) (string, error) {
	if order == nil {
		order = []routing.ProviderID{}
	}
	b, err := json.Marshal(order)
	if err != nil {
		return "", fmt.Errorf("routing: marshal order: %w", err)
	}
	return string(b), nil
}

// marshalProfiles marshals the per-provider profile map, normalising
// nil to "{}" for the same reason as marshalOrder.
func marshalProfiles(profs map[routing.ProviderID]routing.ProviderProfile) (string, error) {
	if profs == nil {
		profs = map[routing.ProviderID]routing.ProviderProfile{}
	}
	b, err := json.Marshal(profs)
	if err != nil {
		return "", fmt.Errorf("routing: marshal profiles: %w", err)
	}
	return string(b), nil
}
