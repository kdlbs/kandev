package dashboard

import (
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// RoutingConfigResponse is the GET /workspaces/:wsId/routing payload.
// The known-provider list is returned alongside the config so the UI
// can render the catalogue without a second round-trip.
type RoutingConfigResponse struct {
	Config         *routing.WorkspaceConfig `json:"config"`
	KnownProviders []routing.ProviderID     `json:"known_providers"`
}

// RoutingRetryResponse is the POST /workspaces/:wsId/routing/retry payload.
// Status is "probed" when a registered ProviderProber ran synchronously,
// or "retrying" when no prober is registered and the retry slot will be
// claimed by the next dispatch.
type RoutingRetryResponse struct {
	Status  string  `json:"status"`
	RetryAt *string `json:"retry_at,omitempty"`
}

// RoutingHealthResponse wraps the list of non-healthy provider rows.
type RoutingHealthResponse struct {
	Health []models.ProviderHealth `json:"health"`
}

// AgentRoutePreview is one row in the workspace routing preview table.
// TierSource is "inherit" when the agent uses the workspace default and
// "override" when its override blob sets a tier explicitly.
//
// PrimaryProviderID/PrimaryModel reflect the configured intent (first
// entry in the effective provider order). CurrentProviderID/
// CurrentModel reflect the candidate the next launch would actually
// pick; equal to primary when not degraded. Both Current fields are
// empty when every candidate is skipped.
type AgentRoutePreview struct {
	AgentID           string              `json:"agent_id"`
	AgentName         string              `json:"agent_name"`
	TierSource        string              `json:"tier_source"`
	EffectiveTier     string              `json:"effective_tier"`
	PrimaryProviderID string              `json:"primary_provider_id,omitempty"`
	PrimaryModel      string              `json:"primary_model,omitempty"`
	CurrentProviderID string              `json:"current_provider_id,omitempty"`
	CurrentModel      string              `json:"current_model,omitempty"`
	FallbackChain     []ProviderModelPair `json:"fallback_chain"`
	Missing           []string            `json:"missing"`
	Degraded          bool                `json:"degraded"`
}

// ProviderModelPair is one (provider, model, tier) triple used to render
// an agent's fallback chain.
type ProviderModelPair struct {
	ProviderID string `json:"provider_id"`
	Model      string `json:"model"`
	Tier       string `json:"tier"`
}

// RoutingPreviewResponse wraps the workspace-level preview list.
type RoutingPreviewResponse struct {
	Agents []AgentRoutePreview `json:"agents"`
}

// RouteAttemptDTO mirrors models.RouteAttempt with stable JSON tags for
// the dashboard run-detail UI. Timestamps are formatted as RFC3339.
type RouteAttemptDTO struct {
	Seq             int     `json:"seq"`
	ProviderID      string  `json:"provider_id"`
	Model           string  `json:"model,omitempty"`
	Tier            string  `json:"tier"`
	Outcome         string  `json:"outcome"`
	ErrorCode       string  `json:"error_code,omitempty"`
	ErrorConfidence string  `json:"error_confidence,omitempty"`
	AdapterPhase    string  `json:"adapter_phase,omitempty"`
	ClassifierRule  string  `json:"classifier_rule,omitempty"`
	ExitCode        *int    `json:"exit_code,omitempty"`
	RawExcerpt      string  `json:"raw_excerpt,omitempty"`
	ResetHint       *string `json:"reset_hint,omitempty"`
	StartedAt       string  `json:"started_at"`
	FinishedAt      *string `json:"finished_at,omitempty"`
}

// RunRouting is embedded on the run-detail response so the routing
// metadata travels alongside the existing run fields. Empty
// (zero-valued) when the run did not go through the routing path.
type RunRouting struct {
	LogicalProviderOrder []string          `json:"logical_provider_order"`
	RequestedTier        string            `json:"requested_tier,omitempty"`
	ResolvedProviderID   string            `json:"resolved_provider_id,omitempty"`
	ResolvedModel        string            `json:"resolved_model,omitempty"`
	BlockedStatus        string            `json:"blocked_status,omitempty"`
	EarliestRetryAt      *string           `json:"earliest_retry_at,omitempty"`
	Attempts             []RouteAttemptDTO `json:"attempts"`
}

// RouteAttemptsResponse is the GET /runs/:id/attempts payload. The UI
// reads this when a WS route_attempt_appended event arrives.
type RouteAttemptsResponse struct {
	Attempts []RouteAttemptDTO `json:"attempts"`
}

// AgentRouteResponse is the GET /agents/:id/route payload. It mirrors
// AgentRoutePreview plus the persisted override blob (so the agent
// routing UI can hydrate from the response on first paint instead of
// defaulting every toggle to "inherit") and the last classifier verdict
// (if any) plus the run id of the most recent attempt the verdict came
// from.
type AgentRouteResponse struct {
	Preview         AgentRoutePreview      `json:"preview"`
	Overrides       routing.AgentOverrides `json:"overrides"`
	LastFailureCode string                 `json:"last_failure_code,omitempty"`
	LastFailureRun  string                 `json:"last_failure_run,omitempty"`
}

// routeAttemptToDTO converts the model into the JSON-shaped DTO.
func routeAttemptToDTO(a models.RouteAttempt) RouteAttemptDTO {
	dto := RouteAttemptDTO{
		Seq:             a.Seq,
		ProviderID:      a.ProviderID,
		Model:           a.Model,
		Tier:            a.Tier,
		Outcome:         string(a.Outcome),
		ErrorCode:       a.ErrorCode,
		ErrorConfidence: string(a.ErrorConfidence),
		AdapterPhase:    string(a.AdapterPhase),
		ClassifierRule:  a.ClassifierRule,
		ExitCode:        a.ExitCode,
		RawExcerpt:      a.RawExcerpt,
		StartedAt:       a.StartedAt.UTC().Format(time.RFC3339),
	}
	if a.FinishedAt != nil && !a.FinishedAt.IsZero() {
		s := a.FinishedAt.UTC().Format(time.RFC3339)
		dto.FinishedAt = &s
	}
	if a.ResetHint != nil && !a.ResetHint.IsZero() {
		s := a.ResetHint.UTC().Format(time.RFC3339)
		dto.ResetHint = &s
	}
	return dto
}
