package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// fakeRoutingProvider is a hand-rolled stub for dashboard.RoutingProvider.
type fakeRoutingProvider struct {
	cfg            *routing.WorkspaceConfig
	known          []routing.ProviderID
	updateCfg      routing.WorkspaceConfig
	updateErr      error
	retryStatus    string
	retryErr       error
	healthRows     []models.ProviderHealth
	preview        []routing.PreviewItem
	previewAgent   *routing.PreviewItem
	agentOverrides routing.AgentOverrides
	overridesErr   error
}

func (f *fakeRoutingProvider) GetConfig(_ context.Context, _ string) (*routing.WorkspaceConfig, []routing.ProviderID, error) {
	return f.cfg, f.known, nil
}

func (f *fakeRoutingProvider) UpdateConfig(_ context.Context, _ string, cfg routing.WorkspaceConfig) error {
	f.updateCfg = cfg
	return f.updateErr
}

func (f *fakeRoutingProvider) Retry(_ context.Context, _, _ string) (string, *time.Time, error) {
	if f.retryErr != nil {
		return "", nil, f.retryErr
	}
	status := f.retryStatus
	if status == "" {
		status = "retrying"
	}
	return status, nil, nil
}

func (f *fakeRoutingProvider) Health(_ context.Context, _ string) ([]models.ProviderHealth, error) {
	return f.healthRows, nil
}

func (f *fakeRoutingProvider) Preview(_ context.Context, _ string) ([]routing.PreviewItem, error) {
	return f.preview, nil
}

func (f *fakeRoutingProvider) PreviewAgent(_ context.Context, _ string) (*routing.PreviewItem, error) {
	return f.previewAgent, nil
}

func (f *fakeRoutingProvider) AgentOverrides(_ context.Context, _ string) (routing.AgentOverrides, error) {
	return f.agentOverrides, f.overridesErr
}

// fakeAttemptLister stubs the route_attempts lister.
type fakeAttemptLister struct {
	byRun map[string][]models.RouteAttempt
	err   error
}

func (f *fakeAttemptLister) ListRouteAttempts(_ context.Context, runID string) ([]models.RouteAttempt, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byRun[runID], nil
}

func newRoutingTestDeps(t *testing.T) (*testDeps, *fakeRoutingProvider) {
	t.Helper()
	deps := newTestDeps(t)
	fake := &fakeRoutingProvider{
		cfg: &routing.WorkspaceConfig{
			DefaultTier:      routing.TierBalanced,
			ProviderOrder:    []routing.ProviderID{},
			ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{},
		},
		known: routing.KnownProviders(nil),
	}
	deps.svc.SetRoutingProvider(fake)
	return deps, fake
}

func TestRouting_GetReturnsDefaultsWhenNoRow(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Config == nil || resp.Config.Enabled {
		t.Fatalf("default config should be disabled, got %+v", resp.Config)
	}
	if len(resp.KnownProviders) == 0 {
		t.Fatalf("known_providers should fall back to static allow-list")
	}
}

func TestRouting_PutRejectsInvalidConfig(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.updateErr = &routing.ValidationError{
		Field:   "provider_order",
		Message: "unknown provider \"foo\"",
	}
	body, _ := json.Marshal(routing.WorkspaceConfig{
		Enabled:       false,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"foo"},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/routing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var out map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out["field"] != "provider_order" {
		t.Fatalf("expected field=provider_order, got %v", out)
	}
}

func TestRouting_PutStrictModeRejectsEmptyOrder(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.updateErr = &routing.ValidationError{
		Field:   "provider_order",
		Message: "routing is enabled but no providers are configured",
	}
	body, _ := json.Marshal(routing.WorkspaceConfig{
		Enabled:     true,
		DefaultTier: routing.TierBalanced,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/routing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRouting_PutSuccessPublishesSettingsUpdated(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	eb := bus.NewMemoryEventBus(logger.Default())
	deps.svc.SetEventBus(eb)

	var got map[string]interface{}
	if _, err := eb.Subscribe(events.OfficeRoutingSettingsUpdated, func(_ context.Context, ev *bus.Event) error {
		data, _ := ev.Data.(map[string]interface{})
		got = data
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	body, _ := json.Marshal(routing.WorkspaceConfig{
		Enabled:       false,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-7/routing", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got == nil || got["workspace_id"] != "ws-7" {
		t.Fatalf("expected workspace_id=ws-7 in event payload, got %v", got)
	}
}

func TestRouting_RetryReturnsStatus(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.retryStatus = "retrying"
	body := bytes.NewReader([]byte(`{"provider_id":"claude-acp"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/office/workspaces/ws-1/routing/retry", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingRetryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "retrying" {
		t.Fatalf("status = %q", resp.Status)
	}
}

func TestRouting_RetryRejectsMissingProviderID(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/office/workspaces/ws-1/routing/retry",
		bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRouting_PreviewComposesAgentRows(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.preview = []routing.PreviewItem{
		{
			AgentID: "a1", AgentName: "Alice",
			TierSource: "inherit", EffectiveTier: "balanced",
			PrimaryProviderID: "claude-acp", PrimaryModel: "sonnet",
			FallbackChain: []routing.PreviewProviderModel{
				{ProviderID: "codex-acp", Model: "gpt-5", Tier: "balanced"},
			},
			Missing: []string{"codex-acp: needs user action (auth_required)"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing/preview", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingPreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Agents) != 1 {
		t.Fatalf("expected 1 agent row, got %d", len(resp.Agents))
	}
	got := resp.Agents[0]
	if got.PrimaryProviderID != "claude-acp" || got.PrimaryModel != "sonnet" {
		t.Fatalf("primary = (%s,%s)", got.PrimaryProviderID, got.PrimaryModel)
	}
	if len(got.FallbackChain) != 1 || got.FallbackChain[0].ProviderID != "codex-acp" {
		t.Fatalf("fallback chain unexpected: %+v", got.FallbackChain)
	}
	if len(got.Missing) != 1 {
		t.Fatalf("missing hints = %v", got.Missing)
	}
}

func TestRouting_HealthEndpointReturnsRows(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.healthRows = []models.ProviderHealth{
		{WorkspaceID: "ws-1", ProviderID: "claude-acp", Scope: "provider",
			State: "degraded", ErrorCode: "quota_limited"},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing/health", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RoutingHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Health) != 1 || resp.Health[0].ErrorCode != "quota_limited" {
		t.Fatalf("unexpected response: %+v", resp.Health)
	}
}

func TestRouting_AttemptsEndpoint(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	lister := &fakeAttemptLister{byRun: map[string][]models.RouteAttempt{
		"run-1": {
			{RunID: "run-1", Seq: 1, ProviderID: "claude-acp",
				Outcome: "failed_provider_unavailable", ErrorCode: "quota_limited",
				StartedAt: time.Now().UTC()},
		},
	}}
	deps.svc.SetRouteAttemptLister(lister)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/runs/run-1/attempts", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.RouteAttemptsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Attempts) != 1 || resp.Attempts[0].ProviderID != "claude-acp" {
		t.Fatalf("unexpected attempts: %+v", resp.Attempts)
	}
}

func TestRouting_AttemptsEndpointMissingLister(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/runs/run-1/attempts", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRouting_AgentRouteReturns404OnMissing(t *testing.T) {
	deps, _ := newRoutingTestDeps(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/agents/missing/route", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRouting_AgentRouteSucceeds(t *testing.T) {
	deps, fake := newRoutingTestDeps(t)
	fake.previewAgent = &routing.PreviewItem{
		AgentID: "a1", AgentName: "Alice",
		TierSource: "override", EffectiveTier: "frontier",
		PrimaryProviderID: "claude-acp", PrimaryModel: "opus",
	}
	fake.agentOverrides = routing.AgentOverrides{
		TierSource: routing.TierSourceOverride,
		Tier:       routing.TierFrontier,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/agents/a1/route", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp dashboard.AgentRouteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Preview.AgentID != "a1" || resp.Preview.TierSource != "override" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.Overrides.TierSource != routing.TierSourceOverride ||
		resp.Overrides.Tier != routing.TierFrontier {
		t.Fatalf("expected overrides round-trip, got %+v", resp.Overrides)
	}
}

func TestRouting_NoProviderWired503(t *testing.T) {
	deps := newTestDeps(t)
	// no routing provider set
	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-1/routing", nil)
	rec := httptest.NewRecorder()
	deps.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Ensure ValidationError keeps the structured error path even when wrapped.
func TestRouting_WrappedValidationError(t *testing.T) {
	wrapped := errors.New("wrapped: " + (&routing.ValidationError{Field: "x", Message: "y"}).Error())
	// Should NOT unwrap to ValidationError via errors.As (different chain).
	var ve *routing.ValidationError
	if errors.As(wrapped, &ve) {
		t.Fatalf("plain errors.New should not unwrap to ValidationError")
	}
}
