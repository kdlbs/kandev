package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/utility/dto"
	"github.com/kandev/kandev/internal/utility/models"
)

// fixedUtilityTime pins seeded timestamps so a response body can be compared
// whole rather than field by field.
var fixedUtilityTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// seedUtilityAgent adds an agent row with pinned timestamps.
func seedUtilityAgent(f *utilityFixture, agent *models.UtilityAgent) *models.UtilityAgent {
	agent.CreatedAt = fixedUtilityTime
	agent.UpdatedAt = fixedUtilityTime
	return f.repo.putAgent(agent)
}

// do issues a request against the fixture's live server and returns the status
// plus the raw body.
func do(t *testing.T, f *utilityFixture, method, path, body string) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, f.server.URL+path, reader) //nolint:noctx // httptest server
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

// decodeInto decodes a response body, failing the test on bad JSON.
func decodeInto(t *testing.T, raw []byte, into any) {
	t.Helper()
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("decode body %s: %v", raw, err)
	}
}

// requireUtilityError asserts the response is exactly {"error": <want>}.
func requireUtilityError(t *testing.T, status int, raw []byte, wantStatus int, wantError string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("status = %d, want %d; body = %s", status, wantStatus, raw)
	}
	var body map[string]string
	decodeInto(t, raw, &body)
	if body["error"] != wantError {
		t.Fatalf("error = %q, want %q", body["error"], wantError)
	}
}

// TestListUtilityAgentsEndpoint pins the whole list body in store order and the
// store-failure 500.
func TestListUtilityAgentsEndpoint(t *testing.T) {
	t.Run("returns every agent in store order", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "builtin-1", Name: "Commit message", Prompt: "Summarize {{GitDiff}}",
			AgentID: "claude-acp", Model: "sonnet", Builtin: true, Enabled: true,
			ProfileBindingState: models.ProfileBindingInherit,
		})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "custom-1", Name: "Reviewer", Description: "Reviews diffs", Prompt: "Review",
			AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
		})

		status, raw := do(t, f, http.MethodGet, "/api/v1/utility/agents", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, raw)
		}
		var got dto.UtilityAgentsResponse
		decodeInto(t, raw, &got)
		want := dto.UtilityAgentsResponse{Agents: []dto.UtilityAgentDTO{
			{
				ID: "builtin-1", Name: "Commit message", Prompt: "Summarize {{GitDiff}}",
				AgentID: "claude-acp", Model: "sonnet", Builtin: true, Enabled: true,
				ProfileBindingState: models.ProfileBindingInherit,
				CreatedAt:           "2026-03-04T05:06:07Z", UpdatedAt: "2026-03-04T05:06:07Z",
			},
			{
				ID: "custom-1", Name: "Reviewer", Description: "Reviews diffs", Prompt: "Review",
				AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
				CreatedAt: "2026-03-04T05:06:07Z", UpdatedAt: "2026-03-04T05:06:07Z",
			},
		}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("response =\n%#v\nwant\n%#v", got, want)
		}
	})

	t.Run("store failure is 500", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		f.repo.failWith("ListAgents", errors.New("database is locked"))
		status, raw := do(t, f, http.MethodGet, "/api/v1/utility/agents", "")
		requireUtilityError(t, status, raw, http.StatusInternalServerError, "failed to list utility agents")
	})
}

// TestGetUtilityAgentEndpoint separates the missing-row 404 from a store
// failure; both used to be reported as 500 by an earlier revision.
func TestGetUtilityAgentEndpoint(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{})
	seedUtilityAgent(f, &models.UtilityAgent{
		ID: "custom-1", Name: "Reviewer", Prompt: "Review", AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
	})

	status, raw := do(t, f, http.MethodGet, "/api/v1/utility/agents/custom-1", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, raw)
	}
	var got dto.UtilityAgentDTO
	decodeInto(t, raw, &got)
	if got.ID != "custom-1" || got.Name != "Reviewer" || got.Model != "gpt-5" {
		t.Fatalf("agent = %#v", got)
	}

	missingStatus, missingRaw := do(t, f, http.MethodGet, "/api/v1/utility/agents/nope", "")
	requireUtilityError(t, missingStatus, missingRaw, http.StatusNotFound, "failed to get utility agent")

	f.repo.failWith("GetAgentByID", errors.New("database is locked"))
	brokenStatus, brokenRaw := do(t, f, http.MethodGet, "/api/v1/utility/agents/custom-1", "")
	requireUtilityError(t, brokenStatus, brokenRaw, http.StatusInternalServerError, "failed to get utility agent")
}

// TestCreateUtilityAgentEndpoint pins the created row (including the enabled
// default that a bad revision once left false) and each rejection.
func TestCreateUtilityAgentEndpoint(t *testing.T) {
	t.Run("creates an enabled custom agent", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		status, raw := do(t, f, http.MethodPost, "/api/v1/utility/agents",
			`{"name":"Reviewer","description":"Reviews diffs","prompt":"Review {{GitDiff}}",`+
				`"agent_id":"codex-acp","model":"gpt-5"}`)
		if status != http.StatusCreated {
			t.Fatalf("status = %d, body = %s", status, raw)
		}
		var got dto.UtilityAgentDTO
		decodeInto(t, raw, &got)
		if got.ID == "" || got.Name != "Reviewer" || got.Prompt != "Review {{GitDiff}}" {
			t.Fatalf("created agent = %#v", got)
		}
		if got.Builtin || !got.Enabled {
			t.Errorf("created agent should be custom and enabled: %#v", got)
		}
		stored, err := f.repo.GetAgentByID(t.Context(), got.ID)
		if err != nil {
			t.Fatalf("stored agent lookup: %v", err)
		}
		if stored.AgentID != "codex-acp" || stored.Model != "gpt-5" || !stored.Enabled {
			t.Fatalf("stored agent = %#v", stored)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		cases := []struct {
			name       string
			body       string
			wantStatus int
			wantError  string
		}{
			{
				name: "malformed json", body: "{",
				wantStatus: http.StatusBadRequest, wantError: "invalid payload",
			},
			{
				// name and prompt are `binding:"required"`, so this is rejected
				// by the binder before the service sees it.
				name: "missing prompt", body: `{"name":"Reviewer"}`,
				wantStatus: http.StatusBadRequest, wantError: "invalid payload",
			},
			{
				// Past the binder: the service rejects a custom agent with no
				// inference target, which is a 400 rather than a 500.
				name: "no agent_id or model", body: `{"name":"Reviewer","prompt":"Review"}`,
				wantStatus: http.StatusBadRequest, wantError: "failed to create utility agent",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := newUtilityFixture(t, utilityFixtureOptions{})
				status, raw := do(t, f, http.MethodPost, "/api/v1/utility/agents", tc.body)
				requireUtilityError(t, status, raw, tc.wantStatus, tc.wantError)
				agents, err := f.repo.ListAgents(t.Context())
				if err != nil {
					t.Fatalf("list agents: %v", err)
				}
				if len(agents) != 0 {
					t.Fatalf("rejected create wrote %d rows", len(agents))
				}
			})
		}
	})

	t.Run("store failure is 500", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		f.repo.failWith("CreateAgent", errors.New("database is locked"))
		status, raw := do(t, f, http.MethodPost, "/api/v1/utility/agents",
			`{"name":"Reviewer","prompt":"Review","agent_id":"codex-acp","model":"gpt-5"}`)
		requireUtilityError(t, status, raw, http.StatusInternalServerError, "failed to create utility agent")
	})
}

// TestUpdateUtilityAgentEndpoint covers each of the four statuses the handler
// can return, which is the point: not-found, invalid and built-in are three
// different causes that a "not 200" assertion would conflate.
func TestUpdateUtilityAgentEndpoint(t *testing.T) {
	t.Run("applies the patch and persists it", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "custom-1", Name: "Reviewer", Prompt: "Review", AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
		})

		status, raw := do(t, f, http.MethodPatch, "/api/v1/utility/agents/custom-1",
			`{"name":"  Renamed  ","description":"New","enabled":false}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, raw)
		}
		var got dto.UtilityAgentDTO
		decodeInto(t, raw, &got)
		if got.Name != "Renamed" || got.Description != "New" || got.Enabled {
			t.Fatalf("updated agent = %#v", got)
		}
		stored, err := f.repo.GetAgentByID(t.Context(), "custom-1")
		if err != nil {
			t.Fatalf("stored agent lookup: %v", err)
		}
		if stored.Name != "Renamed" || stored.Enabled {
			t.Fatalf("stored agent = %#v", stored)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		cases := []struct {
			name       string
			id         string
			body       string
			wantStatus int
			wantError  string
		}{
			{
				// Rejected by the binder, before the service is reached, so it
				// carries the binder's own message rather than the service's.
				name: "malformed json", id: "custom-1", body: "{",
				wantStatus: http.StatusBadRequest, wantError: "invalid payload",
			},
			{
				name: "unknown id", id: "nope", body: `{"name":"X"}`,
				wantStatus: http.StatusNotFound, wantError: "failed to update utility agent",
			},
			{
				name: "blank name", id: "custom-1", body: `{"name":"   "}`,
				wantStatus: http.StatusBadRequest, wantError: "failed to update utility agent",
			},
			{
				name: "blank prompt", id: "custom-1", body: `{"prompt":"  "}`,
				wantStatus: http.StatusBadRequest, wantError: "failed to update utility agent",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := newUtilityFixture(t, utilityFixtureOptions{})
				seedUtilityAgent(f, &models.UtilityAgent{
					ID: "custom-1", Name: "Reviewer", Prompt: "Review",
					AgentID: "codex-acp", Model: "gpt-5", Enabled: true,
				})
				status, raw := do(t, f, http.MethodPatch, "/api/v1/utility/agents/"+tc.id, tc.body)
				requireUtilityError(t, status, raw, tc.wantStatus, tc.wantError)
				stored, err := f.repo.GetAgentByID(t.Context(), "custom-1")
				if err != nil {
					t.Fatalf("stored agent lookup: %v", err)
				}
				if stored.Name != "Reviewer" || stored.Prompt != "Review" {
					t.Fatalf("rejected update mutated the row: %#v", stored)
				}
			})
		}
	})

	t.Run("a built-in with no inference target may still be enabled", func(t *testing.T) {
		// Built-ins fall back to the user's default agent/model at execution
		// time, so the "custom agent needs a target" rule must not apply.
		f := newUtilityFixture(t, utilityFixtureOptions{})
		seedUtilityAgent(f, &models.UtilityAgent{
			ID: "builtin-1", Name: "Commit", Prompt: "Sum", Builtin: true,
			ProfileBindingState: models.ProfileBindingInherit,
		})
		status, raw := do(t, f, http.MethodPatch, "/api/v1/utility/agents/builtin-1", `{"enabled":true}`)
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, raw)
		}
		stored, err := f.repo.GetAgentByID(t.Context(), "builtin-1")
		if err != nil {
			t.Fatalf("stored agent lookup: %v", err)
		}
		if !stored.Enabled {
			t.Fatal("built-in was not enabled")
		}
	})

	t.Run("enabling a custom agent with no inference target is 400", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		seedUtilityAgent(f, &models.UtilityAgent{ID: "custom-1", Name: "Reviewer", Prompt: "Review"})
		status, raw := do(t, f, http.MethodPatch, "/api/v1/utility/agents/custom-1", `{"enabled":true}`)
		requireUtilityError(t, status, raw, http.StatusBadRequest, "failed to update utility agent")
		stored, err := f.repo.GetAgentByID(t.Context(), "custom-1")
		if err != nil {
			t.Fatalf("stored agent lookup: %v", err)
		}
		if stored.Enabled {
			t.Fatal("rejected update still enabled the row")
		}
	})
}

// TestDeleteUtilityAgentEndpoint keeps forbidden (built-in) distinct from
// not-found: the UI hides the delete affordance on the former, so collapsing
// them would hide a real regression.
func TestDeleteUtilityAgentEndpoint(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{})
	seedUtilityAgent(f, &models.UtilityAgent{ID: "custom-1", Name: "Reviewer", Prompt: "Review"})
	seedUtilityAgent(f, &models.UtilityAgent{ID: "builtin-1", Name: "Commit", Prompt: "Sum", Builtin: true})

	status, raw := do(t, f, http.MethodDelete, "/api/v1/utility/agents/custom-1", "")
	if status != http.StatusOK || string(raw) != `{"success":true}` {
		t.Fatalf("delete = %d %s", status, raw)
	}
	if _, err := f.repo.GetAgentByID(t.Context(), "custom-1"); err == nil {
		t.Fatal("row survived the delete")
	}

	builtinStatus, builtinRaw := do(t, f, http.MethodDelete, "/api/v1/utility/agents/builtin-1", "")
	requireUtilityError(t, builtinStatus, builtinRaw, http.StatusForbidden, "failed to delete utility agent")
	if _, err := f.repo.GetAgentByID(t.Context(), "builtin-1"); err != nil {
		t.Fatal("forbidden delete removed the built-in row")
	}

	missingStatus, missingRaw := do(t, f, http.MethodDelete, "/api/v1/utility/agents/nope", "")
	requireUtilityError(t, missingStatus, missingRaw, http.StatusNotFound, "failed to delete utility agent")

	f.repo.failWith("DeleteAgent", errors.New("database is locked"))
	seedUtilityAgent(f, &models.UtilityAgent{ID: "custom-2", Name: "Other", Prompt: "P"})
	brokenStatus, brokenRaw := do(t, f, http.MethodDelete, "/api/v1/utility/agents/custom-2", "")
	requireUtilityError(t, brokenStatus, brokenRaw, http.StatusInternalServerError, "failed to delete utility agent")
}

// TestTemplateVariablesEndpoint pins that the documented variables reach the
// wire: the settings editor renders this list as the insertable placeholders,
// so an empty or renamed entry breaks prompt authoring silently.
func TestTemplateVariablesEndpoint(t *testing.T) {
	f := newUtilityFixture(t, utilityFixtureOptions{})
	status, raw := do(t, f, http.MethodGet, "/api/v1/utility/template-variables", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", status, raw)
	}
	var got dto.TemplateVariablesResponse
	decodeInto(t, raw, &got)
	if len(got.Variables) == 0 {
		t.Fatal("no template variables returned")
	}
	names := make(map[string]bool, len(got.Variables))
	for _, v := range got.Variables {
		names[v.Name] = true
	}
	for _, want := range []string{"GitDiff", "TaskTitle", "UserPrompt"} {
		if !names[want] {
			t.Errorf("variable %q missing from %v", want, names)
		}
	}
}

// TestListCallsEndpointClampsLimit pins the limit rules the handler applies
// before touching the store: only 1..100 is honoured, everything else falls
// back to 50. The store records what it was asked for, so a dropped clamp is
// visible rather than merely "still 200".
func TestListCallsEndpointClampsLimit(t *testing.T) {
	cases := []struct {
		query     string
		wantLimit int
	}{
		{query: "", wantLimit: 50},
		{query: "?limit=10", wantLimit: 10},
		{query: "?limit=100", wantLimit: 100},
		{query: "?limit=101", wantLimit: 50},
		{query: "?limit=0", wantLimit: 50},
		{query: "?limit=-5", wantLimit: 50},
		{query: "?limit=abc", wantLimit: 50},
	}
	for _, tc := range cases {
		t.Run("limit"+tc.query, func(t *testing.T) {
			f := newUtilityFixture(t, utilityFixtureOptions{})
			status, raw := do(t, f, http.MethodGet, "/api/v1/utility/agents/custom-1/calls"+tc.query, "")
			if status != http.StatusOK {
				t.Fatalf("status = %d, body = %s", status, raw)
			}
			if f.repo.lastListCallsLimit != tc.wantLimit {
				t.Fatalf("store limit = %d, want %d", f.repo.lastListCallsLimit, tc.wantLimit)
			}
		})
	}

	t.Run("returns the stored calls and 500s on a store failure", func(t *testing.T) {
		f := newUtilityFixture(t, utilityFixtureOptions{})
		completed := fixedUtilityTime.Add(time.Minute)
		if err := f.repo.CreateCall(t.Context(), &models.UtilityAgentCall{
			ID: "call-a", UtilityID: "custom-1", Status: "completed", Response: "done",
			CreatedAt: fixedUtilityTime, CompletedAt: &completed,
		}); err != nil {
			t.Fatalf("seed call: %v", err)
		}
		status, raw := do(t, f, http.MethodGet, "/api/v1/utility/agents/custom-1/calls", "")
		if status != http.StatusOK {
			t.Fatalf("status = %d, body = %s", status, raw)
		}
		var got dto.UtilityAgentCallsResponse
		decodeInto(t, raw, &got)
		if len(got.Calls) != 1 || got.Calls[0].ID != "call-a" || got.Calls[0].Response != "done" {
			t.Fatalf("calls = %#v", got.Calls)
		}
		if got.Calls[0].CompletedAt == nil || *got.Calls[0].CompletedAt != "2026-03-04T05:07:07Z" {
			t.Fatalf("completed_at = %v", got.Calls[0].CompletedAt)
		}

		f.repo.failWith("ListCalls", errors.New("database is locked"))
		brokenStatus, brokenRaw := do(t, f, http.MethodGet, "/api/v1/utility/agents/custom-1/calls", "")
		requireUtilityError(t, brokenStatus, brokenRaw, http.StatusInternalServerError, "failed to list calls")
	})
}
