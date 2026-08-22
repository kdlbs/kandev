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
			"claude-acp": {
				ExecutionProfileIDs: routing.ExecutionProfileIDs{
					Frontier: "claude-opus", Balanced: "claude-sonnet",
				},
				TierMap: routing.TierMap{Frontier: "opus", Balanced: "sonnet"},
			},
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

// newGetRecorder issues a GET against the agents router and returns the
// recorded response.
func newGetRecorder(t *testing.T, svc *AgentService, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	RegisterRoutes(group, svc, logger.Default())

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// decodeAgentMap decodes body's top-level "agent" key into a plain map, so
// tests can assert on key *presence* rather than on a typed zero value —
// AC-13c requires the "model" key to be absent, not merely empty.
func decodeAgentMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	var agent map[string]interface{}
	if err := json.Unmarshal(resp["agent"], &agent); err != nil {
		t.Fatalf("decode agent: %v", err)
	}
	return agent
}

// AC-13/AC-13b/AC-13c: the "model" key is omitted entirely (not merely
// empty) for an Office agent (role != "") across every handler that
// serialises an agentResponseBody.
func TestGetAgent_OmitsModelFieldForOfficeAgent(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rec := newGetRecorder(t, svc, "/api/v1/agents/"+agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	if _, ok := got["model"]; ok {
		t.Errorf("model key present for Office agent: %+v", got)
	}
}

// AC-13: a legacy execution-profile row (role == "") keeps the "model" key,
// even when the value itself is the empty string. Such rows predate the
// Office identity model and are constructed directly against the
// repository, bypassing CreateAgentInstance's role validation (empty role
// is not a creatable value through the service layer).
func TestGetAgent_IncludesModelFieldForExecutionProfileAgent(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{
		ID: "legacy-1", WorkspaceID: "ws-1", Name: "Legacy Profile", Model: "gpt-5",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create legacy agent: %v", err)
	}

	rec := newGetRecorder(t, svc, "/api/v1/agents/"+agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	if v, ok := got["model"]; !ok || v != "gpt-5" {
		t.Errorf("model key = %v (present=%v), want present with gpt-5", v, ok)
	}
}

// AC-13a: the shadow field on agentResponseBody must be a *string, not a
// plain string. This is the regression the type choice guards against:
// a legacy execution-profile row (role == "") with an EMPTY model value
// must still emit the "model" key (present, empty string) — this only
// passes because the shadow field's omitempty is checked against a nil
// *pointer*, not the empty string it points to. A plain `string` field
// with omitempty would silently drop this key when the value is "",
// which is exactly the regression TestGetAgent_IncludesModelFieldForExecutionProfileAgent's
// non-empty "gpt-5" fixture cannot catch.
func TestGetAgent_IncludesEmptyModelKeyForExecutionProfileAgent(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{
		ID: "legacy-empty-model", WorkspaceID: "ws-1", Name: "Legacy No Model", Model: "",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create legacy agent: %v", err)
	}

	rec := newGetRecorder(t, svc, "/api/v1/agents/"+agent.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	v, ok := got["model"]
	if !ok {
		t.Fatalf("model key absent for execution-profile agent with empty model: %+v", got)
	}
	if v != "" {
		t.Errorf("model = %v, want empty string", v)
	}
}

// AC-13b: the omission also applies to the list endpoint.
func TestListAgents_OmitsModelFieldForOfficeAgents(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rec := newGetRecorder(t, svc, "/api/v1/workspaces/ws-1/agents")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(resp["agents"], &list); err != nil {
		t.Fatalf("decode agents: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(list))
	}
	if _, ok := list[0]["model"]; ok {
		t.Errorf("model key present for Office agent in list: %+v", list[0])
	}
}

// AC-13b: create returns the same shape as get/list.
func TestCreateAgent_OmitsModelFieldForOfficeAgent(t *testing.T) {
	svc, _ := newTestAgentService(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	RegisterRoutes(group, svc, logger.Default())

	body := `{"name":"Worker","role":"worker"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-1/agents", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	if _, ok := got["model"]; ok {
		t.Errorf("model key present for created Office agent: %+v", got)
	}
}

// AC-13b: status transitions return the same shape too.
func TestUpdateAgentStatus_OmitsModelFieldForOfficeAgent(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api/v1")
	RegisterRoutes(group, svc, logger.Default())

	body := `{"status":"paused","pause_reason":"testing"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/agents/"+agent.ID+"/status", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	if _, ok := got["model"]; ok {
		t.Errorf("model key present for Office agent after status update: %+v", got)
	}
}

// AC-13b: a routine field-only PATCH also omits "model" from the response.
func TestUpdateAgent_OmitsModelFieldForOfficeAgent(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	rec := newPatchAgentRecorder(t, svc, agent.ID, `{"icon":"robot"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	got := decodeAgentMap(t, rec.Body.Bytes())
	if _, ok := got["model"]; ok {
		t.Errorf("model key present for Office agent after PATCH: %+v", got)
	}
}

// AC-14/AC-14a: PATCH rejects a "model" key for an Office agent with a
// structured 400 naming the field, regardless of the key's value shape —
// a string, an empty string, and a JSON null must all be detected
// identically since requestBodyHasKey only checks key presence.
func TestUpdateAgent_RejectsModelKeyForOfficeAgent(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()

	cases := []struct {
		name string
		body string
	}{
		{"string value", `{"model":"gpt-5"}`},
		{"empty string value", `{"model":""}`},
		{"null value", `{"model":null}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker " + tc.name, Role: models.AgentRoleWorker}
			if err := svc.CreateAgentInstance(ctx, agent); err != nil {
				t.Fatalf("create agent: %v", err)
			}

			rec := newPatchAgentRecorder(t, svc, agent.ID, tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
			}
			var resp map[string]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if resp["field"] != "model" {
				t.Errorf("response field = %v, want model", resp["field"])
			}

			stored, err := svc.GetAgentFromConfig(ctx, agent.ID)
			if err != nil {
				t.Fatalf("get agent: %v", err)
			}
			if stored.Model != "" {
				t.Errorf("model persisted despite 400: %q", stored.Model)
			}
		})
	}
}

// TestUpdateAgent_RejectsOversizedBody pins the request-body size cap on
// PATCH /agents/:id: a body larger than maxUpdateAgentBodyBytes must be
// rejected with 413 before JSON decoding runs, not buffered unbounded into
// memory.
func TestUpdateAgent_RejectsOversizedBody(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	oversized := `{"name":"` + strings.Repeat("x", maxUpdateAgentBodyBytes+1) + `"}`
	rec := newPatchAgentRecorder(t, svc, agent.ID, oversized)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body=%s", rec.Code, rec.Body.String())
	}

	stored, err := svc.GetAgentFromConfig(ctx, agent.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if stored.Name != "Worker" {
		t.Errorf("name changed despite 413: %q", stored.Name)
	}
}

// AC-14c (AC-40 test #4): the model-key rejection uses the same structured
// ValidationError shape (field + details) as applyRoutingOverride, not the
// bare {"error": "..."} shape the pre-existing agent_profile_id rejection
// uses below in the same handler — proving these are genuinely distinct
// response contracts, not incidentally similar ones.
func TestUpdateAgent_ModelRejection_UsesStructuredShapeUnlikeAgentProfileIDRejection(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()

	modelAgent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker Model", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, modelAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profileAgent := &models.AgentInstance{WorkspaceID: "ws-1", Name: "Worker Profile", Role: models.AgentRoleWorker}
	if err := svc.CreateAgentInstance(ctx, profileAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	modelRec := newPatchAgentRecorder(t, svc, modelAgent.ID, `{"model":"gpt-5"}`)
	profileRec := newPatchAgentRecorder(t, svc, profileAgent.ID, `{"agent_profile_id":"prof-1"}`)

	var modelResp, profileResp map[string]interface{}
	if err := json.Unmarshal(modelRec.Body.Bytes(), &modelResp); err != nil {
		t.Fatalf("decode model response: %v", err)
	}
	if err := json.Unmarshal(profileRec.Body.Bytes(), &profileResp); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}

	if _, ok := modelResp["field"]; !ok {
		t.Errorf("model rejection missing structured field key: %+v", modelResp)
	}
	if _, ok := profileResp["field"]; ok {
		t.Errorf("agent_profile_id rejection unexpectedly carries a field key: %+v", profileResp)
	}
}

// AC-14b: the model-key gate is scoped to Office agents (role != "") —
// requestBodyHasKey is only consulted when agent.Role != "". A legacy
// execution-profile row (role == "") never reaches this endpoint
// successfully at all (validateAgentUpdate rejects any empty role,
// independent of this feature), so the gate being scoped correctly is
// what keeps that pre-existing, unrelated 500 as the response instead of
// this feature's 400 "field":"model" shape shadowing it.
func TestUpdateAgent_ModelKeyGateSkipped_ForExecutionProfileAgent(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	agent := &models.AgentInstance{
		ID: "legacy-2", WorkspaceID: "ws-1", Name: "Legacy Profile", Model: "gpt-5",
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create legacy agent: %v", err)
	}

	rec := newPatchAgentRecorder(t, svc, agent.ID, `{"model":"claude-opus"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (pre-existing invalid-role rejection); body=%s",
			rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["field"] == "model" {
		t.Errorf("model-key gate fired for role==\"\" agent, want it scoped to Office agents: %+v", resp)
	}
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
