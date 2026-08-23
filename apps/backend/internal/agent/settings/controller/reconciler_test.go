package controller

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	"github.com/kandev/kandev/internal/common/logger"
)

// fakeCapReader returns pre-baked AgentCapabilities for a fixed agent type.
type fakeCapReader struct {
	caps map[string]hostutility.AgentCapabilities
}

func (f *fakeCapReader) Get(agentType string) (hostutility.AgentCapabilities, bool) {
	c, ok := f.caps[agentType]
	return c, ok
}

// fakeStore implements just enough of store.Repository for the reconciler.
type fakeStore struct {
	agents               map[string]*models.Agent                 // keyed by DB ID
	byName               map[string]*models.Agent                 // keyed by Name
	profiles             map[string][]*models.AgentProfile        // keyed by DB agent ID (live)
	deleted              map[string][]*models.AgentProfile        // keyed by DB agent ID (soft-deleted)
	mcpConfigs           map[string]*models.AgentProfileMcpConfig // keyed by profile ID
	created              []*models.AgentProfile
	updated              []*models.AgentProfile
	softDeleted          []string
	nextAgentID          int
	nextProfID           int
	getByNameErr         error
	listAgentsErr        error
	listProfErr          map[string]error
	duplicateProfErr     error
	duplicateChangedOnce bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		agents:     map[string]*models.Agent{},
		byName:     map[string]*models.Agent{},
		profiles:   map[string][]*models.AgentProfile{},
		deleted:    map[string][]*models.AgentProfile{},
		mcpConfigs: map[string]*models.AgentProfileMcpConfig{},
	}
}

// copyAgent deep-copies an agent so the fake store behaves like a real one:
// callers get a snapshot, not a handle on stored state.
//
// This matters more than it looks. When Get returned the stored pointer and
// UpdateAgent was a no-op, a controller could mutate a nested field, never
// persist it, and every assertion still passed — the test was reading the same
// struct it had just written through. That masked UpdateAgent silently dropping
// the tui_config column. Round-tripping through JSON forces writes to go through
// UpdateAgent to be observable, the same as SQL.
func copyAgent(a *models.Agent) *models.Agent {
	if a == nil {
		return nil
	}
	data, err := json.Marshal(a)
	if err != nil {
		panic("fakeStore: marshal agent: " + err.Error())
	}
	var out models.Agent
	if err := json.Unmarshal(data, &out); err != nil {
		panic("fakeStore: unmarshal agent: " + err.Error())
	}
	return &out
}

// copyProfile deep-copies a profile so the fake store behaves like a real one:
// callers get a snapshot, not a handle on stored state. Keep the legacy field
// that is intentionally omitted from the public JSON representation.
func copyProfile(p *models.AgentProfile) *models.AgentProfile {
	if p == nil {
		return nil
	}
	data, err := json.Marshal(p)
	if err != nil {
		panic("fakeStore: marshal profile: " + err.Error())
	}
	var out models.AgentProfile
	if err := json.Unmarshal(data, &out); err != nil {
		panic("fakeStore: unmarshal profile: " + err.Error())
	}
	out.DangerouslySkipPermissions = p.DangerouslySkipPermissions
	return &out
}

func TestFakeStoreAgentProfileReadRequiresExplicitUpdate(t *testing.T) {
	st := newFakeStore()
	st.profiles["agent-1"] = []*models.AgentProfile{{
		ID:      "profile-1",
		AgentID: "agent-1",
		Name:    "Original",
	}}

	profile, err := st.GetAgentProfile(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("GetAgentProfile: %v", err)
	}
	profile.Name = "Changed"

	stored, err := st.GetAgentProfile(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("GetAgentProfile after local mutation: %v", err)
	}
	if stored.Name != "Original" {
		t.Fatalf("local mutation changed stored profile name to %q", stored.Name)
	}

	if err := st.UpdateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("UpdateAgentProfile: %v", err)
	}
	stored, err = st.GetAgentProfile(context.Background(), "profile-1")
	if err != nil {
		t.Fatalf("GetAgentProfile after update: %v", err)
	}
	if stored.Name != "Changed" {
		t.Fatalf("stored profile name = %q, want Changed after explicit update", stored.Name)
	}
}

func (f *fakeStore) CreateAgent(_ context.Context, a *models.Agent) error {
	if a.ID == "" {
		f.nextAgentID++
		a.ID = "agent-" + strconv.Itoa(f.nextAgentID)
	}
	stored := copyAgent(a)
	f.agents[a.ID] = stored
	f.byName[a.Name] = stored
	return nil
}

func (f *fakeStore) GetAgent(_ context.Context, id string) (*models.Agent, error) {
	return copyAgent(f.agents[id]), nil
}

func (f *fakeStore) GetAgentByName(_ context.Context, name string) (*models.Agent, error) {
	if f.getByNameErr != nil {
		return nil, f.getByNameErr
	}
	if a, ok := f.byName[name]; ok {
		return copyAgent(a), nil
	}
	return nil, sql.ErrNoRows
}

func (f *fakeStore) UpdateAgent(_ context.Context, a *models.Agent) error {
	existing, ok := f.agents[a.ID]
	if !ok {
		return fmt.Errorf("agent not found: %s", a.ID)
	}
	stored := copyAgent(a)
	// Mirror the real store's column list so the fake cannot accept a write the
	// database would reject or ignore.
	existing.WorkspaceID = stored.WorkspaceID
	existing.SupportsMCP = stored.SupportsMCP
	existing.MCPConfigPath = stored.MCPConfigPath
	existing.TUIConfig = stored.TUIConfig
	f.byName[existing.Name] = existing
	return nil
}

func (f *fakeStore) DeleteAgent(context.Context, string) error { return nil }

func (f *fakeStore) ListAgents(_ context.Context) ([]*models.Agent, error) {
	if f.listAgentsErr != nil {
		return nil, f.listAgentsErr
	}
	out := make([]*models.Agent, 0, len(f.agents))
	for _, a := range f.agents {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeStore) ListTUIAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}

// GetAgentProfileMcpConfig implements the settings store interface for the reconciler test fakes.
func (f *fakeStore) GetAgentProfileMcpConfig(_ context.Context, profileID string) (*models.AgentProfileMcpConfig, error) {
	if cfg, ok := f.mcpConfigs[profileID]; ok {
		return cfg, nil
	}
	return nil, nil
}

// UpsertAgentProfileMcpConfig implements the settings store interface for the reconciler test fakes.
func (f *fakeStore) UpsertAgentProfileMcpConfig(_ context.Context, config *models.AgentProfileMcpConfig) error {
	f.mcpConfigs[config.ProfileID] = config
	return nil
}

func (f *fakeStore) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	f.nextProfID++
	p.ID = "profile-" + strconv.Itoa(f.nextProfID)
	stored := copyProfile(p)
	f.profiles[p.AgentID] = append(f.profiles[p.AgentID], stored)
	f.created = append(f.created, stored)
	return nil
}

// DuplicateAgentProfile implements the settings store interface for the reconciler test fakes.
func (f *fakeStore) DuplicateAgentProfile(_ context.Context, input store.DuplicateAgentProfileInput) error {
	if f.duplicateProfErr != nil {
		return f.duplicateProfErr
	}
	if f.duplicateChangedOnce {
		f.duplicateChangedOnce = false
		// Simulate a concurrent writer: the stored source row changes between
		// the controller's first read and its duplicate attempt, so a retry
		// must re-read the source and copy the NEW state.
		input.Source.Name = "Changed Name"
		input.Source.UpdatedAt = input.Source.UpdatedAt.Add(time.Second)
		if _, err := f.persistAgentProfile(input.Source); err != nil {
			return err
		}
		return store.ErrProfileChanged
	}
	// Version check mirrors the sqlite store: the stored source rows must
	// still match the revisions the copy was built from.
	if f.profiles[input.Source.AgentID] != nil {
		for _, p := range f.profiles[input.Source.AgentID] {
			if p.ID == input.Source.ID && !p.UpdatedAt.Equal(input.Source.UpdatedAt) {
				return store.ErrProfileChanged
			}
		}
	}
	if input.SourceMcp != nil {
		mcp, ok := f.mcpConfigs[input.Source.ID]
		if !ok || !mcp.UpdatedAt.Equal(input.SourceMcp.UpdatedAt) {
			return store.ErrProfileChanged
		}
	}
	p := input.Profile
	f.nextProfID++
	p.ID = "profile-" + strconv.Itoa(f.nextProfID)
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now
	stored := copyProfile(p)
	f.profiles[p.AgentID] = append(f.profiles[p.AgentID], stored)
	f.created = append(f.created, stored)
	if input.McpConfig != nil {
		input.McpConfig.ProfileID = p.ID
		f.mcpConfigs[p.ID] = input.McpConfig
	}
	return nil
}

func (f *fakeStore) persistAgentProfile(p *models.AgentProfile) (*models.AgentProfile, error) {
	stored := copyProfile(p)
	for agentID, profiles := range f.profiles {
		for i, existing := range profiles {
			if existing.ID == p.ID {
				f.profiles[agentID][i] = stored
				return stored, nil
			}
		}
	}
	return nil, fmt.Errorf("agent profile not found: %s", p.ID)
}

func (f *fakeStore) UpdateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	stored, err := f.persistAgentProfile(p)
	if err != nil {
		return err
	}
	f.updated = append(f.updated, copyProfile(stored))
	return nil
}

func (f *fakeStore) UpdateAgentProfileEnabled(ctx context.Context, id string, enabled bool) (time.Time, error) {
	profile, err := f.GetAgentProfile(ctx, id)
	if err != nil {
		return time.Time{}, err
	}
	profile.Enabled = enabled
	profile.UserModified = true
	profile.UpdatedAt = time.Now().UTC()
	stored, err := f.persistAgentProfile(profile)
	if err != nil {
		return time.Time{}, err
	}
	f.updated = append(f.updated, copyProfile(stored))
	return stored.UpdatedAt, nil
}

func (f *fakeStore) DeleteAgentProfile(_ context.Context, id string) error {
	f.softDeleted = append(f.softDeleted, id)
	// Faithfully model soft-delete: move the row out of the live list into the
	// deleted list so HasDeletedAgentProfiles and GetAgentProfileIncludingDeleted
	// stay consistent with the live ListAgentProfiles view.
	for agentID, list := range f.profiles {
		kept := list[:0:0]
		for _, p := range list {
			if p.ID == id {
				f.deleted[agentID] = append(f.deleted[agentID], p)
				continue
			}
			kept = append(kept, p)
		}
		f.profiles[agentID] = kept
	}
	return nil
}

func (f *fakeStore) HasDeletedAgentProfiles(_ context.Context, agentID string) (bool, error) {
	return len(f.deleted[agentID]) > 0, nil
}

func (f *fakeStore) GetAgentProfile(_ context.Context, id string) (*models.AgentProfile, error) {
	for _, list := range f.profiles {
		for _, p := range list {
			if p.ID == id {
				return copyProfile(p), nil
			}
		}
	}
	return nil, sql.ErrNoRows
}

// GetAgentProfileTx is not exercised by this package's tests; it exists only
// to satisfy store.Repository.
func (f *fakeStore) GetAgentProfileTx(_ context.Context, _ *sqlx.Tx, _ string) (*models.AgentProfile, bool, error) {
	return nil, false, errors.New("GetAgentProfileTx not implemented")
}

func (f *fakeStore) GetAgentProfileIncludingDeleted(ctx context.Context, id string) (*models.AgentProfile, error) {
	if p, err := f.GetAgentProfile(ctx, id); err == nil {
		return p, nil
	}
	// Fall back to the soft-deleted set so this method, unlike GetAgentProfile,
	// surfaces rows that DeleteAgentProfile moved out of the live list.
	for _, list := range f.deleted {
		for _, p := range list {
			if p.ID == id {
				return copyProfile(p), nil
			}
		}
	}
	return nil, sql.ErrNoRows
}

func (f *fakeStore) ListAgentProfiles(_ context.Context, agentID string) ([]*models.AgentProfile, error) {
	if f.listProfErr != nil {
		if err, ok := f.listProfErr[agentID]; ok {
			return nil, err
		}
	}
	profiles := f.profiles[agentID]
	out := make([]*models.AgentProfile, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, copyProfile(p))
	}
	return out, nil
}

func (f *fakeStore) Close() error { return nil }

// mockInferenceAgent is a minimal fake agent for the registry.
type mockInferenceAgent struct {
	id          string
	displayName string
	enabled     bool
}

func (m *mockInferenceAgent) ID() string                     { return m.id }
func (m *mockInferenceAgent) Name() string                   { return m.id }
func (m *mockInferenceAgent) DisplayName() string            { return m.displayName }
func (m *mockInferenceAgent) Description() string            { return "" }
func (m *mockInferenceAgent) Enabled() bool                  { return m.enabled }
func (m *mockInferenceAgent) DisplayOrder() int              { return 0 }
func (m *mockInferenceAgent) Logo(agents.LogoVariant) []byte { return nil }
func (m *mockInferenceAgent) IsInstalled(context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: true}, nil
}
func (m *mockInferenceAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return agents.Command{}
}
func (m *mockInferenceAgent) PermissionSettings() map[string]agents.PermissionSetting { return nil }
func (m *mockInferenceAgent) Runtime() *agents.RuntimeConfig                          { return &agents.RuntimeConfig{} }
func (m *mockInferenceAgent) BillingType() usage.BillingType                          { return usage.BillingTypeAPIKey }
func (m *mockInferenceAgent) RemoteAuth() *agents.RemoteAuth                          { return nil }
func (m *mockInferenceAgent) InstallScript() string                                   { return "" }
func (m *mockInferenceAgent) InferenceConfig() *agents.InferenceConfig {
	return &agents.InferenceConfig{Supported: true, Command: agents.NewCommand("x")}
}

func newReconciler(t *testing.T, st *fakeStore, caps *fakeCapReader, ag agents.Agent) *ProfileReconciler {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(ag); err != nil {
		t.Fatalf("register: %v", err)
	}
	reg.MarkLoaded()
	return NewProfileReconciler(caps, reg, st, log)
}

func newObserverLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

func TestProfileReconciler_SeedsDefaultProfile(t *testing.T) {
	st := newFakeStore()
	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"claude-acp": {
				AgentType:      "claude-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "claude-sonnet", Name: "Sonnet"}},
				CurrentModelID: "claude-sonnet",
				CurrentModeID:  "default",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.created) != 1 {
		t.Fatalf("expected 1 created profile, got %d", len(st.created))
	}
	p := st.created[0]
	if p.Model != "claude-sonnet" {
		t.Errorf("model = %q, want claude-sonnet", p.Model)
	}
	if p.Mode != "default" {
		t.Errorf("mode = %q, want default", p.Mode)
	}
}

func TestProfileReconciler_SeedsDynamicVirtualFamily(t *testing.T) {
	st := newFakeStore()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(
		&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}},
		reg,
		st,
		log,
	)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	parent, err := st.GetAgentByName(context.Background(), agents.DynamicAgentID)
	if err != nil {
		t.Fatalf("dynamic family lookup: %v", err)
	}
	if parent.ID != agents.DynamicAgentID {
		t.Fatalf("dynamic family ID = %q, want stable ID %q", parent.ID, agents.DynamicAgentID)
	}
	if len(st.created) != 0 {
		t.Fatalf("dynamic family seeded a concrete profile: %+v", st.created)
	}
}

// TestProfileReconciler_DoesNotReseedAfterUserDelete pins the core fix: an
// agent whose only profile the user deleted (zero live rows, but soft-deleted
// rows present) must NOT have a default profile recreated on the next boot.
func TestProfileReconciler_DoesNotReseedAfterUserDelete(t *testing.T) {
	st := newFakeStore()
	// Agent exists and was provisioned before, but the user deleted every
	// profile — modelled by creating a profile and soft-deleting it via the
	// real delete path, leaving zero live rows and one soft-deleted row.
	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	deletedProfile := &models.AgentProfile{AgentID: dbAgent.ID, Name: "Sonnet 4.6", Model: "claude-sonnet"}
	_ = st.CreateAgentProfile(context.Background(), deletedProfile)
	_ = st.DeleteAgentProfile(context.Background(), deletedProfile.ID)
	st.created = nil // discard setup bookkeeping; measure only what Run() seeds

	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"claude-acp": {
				AgentType:      "claude-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "claude-sonnet", Name: "Sonnet"}},
				CurrentModelID: "claude-sonnet",
				CurrentModeID:  "default",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.created) != 0 {
		t.Fatalf("expected no profile to be recreated after user delete, got %d: %+v",
			len(st.created), st.created)
	}
}

// TestProfileReconciler_KeepsGoneModel verifies the reconciler keeps a gone start model instead of healing it.

func TestProfileReconciler_KeepsGoneModel(t *testing.T) {
	st := newFakeStore()
	// Seed an existing DB agent and profile whose model is no longer
	// advertised (e.g. provider auth expired). The reconciler must NOT
	// silently replace it — no implicit model fallback.
	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID: dbAgent.ID,
		Name:    "Claude",
		Model:   "claude-gone",
		Mode:    "default",
	}
	_ = st.CreateAgentProfile(context.Background(), existing)

	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"claude-acp": {
				AgentType:      "claude-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "claude-new", Name: "New"}},
				CurrentModelID: "claude-new",
				Modes:          []hostutility.Mode{{ID: "default", Name: "Default"}},
				CurrentModeID:  "default",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// No update: the gone model is kept, not overwritten.
	if len(st.updated) != 0 {
		t.Fatalf("expected no profile update, got %d: %+v", len(st.updated), st.updated)
	}
	live, err := st.GetAgentProfile(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if live.Model != "claude-gone" {
		t.Errorf("model = %q, want claude-gone (kept, not healed)", live.Model)
	}
}

// TestProfileReconciler_SeedsEmptyModel verifies the reconciler still seeds an empty model from the probe.

func TestProfileReconciler_SeedsEmptyModel(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID: dbAgent.ID,
		Name:    "Claude",
		Model:   "",
	}
	_ = st.CreateAgentProfile(context.Background(), existing)

	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"claude-acp": {
				AgentType:      "claude-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "claude-sonnet", Name: "Sonnet"}},
				CurrentModelID: "claude-sonnet",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 1 {
		t.Fatalf("expected 1 updated profile, got %d", len(st.updated))
	}
	if updated := st.updated[0]; updated.Model != "claude-sonnet" {
		t.Errorf("seeded model = %q, want claude-sonnet", updated.Model)
	}
}

// TestProfileReconciler_KeepsGoneFallbackModel verifies a gone fallback model is kept, not healed.

func TestProfileReconciler_KeepsGoneFallbackModel(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "omp-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID:       dbAgent.ID,
		Name:          "Hybrid",
		Model:         "claude-sonnet",
		FallbackModel: "gpt-gone",
	}
	_ = st.CreateAgentProfile(context.Background(), existing)

	ag := &mockInferenceAgent{id: "omp-acp", displayName: "OMP", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"omp-acp": {
				AgentType:      "omp-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "claude-sonnet", Name: "Sonnet"}},
				CurrentModelID: "claude-sonnet",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 0 {
		t.Fatalf("expected no profile update, got %d: %+v", len(st.updated), st.updated)
	}
	live, err := st.GetAgentProfile(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	if live.FallbackModel != "gpt-gone" {
		t.Errorf("fallback_model = %q, want gpt-gone (kept, not healed)", live.FallbackModel)
	}
}

func TestProfileReconciler_PreservesUserModifiedCustomRouteOutsideAgentCatalog(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "opencode-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID:      dbAgent.ID,
		Name:         "Custom route",
		Model:        "custom-provider/model",
		Mode:         "custom-mode",
		UserModified: true,
	}
	_ = st.CreateAgentProfile(context.Background(), existing)
	st.created = nil

	ag := &mockInferenceAgent{id: "opencode-acp", displayName: "OpenCode", enabled: true}
	caps := &fakeCapReader{caps: map[string]hostutility.AgentCapabilities{
		"opencode-acp": {
			AgentType:      "opencode-acp",
			Models:         []hostutility.Model{{ID: "catalog-model", Name: "Catalog"}},
			CurrentModelID: "catalog-model",
			Modes:          []hostutility.Mode{{ID: "agent", Name: "Agent"}},
			CurrentModeID:  "agent",
			Status:         hostutility.StatusOK,
		},
	}}

	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 0 {
		t.Fatalf("user-modified custom route was rewritten: %+v", st.updated)
	}
	if existing.Model != "custom-provider/model" || existing.Mode != "custom-mode" {
		t.Fatalf("custom route changed to model=%q mode=%q", existing.Model, existing.Mode)
	}
}

func TestProfileReconciler_LeavesUserModifiedEmptyRouteUnchanged(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID:      dbAgent.ID,
		Name:         "Use provider default",
		UserModified: true,
	}
	_ = st.CreateAgentProfile(context.Background(), existing)
	st.created = nil

	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{caps: map[string]hostutility.AgentCapabilities{
		"claude-acp": {
			AgentType:      "claude-acp",
			Models:         []hostutility.Model{{ID: "claude-sonnet", Name: "Sonnet"}},
			CurrentModelID: "claude-sonnet",
			Modes:          []hostutility.Mode{{ID: "default", Name: "Default"}},
			CurrentModeID:  "default",
			Status:         hostutility.StatusOK,
		},
	}}

	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 0 {
		t.Fatalf("user-modified empty route was rewritten: %+v", st.updated)
	}
	if existing.Model != "" || existing.Mode != "" {
		t.Fatalf("empty route changed to model=%q mode=%q", existing.Model, existing.Mode)
	}
}

func TestProfileReconciler_PreservesOfficeAgentName(t *testing.T) {
	profile := &models.AgentProfile{
		Name:             "Researcher",
		AgentDisplayName: "Researcher",
		WorkspaceID:      "workspace-1",
	}
	caps := hostutility.AgentCapabilities{
		CurrentModelID: "claude-sonnet",
		Models: []hostutility.Model{
			{ID: "claude-sonnet", Name: "Default (recommended)"},
		},
	}

	if changed := healProfileName(profile, caps); changed {
		t.Fatal("office agent name was treated as an untouched global profile name")
	}
	if profile.Name != "Researcher" {
		t.Fatalf("name = %q, want Researcher", profile.Name)
	}
}

func TestProfileReconciler_HealsStaleCodexModeAfterBridgeSwap(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "codex-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID: dbAgent.ID,
		Name:    "Codex",
		Model:   "gpt-5.6-sol",
		Mode:    "auto",
	}
	_ = st.CreateAgentProfile(context.Background(), existing)
	st.created = nil

	ag := &mockInferenceAgent{id: "codex-acp", displayName: "Codex", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"codex-acp": {
				AgentType:      "codex-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "gpt-5.6-sol", Name: "GPT 5.6 Sol"}},
				CurrentModelID: "gpt-5.6-sol",
				Modes: []hostutility.Mode{
					{ID: "read-only", Name: "Read Only"},
					{ID: "agent", Name: "Agent"},
					{ID: "agent-full-access", Name: "Agent Full Access"},
				},
				CurrentModeID: "agent",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 1 {
		t.Fatalf("expected 1 updated profile, got %d", len(st.updated))
	}
	updated := st.updated[0]
	if updated.Model != "gpt-5.6-sol" {
		t.Errorf("model = %q, want gpt-5.6-sol", updated.Model)
	}
	if updated.Mode != "agent" {
		t.Errorf("healed mode = %q, want agent", updated.Mode)
	}
}

func TestProfileReconciler_HealsStaleCodexModeForUserModifiedProfile(t *testing.T) {
	st := newFakeStore()
	dbAgent := &models.Agent{Name: "codex-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID:      dbAgent.ID,
		Name:         "Codex",
		Model:        "gpt-5.6-sol",
		Mode:         "auto",
		UserModified: true,
	}
	_ = st.CreateAgentProfile(context.Background(), existing)
	st.created = nil

	ag := &mockInferenceAgent{id: "codex-acp", displayName: "Codex", enabled: true}
	caps := &fakeCapReader{
		caps: map[string]hostutility.AgentCapabilities{
			"codex-acp": {
				AgentType:      "codex-acp",
				Status:         hostutility.StatusOK,
				Models:         []hostutility.Model{{ID: "gpt-5.6-sol", Name: "GPT 5.6 Sol"}},
				CurrentModelID: "gpt-5.6-sol",
				Modes: []hostutility.Mode{
					{ID: "read-only", Name: "Read Only"},
					{ID: "agent", Name: "Agent"},
					{ID: "agent-full-access", Name: "Agent Full Access"},
				},
				CurrentModeID: "agent",
			},
		},
	}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.updated) != 1 {
		t.Fatalf("expected 1 updated profile, got %d", len(st.updated))
	}
	if got := st.updated[0].Mode; got != "agent" {
		t.Fatalf("healed mode = %q, want agent", got)
	}
}

func TestProfileReconciler_CleansOrphanProfiles(t *testing.T) {
	st := newFakeStore()
	// Seed a DB row for an agent that is NOT registered in the registry.
	orphanAgent := &models.Agent{Name: "removed-old-agent"}
	_ = st.CreateAgent(context.Background(), orphanAgent)
	orphanProfile := &models.AgentProfile{
		AgentID: orphanAgent.ID,
		Name:    "legacy",
		Model:   "x",
	}
	_ = st.CreateAgentProfile(context.Background(), orphanProfile)

	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	caps := &fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 1 || st.softDeleted[0] != orphanProfile.ID {
		t.Fatalf("expected orphan profile to be soft-deleted, got %v", st.softDeleted)
	}
}

func TestProfileReconciler_SkipsOrphanCleanupUntilRegistryReady(t *testing.T) {
	st := newFakeStore()
	orphanAgent := &models.Agent{Name: "removed-old-agent"}
	_ = st.CreateAgent(context.Background(), orphanAgent)
	orphanProfile := &models.AgentProfile{
		AgentID: orphanAgent.ID,
		Name:    "legacy",
		Model:   "x",
	}
	_ = st.CreateAgentProfile(context.Background(), orphanProfile)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}
	if err := reg.Register(ag); err != nil {
		t.Fatalf("register: %v", err)
	}
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected not-ready registry cleanup to fail closed, got soft-deletes %v", st.softDeleted)
	}
}

func TestProfileReconciler_SkipsOrphanCleanupWhenEnabledRegistryEmpty(t *testing.T) {
	st := newFakeStore()
	claudeAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), claudeAgent)
	claudeProfile := &models.AgentProfile{
		AgentID: claudeAgent.ID,
		Name:    "Claude",
		Model:   "claude-sonnet",
	}
	_ = st.CreateAgentProfile(context.Background(), claudeProfile)
	codexAgent := &models.Agent{Name: "codex-acp"}
	_ = st.CreateAgent(context.Background(), codexAgent)
	codexProfile := &models.AgentProfile{
		AgentID: codexAgent.ID,
		Name:    "Codex",
		Model:   "gpt-5",
	}
	_ = st.CreateAgentProfile(context.Background(), codexProfile)

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	reg.MarkLoaded()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected empty registry cleanup to fail closed, got soft-deletes %v", st.softDeleted)
	}
	if len(st.profiles[claudeAgent.ID]) != 1 || len(st.profiles[codexAgent.ID]) != 1 {
		t.Fatalf("expected live profiles to remain, got claude=%d codex=%d",
			len(st.profiles[claudeAgent.ID]), len(st.profiles[codexAgent.ID]))
	}
}

func TestProfileReconciler_SkipsOrphanCleanupWhenAgentListFails(t *testing.T) {
	st := newFakeStore()
	st.listAgentsErr = errors.New("database locked")
	orphanAgent := &models.Agent{Name: "removed-old-agent"}
	_ = st.CreateAgent(context.Background(), orphanAgent)
	orphanProfile := &models.AgentProfile{
		AgentID: orphanAgent.ID,
		Name:    "legacy",
		Model:   "x",
	}
	_ = st.CreateAgentProfile(context.Background(), orphanProfile)

	log, logs := newObserverLogger(t)
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected agent-list failure to fail closed, got soft-deletes %v", st.softDeleted)
	}
	entries := logs.FilterMessage("orphan cleanup summary").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 summary log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["skip_reason"] != "list_agents_failed" {
		t.Errorf("skip_reason = %v, want list_agents_failed", fields["skip_reason"])
	}
}

func TestProfileReconciler_SkipsOrphanCleanupWhenBatchExceedsSafetyLimit(t *testing.T) {
	st := newFakeStore()
	for i := 1; i <= 21; i++ {
		orphanAgent := &models.Agent{Name: "removed-old-agent-" + strconv.Itoa(i)}
		_ = st.CreateAgent(context.Background(), orphanAgent)
		orphanProfile := &models.AgentProfile{
			AgentID: orphanAgent.ID,
			Name:    "legacy",
			Model:   "x",
		}
		_ = st.CreateAgentProfile(context.Background(), orphanProfile)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected oversized cleanup batch to fail closed, got soft-deletes %v", st.softDeleted)
	}
}

func TestProfileReconciler_SkipsOrphanCleanupWhenProfileCountExceedsSafetyLimit(t *testing.T) {
	st := newFakeStore()
	orphanAgent := &models.Agent{Name: "removed-old-agent"}
	_ = st.CreateAgent(context.Background(), orphanAgent)
	for i := 1; i <= 51; i++ {
		orphanProfile := &models.AgentProfile{
			AgentID: orphanAgent.ID,
			Name:    "legacy-" + strconv.Itoa(i),
			Model:   "x",
		}
		_ = st.CreateAgentProfile(context.Background(), orphanProfile)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected oversized profile batch to fail closed, got soft-deletes %v", st.softDeleted)
	}
}

func TestProfileReconciler_SkipsOrphanCleanupWhenCandidateProfileListFails(t *testing.T) {
	st := newFakeStore()
	failingAgent := &models.Agent{Name: "removed-old-agent-failing"}
	_ = st.CreateAgent(context.Background(), failingAgent)
	st.listProfErr = map[string]error{
		failingAgent.ID: errors.New("database locked"),
	}
	deletableAgent := &models.Agent{Name: "removed-old-agent-deletable"}
	_ = st.CreateAgent(context.Background(), deletableAgent)
	deletableProfile := &models.AgentProfile{
		AgentID: deletableAgent.ID,
		Name:    "legacy",
		Model:   "x",
	}
	_ = st.CreateAgentProfile(context.Background(), deletableProfile)

	log, logs := newObserverLogger(t)
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(st.softDeleted) != 0 {
		t.Fatalf("expected profile-list failure to fail closed, got soft-deletes %v", st.softDeleted)
	}
	entries := logs.FilterMessage("orphan cleanup summary").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 summary log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["profiles_candidate_partial"] != true {
		t.Errorf("profiles_candidate_partial = %v, want true", fields["profiles_candidate_partial"])
	}
	if fields["profiles_candidate_count"] != int64(0) {
		t.Errorf("profiles_candidate_count = %v, want 0 for partial collection", fields["profiles_candidate_count"])
	}
}

func TestProfileReconciler_LogsOrphanCleanupSummary(t *testing.T) {
	st := newFakeStore()
	orphanAgent := &models.Agent{Name: "removed-old-agent"}
	_ = st.CreateAgent(context.Background(), orphanAgent)
	orphanProfile := &models.AgentProfile{
		AgentID: orphanAgent.ID,
		Name:    "legacy",
		Model:   "x",
	}
	_ = st.CreateAgentProfile(context.Background(), orphanProfile)

	log, logs := newObserverLogger(t)
	reg := registry.NewRegistry(log)
	reg.LoadDefaults()
	r := NewProfileReconciler(&fakeCapReader{caps: map[string]hostutility.AgentCapabilities{}}, reg, st, log)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	entries := logs.FilterMessage("orphan cleanup summary").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 summary log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["db_agent_count"] != int64(2) {
		t.Errorf("db_agent_count = %v, want 2 including the dynamic virtual family", fields["db_agent_count"])
	}
	if fields["orphan_agent_count"] != int64(1) {
		t.Errorf("orphan_agent_count = %v, want 1", fields["orphan_agent_count"])
	}
	if fields["profiles_deleted_count"] != int64(1) {
		t.Errorf("profiles_deleted_count = %v, want 1", fields["profiles_deleted_count"])
	}
	if fields["profiles_candidate_partial"] != false {
		t.Errorf("profiles_candidate_partial = %v, want false", fields["profiles_candidate_partial"])
	}
	if fields["skipped"] != false {
		t.Errorf("skipped = %v, want false", fields["skipped"])
	}
}

// newDiscoveryController builds a Controller with just the dependencies that
// syncAgentFromDiscovery touches: the agent registry, the store, and a logger.
func newDiscoveryController(t *testing.T, st *fakeStore, ag agents.Agent) *Controller {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(ag); err != nil {
		t.Fatalf("register: %v", err)
	}
	return &Controller{agentRegistry: reg, repo: st, logger: log}
}

// TestSyncAgentFromDiscovery_DoesNotReseedAfterUserDelete pins the discovery
// seed path — the other half of the fix. An agent whose only profile the user
// deleted must NOT have a default recreated when discovery runs on boot.
func TestSyncAgentFromDiscovery_DoesNotReseedAfterUserDelete(t *testing.T) {
	st := newFakeStore()
	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}

	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	deletedProfile := &models.AgentProfile{AgentID: dbAgent.ID, Name: "Sonnet 4.6", Model: "claude-sonnet"}
	_ = st.CreateAgentProfile(context.Background(), deletedProfile)
	_ = st.DeleteAgentProfile(context.Background(), deletedProfile.ID)
	st.created = nil // discard setup bookkeeping; measure only what sync seeds

	c := newDiscoveryController(t, st, ag)
	result := discovery.Availability{Name: "claude-acp", Available: true}
	if err := c.syncAgentFromDiscovery(context.Background(), result); err != nil {
		t.Fatalf("syncAgentFromDiscovery: %v", err)
	}
	if len(st.created) != 0 {
		t.Fatalf("expected no profile to be recreated after user delete, got %d: %+v",
			len(st.created), st.created)
	}
}

// TestSyncAgentFromDiscovery_SeedsFreshAgent is the positive counterpart: an
// agent that has never been provisioned (no live and no deleted rows) still
// gets its default profile, so the guard does not break new-agent detection.
func TestSyncAgentFromDiscovery_SeedsFreshAgent(t *testing.T) {
	st := newFakeStore()
	ag := &mockInferenceAgent{id: "claude-acp", displayName: "Claude", enabled: true}

	c := newDiscoveryController(t, st, ag)
	result := discovery.Availability{Name: "claude-acp", Available: true}
	if err := c.syncAgentFromDiscovery(context.Background(), result); err != nil {
		t.Fatalf("syncAgentFromDiscovery: %v", err)
	}
	if len(st.created) != 1 {
		t.Fatalf("expected fresh agent to be seeded, got %d created", len(st.created))
	}
}

func TestProfileReconciler_MigratesCursorVariantModel(t *testing.T) {
	st := newFakeStore()
	ag := &mockInferenceAgent{id: acpcompat.CursorAgentID, displayName: "Cursor", enabled: true}
	dbAgent := &models.Agent{Name: acpcompat.CursorAgentID}
	if err := st.CreateAgent(context.Background(), dbAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	profile := &models.AgentProfile{
		AgentID: dbAgent.ID,
		Name:    "Grok",
		Model:   "grok-4.5[effort=high,fast=true]",
		ConfigOptions: map[string]string{
			"effort": "low",
		},
	}
	if err := st.CreateAgentProfile(context.Background(), profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	st.updated = nil

	caps := &fakeCapReader{caps: map[string]hostutility.AgentCapabilities{
		acpcompat.CursorAgentID: {
			AgentType:      acpcompat.CursorAgentID,
			Status:         hostutility.StatusOK,
			Models:         []hostutility.Model{{ID: "grok-4.5"}},
			CurrentModelID: "grok-4.5",
		},
	}}
	r := newReconciler(t, st, caps, ag)
	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	stored, err := st.GetAgentProfile(context.Background(), profile.ID)
	if err != nil {
		t.Fatalf("get migrated profile: %v", err)
	}
	if stored.Model != "grok-4.5" {
		t.Fatalf("profile model = %q, want bare Cursor model", stored.Model)
	}
	if got := stored.ConfigOptions["effort"]; got != "low" {
		t.Errorf("profile effort = %q, want existing profile value low", got)
	}
	if got := stored.ConfigOptions["fast"]; got != "true" {
		t.Errorf("profile fast = %q, want migrated value true", got)
	}
	if len(st.updated) == 0 {
		t.Fatal("expected migrated profile to be persisted")
	}
}
