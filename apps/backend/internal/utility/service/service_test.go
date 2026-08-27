package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentsettingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/utility/models"
	"github.com/kandev/kandev/internal/utility/profilebinding"
	"github.com/kandev/kandev/internal/utility/template"
)

type fakeProfileResolver struct {
	profile *agentsettingsmodels.AgentProfile
	err     error
}

func (r fakeProfileResolver) Resolve(context.Context, string) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}
func (r fakeProfileResolver) MatchLegacy(context.Context, string, string) (*agentsettingsmodels.AgentProfile, error) {
	return r.profile, r.err
}

func TestMigrateLegacyBindingsUpdatesOnlyUnambiguousRows(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"legacy":  {ID: "legacy", AgentID: "codex-acp", Model: "gpt-5", ProfileBindingState: models.ProfileBindingExplicit},
		"inherit": {ID: "inherit", ProfileBindingState: models.ProfileBindingInherit},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "profile-1"}})
	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil || updated != 1 {
		t.Fatalf("MigrateLegacyBindings() = (%d, %v), want (1, nil)", updated, err)
	}
	if repo.agents["legacy"].AgentProfileID != "profile-1" || repo.agents["legacy"].ProfileBindingState != models.ProfileBindingExplicit {
		t.Fatalf("legacy row was not migrated: %#v", repo.agents["legacy"])
	}
	if repo.agents["inherit"].ProfileBindingState != models.ProfileBindingInherit {
		t.Fatalf("inherit row changed: %#v", repo.agents["inherit"])
	}
}

func TestPreparePromptRequestUsesFreshUtilityRouteForTaskBoundCall(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"utility": {ID: "utility", AgentProfileID: "dynamic", ProfileBindingState: models.ProfileBindingExplicit, Prompt: "summarize"},
	}}
	resolver := &recordingExecutionDetailsResolver{}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "dynamic", AgentID: "dynamic"}})
	svc.SetExecutionProfileResolver(resolver)

	ctx := &template.Context{SessionID: "task-session"}
	first, err := svc.PreparePromptRequest(context.Background(), "utility", ctx, nil, false)
	if err != nil {
		t.Fatalf("first PreparePromptRequest: %v", err)
	}
	second, err := svc.PreparePromptRequest(context.Background(), "utility", ctx, nil, false)
	if err != nil {
		t.Fatalf("second PreparePromptRequest: %v", err)
	}
	if first.RouteSessionID == "" || second.RouteSessionID == "" || first.RouteSessionID == second.RouteSessionID {
		t.Fatalf("route IDs = %q and %q, want distinct utility invocations", first.RouteSessionID, second.RouteSessionID)
	}
	if len("utility:") > len(first.RouteSessionID) || first.RouteSessionID[:len("utility:")] != "utility:" ||
		len("utility:") > len(second.RouteSessionID) || second.RouteSessionID[:len("utility:")] != "utility:" {
		t.Fatalf("route IDs = %q and %q, want utility prefix", first.RouteSessionID, second.RouteSessionID)
	}
	if resolver.sessions[0] == "task-session" || resolver.sessions[1] == "task-session" {
		t.Fatalf("resolver received task session identity: %#v", resolver.sessions)
	}
}

type recordingExecutionDetailsResolver struct{ sessions []string }

func (r *recordingExecutionDetailsResolver) ResolveExecution(context.Context, string) (*agentsettingsmodels.AgentProfile, string, error) {
	return nil, "", nil
}

func (r *recordingExecutionDetailsResolver) ResolveExecutionDetails(_ context.Context, sessionID, _ string) (agentruntime.ProfileExecution, error) {
	r.sessions = append(r.sessions, sessionID)
	return agentruntime.ProfileExecution{
		LogicalProfileID: "dynamic", ExecutionProfileID: "concrete", RouteSessionID: sessionID,
		Generation: 1, Profile: &agentsettingsmodels.AgentProfile{ID: "concrete", AgentID: "claude-acp"},
	}, nil
}

func TestMigrateLegacyBindingsNormalizesEmptyUnconfiguredBuiltin(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{})

	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil {
		t.Fatalf("MigrateLegacyBindings() error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("MigrateLegacyBindings() updated = %d, want 1", updated)
	}
	if got := repo.agents["builtin"].ProfileBindingState; got != models.ProfileBindingInherit {
		t.Fatalf("profile binding state = %q, want %q", got, models.ProfileBindingInherit)
	}
	if repo.agents["builtin"].Enabled {
		t.Fatal("migration enabled the built-in action")
	}

	updated, err = svc.MigrateLegacyBindings(context.Background())
	if err != nil || updated != 0 {
		t.Fatalf("second MigrateLegacyBindings() = (%d, %v), want (0, nil)", updated, err)
	}
}

func TestMigrateLegacyBindingsDoesNotOverwriteConcurrentRepair(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
	}}
	repo.normalizeEmptyBuiltinBindingHook = func(agent *models.UtilityAgent) {
		agent.AgentProfileID = "profile-1"
		agent.ProfileBindingState = models.ProfileBindingExplicit
	}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{})

	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil || updated != 0 {
		t.Fatalf("MigrateLegacyBindings() = (%d, %v), want (0, nil)", updated, err)
	}
	got := repo.agents["builtin"]
	if got.AgentProfileID != "profile-1" || got.ProfileBindingState != models.ProfileBindingExplicit {
		t.Fatalf("concurrent repair was overwritten: %#v", got)
	}
}

func TestMigrateLegacyBindingsPreservesConcreteAndCustomUnconfiguredBindings(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Builtin:             true,
			AgentProfileID:      "deleted-profile",
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
		"custom": {
			ID:                  "custom",
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{})

	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil || updated != 0 {
		t.Fatalf("MigrateLegacyBindings() = (%d, %v), want (0, nil)", updated, err)
	}
	for id, agent := range repo.agents {
		if agent.ProfileBindingState != models.ProfileBindingUnconfigured {
			t.Fatalf("%s binding state = %q, want %q", id, agent.ProfileBindingState, models.ProfileBindingUnconfigured)
		}
	}
	if got := repo.agents["builtin"].AgentProfileID; got != "deleted-profile" {
		t.Fatalf("builtin profile ID = %q, want %q", got, "deleted-profile")
	}
}

func TestMigrateLegacyBindingsNormalizesEmptyExplicitBuiltin(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingExplicit,
		},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{})

	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil {
		t.Fatalf("MigrateLegacyBindings() error = %v", err)
	}
	if updated != 1 {
		t.Fatalf("MigrateLegacyBindings() updated = %d, want 1", updated)
	}
	if got := repo.agents["builtin"].ProfileBindingState; got != models.ProfileBindingInherit {
		t.Fatalf("profile binding state = %q, want %q", got, models.ProfileBindingInherit)
	}
}

func TestMigrateLegacyBindingsMakesAmbiguousBuiltinInheritButCustomUnconfigured(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			AgentID:             "legacy-agent",
			Model:               "legacy-model",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingExplicit,
		},
		"custom": {
			ID:                  "custom",
			AgentID:             "legacy-agent",
			Model:               "legacy-model",
			ProfileBindingState: models.ProfileBindingExplicit,
		},
	}}
	svc := NewService(repo)
	svc.SetProfileResolver(fakeProfileResolver{err: profilebinding.ErrLegacyBindingAmbiguous})

	updated, err := svc.MigrateLegacyBindings(context.Background())
	if err != nil {
		t.Fatalf("MigrateLegacyBindings() error = %v", err)
	}
	if updated != 2 {
		t.Fatalf("MigrateLegacyBindings() updated = %d, want 2", updated)
	}
	if got := repo.agents["builtin"].ProfileBindingState; got != models.ProfileBindingInherit {
		t.Fatalf("builtin state = %q, want %q", got, models.ProfileBindingInherit)
	}
	if got := repo.agents["custom"].ProfileBindingState; got != models.ProfileBindingUnconfigured {
		t.Fatalf("custom state = %q, want %q", got, models.ProfileBindingUnconfigured)
	}
}

func TestClearAgentProfileBindingsRetainsStaleProfileID(t *testing.T) {
	repo := &fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Builtin:             true,
			AgentProfileID:      "deleted-profile",
			ProfileBindingState: models.ProfileBindingExplicit,
		},
		"inherited": {
			ID:                  "inherited",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingInherit,
		},
	}}
	svc := NewService(repo)

	if err := svc.ClearAgentProfileBindings(context.Background(), "deleted-profile"); err != nil {
		t.Fatalf("ClearAgentProfileBindings() error = %v", err)
	}
	deleted := repo.agents["builtin"]
	if deleted.AgentProfileID != "deleted-profile" {
		t.Fatalf("stale profile ID = %q, want %q", deleted.AgentProfileID, "deleted-profile")
	}
	if deleted.ProfileBindingState != models.ProfileBindingUnconfigured {
		t.Fatalf("deleted profile state = %q, want %q", deleted.ProfileBindingState, models.ProfileBindingUnconfigured)
	}
	if got := repo.agents["inherited"].ProfileBindingState; got != models.ProfileBindingInherit {
		t.Fatalf("inherited state = %q, want %q", got, models.ProfileBindingInherit)
	}
}

type fakeRepository struct {
	agents                           map[string]*models.UtilityAgent
	normalizeEmptyBuiltinBindingHook func(*models.UtilityAgent)
}

func (r *fakeRepository) ListAgents(context.Context) ([]*models.UtilityAgent, error) {
	out := make([]*models.UtilityAgent, 0, len(r.agents))
	for _, agent := range r.agents {
		out = append(out, agent)
	}
	return out, nil
}

func (r *fakeRepository) GetAgentByID(_ context.Context, id string) (*models.UtilityAgent, error) {
	agent, ok := r.agents[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return agent, nil
}

func (r *fakeRepository) GetAgentByName(_ context.Context, name string) (*models.UtilityAgent, error) {
	for _, agent := range r.agents {
		if agent.Name == name {
			return agent, nil
		}
	}
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) CreateAgent(_ context.Context, agent *models.UtilityAgent) error {
	r.agents[agent.ID] = agent
	return nil
}

func (r *fakeRepository) UpdateAgent(_ context.Context, agent *models.UtilityAgent) error {
	r.agents[agent.ID] = agent
	return nil
}

func (r *fakeRepository) NormalizeEmptyBuiltinBinding(_ context.Context, id string) (bool, error) {
	agent := r.agents[id]
	if r.normalizeEmptyBuiltinBindingHook != nil {
		r.normalizeEmptyBuiltinBindingHook(agent)
	}
	if agent == nil || !agent.Builtin || agent.AgentProfileID != "" ||
		agent.ProfileBindingState != models.ProfileBindingUnconfigured {
		return false, nil
	}
	agent.ProfileBindingState = models.ProfileBindingInherit
	return true, nil
}

func (r *fakeRepository) DeleteAgent(_ context.Context, id string) error {
	delete(r.agents, id)
	return nil
}

func (r *fakeRepository) ListCalls(context.Context, string, int) ([]*models.UtilityAgentCall, error) {
	return nil, nil
}

func (r *fakeRepository) GetCallByID(context.Context, string) (*models.UtilityAgentCall, error) {
	return nil, sql.ErrNoRows
}

func (r *fakeRepository) CreateCall(context.Context, *models.UtilityAgentCall) error {
	return nil
}

func (r *fakeRepository) UpdateCall(context.Context, *models.UtilityAgentCall) error {
	return nil
}

func TestPreparePromptRequest_UsesDefaultAgentAndModelAsPairWhenModelIsUnset(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin-enhance-prompt": {
			ID:      "builtin-enhance-prompt",
			Prompt:  "Improve {{UserPrompt}}",
			AgentID: "claude-acp",
			Model:   "",
			Builtin: true,
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin-enhance-prompt",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "opencode-acp",
			Model:   "opencode-go/deepseek-v4-flash",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "opencode-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "opencode-acp")
	}
	if req.Model != "opencode-go/deepseek-v4-flash" {
		t.Fatalf("Model = %q, want %q", req.Model, "opencode-go/deepseek-v4-flash")
	}
}

func TestPreparePromptRequest_UsesDefaultAgentWhenAgentIDIsUnset(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "default-acp",
			Model:   "default-model",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "default-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "default-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}

func TestPreparePromptRequest_PreservesConfiguredAgentAndModelWithoutDefaults(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "custom-acp",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "custom-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "custom-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}

func TestPreparePromptRequest_IgnoresDefaultsWhenAgentAndModelAreConfigured(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"custom": {
			ID:      "custom",
			Prompt:  "Do {{UserPrompt}}",
			AgentID: "custom-acp",
			Model:   "custom-model",
		},
	}})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"custom",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{
			AgentID: "default-acp",
			Model:   "default-model",
		},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}

	if req.AgentCLI != "custom-acp" {
		t.Fatalf("AgentCLI = %q, want %q", req.AgentCLI, "custom-acp")
	}
	if req.Model != "custom-model" {
		t.Fatalf("Model = %q, want %q", req.Model, "custom-model")
	}
}

func TestPreparePromptRequest_ResolvesInheritedBuiltinFromDefaultProfile(t *testing.T) {
	t.Parallel()

	defaultProfile := &agentsettingsmodels.AgentProfile{
		ID:      "default-profile",
		AgentID: "codex-acp",
		Model:   "gpt-5",
	}
	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Prompt:              "Do {{UserPrompt}}",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingInherit,
		},
	}})
	svc.SetProfileResolver(fakeProfileResolver{profile: defaultProfile})

	req, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{ProfileID: "default-profile"},
		false,
	)
	if err != nil {
		t.Fatalf("PreparePromptRequest() error = %v", err)
	}
	if req.AgentProfileID != "default-profile" {
		t.Fatalf("AgentProfileID = %q, want %q", req.AgentProfileID, "default-profile")
	}
}

func TestPreparePromptRequest_InheritedBuiltinRequiresDefault(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Prompt:              "Do {{UserPrompt}}",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingInherit,
		},
	}})
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "unused"}})

	_, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{},
		false,
	)
	if !errors.Is(err, ErrProfileRequired) {
		t.Fatalf("PreparePromptRequest() error = %v, want %v", err, ErrProfileRequired)
	}
}

func TestPreparePromptRequest_EmptyUnconfiguredBuiltinFailsClosed(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Prompt:              "Do {{UserPrompt}}",
			Builtin:             true,
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
	}})
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "default-profile"}})

	_, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{ProfileID: "default-profile"},
		false,
	)
	if !errors.Is(err, ErrProfileUnconfigured) {
		t.Fatalf("PreparePromptRequest() error = %v, want %v", err, ErrProfileUnconfigured)
	}
}

func TestPreparePromptRequest_ConcreteUnconfiguredBindingFailsClosed(t *testing.T) {
	t.Parallel()

	svc := NewService(&fakeRepository{agents: map[string]*models.UtilityAgent{
		"builtin": {
			ID:                  "builtin",
			Prompt:              "Do {{UserPrompt}}",
			Builtin:             true,
			AgentProfileID:      "deleted-profile",
			ProfileBindingState: models.ProfileBindingUnconfigured,
		},
	}})
	svc.SetProfileResolver(fakeProfileResolver{profile: &agentsettingsmodels.AgentProfile{ID: "deleted-profile"}})

	_, err := svc.PreparePromptRequest(
		context.Background(),
		"builtin",
		&template.Context{UserPrompt: "fix the bug"},
		&DefaultUtilitySettings{ProfileID: "default-profile"},
		false,
	)
	if !errors.Is(err, ErrProfileUnconfigured) {
		t.Fatalf("PreparePromptRequest() error = %v, want %v", err, ErrProfileUnconfigured)
	}
}
