package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// seedWorkspaceWithFrontierTier writes a workspace routing config that
// only maps Balanced (no Frontier). Used by the tier-validation tests.
func seedWorkspaceWithBalancedOnly(t *testing.T, svc *AgentService, workspaceID string) {
	t.Helper()
	cfg := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet"}},
			"codex-acp":  {TierMap: routing.TierMap{Balanced: "gpt-5"}},
		},
	}
	if err := svc.repo.UpsertWorkspaceRouting(context.Background(), workspaceID, cfg); err != nil {
		t.Fatalf("seed routing: %v", err)
	}
}

func newPatchAgentRecorder(
	t *testing.T, svc *AgentService, agentID string, bodyJSON string,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	RegisterRoutes(group, svc, logger.Default())

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/agents/"+agentID,
		bytes.NewBufferString(bodyJSON),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// TestUpdateAgent_RejectsTierOverrideWithNoProviderMapping pins the
// save-time guardrail: PATCH /agents/:id with a tier override that no
// provider in the workspace has mapped must return 400 with a structured
// per-field error, not silently persist a broken override.
func TestUpdateAgent_RejectsTierOverrideWithNoProviderMapping(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	seedWorkspaceWithBalancedOnly(t, svc, "ws-1")

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Worker",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	body := `{"routing":{"tier_source":"override","tier":"frontier"}}`
	rec := newPatchAgentRecorder(t, svc, agent.ID, body)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	bodyBytes, _ := io.ReadAll(rec.Body)
	var resp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["field"] != "routing.tier" {
		t.Errorf("response field = %v, want routing.tier", resp["field"])
	}
	if !strings.Contains(asString(resp["error"]), "frontier") {
		t.Errorf("response error = %v, want frontier in message", resp["error"])
	}

	stored, err := svc.GetAgentFromConfig(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if strings.Contains(stored.Settings, "frontier") {
		t.Errorf("settings persisted despite 400: %s", stored.Settings)
	}
}

// TestUpdateAgent_AcceptsTierOverrideWhenMapped is the happy-path
// counterpart: when the workspace does map the tier, the override saves.
func TestUpdateAgent_AcceptsTierOverrideWhenMapped(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()

	cfg := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Frontier: "opus", Balanced: "sonnet"}},
		},
	}
	if err := svc.repo.UpsertWorkspaceRouting(ctx, "ws-1", cfg); err != nil {
		t.Fatalf("seed routing: %v", err)
	}

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Worker",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	body := `{"routing":{"tier_source":"override","tier":"frontier"}}`
	rec := newPatchAgentRecorder(t, svc, agent.ID, body)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	stored, err := svc.GetAgentFromConfig(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if !strings.Contains(stored.Settings, "frontier") {
		t.Errorf("override not persisted: %s", stored.Settings)
	}
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

// TestAgentAuthMiddleware_RejectsCrossWorkspaceToken ensures a token minted
// for workspace A cannot access endpoints scoped to workspace B via the
// :wsId path parameter. Without this check, an agent in one workspace could
// enumerate or mutate resources in any other workspace on the same backend.
func TestAgentAuthMiddleware_RejectsCrossWorkspaceToken(t *testing.T) {
	svc, _ := newTestAgentService(t)
	svc.SetAuth(NewAgentAuth("test-signing-key"))
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Worker",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	token, err := svc.auth.MintAgentJWT(agent.ID, "task-1", "ws-1", "sess-1")
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	group.Use(AgentAuthMiddleware(svc))
	RegisterRoutes(group, svc, logger.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws-2/agents", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rec.Code, rec.Body.String())
	}
}
