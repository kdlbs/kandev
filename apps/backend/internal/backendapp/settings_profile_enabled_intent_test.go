package backendapp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/registry"
	profilectrl "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	mcphandlers "github.com/kandev/kandev/internal/mcp/handlers"
	"github.com/kandev/kandev/internal/settingscatalog"
	ws "github.com/kandev/kandev/pkg/websocket"
	_ "github.com/mattn/go-sqlite3"
)

type enabledCompactStore struct {
	store.Repository
	afterRead func(context.Context) error
}

func (r *enabledCompactStore) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	p, err := r.Repository.GetAgentProfile(ctx, id)
	if err != nil || r.afterRead == nil {
		return p, err
	}
	hook := r.afterRead
	r.afterRead = nil
	return p, hook(ctx)
}

func enabledCompactFixture(t *testing.T, enabled bool) (*ws.Dispatcher, *profilectrl.Controller, *enabledCompactStore, string) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	database.SetMaxOpenConns(1)
	repo, _, err := store.Provide(database, database, log)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "compact-intent", Name: "compact-intent"}); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: "compact-intent", Name: "Original", Model: "original-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, enabled); err != nil {
		t.Fatal(err)
	}
	wrapped := &enabledCompactStore{Repository: repo}
	reg := registry.NewRegistry(log)
	ctrl := profilectrl.NewController(wrapped, nil, reg, nil, log)
	catalog, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handlers := mcphandlers.NewHandlers(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, log)
	handlers.SetSettingsCatalog(catalog)
	handlers.SetSettingsOperations(newSettingsOperations(catalog, ctrl, nil, settingsDomainDependencies{}))
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	return dispatcher, profilectrl.NewController(repo, nil, reg, nil, log), wrapped, profile.ID
}

func callEnabledCompact(t *testing.T, d *ws.Dispatcher, ctx context.Context, id string, changes map[string]any) *ws.Message {
	t.Helper()
	request, err := ws.NewRequest("enabled-intent", ws.ActionMCPUpdateSettings, map[string]any{
		"target": map[string]any{"resource_type": "agent_profile", "resource_id": id}, "changes": changes,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := d.Dispatch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentCompact(t *testing.T) {
	for _, initial := range []bool{true, false} {
		t.Run(fmt.Sprintf("omitted_initial_%t", initial), func(t *testing.T) {
			dispatcher, toggler, repo, id := enabledCompactFixture(t, initial)
			wanted := !initial
			repo.afterRead = func(ctx context.Context) error {
				_, err := toggler.UpdateProfile(ctx, profilectrl.UpdateProfileRequest{ID: id, Enabled: &wanted})
				return err
			}
			ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
			response := callEnabledCompact(t, dispatcher, ctx, id, map[string]any{"name": "Edited"})
			if response.Type != ws.MessageTypeResponse {
				t.Fatalf("save failed: %s", response.Payload)
			}
			var result struct {
				Profile struct {
					Enabled bool   `json:"enabled"`
					Name    string `json:"name"`
				} `json:"profile"`
			}
			if err := json.Unmarshal(response.Payload, &result); err != nil {
				t.Fatal(err)
			}
			saved, err := repo.Repository.GetAgentProfile(ctx, id)
			if err != nil || saved.Enabled != wanted || result.Profile.Enabled != wanted || saved.Name != "Edited" || result.Profile.Name != "Edited" {
				t.Fatalf("saved=%#v result=%#v error=%v", saved, result, err)
			}
		})
	}
	t.Run("controls", testEnabledCompactControls)
}

func testEnabledCompactControls(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		t.Run(fmt.Sprintf("explicit_%t", explicit), func(t *testing.T) {
			dispatcher, _, repo, id := enabledCompactFixture(t, !explicit)
			ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
			response := callEnabledCompact(t, dispatcher, ctx, id, map[string]any{"enabled": explicit, "name": "Mixed"})
			saved, err := repo.GetAgentProfile(ctx, id)
			if response.Type != ws.MessageTypeResponse || err != nil || saved.Enabled != explicit || saved.Name != "Mixed" {
				t.Fatalf("response=%s saved=%#v error=%v", response.Payload, saved, err)
			}
		})
	}
	for _, unauthorized := range []bool{false, true} {
		t.Run(fmt.Sprintf("rejected_unauthorized_%t", unauthorized), func(t *testing.T) {
			dispatcher, _, repo, id := enabledCompactFixture(t, false)
			ctx := context.Background()
			changes := map[string]any{"name": "Rejected"}
			if !unauthorized {
				ctx = authn.WithIdentity(ctx, authn.Identity{Synthetic: true})
				changes["enabled"] = nil
			}
			response := callEnabledCompact(t, dispatcher, ctx, id, changes)
			saved, err := repo.GetAgentProfile(context.Background(), id)
			if response.Type != ws.MessageTypeError || err != nil || saved.Enabled || saved.Name != "Original" {
				t.Fatalf("rejection=%s saved=%#v error=%v", response.Payload, saved, err)
			}
		})
	}
}
