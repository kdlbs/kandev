package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// fakeUtilityDeps reports the utility agents bound to a profile so the
// disable/delete confirmation branches are reachable.
type fakeUtilityDeps struct {
	refs    []controller.UtilityAgentReference
	listErr error
	cleared []string
}

func (f *fakeUtilityDeps) ListUtilityAgentsByAgentProfile(
	context.Context, string,
) ([]controller.UtilityAgentReference, error) {
	return f.refs, f.listErr
}

func (f *fakeUtilityDeps) ClearUtilityAgentProfileBindings(_ context.Context, profileID string) error {
	f.cleared = append(f.cleared, profileID)
	return nil
}

// fakeRoutingTierDeps reports workspace routing tiers seeded from a profile.
// Routing tiers are a hard blocker: force=true must not bypass them.
type fakeRoutingTierDeps struct {
	refs []controller.RoutingTierReference
}

func (f *fakeRoutingTierDeps) ListRoutingTierReferencesByAgentProfile(
	context.Context, string,
) ([]controller.RoutingTierReference, error) {
	return f.refs, nil
}

// decodeProfileBroadcast returns the profile carried by the single recorded
// broadcast of the given action.
func decodeProfileBroadcast(t *testing.T, hub *duplicateHub, action string) dto.AgentProfileDTO {
	t.Helper()
	payloads := hub.payloads(action)
	if len(payloads) != 1 {
		t.Fatalf("recorded %d %s broadcasts (all: %v), want 1", len(payloads), action, hub.actions())
	}
	var envelope struct {
		Profile dto.AgentProfileDTO `json:"profile"`
	}
	if err := json.Unmarshal(payloads[0], &envelope); err != nil {
		t.Fatalf("decode %s payload: %v", action, err)
	}
	return envelope.Profile
}

// TestCreateProfileEndpoint pins the created row, the broadcast that lets open
// settings surfaces pick it up, and each validation rejection with its own
// error message.
func TestCreateProfileEndpoint(t *testing.T) {
	t.Run("creates the profile and broadcasts agent.profile.created", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "profile-agent", false)
		hub := &duplicateHub{}
		router, _, reg := newSettingsHarness(t, repo, hub)
		registerTestTUIAgent(t, reg, "profile-agent", 1)

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-1/profiles",
			`{"name":"Fast","model":"model-x","cli_flags":[{"flag":"--verbose","enabled":true}]}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileDTO
		decodeSettingsJSON(t, response, &got)
		if got.ID == "" || got.AgentID != "agent-1" || got.Name != "Fast" || got.Model != "model-x" {
			t.Fatalf("created profile = %#v", got)
		}
		if !got.Enabled || !got.UserModified {
			t.Errorf("created profile should be enabled and user-modified: %#v", got)
		}
		wantFlags := []dto.CLIFlagDTO{{Flag: "--verbose", Enabled: true}}
		if !reflect.DeepEqual(got.CLIFlags, wantFlags) {
			t.Errorf("cli_flags = %#v, want %#v", got.CLIFlags, wantFlags)
		}
		if len(repo.created) != 1 || repo.created[0].ID != got.ID {
			t.Fatalf("stored profiles = %#v", repo.created)
		}
		if broadcast := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileCreated); broadcast.ID != got.ID {
			t.Errorf("broadcast profile id = %q, want %q", broadcast.ID, got.ID)
		}
	})

	t.Run("rejections keep their specific message and write nothing", func(t *testing.T) {
		cases := []struct {
			name      string
			body      string
			wantError string
		}{
			{name: "malformed json", body: "{", wantError: "invalid payload"},
			{name: "blank name", body: `{"name":"   ","model":"m"}`, wantError: "profile name is required"},
			{
				name:      "unterminated command prefix",
				body:      `{"name":"Fast","command_prefix":"greywall \""}`,
				wantError: `invalid command prefix: unterminated " quote`,
			},
			{
				name:      "env var without a name",
				body:      `{"name":"Fast","env_vars":[{"key":"","value":"v"}]}`,
				wantError: "invalid profile env vars: env_vars[0].key is required",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo := newFakeSettingsRepo()
				seedAgent(repo, "agent-1", "profile-agent", false)
				hub := &duplicateHub{}
				router, _, reg := newSettingsHarness(t, repo, hub)
				registerTestTUIAgent(t, reg, "profile-agent", 1)

				response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-1/profiles", tc.body)
				requireErrorBody(t, response, http.StatusBadRequest, tc.wantError)
				if len(repo.created) != 0 {
					t.Fatalf("rejected create wrote %d profiles", len(repo.created))
				}
				if len(hub.actions()) != 0 {
					t.Fatalf("rejected create broadcast %v", hub.actions())
				}
			})
		}
	})
}

// TestUpdateProfileEndpoint covers the success body plus broadcast, the
// missing-profile 404, and the 409 raised when disabling a profile a utility
// agent still points at.
func TestUpdateProfileEndpoint(t *testing.T) {
	t.Run("applies the update and broadcasts agent.profile.updated", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", "profile-agent", false)
		seedProfile(repo, "profile-1", "agent-1", "Original", "")
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1",
			`{"name":"Renamed","auto_approve":true,"command_prefix":"  greywall --  "}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var got dto.AgentProfileDTO
		decodeSettingsJSON(t, response, &got)
		if got.Name != "Renamed" || !got.AutoApprove || got.CommandPrefix != "greywall --" {
			t.Fatalf("updated profile = %#v", got)
		}
		if len(repo.updatedProfiles) != 1 || repo.updatedProfiles[0].Name != "Renamed" {
			t.Fatalf("persisted updates = %#v", repo.updatedProfiles)
		}
		if broadcast := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileUpdated); broadcast.Name != "Renamed" {
			t.Errorf("broadcast profile name = %q, want Renamed", broadcast.Name)
		}
	})

	t.Run("unknown profile is 404 and broadcasts nothing", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/missing", `{"name":"X"}`)
		requireErrorBody(t, response, http.StatusNotFound, "agent profile not found")
		if len(hub.actions()) != 0 {
			t.Fatalf("404 update broadcast %v", hub.actions())
		}
	})

	t.Run("blank name is 400", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Original", "")
		router := newSettingsRouter(t, repo, nil)
		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", `{"name":"  "}`)
		requireErrorBody(t, response, http.StatusBadRequest, "profile name is required")
	})

	t.Run("disabling a profile a utility agent uses is 409 unless forced", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Original", "")
		hub := &duplicateHub{}
		router, ctrl, _ := newSettingsHarness(t, repo, hub)
		ctrl.SetUtilityDependencyChecker(&fakeUtilityDeps{
			refs: []controller.UtilityAgentReference{{ID: "utility-1", Name: "Commit message"}},
		})

		blocked := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", `{"enabled":false}`)
		if blocked.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", blocked.Code, blocked.Body.String())
		}
		var conflict struct {
			Error         string                             `json:"error"`
			UtilityAgents []controller.UtilityAgentReference `json:"utility_agents"`
		}
		decodeSettingsJSON(t, blocked, &conflict)
		if conflict.Error != "agent profile is in use" {
			t.Errorf("error = %q", conflict.Error)
		}
		want := []controller.UtilityAgentReference{{ID: "utility-1", Name: "Commit message"}}
		if !reflect.DeepEqual(conflict.UtilityAgents, want) {
			t.Errorf("utility_agents = %#v, want %#v", conflict.UtilityAgents, want)
		}
		if repo.profiles["profile-1"].Enabled != true {
			t.Error("blocked disable still flipped the stored enabled flag")
		}
		if len(hub.actions()) != 0 {
			t.Fatalf("blocked update broadcast %v", hub.actions())
		}

		forced := doSettingsRequest(router, http.MethodPatch,
			"/api/v1/agent-profiles/profile-1?force=true", `{"enabled":false}`)
		if forced.Code != http.StatusOK {
			t.Fatalf("forced status = %d, body = %s", forced.Code, forced.Body.String())
		}
		if repo.profiles["profile-1"].Enabled {
			t.Error("forced disable did not flip the stored enabled flag")
		}
	})
}

// TestDeleteProfileEndpoint separates the success path, the 404, and the two
// distinct 409 shapes (soft blocker vs. routing tier, which force cannot skip).
func TestDeleteProfileEndpoint(t *testing.T) {
	t.Run("deletes the row and broadcasts agent.profile.deleted", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Doomed", "")
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/profile-1", "")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		var body map[string]bool
		decodeSettingsJSON(t, response, &body)
		if !body["success"] {
			t.Fatalf("body = %s, want success true", response.Body.String())
		}
		if !reflect.DeepEqual(repo.deletedProfiles, []string{"profile-1"}) {
			t.Fatalf("deleted profiles = %v", repo.deletedProfiles)
		}
		if broadcast := decodeProfileBroadcast(t, hub, ws.ActionAgentProfileDeleted); broadcast.ID != "profile-1" {
			t.Errorf("broadcast profile id = %q, want profile-1", broadcast.ID)
		}
	})

	// KNOWN DEFECT, asserted as-is rather than fixed here: this test is
	// test-only work. controller.DeleteProfile classifies the missing-source
	// lookup with a `strings.Contains(err.Error(), "agent profile not found")`
	// check, but the sqlite store returns sql.ErrNoRows from GetAgentProfile
	// (its soft-delete tests pin that), so the message never matches and the
	// error falls through as a 500. The sibling helper isProfileNotFoundErr in
	// the same file already handles both shapes and is what DuplicateProfile
	// uses. When DeleteProfile is switched to it, this expectation becomes 404
	// with an "agent profile not found" body.
	t.Run("unknown profile currently reports 500 instead of 404", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)
		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/missing", "")
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to delete profile")
		if len(hub.actions()) != 0 {
			t.Fatalf("failed delete broadcast %v", hub.actions())
		}
		if len(repo.deletedProfiles) != 0 {
			t.Fatalf("failed delete removed %v", repo.deletedProfiles)
		}
	})

	t.Run("routing tier references block the delete even with force", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Tiered", "")
		router, ctrl, _ := newSettingsHarness(t, repo, &duplicateHub{})
		ctrl.SetRoutingTierDependencyChecker(&fakeRoutingTierDeps{
			refs: []controller.RoutingTierReference{{WorkspaceID: "ws-1", ProviderID: "anthropic", Tier: "primary"}},
		})

		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/profile-1?force=true", "")
		if response.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", response.Code, response.Body.String())
		}
		var conflict struct {
			Error        string                            `json:"error"`
			RoutingTiers []controller.RoutingTierReference `json:"routing_tiers"`
			Watchers     []controller.WatcherReference     `json:"watchers"`
		}
		decodeSettingsJSON(t, response, &conflict)
		if conflict.Error != "agent profile is in use" {
			t.Errorf("error = %q", conflict.Error)
		}
		want := []controller.RoutingTierReference{{WorkspaceID: "ws-1", ProviderID: "anthropic", Tier: "primary"}}
		if !reflect.DeepEqual(conflict.RoutingTiers, want) {
			t.Errorf("routing_tiers = %#v, want %#v", conflict.RoutingTiers, want)
		}
		if len(repo.deletedProfiles) != 0 {
			t.Fatalf("blocked delete removed %v", repo.deletedProfiles)
		}
	})

	t.Run("force clears utility bindings that block an unforced delete", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Bound", "")
		router, ctrl, _ := newSettingsHarness(t, repo, &duplicateHub{})
		deps := &fakeUtilityDeps{refs: []controller.UtilityAgentReference{{ID: "utility-1", Name: "Title"}}}
		ctrl.SetUtilityDependencyChecker(deps)

		blocked := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/profile-1", "")
		if blocked.Code != http.StatusConflict {
			t.Fatalf("unforced status = %d, want 409; body = %s", blocked.Code, blocked.Body.String())
		}
		if len(repo.deletedProfiles) != 0 {
			t.Fatalf("unforced delete removed %v", repo.deletedProfiles)
		}

		forced := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/profile-1?force=true", "")
		if forced.Code != http.StatusOK {
			t.Fatalf("forced status = %d, body = %s", forced.Code, forced.Body.String())
		}
		if !reflect.DeepEqual(deps.cleared, []string{"profile-1"}) {
			t.Fatalf("cleared utility bindings = %v, want [profile-1]", deps.cleared)
		}
	})

	t.Run("store failure is 500", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Doomed", "")
		repo.failWith("DeleteAgentProfile", errors.New("database is locked"))
		router := newSettingsRouter(t, repo, nil)
		response := doSettingsRequest(router, http.MethodDelete, "/api/v1/agent-profiles/profile-1", "")
		requireErrorBody(t, response, http.StatusInternalServerError, "failed to delete profile")
	})
}

// TestProfileWriteStoreFailuresAre500 keeps a store failure distinguishable
// from a validation rejection on every profile write: each of these inputs is
// valid, so a 400 or 404 here would be the handler mis-classifying an outage.
func TestProfileWriteStoreFailuresAre500(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		failMethod string
		wantError  string
	}{
		{
			name: "create profile", method: http.MethodPost, path: "/api/v1/agents/agent-1/profiles",
			body: `{"name":"Fast"}`, failMethod: "CreateAgentProfile", wantError: "failed to create profile",
		},
		{
			name: "update profile", method: http.MethodPatch, path: "/api/v1/agent-profiles/profile-1",
			body: `{"name":"Renamed"}`, failMethod: "UpdateAgentProfile", wantError: "failed to update profile",
		},
		{
			name: "duplicate profile", method: http.MethodPost, path: "/api/v1/agent-profiles/profile-1/duplicate",
			failMethod: "GetAgentProfile", wantError: "failed to duplicate profile",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeSettingsRepo()
			seedAgent(repo, "agent-1", "profile-agent", false)
			seedProfile(repo, "profile-1", "agent-1", "Original", "")
			repo.failWith(tc.failMethod, errors.New("database is locked"))
			hub := &duplicateHub{}
			router, _, reg := newSettingsHarness(t, repo, hub)
			registerTestTUIAgent(t, reg, "profile-agent", 1)

			response := doSettingsRequest(router, tc.method, tc.path, tc.body)
			requireErrorBody(t, response, http.StatusInternalServerError, tc.wantError)
			if len(hub.actions()) != 0 {
				t.Fatalf("failed write broadcast %v", hub.actions())
			}
		})
	}
}

// TestProfileBroadcastIsWorkspaceScoped guards the boundary in
// broadcastProfileEvent: an office-scoped profile must never reach the global
// settings channel, and must be dropped entirely when the hub cannot route by
// workspace.
func TestProfileBroadcastIsWorkspaceScoped(t *testing.T) {
	t.Run("workspace-aware hub receives it on the workspace channel only", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Office", "ws-7")
		hub := newWorkspaceDuplicateHub()
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", `{"name":"Renamed"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if len(hub.global) != 0 {
			t.Fatalf("office profile leaked to the global channel: %d messages", len(hub.global))
		}
		msgs := hub.workspaceMsgs["ws-7"]
		if len(msgs) != 1 || msgs[0].Action != ws.ActionAgentProfileUpdated {
			t.Fatalf("workspace messages = %#v", msgs)
		}
	})

	t.Run("hub without workspace routing drops the event fail-closed", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedProfile(repo, "profile-1", "agent-1", "Office", "ws-7")
		hub := &duplicateHub{}
		router := newSettingsRouter(t, repo, hub)

		response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", `{"name":"Renamed"}`)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		if len(hub.actions()) != 0 {
			t.Fatalf("office profile reached the global channel: %v", hub.actions())
		}
	})
}
