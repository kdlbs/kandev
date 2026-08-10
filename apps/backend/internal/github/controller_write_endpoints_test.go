package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doControllerJSON issues a request carrying a JSON body (marshalled from
// payload, or sent verbatim when payload is a string) and returns the recorder.
func doControllerJSON(
	t *testing.T, router http.Handler, method, target string, payload any,
) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	switch typed := payload.(type) {
	case nil:
		body = nil
	case string:
		body = []byte(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		body = encoded
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestController_ListTaskPRs_RequiresAWorkspaceOrTaskIDs(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/task-prs", nil)
	request.Header.Set("X-Test-Omit-Workspace", "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "workspace_id or task_ids query parameter required" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_ListTaskPRs_WorkspaceScoped(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	seedControllerTaskPR(t, store, "task-1", 7)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/task-prs?workspace_id=ws-1")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body struct {
		TaskPRs map[string][]TaskPR `json:"task_prs"`
	}
	decodeControllerBody(t, response, &body)
	if len(body.TaskPRs) != 1 || len(body.TaskPRs["task-1"]) != 1 {
		t.Fatalf("task_prs = %#v, want one association keyed by task-1", body.TaskPRs)
	}
	got := body.TaskPRs["task-1"][0]
	if got.TaskID != "task-1" || got.PRNumber != 7 || got.Owner != "acme" || got.Repo != "widget" {
		t.Errorf("task PR = %#v", got)
	}
	if got.WorkspaceID != "ws-1" || got.State != "open" || got.BaseBranch != "main" {
		t.Errorf("task PR = %#v", got)
	}
}

// The legacy task_ids form filters by explicit IDs rather than by workspace.
func TestController_ListTaskPRs_LegacyTaskIDsFilter(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	seedControllerTaskPR(t, store, "task-1", 7)
	seedControllerTaskPR(t, store, "task-2", 8)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/github/task-prs?task_ids=task-2", nil)
	request.Header.Set("X-Test-Omit-Workspace", "1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var body struct {
		TaskPRs map[string][]TaskPR `json:"task_prs"`
	}
	decodeControllerBody(t, response, &body)
	if len(body.TaskPRs) != 1 {
		t.Fatalf("task_prs = %#v, want only the requested task-2", body.TaskPRs)
	}
	rows := body.TaskPRs["task-2"]
	if len(rows) != 1 || rows[0].PRNumber != 8 {
		t.Fatalf("task-2 rows = %#v, want PR 8", rows)
	}
}

func TestController_GetTaskPR(t *testing.T) {
	router, store := setupControllerStoreTest(t)
	seedControllerTaskPR(t, store, "task-1", 7)

	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/task-prs/task-1")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
	}
	var got TaskPR
	decodeControllerBody(t, response, &got)
	if got.TaskID != "task-1" || got.PRNumber != 7 {
		t.Errorf("task PR = %#v", got)
	}
}

// No association is a 204 with no body, not a 404 — the panel treats an empty
// response as "this task has no PR yet".
func TestController_GetTaskPR_MissingAssociationIsNoContent(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerRequest(router, http.MethodGet, "/api/v1/github/task-prs/task-unknown")
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", response.Code, response.Body)
	}
	if response.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", response.Body.String())
	}
}

func TestController_CreateTaskPR_Validation(t *testing.T) {
	cases := []struct {
		name    string
		payload any
		want    string
	}{
		{"malformed JSON", "{not json", "invalid payload"},
		{
			"missing workspace_id",
			map[string]string{"task_id": "task-1", "pr_url": "https://github.com/acme/widget/pull/7"},
			"workspace_id, task_id and pr_url are required",
		},
		{
			"missing task_id",
			map[string]string{"workspace_id": "ws-1", "pr_url": "https://github.com/acme/widget/pull/7"},
			"workspace_id, task_id and pr_url are required",
		},
		{
			"missing pr_url",
			map[string]string{"workspace_id": "ws-1", "task_id": "task-1"},
			"workspace_id, task_id and pr_url are required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, _ := setupControllerStoreTest(t)
			response := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/task-prs", tc.payload)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			decodeControllerBody(t, response, &body)
			if body.Error != tc.want {
				t.Errorf("error = %q, want %q", body.Error, tc.want)
			}
		})
	}
}

// A URL that is not a PR link is the caller's fault, so it maps to 400 rather
// than collapsing into the generic 500.
func TestController_CreateTaskPR_InvalidPRURLIs400(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/task-prs", map[string]string{
		"workspace_id": "ws-1",
		"task_id":      "task-1",
		"pr_url":       "https://example.com/not-a-pr",
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
}

func TestController_ActionPresets(t *testing.T) {
	router, _ := setupControllerStoreTest(t)

	t.Run("defaults are returned when nothing is stored", func(t *testing.T) {
		response := doControllerRequest(
			router, http.MethodGet, "/api/v1/github/action-presets?workspace_id=ws-1",
		)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var presets ActionPresets
		decodeControllerBody(t, response, &presets)
		if presets.WorkspaceID != "ws-1" {
			t.Errorf("workspace_id = %q", presets.WorkspaceID)
		}
		if len(presets.PR) != len(DefaultPRActionPresets()) {
			t.Errorf("pr presets = %d, want the %d built-ins",
				len(presets.PR), len(DefaultPRActionPresets()))
		}
		if len(presets.Issue) == 0 {
			t.Error("issue presets are empty, want the built-ins")
		}
	})

	t.Run("update then read back", func(t *testing.T) {
		custom := []ActionPreset{{
			ID: "triage", Label: "Triage", Hint: "sort it", Icon: "list",
			PromptTemplate: "Triage {{url}}",
		}}
		response := doControllerJSON(t, router, http.MethodPut, "/api/v1/github/action-presets",
			UpdateActionPresetsRequest{WorkspaceID: "ws-1", PR: &custom})
		if response.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var updated ActionPresets
		decodeControllerBody(t, response, &updated)
		if len(updated.PR) != 1 || updated.PR[0].ID != "triage" || updated.PR[0].Label != "Triage" {
			t.Fatalf("pr presets = %#v", updated.PR)
		}
		// Issue presets were not supplied, so they must be left untouched.
		if len(updated.Issue) == 0 {
			t.Error("issue presets were cleared, want them unchanged")
		}

		readBack := doControllerRequest(
			router, http.MethodGet, "/api/v1/github/action-presets?workspace_id=ws-1",
		)
		var stored ActionPresets
		decodeControllerBody(t, readBack, &stored)
		if len(stored.PR) != 1 || stored.PR[0].ID != "triage" {
			t.Fatalf("stored pr presets = %#v, want the update persisted", stored.PR)
		}
	})

	t.Run("reset restores the built-ins", func(t *testing.T) {
		response := doControllerJSON(t, router, http.MethodPost,
			"/api/v1/github/action-presets/reset?workspace_id=ws-1", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("reset status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var presets ActionPresets
		decodeControllerBody(t, response, &presets)
		if len(presets.PR) != len(DefaultPRActionPresets()) {
			t.Fatalf("pr presets = %d, want the built-ins back", len(presets.PR))
		}
	})
}

func TestController_ActionPresets_Validation(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	cases := []struct {
		name    string
		method  string
		target  string
		payload any
		want    string
	}{
		{
			"get without a workspace", http.MethodGet, "/api/v1/github/action-presets", nil,
			"workspace_id query parameter required",
		},
		{
			"reset without a workspace", http.MethodPost, "/api/v1/github/action-presets/reset", nil,
			"workspace_id query parameter required",
		},
		{
			"update with malformed JSON", http.MethodPut, "/api/v1/github/action-presets", "{nope",
			"invalid payload",
		},
		{
			"update without a workspace", http.MethodPut, "/api/v1/github/action-presets",
			map[string]any{}, "workspace_id is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, tc.target, bytes.NewReader(marshalPayload(t, tc.payload)))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("X-Test-Omit-Workspace", "1")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			decodeControllerBody(t, response, &body)
			if body.Error != tc.want {
				t.Errorf("error = %q, want %q", body.Error, tc.want)
			}
		})
	}
}

func TestController_IssueWatchCRUD(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	created := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/watches/issue",
		CreateIssueWatchRequest{
			WorkspaceID:         "ws-1",
			WorkflowID:          "wf-1",
			WorkflowStepID:      "step-1",
			Repos:               []RepoFilter{{Owner: "acme", Name: "widget"}},
			Prompt:              "fix it",
			Labels:              []string{"bug"},
			CustomQuery:         "is:open",
			PollIntervalSeconds: 300,
		})
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body = %s", created.Code, created.Body)
	}
	var watch IssueWatch
	decodeControllerBody(t, created, &watch)
	if watch.ID == "" {
		t.Fatal("created watch has no ID")
	}
	if watch.WorkspaceID != "ws-1" || watch.Prompt != "fix it" || watch.CustomQuery != "is:open" {
		t.Errorf("created watch = %#v", watch)
	}
	if len(watch.Repos) != 1 || watch.Repos[0].Owner != "acme" || watch.Repos[0].Name != "widget" {
		t.Errorf("created repos = %#v", watch.Repos)
	}
	if watch.PollIntervalSeconds != 300 {
		t.Errorf("poll interval = %d, want 300", watch.PollIntervalSeconds)
	}

	t.Run("list returns the stored watch", func(t *testing.T) {
		response := doControllerRequest(
			router, http.MethodGet, "/api/v1/github/watches/issue?workspace_id=ws-1",
		)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var body struct {
			Watches []IssueWatch `json:"watches"`
		}
		decodeControllerBody(t, response, &body)
		if len(body.Watches) != 1 || body.Watches[0].ID != watch.ID {
			t.Fatalf("watches = %#v, want the seeded watch", body.Watches)
		}
		got := body.Watches[0]
		if got.Prompt != "fix it" || got.CustomQuery != "is:open" || !got.Enabled {
			t.Errorf("watch = %#v", got)
		}
		if len(got.Repos) != 1 || got.Repos[0].Owner != "acme" || got.Repos[0].Name != "widget" {
			t.Errorf("repos = %#v", got.Repos)
		}
		if got.PollIntervalSeconds != 300 {
			t.Errorf("poll interval = %d, want 300", got.PollIntervalSeconds)
		}
	})

	t.Run("update mutates only the supplied fields", func(t *testing.T) {
		newPrompt := "handle it"
		disabled := false
		response := doControllerJSON(t, router, http.MethodPut,
			"/api/v1/github/watches/issue/"+watch.ID+"?workspace_id=ws-1",
			UpdateIssueWatchRequest{Prompt: &newPrompt, Enabled: &disabled})
		if response.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var updated IssueWatch
		decodeControllerBody(t, response, &updated)
		if updated.Prompt != "handle it" {
			t.Errorf("prompt = %q, want the update applied", updated.Prompt)
		}
		if updated.Enabled {
			t.Error("enabled = true, want the update applied")
		}
		if updated.CustomQuery != "is:open" {
			t.Errorf("custom_query = %q, want it left untouched", updated.CustomQuery)
		}
		if len(updated.Repos) != 1 || updated.Repos[0].Owner != "acme" {
			t.Errorf("repos = %#v, want them left untouched", updated.Repos)
		}
	})

	t.Run("delete removes the watch", func(t *testing.T) {
		response := doControllerJSON(t, router, http.MethodDelete,
			"/api/v1/github/watches/issue/"+watch.ID+"?workspace_id=ws-1", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("delete status = %d, want 200; body = %s", response.Code, response.Body)
		}
		var body struct {
			Deleted bool `json:"deleted"`
		}
		decodeControllerBody(t, response, &body)
		if !body.Deleted {
			t.Error("deleted = false, want true")
		}
		list := doControllerRequest(router, http.MethodGet, "/api/v1/github/watches/issue?workspace_id=ws-1")
		if strings.Contains(list.Body.String(), watch.ID) {
			t.Errorf("watch %s still listed after delete: %s", watch.ID, list.Body)
		}
	})
}

func TestController_IssueWatch_MalformedPayloadIs400(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/watches/issue", "{nope")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "invalid payload" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_CreateReviewWatch(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/watches/review",
		CreateReviewWatchRequest{
			WorkspaceID:    "ws-1",
			WorkflowID:     "wf-1",
			WorkflowStepID: "step-1",
			Repos:          []RepoFilter{{Owner: "acme", Name: "widget"}},
		})
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body)
	}
	var watch ReviewWatch
	decodeControllerBody(t, response, &watch)
	if watch.ID == "" {
		t.Fatal("created watch has no ID")
	}
	if watch.WorkspaceID != "ws-1" || watch.WorkflowID != "wf-1" {
		t.Errorf("created watch = %#v", watch)
	}
	if len(watch.Repos) != 1 || watch.Repos[0].Owner != "acme" || watch.Repos[0].Name != "widget" {
		t.Errorf("created repos = %#v", watch.Repos)
	}
}

func TestController_CreateReviewWatch_MalformedPayloadIs400(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPost, "/api/v1/github/watches/review", "{nope")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
}

// The trigger-all endpoints are workspace-scoped sweeps; without a workspace
// they would fan out across every workspace, so they refuse.
func TestController_TriggerAllChecks_RequireAWorkspace(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	for _, target := range []string{
		"/api/v1/github/watches/review/trigger-all",
		"/api/v1/github/watches/issue/trigger-all",
	} {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, target, nil)
			request.Header.Set("X-Test-Omit-Workspace", "1")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			decodeControllerBody(t, response, &body)
			if body.Error != "workspace_id query parameter required" {
				t.Errorf("error = %q", body.Error)
			}
		})
	}
}

func TestController_TriggerAllChecks_ReportNothingFoundOnAnEmptyWorkspace(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	cases := []struct {
		target  string
		countAt string
	}{
		{"/api/v1/github/watches/review/trigger-all?workspace_id=ws-1", "new_prs_found"},
		{"/api/v1/github/watches/issue/trigger-all?workspace_id=ws-1", "new_issues_found"},
	}
	for _, tc := range cases {
		t.Run(tc.countAt, func(t *testing.T) {
			response := doControllerJSON(t, router, http.MethodPost, tc.target, nil)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
			}
			var body map[string]int
			decodeControllerBody(t, response, &body)
			count, ok := body[tc.countAt]
			if !ok {
				t.Fatalf("body = %s, want a %q field", response.Body, tc.countAt)
			}
			if count != 0 {
				t.Errorf("%s = %d, want 0 with no watches configured", tc.countAt, count)
			}
		})
	}
}

func TestController_CleanupEndpoints(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	for _, target := range []string{
		"/api/v1/github/cleanup/review-tasks?workspace_id=ws-1",
		"/api/v1/github/cleanup/issue-tasks?workspace_id=ws-1",
		// No workspace_id takes the cleanup-everything branch.
		"/api/v1/github/cleanup/review-tasks",
		"/api/v1/github/cleanup/issue-tasks",
	} {
		t.Run(target, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, target, nil)
			request.Header.Set("X-Test-Omit-Workspace", "1")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body)
			}
			var body struct {
				Deleted int `json:"deleted"`
			}
			decodeControllerBody(t, response, &body)
			if body.Deleted != 0 {
				t.Errorf("deleted = %d, want 0 with nothing to clean up", body.Deleted)
			}
		})
	}
}

func TestController_WorkspaceSettings_UpdateAndRead(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	mode := RepoScopeModeOrgs
	orgs := []string{"acme"}

	updated := doControllerJSON(t, router, http.MethodPut, "/api/v1/github/workspace-settings",
		UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &mode, RepoScopeOrgs: &orgs})
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, want 200; body = %s", updated.Code, updated.Body)
	}
	var settings WorkspaceSettings
	decodeControllerBody(t, updated, &settings)
	if settings.RepoScopeMode != RepoScopeModeOrgs {
		t.Errorf("repo_scope_mode = %q, want orgs", settings.RepoScopeMode)
	}
	if len(settings.RepoScopeOrgs) != 1 || settings.RepoScopeOrgs[0] != "acme" {
		t.Errorf("repo_scope_orgs = %#v", settings.RepoScopeOrgs)
	}

	readBack := doControllerRequest(
		router, http.MethodGet, "/api/v1/github/workspace-settings?workspace_id=ws-1",
	)
	if readBack.Code != http.StatusOK {
		t.Fatalf("read status = %d, want 200; body = %s", readBack.Code, readBack.Body)
	}
	var stored WorkspaceSettings
	decodeControllerBody(t, readBack, &stored)
	if stored.RepoScopeMode != RepoScopeModeOrgs || len(stored.RepoScopeOrgs) != 1 {
		t.Fatalf("stored settings = %#v, want the update persisted", stored)
	}
}

func TestController_WorkspaceSettings_RejectsAnInvalidScopeMode(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	mode := "not-a-mode"
	response := doControllerJSON(t, router, http.MethodPut, "/api/v1/github/workspace-settings",
		UpdateWorkspaceSettingsRequest{WorkspaceID: "ws-1", RepoScopeMode: &mode})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
}

func TestController_WorkspaceSettings_MalformedPayloadIs400(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPut, "/api/v1/github/workspace-settings", "{nope")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
	var body struct {
		Error string `json:"error"`
	}
	decodeControllerBody(t, response, &body)
	if body.Error != "invalid payload" {
		t.Errorf("error = %q", body.Error)
	}
}

func TestController_CopyWorkspaceSettings_Validation(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	cases := []struct {
		name       string
		target     string
		payload    any
		omitWS     bool
		wantErrMsg string
	}{
		{
			"missing source workspace", "/api/v1/github/workspace-settings/copy",
			map[string]string{"targetWorkspaceId": "ws-2"}, true,
			"workspace_id query parameter required",
		},
		{
			"malformed payload", "/api/v1/github/workspace-settings/copy?workspace_id=ws-1",
			"{nope", false, "invalid payload",
		},
		{
			"missing target", "/api/v1/github/workspace-settings/copy?workspace_id=ws-1",
			map[string]string{}, false, "targetWorkspaceId required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost, tc.target, bytes.NewReader(marshalPayload(t, tc.payload)),
			)
			request.Header.Set("Content-Type", "application/json")
			if tc.omitWS {
				request.Header.Set("X-Test-Omit-Workspace", "1")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
			}
			var body struct {
				Error string `json:"error"`
			}
			decodeControllerBody(t, response, &body)
			if body.Error != tc.wantErrMsg {
				t.Errorf("error = %q, want %q", body.Error, tc.wantErrMsg)
			}
		})
	}
}

// Copying a workspace onto itself is rejected rather than silently no-oping.
func TestController_CopyWorkspaceSettings_RejectsTheSameWorkspace(t *testing.T) {
	router, _ := setupControllerStoreTest(t)
	response := doControllerJSON(t, router, http.MethodPost,
		"/api/v1/github/workspace-settings/copy?workspace_id=ws-1",
		map[string]string{"targetWorkspaceId": "ws-1"})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
	}
}

func TestController_ConfigureToken_Validation(t *testing.T) {
	router, _ := setupControllerStoreTest(t)

	t.Run("missing workspace", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/github/token",
			strings.NewReader(`{"token":"ghp_x"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("X-Test-Omit-Workspace", "1")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			t.Fatalf("status = %d, want a failure; body = %s", response.Code, response.Body)
		}
	})
	t.Run("malformed payload", func(t *testing.T) {
		response := doControllerJSON(t, router, http.MethodPost,
			"/api/v1/github/token?workspace_id=ws-1", "{nope")
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", response.Code, response.Body)
		}
		var body struct {
			Error string `json:"error"`
		}
		decodeControllerBody(t, response, &body)
		if body.Error != "invalid payload: token is required" {
			t.Errorf("error = %q", body.Error)
		}
	})
}

// marshalPayload encodes a test payload, passing strings through verbatim so a
// case can send deliberately malformed JSON.
func marshalPayload(t *testing.T, payload any) []byte {
	t.Helper()
	switch typed := payload.(type) {
	case nil:
		return nil
	case string:
		return []byte(typed)
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		return encoded
	}
}

// seedControllerTaskPR persists one task→PR association in the ws-1 workspace,
// along with the task row the workspace-scoped listing joins against.
func seedControllerTaskPR(t *testing.T, store *Store, taskID string, prNumber int) *TaskPR {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO tasks (id, workspace_id, title) VALUES (?, 'ws-1', ?)`, taskID, taskID,
	); err != nil {
		t.Fatalf("seed task row: %v", err)
	}
	tp := &TaskPR{
		WorkspaceID: "ws-1",
		TaskID:      taskID,
		Owner:       "acme",
		Repo:        "widget",
		PRNumber:    prNumber,
		PRURL:       fmt.Sprintf("https://github.com/acme/widget/pull/%d", prNumber),
		PRTitle:     fmt.Sprintf("PR %d", prNumber),
		HeadBranch:  fmt.Sprintf("feature-%d", prNumber),
		BaseBranch:  "main",
		AuthorLogin: "octocat",
		State:       "open",
	}
	if err := store.CreateTaskPR(context.Background(), tp); err != nil {
		t.Fatalf("seed task PR: %v", err)
	}
	return tp
}
