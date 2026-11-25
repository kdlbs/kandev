package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

const (
	claudeAliasConflictCode = "claude_exact_model_alias_conflict"
)

func TestClaudeExactModelConflictProfileSave(t *testing.T) {
	t.Run("create rejects before writing or broadcasting", func(t *testing.T) {
		repo := newFakeSettingsRepo()
		seedAgent(repo, "agent-1", agents.NewClaudeACP().ID(), false)
		hub := &duplicateHub{}
		router, _, registry := newSettingsHarness(t, repo, hub)
		if err := registry.Register(agents.NewClaudeACP()); err != nil {
			t.Fatalf("register Claude ACP: %v", err)
		}

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents/agent-1/profiles",
			`{"name":"Default","model":"sonnet","require_exact_model":true,"env_vars":[{"key":"ANTHROPIC_MODEL","value":"claude-sonnet-5-5"}]}`)
		requireClaudeAliasConflict(t, response)
		if len(repo.created) != 0 {
			t.Fatalf("rejected create wrote profiles: %#v", repo.created)
		}
		if len(hub.actions()) != 0 {
			t.Fatalf("rejected create broadcast %v", hub.actions())
		}
	})

	t.Run("create agent rejects nested profile before writing", func(t *testing.T) {
		t.Setenv("KANDEV_E2E_MOCK", "true")
		repo := newFakeSettingsRepo()
		router, _, registry := newSettingsHarness(t, repo, nil)
		mockClaude := agents.NewMockAgentWithID(agents.NewClaudeACP().ID(), "Claude", "Claude")
		mockClaude.SetEnabled(true)
		if err := registry.Register(mockClaude); err != nil {
			t.Fatalf("register mock Claude ACP: %v", err)
		}

		response := doSettingsRequest(router, http.MethodPost, "/api/v1/agents",
			`{"name":"claude-acp","profiles":[{"name":"Default","model":"sonnet","require_exact_model":true,"env_vars":[{"key":"ANTHROPIC_MODEL","value":"claude-sonnet-5-5"}]}]}`)
		requireClaudeAliasConflict(t, response)
		if len(repo.agents) != 0 || len(repo.created) != 0 {
			t.Fatalf("rejected create wrote agents=%#v profiles=%#v", repo.agents, repo.created)
		}
	})

	t.Run("update rejects a newly conflicting pair before writing or broadcasting", func(t *testing.T) {
		cases := []struct {
			name        string
			body        string
			strictSaved bool
		}{
			{name: "model-only patch", body: `{"model":"opus"}`, strictSaved: true},
			{name: "environment-only patch", body: `{"env_vars":[{"key":"ANTHROPIC_MODEL","value":"claude-haiku-5-5"}]}`, strictSaved: true},
			{name: "strictness-only patch", body: `{"require_exact_model":true}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				repo := newFakeSettingsRepo()
				seedAgent(repo, "agent-1", agents.NewClaudeACP().ID(), false)
				seeded := seedProfile(repo, "profile-1", "agent-1", "Default", "")
				seeded.Model = "sonnet"
				seeded.RequireExactModel = tc.strictSaved
				seeded.FallbackModel = "haiku"
				seeded.AutoFallback = true
				seeded.EnvVars = []models.ProfileEnvVar{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}
				hub := &duplicateHub{}
				router, _, _ := newSettingsHarness(t, repo, hub)

				response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", tc.body)
				requireClaudeAliasConflict(t, response)
				if len(repo.updatedProfiles) != 0 {
					t.Fatalf("rejected update wrote profiles: %#v", repo.updatedProfiles)
				}
				if len(hub.actions()) != 0 {
					t.Fatalf("rejected update broadcast %v", hub.actions())
				}
			})
		}
	})

	t.Run("unchanged legacy conflict allows unrelated and identical edits", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			body string
		}{
			{name: "unrelated name", body: `{"name":"Renamed"}`},
			{name: "identical full-form pair", body: `{"model":"sonnet","require_exact_model":true,"env_vars":[{"key":"ANTHROPIC_MODEL","value":"claude-sonnet-5-5"}]}`},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repo := newFakeSettingsRepo()
				seedAgent(repo, "agent-1", agents.NewClaudeACP().ID(), false)
				profile := seedProfile(repo, "profile-1", "agent-1", "Default", "")
				profile.Model = "sonnet"
				profile.RequireExactModel = true
				profile.FallbackModel = "haiku"
				profile.AutoFallback = true
				profile.EnvVars = []models.ProfileEnvVar{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}
				router, _, _ := newSettingsHarness(t, repo, nil)

				response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", tc.body)
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
				}
				stored := repo.profiles[profile.ID]
				if !stored.RequireExactModel || stored.FallbackModel != "haiku" || !stored.AutoFallback {
					t.Errorf("policy fields after edit = exact:%v fallback:%q auto:%v", stored.RequireExactModel, stored.FallbackModel, stored.AutoFallback)
				}
			})
		}
	})

	t.Run("profile repair accepts full ID or removing the environment entry", func(t *testing.T) {
		for _, tc := range []struct {
			name           string
			body           string
			wantEnvRemoved bool
		}{
			{name: "select full model ID", body: `{"model":"claude-sonnet-5-5"}`},
			{name: "remove environment entry", body: `{"env_vars":[]}`, wantEnvRemoved: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				repo := newFakeSettingsRepo()
				seedAgent(repo, "agent-1", agents.NewClaudeACP().ID(), false)
				profile := seedProfile(repo, "profile-1", "agent-1", "Default", "")
				profile.Model = "sonnet"
				profile.RequireExactModel = true
				profile.FallbackModel = "haiku"
				profile.AutoFallback = true
				profile.EnvVars = []models.ProfileEnvVar{{Key: "ANTHROPIC_MODEL", Value: "claude-sonnet-5-5"}}
				router, _, _ := newSettingsHarness(t, repo, nil)

				response := doSettingsRequest(router, http.MethodPatch, "/api/v1/agent-profiles/profile-1", tc.body)
				if response.Code != http.StatusOK {
					t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
				}
				stored := repo.profiles[profile.ID]
				if !stored.RequireExactModel || stored.FallbackModel != "haiku" || !stored.AutoFallback {
					t.Errorf("policy fields after repair = exact:%v fallback:%q auto:%v", stored.RequireExactModel, stored.FallbackModel, stored.AutoFallback)
				}
				if tc.wantEnvRemoved {
					if len(repo.updatedProfiles) != 1 {
						t.Fatalf("repair wrote %d profiles, want 1", len(repo.updatedProfiles))
					}
					written := repo.updatedProfiles[0]
					if len(written.EnvVars) != 0 {
						t.Errorf("environment entries in saved repair = %#v, want none", written.EnvVars)
					}
				}
			})
		}
	})
}

func requireClaudeAliasConflict(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Error     string `json:"error"`
		ErrorCode string `json:"error_code"`
	}
	decodeSettingsJSON(t, response, &body)
	if body.ErrorCode != claudeAliasConflictCode || body.Error != claudeExactModelAliasConflictMessage {
		t.Fatalf("error response = %#v, want code %q and fixed correction %q", body, claudeAliasConflictCode, claudeExactModelAliasConflictMessage)
	}
}
