package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// inboxProviderDegradedType is the InboxItem.Type the frontend reads to
// render a provider-degraded row.
const inboxProviderDegradedType = "provider_degraded"

// Recommended-action constants matched on the frontend.
const (
	suggestedActionReconnect       = "reconnect"
	suggestedActionConfigure       = "configure"
	suggestedActionWaitForCapacity = "wait_for_capacity"
)

// inboxProviderDegradedItems surfaces one InboxItem per non-healthy
// provider health row in the workspace. Each item lists the affected
// agent ids so the UI can show "blocks agents X, Y, Z" without
// requiring a second round-trip. No-op when no RoutingProvider is
// wired (the routing feature is then disabled for this deployment).
func (s *DashboardService) inboxProviderDegradedItems(
	ctx context.Context, workspaceID string,
) ([]*models.InboxItem, error) {
	if s.routingProvider == nil {
		return nil, nil
	}
	health, err := s.routingProvider.Health(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(health) == 0 {
		return nil, nil
	}
	affected, err := s.collectAffectedAgents(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	items := make([]*models.InboxItem, 0, len(health))
	for _, h := range health {
		items = append(items, buildProviderDegradedItem(h, affected))
	}
	return items, nil
}

// collectAffectedAgents runs the per-agent preview once for the
// workspace and groups agent ids by (provider, scope, scope_value)
// so the per-health-row lookup is O(1) per row.
func (s *DashboardService) collectAffectedAgents(
	ctx context.Context, workspaceID string,
) (map[providerScopeKey][]string, error) {
	out := map[providerScopeKey][]string{}
	if s.routingProvider == nil {
		return out, nil
	}
	previews, err := s.routingProvider.Preview(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	for _, p := range previews {
		appendAffectedForPreview(out, p)
	}
	return out, nil
}

// providerScopeKey is the lookup key for the affected-agents map. v1
// only groups by provider id — the scope+scope_value pair is surfaced
// from the ProviderHealth row itself when the inbox item renders.
type providerScopeKey struct {
	provider string
}

// appendAffectedForPreview appends the agent id to every degraded
// scope key the preview's primary provider currently touches. Today
// the preview surfaces "Degraded" as a boolean; we attach the agent
// to the provider scope unconditionally when the flag is set so the
// inbox row at least lists who is affected. Tier/model scopes flow
// through the same boolean — the resolver records them as skipped
// reasons under SkippedDegraded with the original scope, so the UI
// receiving the inbox item already has the canonical scope from the
// provider-health row itself.
func appendAffectedForPreview(out map[providerScopeKey][]string, p routing.PreviewItem) {
	if !p.Degraded {
		return
	}
	if p.PrimaryProviderID == "" {
		return
	}
	key := providerScopeKey{provider: p.PrimaryProviderID}
	out[key] = append(out[key], p.AgentID)
}

// buildProviderDegradedItem assembles one InboxItem for a non-healthy
// health row. CreatedAt uses last_failure (falling back to updated_at)
// so the inbox sort treats the most recent failure as newest.
func buildProviderDegradedItem(
	h models.ProviderHealth, affected map[providerScopeKey][]string,
) *models.InboxItem {
	agentIDs := affected[providerScopeKey{provider: h.ProviderID}]
	createdAt := h.UpdatedAt
	if h.LastFailure != nil && !h.LastFailure.IsZero() {
		createdAt = *h.LastFailure
	}
	retryAt := ""
	if h.RetryAt != nil && !h.RetryAt.IsZero() {
		retryAt = h.RetryAt.UTC().Format(time.RFC3339)
	}
	return &models.InboxItem{
		ID:          providerDegradedItemID(h),
		Type:        inboxProviderDegradedType,
		Title:       providerDegradedTitle(h),
		Description: truncateInboxDescription(h.RawExcerpt),
		Status:      string(h.State),
		EntityID:    h.ProviderID,
		EntityType:  "provider",
		Payload: map[string]interface{}{
			"workspace_id":       h.WorkspaceID,
			"provider_id":        h.ProviderID,
			"scope":              h.Scope,
			"scope_value":        h.ScopeValue,
			"error_code":         h.ErrorCode,
			"retry_at":           retryAt,
			"raw_excerpt":        h.RawExcerpt,
			"affected_agent_ids": agentIDs,
			"action":             suggestedActionForCode(h.ErrorCode),
		},
		CreatedAt: createdAt,
	}
}

// providerDegradedItemID derives a stable id for a degraded row so the
// inbox can dedupe and the UI can key on it.
func providerDegradedItemID(h models.ProviderHealth) string {
	return fmt.Sprintf("provider-degraded:%s:%s:%s:%s",
		h.WorkspaceID, h.ProviderID, h.Scope, h.ScopeValue)
}

// providerDegradedTitle is the headline shown on the inbox row.
func providerDegradedTitle(h models.ProviderHealth) string {
	if h.ScopeValue != "" {
		return fmt.Sprintf("%s (%s %s) degraded",
			h.ProviderID, h.Scope, h.ScopeValue)
	}
	return fmt.Sprintf("%s degraded", h.ProviderID)
}

// suggestedActionForCode maps a normalized routing error code to the
// recommended next-step the UI should surface.
func suggestedActionForCode(code string) string {
	switch code {
	case "auth_required", "missing_credentials":
		return suggestedActionReconnect
	case "subscription_required", "provider_not_configured", "model_unavailable":
		return suggestedActionConfigure
	case "quota_limited", "rate_limited", "provider_unavailable", "unknown_provider_error":
		return suggestedActionWaitForCapacity
	}
	return suggestedActionWaitForCapacity
}
