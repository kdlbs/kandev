package handlers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type enabledHTTPStore struct {
	store.Repository
	store.DynamicProfileRepository
	afterRead func(context.Context) error
}

func (r *enabledHTTPStore) GetAgentProfile(ctx context.Context, id string) (*models.AgentProfile, error) {
	profile, err := r.Repository.GetAgentProfile(ctx, id)
	if err != nil || r.afterRead == nil {
		return profile, err
	}
	hook := r.afterRead
	r.afterRead = nil
	return profile, hook(ctx)
}

func enabledHTTPFixture(t *testing.T, enabled bool) (*gin.Engine, *controller.Controller, *enabledHTTPStore, *duplicateHub, string) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
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
	if err := repo.CreateAgent(ctx, &models.Agent{ID: "http-intent-agent", Name: "http-intent-agent"}); err != nil {
		t.Fatal(err)
	}
	profile := &models.AgentProfile{AgentID: "http-intent-agent", Name: "Original", Model: "original-model"}
	if err := repo.CreateAgentProfile(ctx, profile); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.UpdateAgentProfileEnabled(ctx, profile.ID, enabled); err != nil {
		t.Fatal(err)
	}
	wrapped := &enabledHTTPStore{Repository: repo, DynamicProfileRepository: repo}
	hub := &duplicateHub{}
	router, _, reg := newSettingsHarness(t, wrapped, hub)
	toggler := controller.NewController(repo, nil, reg, nil, log)
	return router, toggler, wrapped, hub, profile.ID
}

// @covers AC-PLATFORM-AGENT-SETTINGS-PARITY-002.5 AC-PLATFORM-AGENT-SETTINGS-PARITY-002.10
func TestProfileEnabledIntentHTTP(t *testing.T) {
	for _, initial := range []bool{true, false} {
		for _, null := range []bool{false, true} {
			t.Run(fmt.Sprintf("initial_%t_null_%t", initial, null), func(t *testing.T) {
				router, toggler, repo, hub, id := enabledHTTPFixture(t, initial)
				wanted := !initial
				repo.afterRead = func(ctx context.Context) error {
					_, err := toggler.UpdateProfile(ctx, controller.UpdateProfileRequest{ID: id, Enabled: &wanted})
					return err
				}
				body := `{"name":"Edited"}`
				if null {
					body = `{"name":"Edited","enabled":null}`
				}
				response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id, body)
				if response.Code != http.StatusOK {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
				var result dto.AgentProfileDTO
				decodeSettingsJSON(t, response, &result)
				event := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileUpdated)
				saved, err := repo.Repository.GetAgentProfile(context.Background(), id)
				if err != nil || result.Enabled != wanted || event.Enabled != wanted || saved.Enabled != wanted || saved.Name != "Edited" || saved.Model != "original-model" {
					t.Fatalf("saved=%#v result=%#v event=%#v error=%v", saved, result, event, err)
				}
			})
		}
	}
	t.Run("explicit_controls", testEnabledHTTPControls)
	t.Run("dependency_force", testEnabledHTTPDependency)
	t.Run("rejection_has_no_event", testEnabledHTTPRejection)
}

func testEnabledHTTPControls(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{{`{"enabled":true}`, true}, {`{"enabled":false}`, false},
		{`{"name":"Mixed","enabled":false}`, false}, {`{"name":"Mixed","enabled":true}`, true}} {
		t.Run(tc.body, func(t *testing.T) {
			router, _, repo, hub, id := enabledHTTPFixture(t, true)
			response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id, tc.body)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var result dto.AgentProfileDTO
			decodeSettingsJSON(t, response, &result)
			event := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileUpdated)
			saved, err := repo.GetAgentProfile(context.Background(), id)
			if err != nil || saved.Enabled != result.Enabled || event.Enabled != result.Enabled || result.Enabled != tc.want {
				t.Fatalf("explicit result=%#v event=%#v saved=%#v error=%v", result, event, saved, err)
			}
		})
	}
}

func testEnabledHTTPRejection(t *testing.T) {
	for _, deleted := range []bool{false, true} {
		t.Run(fmt.Sprintf("deleted_%t", deleted), func(t *testing.T) {
			router, _, repo, hub, id := enabledHTTPFixture(t, true)
			body, status := `{"name":"Edited","require_exact_model":true,"model":""}`, http.StatusBadRequest
			if deleted {
				repo.afterRead = func(ctx context.Context) error { return repo.DeleteAgentProfile(ctx, id) }
				body, status = `{"name":"Edited"}`, http.StatusInternalServerError
			}
			response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id, body)
			if response.Code != status || len(hub.payloads(ws.ActionAgentProfileUpdated)) != 0 {
				t.Fatalf("rejection status=%d body=%s events=%v", response.Code, response.Body.String(), hub.actions())
			}
			saved, err := repo.GetAgentProfileIncludingDeleted(context.Background(), id)
			if err != nil || saved.Name != "Original" || !saved.Enabled {
				t.Fatalf("rejected save changed state: %#v error=%v", saved, err)
			}
		})
	}
}

func testEnabledHTTPDependency(t *testing.T) {
	router, _, repo, hub, id := enabledHTTPFixture(t, true)
	ctx := context.Background()
	parent := &models.AgentProfile{AgentID: "http-intent-agent", Name: "Dynamic", Model: ""}
	if err := repo.CreateAgentProfile(ctx, parent); err != nil {
		t.Fatal(err)
	}
	doc := &models.DynamicAgentProfile{ProfileID: parent.ID}
	routes := []models.DynamicAgentRoute{{DynamicProfileID: parent.ID, Position: 0, ExecutionProfileID: id, Enabled: true, RulesJSON: `{}`}}
	if err := repo.CreateDynamicAgentProfile(ctx, doc, routes); err != nil {
		t.Fatal(err)
	}
	response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id, `{"name":"Rejected","enabled":false}`)
	if response.Code != http.StatusConflict || len(hub.actions()) != 0 {
		t.Fatalf("dependency rejection=%d/%s events=%v", response.Code, response.Body.String(), hub.actions())
	}
	saved, err := repo.GetAgentProfile(ctx, id)
	if err != nil || !saved.Enabled || saved.Name != "Original" {
		t.Fatalf("rejected save mutated row: %#v %v", saved, err)
	}
	response = doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/"+id+"?force=true", `{"name":"Forced","enabled":false}`)
	if response.Code != http.StatusOK {
		t.Fatalf("forced save=%d/%s", response.Code, response.Body.String())
	}
	var result dto.AgentProfileDTO
	decodeSettingsJSON(t, response, &result)
	event := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileUpdated)
	saved, err = repo.GetAgentProfile(ctx, id)
	if err != nil || result.Enabled || event.Enabled || saved.Enabled || saved.Name != "Forced" {
		t.Fatalf("forced saved=%#v result=%#v event=%#v error=%v", saved, result, event, err)
	}
}
