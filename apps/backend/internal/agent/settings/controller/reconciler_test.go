package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
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
	agents       map[string]*models.Agent          // keyed by DB ID
	byName       map[string]*models.Agent          // keyed by Name
	profiles     map[string][]*models.AgentProfile // keyed by DB agent ID
	created      []*models.AgentProfile
	updated      []*models.AgentProfile
	softDeleted  []string
	nextAgentID  int
	nextProfID   int
	getByNameErr error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		agents:   map[string]*models.Agent{},
		byName:   map[string]*models.Agent{},
		profiles: map[string][]*models.AgentProfile{},
	}
}

func (f *fakeStore) CreateAgent(_ context.Context, a *models.Agent) error {
	f.nextAgentID++
	a.ID = "agent-" + itoa(f.nextAgentID)
	f.agents[a.ID] = a
	f.byName[a.Name] = a
	return nil
}

func (f *fakeStore) GetAgent(_ context.Context, id string) (*models.Agent, error) {
	return f.agents[id], nil
}

func (f *fakeStore) GetAgentByName(_ context.Context, name string) (*models.Agent, error) {
	if f.getByNameErr != nil {
		return nil, f.getByNameErr
	}
	if a, ok := f.byName[name]; ok {
		return a, nil
	}
	return nil, sql.ErrNoRows
}

func (f *fakeStore) UpdateAgent(context.Context, *models.Agent) error { return nil }
func (f *fakeStore) DeleteAgent(context.Context, string) error        { return nil }

func (f *fakeStore) ListAgents(_ context.Context) ([]*models.Agent, error) {
	out := make([]*models.Agent, 0, len(f.agents))
	for _, a := range f.agents {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeStore) ListTUIAgents(context.Context) ([]*models.Agent, error) {
	return nil, nil
}

func (f *fakeStore) GetAgentProfileMcpConfig(context.Context, string) (*models.AgentProfileMcpConfig, error) {
	return nil, nil
}
func (f *fakeStore) UpsertAgentProfileMcpConfig(context.Context, *models.AgentProfileMcpConfig) error {
	return nil
}

func (f *fakeStore) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	f.nextProfID++
	p.ID = "profile-" + itoa(f.nextProfID)
	f.profiles[p.AgentID] = append(f.profiles[p.AgentID], p)
	f.created = append(f.created, p)
	return nil
}

func (f *fakeStore) UpdateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	f.updated = append(f.updated, p)
	return nil
}

func (f *fakeStore) DeleteAgentProfile(_ context.Context, id string) error {
	f.softDeleted = append(f.softDeleted, id)
	return nil
}

func (f *fakeStore) GetAgentProfile(context.Context, string) (*models.AgentProfile, error) {
	return nil, nil
}

func (f *fakeStore) ListAgentProfiles(_ context.Context, agentID string) ([]*models.AgentProfile, error) {
	return f.profiles[agentID], nil
}

func (f *fakeStore) Close() error { return nil }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

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
	return NewProfileReconciler(caps, reg, st, log)
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

func TestProfileReconciler_HealsStaleModel(t *testing.T) {
	st := newFakeStore()
	// Seed an existing DB agent and profile with a stale model.
	dbAgent := &models.Agent{Name: "claude-acp"}
	_ = st.CreateAgent(context.Background(), dbAgent)
	existing := &models.AgentProfile{
		AgentID: dbAgent.ID,
		Name:    "Claude",
		Model:   "claude-gone",
		Mode:    "",
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
	if len(st.updated) != 1 {
		t.Fatalf("expected 1 updated profile, got %d", len(st.updated))
	}
	updated := st.updated[0]
	if updated.Model != "claude-new" {
		t.Errorf("healed model = %q, want claude-new", updated.Model)
	}
	if updated.Mode != "default" {
		t.Errorf("backfilled mode = %q, want default", updated.Mode)
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
