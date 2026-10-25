package controller

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
)

type enabledIntentSnapshotStore struct {
	store.Repository
	afterSnapshot func(context.Context) error
}

type enabledIntentAfterWriteStore struct {
	store.Repository
	store.DynamicProfileRepository
	store.AtomicDynamicProfileRepository
	afterWrite func(context.Context) error
}

func (r *enabledIntentAfterWriteStore) after(ctx context.Context, err error) error {
	if err != nil || r.afterWrite == nil {
		return err
	}
	hook := r.afterWrite
	r.afterWrite = nil
	return hook(ctx)
}

func (r *enabledIntentAfterWriteStore) UpdateAgentProfileWithEnabledIntent(ctx context.Context, p *models.AgentProfile, enabled *bool) error {
	return r.after(ctx, r.Repository.UpdateAgentProfileWithEnabledIntent(ctx, p, enabled))
}

func (r *enabledIntentAfterWriteStore) UpdateAgentProfileWithDynamicEnabledIntent(ctx context.Context,
	p *models.AgentProfile, dynamic *models.DynamicAgentProfile, version int64, routes []models.DynamicAgentRoute, enabled *bool,
) error {
	return r.after(ctx, r.AtomicDynamicProfileRepository.UpdateAgentProfileWithDynamicEnabledIntent(ctx, p, dynamic, version, routes, enabled))
}

func TestProfileEnabledIntentOwnCommit(t *testing.T) {
	for _, dynamic := range []bool{false, true} {
		for _, committed := range []bool{false, true} {
			t.Run(fmt.Sprintf("dynamic_%t_committed_%t", dynamic, committed), func(t *testing.T) {
				ctrl, repo, id := enabledIntentFixture(t, committed)
				req := UpdateProfileRequest{ID: id}
				if dynamic {
					var profile *dto.AgentProfileDTO
					ctrl, repo, profile = enabledIntentDynamicFixture(t)
					id = profile.ID
					req = UpdateProfileRequest{ID: id, Dynamic: profile.Dynamic}
					if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: id, Enabled: &committed}); err != nil {
						t.Fatal(err)
					}
				}
				toggler := NewController(repo, nil, ctrl.agentRegistry, nil, ctrl.logger)
				toggler.SetDynamicAgentRoutingEnabled(dynamic)
				later := !committed
				ctrl.repo = &enabledIntentAfterWriteStore{
					Repository: repo, DynamicProfileRepository: repo.(store.DynamicProfileRepository),
					AtomicDynamicProfileRepository: repo.(store.AtomicDynamicProfileRepository),
					afterWrite: func(ctx context.Context) error {
						_, err := toggler.UpdateProfile(ctx, UpdateProfileRequest{ID: id, Enabled: &later})
						return err
					},
				}
				name := "Own commit"
				req.Name = &name
				result, err := ctrl.UpdateProfile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				saved, err := repo.GetAgentProfile(context.Background(), id)
				if err != nil {
					t.Fatal(err)
				}
				if result.Enabled != committed || saved.Enabled != later || result.Name != name || saved.Name != name {
					t.Fatalf("own enabled=%t later stored=%t; want %t/%t", result.Enabled, saved.Enabled, committed, later)
				}
			})
		}
	}
}

func TestProfileEnabledIntentControls(t *testing.T) {
	t.Run("policy_and_mcp", testEnabledIntentPolicyAndMCP)
	t.Run("dependency_force_and_dynamic_explicit", testEnabledIntentDependencyAndDynamicExplicit)
	for _, enabled := range []bool{true, false} {
		for _, explicit := range []bool{false, true} {
			t.Run(fmt.Sprintf("enabled_%t_explicit_%t", enabled, explicit), func(t *testing.T) {
				ctrl, repo, id := enabledIntentFixture(t, enabled)
				other := &models.AgentProfile{AgentID: "intent-agent", Name: "Other", Model: "other-model"}
				if err := repo.CreateAgentProfile(context.Background(), other); err != nil {
					t.Fatal(err)
				}
				name := "Renamed"
				req := UpdateProfileRequest{ID: id, Name: &name}
				wanted := enabled
				if explicit {
					wanted = !enabled
					req.Enabled = &wanted
				}
				result, err := ctrl.UpdateProfile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				assertEnabledIntentSaved(t, repo, id, result, wanted, "name", name)
				for _, noop := range []UpdateProfileRequest{{ID: id}, {ID: id, Name: &name}} {
					result, err := ctrl.UpdateProfile(context.Background(), noop)
					if err != nil || result.Enabled != wanted || result.UpdatedAt.IsZero() || !result.UserModified {
						t.Fatalf("noop result=%#v error=%v", result, err)
					}
				}
				savedOther, err := repo.GetAgentProfile(context.Background(), other.ID)
				if err != nil || savedOther.Name != other.Name || savedOther.Model != other.Model || !savedOther.Enabled {
					t.Fatalf("independent profile changed: %#v error=%v", savedOther, err)
				}
			})
		}
	}
}

func TestProfileEnabledIntentDynamicRejection(t *testing.T) {
	ctrl, repo, profile := enabledIntentDynamicFixture(t)
	ctx := context.Background()
	disabled := false
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	before, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	dynamicRepo := repo.(store.DynamicProfileRepository)
	beforeDynamic, beforeRoutes, err := dynamicRepo.GetDynamicAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	profile.Dynamic.Version++
	name, enabled := "Rejected", true
	result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{
		ID: profile.ID, Name: &name, Enabled: &enabled, Dynamic: profile.Dynamic,
	})
	if err != store.ErrDynamicProfileVersionConflict || result != nil {
		t.Fatalf("stale route result=%#v error=%v", result, err)
	}
	after, err := repo.GetAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterDynamic, afterRoutes, err := dynamicRepo.GetDynamicAgentProfile(ctx, profile.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeDynamic, afterDynamic) || !reflect.DeepEqual(beforeRoutes, afterRoutes) {
		t.Fatal("stale routing save partially changed persisted profile or routes")
	}
}

type enabledIntentDynamicSnapshotStore struct {
	*enabledIntentSnapshotStore
	store.DynamicProfileRepository
	store.AtomicDynamicProfileRepository
}

func enabledIntentDynamicFixture(t *testing.T) (*Controller, store.Repository, *dto.AgentProfileDTO) {
	t.Helper()
	ctrl, repo := newSQLiteBackedController(t)
	for _, agent := range []agents.Agent{agents.NewDynamicAgent(), agents.NewClaudeACP()} {
		if err := ctrl.agentRegistry.Register(agent); err != nil {
			t.Fatal(err)
		}
		if err := repo.CreateAgent(context.Background(), &models.Agent{ID: agent.ID(), Name: agent.ID()}); err != nil {
			t.Fatal(err)
		}
	}
	ctrl.SetDynamicAgentRoutingEnabled(true)
	candidate := &models.AgentProfile{AgentID: agents.NewClaudeACP().ID(), Name: "Candidate", Model: "candidate-model"}
	if err := repo.CreateAgentProfile(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	profile, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: agents.DynamicAgentID, Name: "Original",
		Dynamic: &dto.DynamicAgentProfileDTO{Candidates: []dto.DynamicAgentCandidateDTO{{
			Position: 0, ExecutionProfileID: candidate.ID, Enabled: true,
			Rules: map[string]string{"on_provider_error": "try_next"},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctrl, repo, profile
}

func TestProfileEnabledIntentDynamicCaller(t *testing.T) {
	for _, routes := range []bool{false, true} {
		for _, initial := range []bool{true, false} {
			t.Run(fmt.Sprintf("routes_%t_%t_to_%t", routes, initial, !initial), func(t *testing.T) {
				ctrl, repo, profile := enabledIntentDynamicFixture(t)
				ctx := context.Background()
				if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Enabled: &initial}); err != nil {
					t.Fatal(err)
				}
				toggler := NewController(repo, nil, ctrl.agentRegistry, nil, ctrl.logger)
				toggler.SetDynamicAgentRoutingEnabled(true)
				wanted := !initial
				ctrl.repo = &enabledIntentDynamicSnapshotStore{
					enabledIntentSnapshotStore: &enabledIntentSnapshotStore{Repository: repo, afterSnapshot: func(ctx context.Context) error {
						_, err := toggler.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Enabled: &wanted})
						return err
					}},
					DynamicProfileRepository:       repo.(store.DynamicProfileRepository),
					AtomicDynamicProfileRepository: repo.(store.AtomicDynamicProfileRepository),
				}
				name := "Edited"
				req := UpdateProfileRequest{ID: profile.ID, Name: &name}
				if routes {
					req.Dynamic = profile.Dynamic
				}
				result, err := ctrl.UpdateProfile(ctx, req)
				if err != nil {
					t.Fatal(err)
				}
				saved, err := repo.GetAgentProfile(ctx, profile.ID)
				if err != nil {
					t.Fatal(err)
				}
				if saved.Enabled != wanted || result.Enabled != wanted || saved.Name != name || result.Name != name {
					t.Errorf("dynamic save stored=%s/%t returned=%s/%t want=%s/%t", saved.Name, saved.Enabled, result.Name, result.Enabled, name, wanted)
				}
				if routes && result.Dynamic.Version != profile.Dynamic.Version+1 {
					t.Errorf("route version=%d, want %d", result.Dynamic.Version, profile.Dynamic.Version+1)
				}
			})
		}
	}
}

func (r *enabledIntentSnapshotStore) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	profile, err := r.Repository.GetAgentProfile(ctx, id)
	if err != nil || r.afterSnapshot == nil {
		return profile, err
	}
	hook := r.afterSnapshot
	r.afterSnapshot = nil
	if err := hook(ctx); err != nil {
		return nil, err
	}
	return profile, nil
}

func enabledIntentFixture(t *testing.T, enabled bool) (*Controller, store.Repository, string) {
	t.Helper()
	ctrl, repo := newSQLiteBackedController(t)
	ctx := context.Background()
	agent := &models.Agent{ID: "intent-agent", Name: "custom-acp"}
	if err := repo.CreateAgent(ctx, agent); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: agent.ID, Name: "Original", Model: "original-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	return ctrl, repo, profile.ID
}

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.2 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentCaller(t *testing.T) {
	for _, initial := range []bool{true, false} {
		for _, field := range []string{"name", "model"} {
			t.Run(fmt.Sprintf("%s_%t_to_%t", field, initial, !initial), func(t *testing.T) {
				ctrl, repo, id := enabledIntentFixture(t, initial)
				toggler := NewController(repo, nil, ctrl.agentRegistry, nil, ctrl.logger)
				wanted := !initial
				ctrl.repo = &enabledIntentSnapshotStore{Repository: repo, afterSnapshot: func(ctx context.Context) error {
					_, err := toggler.UpdateProfile(ctx, UpdateProfileRequest{ID: id, Enabled: &wanted})
					return err
				}}
				req := UpdateProfileRequest{ID: id}
				value := "Edited"
				if field == "name" {
					req.Name = &value
				} else {
					req.Model = &value
				}
				result, err := ctrl.UpdateProfile(context.Background(), req)
				if err != nil {
					t.Fatal(err)
				}
				assertEnabledIntentSaved(t, repo, id, result, wanted, field, value)
			})
		}
	}
}

func assertEnabledIntentSaved(t *testing.T, repo store.Repository, id string, result *dto.AgentProfileDTO,
	wantEnabled bool, field, value string,
) {
	t.Helper()
	saved, err := repo.GetAgentProfile(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Enabled != wantEnabled || result.Enabled != wantEnabled {
		t.Errorf("committed toggle lost: stored enabled=%t, returned enabled=%t, want %t", saved.Enabled, result.Enabled, wantEnabled)
	}
	wantName, wantModel := "Original", "original-model"
	if field == "name" {
		wantName = value
	} else {
		wantModel = value
	}
	if saved.Name != wantName || saved.Model != wantModel || result.Name != wantName || result.Model != wantModel {
		t.Errorf("metadata changed incorrectly: stored=%s/%s returned=%s/%s want=%s/%s",
			saved.Name, saved.Model, result.Name, result.Model, wantName, wantModel)
	}
}

func testEnabledIntentPolicyAndMCP(t *testing.T) {
	ctrl, repo, id := enabledIntentFixture(t, false)
	ctx := context.Background()
	name, empty, exact := "Rejected", "", true
	if result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: id, Name: &name, Model: &empty, RequireExactModel: &exact}); err == nil || result != nil {
		t.Fatal("invalid exact-model policy succeeded")
	}
	before, err := repo.GetAgentProfile(ctx, id)
	if err != nil || before.Enabled || before.Name != "Original" {
		t.Fatalf("policy rejection changed row: %#v %v", before, err)
	}
	mode, servers := "selected", []string{"server-a"}
	_, err = ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: id, MCPSelectionMode: &mode, MCPSelectedServers: &servers})
	if err != nil {
		t.Fatal(err)
	}
	name = "With MCP"
	result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: id, Name: &name})
	saved, readErr := repo.GetAgentProfile(ctx, id)
	if err != nil || readErr != nil || result.Enabled || saved.Enabled || saved.MCPSelectionMode != mode || !reflect.DeepEqual(saved.MCPSelectedServers, servers) {
		t.Fatalf("metadata update lost MCP/flag: %#v errors=%v/%v", saved, err, readErr)
	}
}

func testEnabledIntentDependencyAndDynamicExplicit(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("mixed_%t", mixed), func(t *testing.T) {
			ctrl, repo, profile := enabledIntentDynamicFixture(t)
			ctx := context.Background()
			_, routes, err := repo.(store.DynamicProfileRepository).GetDynamicAgentProfile(ctx, profile.ID)
			if err != nil {
				t.Fatal(err)
			}
			candidate := routes[0].ExecutionProfileID
			enabled, name := false, "Candidate renamed"
			req := UpdateProfileRequest{ID: candidate, Enabled: &enabled}
			if mixed {
				req.Name = &name
			}
			if result, err := ctrl.UpdateProfile(ctx, req); err == nil || result != nil {
				t.Fatal("dependent profile disable succeeded without force")
			}
			saved, err := repo.GetAgentProfile(ctx, candidate)
			if err != nil || !saved.Enabled {
				t.Fatalf("rejected disable mutated candidate: %#v %v", saved, err)
			}
			req.Force = true
			if result, err := ctrl.UpdateProfile(ctx, req); err != nil || result.Enabled {
				t.Fatalf("forced disable failed: %#v %v", result, err)
			}
			for _, wanted := range []bool{false, true} {
				name = "Dynamic explicit"
				result, err := ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: profile.ID, Name: &name, Enabled: &wanted, Dynamic: profile.Dynamic})
				if err != nil {
					t.Fatal(err)
				}
				stored, err := repo.GetAgentProfile(ctx, profile.ID)
				if err != nil || stored.Enabled != wanted || result.Enabled != wanted || stored.Name != name || result.Dynamic.Version != profile.Dynamic.Version+1 {
					t.Fatalf("explicit dynamic save: stored=%#v result=%#v error=%v", stored, result, err)
				}
				profile = result
			}
		})
	}
}
