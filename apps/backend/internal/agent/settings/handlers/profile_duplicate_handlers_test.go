package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// duplicateRepo is a minimal store.Repository stub: the duplicate path only
// reads one profile, inserts the copy, flips enabled, and reads/writes the
// MCP config row. Everything else is a no-op.
type duplicateRepo struct {
	profiles map[string]*models.AgentProfile
	created  []*models.AgentProfile
}

// newDuplicateRepo returns a fake repository preloaded with one kanban profile (p-1) under agent-1.
func newDuplicateRepo() *duplicateRepo {
	return &duplicateRepo{profiles: map[string]*models.AgentProfile{}}
}

// GetAgentProfile implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) GetAgentProfile(_ context.Context, id string) (*models.AgentProfile, error) {
	if p, ok := r.profiles[id]; ok {
		return p, nil
	}
	return nil, fmt.Errorf("agent profile not found: %s", id)
}

// GetAgentProfileIncludingDeleted implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error) {
	return r.GetAgentProfile(ctx, id)
}

// CreateAgentProfile implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	if p.ID == "" {
		p.ID = "duplicate-" + p.Name
	}
	r.profiles[p.ID] = p
	r.created = append(r.created, p)
	return nil
}

// DuplicateAgentProfile implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) DuplicateAgentProfile(_ context.Context, input store.DuplicateAgentProfileInput) error {
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

// UpdateAgentProfileEnabled implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) UpdateAgentProfileEnabled(_ context.Context, id string, enabled bool) (time.Time, error) {
	p, ok := r.profiles[id]
	if !ok {
		return time.Time{}, fmt.Errorf("agent profile not found: %s", id)
	}
	p.Enabled = enabled
	p.UpdatedAt = time.Now().UTC()
	return p.UpdatedAt, nil
}

// GetAgentProfileMcpConfig implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) GetAgentProfileMcpConfig(context.Context, string) (*models.AgentProfileMcpConfig, error) {
	return nil, nil
}

// UpsertAgentProfileMcpConfig implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) UpsertAgentProfileMcpConfig(context.Context, *models.AgentProfileMcpConfig) error {
	return nil
}

// CreateAgent implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) CreateAgent(context.Context, *models.Agent) error { return nil }

// GetAgent implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) GetAgent(context.Context, string) (*models.Agent, error) {
	return nil, nil
}

// GetAgentByName implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) GetAgentByName(context.Context, string) (*models.Agent, error) {
	return nil, nil
}

// UpdateAgent implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) UpdateAgent(context.Context, *models.Agent) error { return nil }

// DeleteAgent implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) DeleteAgent(context.Context, string) error { return nil }

// ListAgents implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) ListAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}

// ListTUIAgents implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) ListTUIAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}

// UpdateAgentProfile implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) UpdateAgentProfile(context.Context, *models.AgentProfile) error {
	return nil
}

// DeleteAgentProfile implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) DeleteAgentProfile(context.Context, string) error { return nil }

// ListAgentProfiles implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) ListAgentProfiles(context.Context, string) ([]*models.AgentProfile, error) {
	return nil, nil
}

// HasDeletedAgentProfiles implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) HasDeletedAgentProfiles(context.Context, string) (bool, error) {
	return false, nil
}

// Close implements the settings store interface for the duplicate handler tests.
func (r *duplicateRepo) Close() error { return nil }

var _ store.Repository = (*duplicateRepo)(nil)

// duplicateHub captures every WS message so tests can assert the broadcast.
type duplicateHub struct {
	mu   sync.Mutex
	msgs []*ws.Message
}

// Broadcast records a global notification so tests can assert (non-)delivery.
func (h *duplicateHub) Broadcast(msg *ws.Message) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.msgs = append(h.msgs, msg)
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

// workspaceDuplicateHub models the gateway hub: global Broadcast plus the
// workspace-scoped, fail-closed BroadcastToWorkspaceOrDrop.
type workspaceDuplicateHub struct {
	mu            sync.Mutex
	global        []*ws.Message
	workspaceMsgs map[string][]*ws.Message
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

// newDuplicateRouter builds a gin router with the settings handlers wired to the given repo and hub.
func newDuplicateRouter(t *testing.T, repo store.Repository, hub Broadcaster) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	ctrl := controller.NewController(repo, nil, nil, nil, log)
	router := gin.New()
	NewHandlers(ctrl, hub, log, "test-interlock").registerHTTP(router)
	return router
}

// TestDuplicateProfileEndpoint_CopiesAndBroadcasts verifies the HTTP endpoint copies a kanban profile and broadcasts agent.profile.created.
func TestDuplicateProfileEndpoint_CopiesAndBroadcasts(t *testing.T) {
	repo := newDuplicateRepo()
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
	router := newDuplicateRouter(t, repo, hub)

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
	router := newDuplicateRouter(t, newDuplicateRepo(), nil)

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
	repo := newDuplicateRepo()
	repo.profiles["source-office"] = &models.AgentProfile{
		ID:          "source-office",
		AgentID:     "agent-1",
		Name:        "Office Agent",
		WorkspaceID: "ws-1",
		Enabled:     true,
	}
	hub := &workspaceDuplicateHub{workspaceMsgs: map[string][]*ws.Message{}}
	router := newDuplicateRouter(t, repo, hub)

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
	router := newDuplicateRouter(t, newDuplicateRepo(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agent-profiles/source-1/duplicate", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 without interlock token", rec.Code)
	}
}
