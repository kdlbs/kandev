package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/registry"
	profilectrl "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	backendmcp "github.com/kandev/kandev/internal/mcp/handlers"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/mark3labs/mcp-go/mcp"
	_ "github.com/mattn/go-sqlite3"
)

type enabledMCPStore struct {
	store.Repository
	afterRead func(context.Context) error
}

func (r *enabledMCPStore) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	profile, err := r.Repository.GetAgentProfile(ctx, id)
	if err != nil || r.afterRead == nil {
		return profile, err
	}
	hook := r.afterRead
	r.afterRead = nil
	return profile, hook(ctx)
}

type enabledMCPEvents struct{ profiles []dto.AgentProfileDTO }

func (b *enabledMCPEvents) Publish(_ context.Context, subject string, event *bus.Event) error {
	if subject == events.AgentProfileUpdated {
		profile := event.Data.(map[string]interface{})["profile"].(*dto.AgentProfileDTO)
		b.profiles = append(b.profiles, *profile)
	}
	return nil
}

func enabledMCPFixture(t *testing.T, enabled bool) (*Server, *profilectrl.Controller, *enabledMCPStore, *enabledMCPEvents, string) {
	t.Helper()
	log := newTestLogger(t)
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	repo, _, err := store.Provide(db, db, log)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "mcp-intent", Name: "mcp-intent"}); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: "mcp-intent", Name: "Original", Model: "original-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, enabled); err != nil {
		t.Fatal(err)
	}
	wrapped := &enabledMCPStore{Repository: repo}
	reg := registry.NewRegistry(log)
	ctrl := profilectrl.NewController(wrapped, nil, reg, nil, log)
	toggler := profilectrl.NewController(repo, nil, reg, nil, log)
	eventBus := &enabledMCPEvents{}
	handlers := backendmcp.NewHandlers(nil, nil, nil, nil, nil, nil, nil, eventBus, nil, nil, nil, nil, log)
	handlers.SetConfigDeps(nil, ctrl, nil)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	server := NewExternal(NewExternalDispatcherBackendClient(dispatcher, log), log, "")
	return server, toggler, wrapped, eventBus, profile.ID
}

func callEnabledMCP(t *testing.T, server *Server, ctx context.Context, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	tool, ok := server.mcpServer.ListTools()["update_agent_profile_kandev"]
	if !ok {
		t.Fatal("compatibility update tool is not registered")
	}
	req := mcp.CallToolRequest{}
	req.Params.Name, req.Params.Arguments = tool.Tool.Name, args
	result, err := tool.Handler(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentMCP(t *testing.T) {
	for _, initial := range []bool{true, false} {
		for _, field := range []string{"name", "model"} {
			t.Run(fmt.Sprintf("%s_initial_%t", field, initial), func(t *testing.T) {
				server, toggler, repo, eventBus, id := enabledMCPFixture(t, initial)
				wanted := !initial
				repo.afterRead = func(ctx context.Context) error {
					_, err := toggler.UpdateProfile(ctx, profilectrl.UpdateProfileRequest{ID: id, Enabled: &wanted})
					return err
				}
				ctx := authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true})
				result := callEnabledMCP(t, server, ctx, map[string]any{"profile_id": id, field: "Edited"})
				if result.IsError {
					t.Fatalf("supported metadata save failed: %s", firstText(t, result))
				}
				var profile dto.AgentProfileDTO
				if err := json.Unmarshal([]byte(firstText(t, result)), &profile); err != nil {
					t.Fatal(err)
				}
				saved, err := repo.Repository.GetAgentProfile(ctx, id)
				if err != nil || saved.Enabled != wanted || profile.Enabled != wanted || len(eventBus.profiles) != 1 || eventBus.profiles[0].Enabled != wanted {
					t.Fatalf("saved=%#v returned=%#v events=%#v error=%v", saved, profile, eventBus.profiles, err)
				}
				if (field == "name" && saved.Name != "Edited") || (field == "model" && saved.Model != "Edited") {
					t.Fatal("metadata edit did not persist")
				}
			})
		}
	}
	t.Run("guard_controls", testEnabledMCPGuards)
}

func testEnabledMCPGuards(t *testing.T) {
	for _, bad := range []map[string]any{{"enabled": false}, {"enabled": nil}, {"model": nil}, {"auto_approve": nil}} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			server, _, repo, events, id := enabledMCPFixture(t, false)
			bad["profile_id"] = id
			result := callEnabledMCP(t, server, context.Background(), bad)
			if !result.IsError || len(events.profiles) != 0 {
				t.Fatalf("published schema did not reject invalid arguments: %#v", result)
			}
			saved, err := repo.GetAgentProfile(context.Background(), id)
			if err != nil || saved.Enabled || saved.Name != "Original" {
				t.Fatalf("invalid call mutated state: %#v error=%v", saved, err)
			}
		})
	}
	t.Run("automation_surface_denied", func(t *testing.T) {
		server, _, repo, events, id := enabledMCPFixture(t, false)
		ctx := mcpscope.WithPrincipal(context.Background(), mcpscope.Principal{AutomationID: "automation", CallerTaskID: "automation-task", CallerSessionID: "automation-session", Surface: mcpprofile.SurfaceAutomation})
		result := callEnabledMCP(t, server, ctx, map[string]any{"profile_id": id, "name": "Forbidden"})
		if !result.IsError || len(events.profiles) != 0 {
			t.Fatal("guarded dispatcher accepted an automation profile mutation")
		}
		saved, err := repo.GetAgentProfile(context.Background(), id)
		if err != nil || saved.Name != "Original" || saved.Enabled {
			t.Fatalf("guard rejection changed profile: %#v error=%v", saved, err)
		}
	})
}
