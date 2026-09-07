package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// fakeSettingsRepo is the in-memory store.Repository shared by every handler
// test in this package. Agents, profiles and MCP config rows live in maps;
// every mutation the controller performs is recorded so a test can assert the
// downstream call, and `errs` injects a failure per repository method so the
// handlers' 500 branches are reachable.
//
// Missing rows are reported the way the sqlite store reports them, because the
// controller branches on both shapes: sql.ErrNoRows for agents, and an
// "agent profile not found" message for profiles.
type fakeSettingsRepo struct {
	agents     map[string]*models.Agent
	agentOrder []string
	profiles   map[string]*models.AgentProfile
	mcpConfigs map[string]*models.AgentProfileMcpConfig

	created         []*models.AgentProfile
	updatedProfiles []*models.AgentProfile
	deletedProfiles []string
	updatedAgents   []*models.Agent
	deletedAgents   []string
	upsertedMcp     []*models.AgentProfileMcpConfig

	// errs maps a repository method name to the error it should return.
	errs map[string]error
	seq  int
}

// newFakeSettingsRepo returns an empty in-memory settings repository.
func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{
		agents:     map[string]*models.Agent{},
		profiles:   map[string]*models.AgentProfile{},
		mcpConfigs: map[string]*models.AgentProfileMcpConfig{},
		errs:       map[string]error{},
	}
}

// failWith makes the named repository method return err.
func (r *fakeSettingsRepo) failWith(method string, err error) *fakeSettingsRepo {
	r.errs[method] = err
	return r
}

// putAgent seeds an agent row, preserving insertion order for ListAgents.
func (r *fakeSettingsRepo) putAgent(agent *models.Agent) *models.Agent {
	if _, exists := r.agents[agent.ID]; !exists {
		r.agentOrder = append(r.agentOrder, agent.ID)
	}
	r.agents[agent.ID] = agent
	return agent
}

// putProfile seeds a profile row.
func (r *fakeSettingsRepo) putProfile(profile *models.AgentProfile) *models.AgentProfile {
	r.profiles[profile.ID] = profile
	return profile
}

func (r *fakeSettingsRepo) nextID(prefix string) string {
	r.seq++
	return fmt.Sprintf("%s-%d", prefix, r.seq)
}

// GetAgentProfile implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) GetAgentProfile(_ context.Context, id string) (*models.AgentProfile, error) {
	if err := r.errs["GetAgentProfile"]; err != nil {
		return nil, err
	}
	if p, ok := r.profiles[id]; ok {
		return p, nil
	}
	// The sqlite store returns sql.ErrNoRows here (see its soft-delete tests);
	// only the update/delete paths report the "agent profile not found"
	// message. The controller branches on both shapes, so the fake has to
	// reproduce the split rather than pick one.
	return nil, sql.ErrNoRows
}

// GetAgentProfileIncludingDeleted implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error) {
	return r.GetAgentProfile(ctx, id)
}

// GetAgentProfileTx is not exercised by this package's tests; it exists only
// to satisfy store.Repository.
func (r *fakeSettingsRepo) GetAgentProfileTx(_ context.Context, _ *sqlx.Tx, _ string) (*models.AgentProfile, bool, error) {
	return nil, false, fmt.Errorf("GetAgentProfileTx not implemented")
}

// CreateAgentProfile implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	if err := r.errs["CreateAgentProfile"]; err != nil {
		return err
	}
	if p.ID == "" {
		p.ID = r.nextID("profile")
	}
	now := time.Now().UTC()
	p.CreatedAt, p.UpdatedAt = now, now
	r.profiles[p.ID] = p
	r.created = append(r.created, p)
	return nil
}

// DuplicateAgentProfile implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) DuplicateAgentProfile(_ context.Context, input store.DuplicateAgentProfileInput) error {
	// Version check: the stored source must match the caller's snapshot.
	if src, ok := r.profiles[input.Source.ID]; ok && !src.UpdatedAt.Equal(input.Source.UpdatedAt) {
		return store.ErrProfileChanged
	}
	p := input.Profile
	if p.ID == "" {
		p.ID = "duplicate-" + p.Name
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	r.profiles[p.ID] = p
	r.created = append(r.created, p)
	if input.McpConfig != nil {
		input.McpConfig.ProfileID = p.ID
	}
	return nil
}

// UpdateAgentProfileEnabled implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) UpdateAgentProfileEnabled(_ context.Context, id string, enabled bool) (time.Time, error) {
	if err := r.errs["UpdateAgentProfileEnabled"]; err != nil {
		return time.Time{}, err
	}
	p, ok := r.profiles[id]
	if !ok {
		return time.Time{}, fmt.Errorf("agent profile not found: %s", id)
	}
	p.Enabled = enabled
	p.UpdatedAt = time.Now().UTC()
	return p.UpdatedAt, nil
}

// GetAgentProfileMcpConfig implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) GetAgentProfileMcpConfig(_ context.Context, profileID string) (*models.AgentProfileMcpConfig, error) {
	if err := r.errs["GetAgentProfileMcpConfig"]; err != nil {
		return nil, err
	}
	if cfg, ok := r.mcpConfigs[profileID]; ok {
		return cfg, nil
	}
	return nil, sql.ErrNoRows
}

// UpsertAgentProfileMcpConfig implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) UpsertAgentProfileMcpConfig(_ context.Context, config *models.AgentProfileMcpConfig) error {
	if err := r.errs["UpsertAgentProfileMcpConfig"]; err != nil {
		return err
	}
	r.mcpConfigs[config.ProfileID] = config
	r.upsertedMcp = append(r.upsertedMcp, config)
	return nil
}

// CreateAgent implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) CreateAgent(_ context.Context, agent *models.Agent) error {
	if err := r.errs["CreateAgent"]; err != nil {
		return err
	}
	if agent.ID == "" {
		agent.ID = r.nextID("agent")
	}
	r.putAgent(agent)
	return nil
}

// GetAgent implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) GetAgent(_ context.Context, id string) (*models.Agent, error) {
	if err := r.errs["GetAgent"]; err != nil {
		return nil, err
	}
	if agent, ok := r.agents[id]; ok {
		return agent, nil
	}
	return nil, sql.ErrNoRows
}

// GetAgentByName implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) GetAgentByName(_ context.Context, name string) (*models.Agent, error) {
	if err := r.errs["GetAgentByName"]; err != nil {
		return nil, err
	}
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, nil
		}
	}
	return nil, sql.ErrNoRows
}

// UpdateAgent implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) UpdateAgent(_ context.Context, agent *models.Agent) error {
	if err := r.errs["UpdateAgent"]; err != nil {
		return err
	}
	r.putAgent(agent)
	r.updatedAgents = append(r.updatedAgents, agent)
	return nil
}

// DeleteAgent implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) DeleteAgent(_ context.Context, id string) error {
	if err := r.errs["DeleteAgent"]; err != nil {
		return err
	}
	if _, ok := r.agents[id]; !ok {
		return fmt.Errorf("agent not found: %s", id)
	}
	delete(r.agents, id)
	r.deletedAgents = append(r.deletedAgents, id)
	return nil
}

// ListAgents implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) ListAgents(context.Context) ([]*models.Agent, error) {
	if err := r.errs["ListAgents"]; err != nil {
		return nil, err
	}
	out := make([]*models.Agent, 0, len(r.agentOrder))
	for _, id := range r.agentOrder {
		if agent, ok := r.agents[id]; ok {
			out = append(out, agent)
		}
	}
	return out, nil
}

// ListTUIAgents implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) ListTUIAgents(ctx context.Context) ([]*models.Agent, error) {
	agents, err := r.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*models.Agent, 0, len(agents))
	for _, agent := range agents {
		if agent.TUIConfig != nil {
			out = append(out, agent)
		}
	}
	return out, nil
}

// UpdateAgentProfile implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) UpdateAgentProfile(_ context.Context, profile *models.AgentProfile) error {
	if err := r.errs["UpdateAgentProfile"]; err != nil {
		return err
	}
	profile.UpdatedAt = time.Now().UTC()
	r.profiles[profile.ID] = profile
	r.updatedProfiles = append(r.updatedProfiles, profile)
	return nil
}

// DeleteAgentProfile implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) DeleteAgentProfile(_ context.Context, id string) error {
	if err := r.errs["DeleteAgentProfile"]; err != nil {
		return err
	}
	if _, ok := r.profiles[id]; !ok {
		return fmt.Errorf("agent profile not found: %s", id)
	}
	delete(r.profiles, id)
	r.deletedProfiles = append(r.deletedProfiles, id)
	return nil
}

// ListAgentProfiles implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) ListAgentProfiles(_ context.Context, agentID string) ([]*models.AgentProfile, error) {
	if err := r.errs["ListAgentProfiles"]; err != nil {
		return nil, err
	}
	out := make([]*models.AgentProfile, 0, len(r.profiles))
	for _, p := range r.profiles {
		if p.AgentID == agentID && p.DeletedAt == nil {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// HasDeletedAgentProfiles implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) HasDeletedAgentProfiles(context.Context, string) (bool, error) {
	return false, nil
}

// Close implements the settings store interface for the handler tests.
func (r *fakeSettingsRepo) Close() error { return nil }

var _ store.Repository = (*fakeSettingsRepo)(nil)

// duplicateHub captures every WS message so tests can assert the broadcast.
// It deliberately does NOT implement BroadcastToWorkspaceOrDrop, which is what
// makes it the fixture for broadcastProfileEvent's fail-closed drop branch.
type duplicateHub struct {
	mu   sync.Mutex
	msgs []*ws.Message
	// signal receives every broadcast action, so a test can join an
	// asynchronous broadcast goroutine instead of sleeping. Nil by default:
	// the non-blocking send then falls straight through to the default case.
	signal chan string
}

// newSignallingHub returns a hub that also publishes each broadcast action on
// a buffered channel, for tests that must wait on an async broadcast.
func newSignallingHub() *duplicateHub {
	return &duplicateHub{signal: make(chan string, 8)}
}

// Broadcast records a global notification so tests can assert (non-)delivery.
func (h *duplicateHub) Broadcast(msg *ws.Message) {
	h.mu.Lock()
	h.msgs = append(h.msgs, msg)
	h.mu.Unlock()
	select {
	case h.signal <- msg.Action:
	default:
	}
}

// awaitAction blocks until the hub broadcasts the given action.
func (h *duplicateHub) awaitAction(t *testing.T, action string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case got := <-h.signal:
			if got == action {
				return
			}
		case <-deadline:
			t.Fatalf("no %q broadcast within 2s; recorded %v", action, h.actions())
		}
	}
}

// actions returns the global notifications recorded by Broadcast.
func (h *duplicateHub) actions() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.msgs))
	for _, m := range h.msgs {
		out = append(out, m.Action)
	}
	return out
}

// payloads returns the recorded notifications for one action.
func (h *duplicateHub) payloads(action string) []json.RawMessage {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]json.RawMessage, 0, len(h.msgs))
	for _, m := range h.msgs {
		if m.Action == action {
			out = append(out, m.Payload)
		}
	}
	return out
}

// workspaceDuplicateHub models the gateway hub: global Broadcast plus the
// workspace-scoped, fail-closed BroadcastToWorkspaceOrDrop.
type workspaceDuplicateHub struct {
	mu            sync.Mutex
	global        []*ws.Message
	workspaceMsgs map[string][]*ws.Message
}

// newWorkspaceDuplicateHub returns a workspace-routing-aware hub fake.
func newWorkspaceDuplicateHub() *workspaceDuplicateHub {
	return &workspaceDuplicateHub{workspaceMsgs: map[string][]*ws.Message{}}
}

// Broadcast records a global notification so tests can assert (non-)delivery.
func (h *workspaceDuplicateHub) Broadcast(msg *ws.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.global = append(h.global, msg)
}

// BroadcastToWorkspaceOrDrop records a workspace-scoped notification so tests can assert (non-)delivery.
func (h *workspaceDuplicateHub) BroadcastToWorkspaceOrDrop(workspaceID string, msg *ws.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.workspaceMsgs[workspaceID] = append(h.workspaceMsgs[workspaceID], msg)
}

// syntheticSettingsIdentity is the single-user admin identity the auth
// middleware installs when auth is disabled.
func syntheticSettingsIdentity() authn.Identity {
	return authn.Identity{UserID: "default", Role: authn.RoleAdmin, Synthetic: true}
}

// useSettingsIdentity puts an identity on every request. The agent and
// agent-profile mutations are gated on org.config.manage, so a router built
// without the production middleware chain has no identity at all and every
// mutation answers 401 before reaching the handler under test.
func useSettingsIdentity(router *gin.Engine, identity authn.Identity) {
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, identity)
		c.Next()
	})
}

func useSyntheticSettingsIdentity(router *gin.Engine) {
	useSettingsIdentity(router, syntheticSettingsIdentity())
}

// newSettingsHarness wires the settings handlers to the given repo and hub and
// returns the router plus the controller and agent registry, so a test can
// register a deterministic agent implementation or attach a dependency checker.
func newSettingsHarness(
	t *testing.T,
	repo store.Repository,
	hub Broadcaster,
) (*gin.Engine, *controller.Controller, *registry.Registry) {
	t.Helper()
	return newSettingsHarnessAs(t, repo, hub, syntheticSettingsIdentity())
}

// newSettingsHarnessAs is newSettingsHarness with the caller's identity, for
// tests that assert what a non-admin may and may not reach.
func newSettingsHarnessAs(
	t *testing.T,
	repo store.Repository,
	hub Broadcaster,
	identity authn.Identity,
) (*gin.Engine, *controller.Controller, *registry.Registry) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	reg := registry.NewRegistry(log)
	// The async available-agents broadcast dereferences the discovery registry,
	// and it runs in a goroutine where a nil panic would take the test binary
	// down rather than failing one test. Load an (empty) real one instead.
	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), reg, log)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	ctrl := controller.NewController(repo, discoveryRegistry, reg, nil, log)
	router := gin.New()
	useSettingsIdentity(router, identity)
	// Register through the production entry point so the install/update job
	// stores are wired from the hub exactly as they are at startup — with a nil
	// hub they stay unavailable, which is itself a behaviour under test.
	RegisterRoutes(router, ctrl, hub, log, "test-interlock")
	return router, ctrl, reg
}

// newSettingsRouter builds a gin router with the settings handlers wired to the given repo and hub.
func newSettingsRouter(t *testing.T, repo store.Repository, hub Broadcaster) *gin.Engine {
	t.Helper()
	router, _, _ := newSettingsHarness(t, repo, hub)
	return router
}

// TestDuplicateProfileEndpoint_CopiesAndBroadcasts verifies the HTTP endpoint copies a kanban profile and broadcasts agent.profile.created.
func TestDuplicateProfileEndpoint_CopiesAndBroadcasts(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.profiles["source-1"] = &models.AgentProfile{
		ID:               "source-1",
		AgentID:          "agent-1",
		Name:             "Default",
		AgentDisplayName: "Claude Code",
		Model:            "claude-sonnet",
		CLIFlags:         []models.CLIFlag{{Description: "Tools", Flag: "--allow-all-tools", Enabled: true}},
		Enabled:          true,
	}
	hub := &duplicateHub{}
	router := newSettingsRouter(t, repo, hub)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-1/duplicate", nil)
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var created dto.AgentProfileDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "source-1" || created.ID == "" {
		t.Errorf("response profile ID = %q, want a fresh ID", created.ID)
	}
	if created.Name != "Default Copy" {
		t.Errorf("response name = %q, want %q", created.Name, "Default Copy")
	}
	if created.Model != "claude-sonnet" {
		t.Errorf("response model = %q, want claude-sonnet", created.Model)
	}
	if len(created.CLIFlags) != 1 || created.CLIFlags[0].Flag != "--allow-all-tools" {
		t.Errorf("response cli flags = %+v, want the source entry", created.CLIFlags)
	}

	found := false
	for _, action := range hub.actions() {
		if action == ws.ActionAgentProfileCreated {
			found = true
		}
	}
	if !found {
		t.Errorf("no %s broadcast, got %v", ws.ActionAgentProfileCreated, hub.actions())
	}
	if len(repo.created) != 1 {
		t.Fatalf("stored copies = %d, want 1", len(repo.created))
	}
}

// TestDuplicateProfileEndpoint_NotFound verifies an unknown source ID surfaces as HTTP 404.
func TestDuplicateProfileEndpoint_NotFound(t *testing.T) {
	router := newSettingsRouter(t, newFakeSettingsRepo(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/missing/duplicate", nil)
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "agent profile not found") {
		t.Errorf("body = %q, want agent profile not found message", rec.Body.String())
	}
}

// TestDuplicateProfileEndpoint_RejectsOfficeScopedSource verifies the
// settings duplicate endpoint refuses office-scoped profiles fail-closed:
// the source is owned by the workspace-scoped office surface, so the request
// surfaces as 404 (existence hidden) and no event is broadcast on any channel
// (workspace-aware or global).
func TestDuplicateProfileEndpoint_RejectsOfficeScopedSource(t *testing.T) {
	repo := newFakeSettingsRepo()
	repo.profiles["source-office"] = &models.AgentProfile{
		ID:          "source-office",
		AgentID:     "agent-1",
		Name:        "Office Agent",
		WorkspaceID: "ws-1",
		Enabled:     true,
	}
	hub := newWorkspaceDuplicateHub()
	router := newSettingsRouter(t, repo, hub)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-office/duplicate", nil)
	req.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if len(hub.global) != 0 {
		t.Errorf("office duplicate leaked to the global settings broadcast: %d messages", len(hub.global))
	}
	if len(hub.workspaceMsgs) != 0 {
		t.Errorf("office duplicate broadcast workspace-scoped messages: %v", hub.workspaceMsgs)
	}
}

// TestDuplicateProfileEndpoint_RequiresInterlock verifies the duplicate route is rejected without the interlock token.
func TestDuplicateProfileEndpoint_RequiresInterlock(t *testing.T) {
	router := newSettingsRouter(t, newFakeSettingsRepo(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-1/duplicate", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without interlock token", rec.Code)
	}
}
