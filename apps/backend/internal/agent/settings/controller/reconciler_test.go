package controller

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/usage"
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
	agents        map[string]*models.Agent          // keyed by DB ID
	byName        map[string]*models.Agent          // keyed by Name
	profiles      map[string][]*models.AgentProfile // keyed by DB agent ID (live)
	deleted       map[string][]*models.AgentProfile // keyed by DB agent ID (soft-deleted)
	created       []*models.AgentProfile
	updated       []*models.AgentProfile
	softDeleted   []string
	nextAgentID   int
	nextProfID    int
	getByNameErr  error
	listAgentsErr error
	listProfErr   map[string]error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		agents:   map[string]*models.Agent{},
		byName:   map[string]*models.Agent{},
		profiles: map[string][]*models.AgentProfile{},
		deleted:  map[string][]*models.AgentProfile{},
	}
}

func (f *fakeStore) CreateAgent(_ context.Context, a *models.Agent) error {
	f.nextAgentID++
	a.ID = "agent-" + strconv.Itoa(f.nextAgentID)
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

func (f *fakeStore) GetAgentProfileMcpConfig(context.Context, string) (*models.AgentProfileMcpConfig, error) {
	return nil, nil
}
func (f *fakeStore) UpsertAgentProfileMcpConfig(context.Context, *models.AgentProfileMcpConfig) error {
	return nil
}

func (f *fakeStore) CreateAgentProfile(_ context.Context, p *models.AgentProfile) error {
	f.nextProfID++
	p.ID = "profile-" + strconv.Itoa(f.nextProfID)
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
				return p, nil
			}
		}
	}
	return nil, sql.ErrNoRows
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
				return p, nil
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
	return f.profiles[agentID], nil
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
	if fields["db_agent_count"] != int64(1) {
		t.Errorf("db_agent_count = %v, want 1", fields["db_agent_count"])
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
