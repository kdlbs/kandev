package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

// capturedRequest stores details from the mock server for assertion.
type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
	Header http.Header
}

// setupMockServer creates an httptest server that records the request and
// responds with the given status and body.
func setupMockServer(t *testing.T, status int, respBody string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	captured := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.Method = r.Method
		captured.Path = r.URL.Path
		captured.Query = r.URL.RawQuery
		captured.Header = r.Header.Clone()
		body, _ := io.ReadAll(r.Body)
		captured.Body = string(body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(respBody))
	}))
	t.Cleanup(srv.Close)
	return srv, captured
}

// setEnvVars sets the required env vars for tests and returns a cleanup function.
func setEnvVars(t *testing.T, srv *httptest.Server) {
	t.Helper()
	t.Setenv("KANDEV_API_URL", srv.URL)
	t.Setenv("KANDEV_API_KEY", "test-key-123")
	t.Setenv("KANDEV_RUN_ID", "run-456")
	t.Setenv("KANDEV_AGENT_ID", "agent-789")
	t.Setenv("KANDEV_TASK_ID", "task-abc")
	t.Setenv("KANDEV_WORKSPACE_ID", "ws-def")
}

func TestNewKandevClient_NormalizesVersionedAPIURL(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"task":{"id":"task-abc"}}`)
	setEnvVars(t, srv)
	t.Setenv("KANDEV_API_URL", srv.URL+"/api/v1")

	code := runKandevCLI([]string{"task", "get"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
}

// --- Task Tests ---

func TestTaskGet_CallsCorrectEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"task":{"id":"task-abc"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"task", "get"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "GET" {
		t.Errorf("expected GET, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	assertAuthHeader(t, captured)
}

func TestTaskGet_ExplicitID(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"task":{"id":"explicit-1"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"task", "get", "--id", "explicit-1"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/tasks/explicit-1" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
}

func TestTaskUpdate_PostsToRuntimeStatusEndpoint(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{"ok":true}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "test-key-123")
	t.Setenv("KANDEV_RUN_ID", "run-456")
	t.Setenv("KANDEV_TASK_ID", "task-abc")

	code := runKandevCLI([]string{
		"task", "update", "--status", "done", "--comment", "finished",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/runtime/tasks/task-abc/status" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	assertAuthHeader(t, captured)
	assertRunIDHeader(t, captured, "run-456")

	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["status"] != "done" {
		t.Errorf("expected status=done, got %s", body["status"])
	}
	if body["comment"] != "finished" {
		t.Errorf("expected comment=finished, got %s", body["comment"])
	}
}

func TestTaskUpdate_PropagatesRuntimeScopeDenial(t *testing.T) {
	captured := setupMockTransport(t, http.StatusForbidden, `{"error":"task outside run scope"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	code := runKandevCLI([]string{
		"task", "update", "--id", "task-forbidden", "--status", "done",
	})
	if code != 1 {
		t.Fatalf("task update exit = %d, want 1 for HTTP 403", code)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/v1/office/runtime/tasks/task-forbidden/status" {
		t.Fatalf("request = %s %s, want signed runtime status endpoint", captured.Method, captured.Path)
	}
}

func TestTaskUpdate_CommentOnlyDirectsCallerToTasksMessage(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	code := runKandevCLI([]string{"task", "update", "--comment", "progress"})
	_ = w.Close()
	os.Stderr = oldStderr
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}
	if code != 1 {
		t.Fatalf("task update exit = %d, want 1", code)
	}
	if captured.Method != "" {
		t.Fatalf("server contacted for comment-only update: %s %s", captured.Method, captured.Path)
	}
	if !strings.Contains(string(output), "tasks message") {
		t.Fatalf("stderr = %q, want tasks message guidance", output)
	}
}

func TestTaskUpdate_EmptyPayload_ReturnsError(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	code := runKandevCLI([]string{"task", "update"})
	if code == 0 {
		t.Fatal("expected non-zero exit when no update fields provided, got 0")
	}
	if captured.Method != "" {
		t.Errorf("server must not be contacted on empty payload; got %s %s", captured.Method, captured.Path)
	}
}

func TestTaskCreate_PostsToRuntimeEndpointWithoutWorkspace(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task":{"id":"new-1"}}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "test-key-123")
	t.Setenv("KANDEV_RUN_ID", "run-456")
	t.Setenv("KANDEV_WORKSPACE_ID", "ws-def")

	code := runKandevCLI([]string{
		"task", "create", "--title", "New task", "--description", "Detailed task context",
		"--parent", "parent-1", "--assignee", "agent-2",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "POST" {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/runtime/tasks" {
		t.Errorf("unexpected path: %s", captured.Path)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["title"] != "New task" {
		t.Errorf("expected title='New task', got %s", body["title"])
	}
	if body["description"] != "Detailed task context" {
		t.Errorf("expected description='Detailed task context', got %s", body["description"])
	}
	if body["parent_id"] != "parent-1" {
		t.Errorf("expected parent_id='parent-1', got %s", body["parent_id"])
	}
	if _, ok := body["workspace_id"]; ok {
		t.Errorf("workspace_id must not be sent by the runtime client: %#v", body)
	}
}

func TestTaskCreate_DoesNotRequireWorkspaceID(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"new-1"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "test-key-123")
	t.Setenv("KANDEV_WORKSPACE_ID", "")

	if code := runKandevCLI([]string{"task", "create", "--title", "New task"}); code != 0 {
		t.Fatalf("task create exit = %d, want 0", code)
	}
	if captured.Path != "/api/v1/office/runtime/tasks" {
		t.Fatalf("request path = %q, want runtime task endpoint", captured.Path)
	}
}

func TestTaskCreate_ProjectID(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"new-project-task"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")

	code := runKandevCLI([]string{
		"task", "create", "--title", "Project task", "--project", "project-1",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["project_id"] != "project-1" {
		t.Errorf("expected project_id=project-1, got %v", body["project_id"])
	}
}

func TestTaskCreate_RejectsUnsupportedFieldsBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "priority", args: []string{"--priority", "high"}},
		{name: "blockers", args: []string{"--blocked-by", "task-1,task-2"}},
		{name: "workspace mode", args: []string{"--workspace-mode", "new_workspace"}},
		{name: "workspace group", args: []string{"--workspace-group-id", "group-1"}},
		{name: "child workspace", args: []string{"--default-child-workspace", "inherit_parent"}},
		{name: "child ordering", args: []string{"--default-child-ordering", "sequential"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"new-2"}`)
			t.Setenv("KANDEV_API_URL", "http://kandev.test")
			t.Setenv("KANDEV_API_KEY", "test-key")
			args := append([]string{"task", "create", "--title", "Unsupported task"}, tc.args...)

			if code := runKandevCLI(args); code == 0 {
				t.Fatal("expected unsupported flag to fail")
			}
			if captured.Method != "" {
				t.Fatalf("server contacted for unsupported flag: %s %s", captured.Method, captured.Path)
			}
		})
	}
}

func TestTaskCreate_NoExecutionPolicyOrBlockedBy_OmitsFields(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"task":{"id":"new-4"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"task", "create", "--title", "Plain task"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	if _, exists := body["execution_policy"]; exists {
		t.Error("execution_policy should not be present when flag is not set")
	}
	if _, exists := body["blocked_by"]; exists {
		t.Error("blocked_by should not be present when flag is not set")
	}
	// Workspace-policy fields are office-only and must stay absent unless the
	// corresponding flag is supplied — the backend resolves sensible defaults.
	for _, k := range []string{"workspace_mode", "workspace_group_id", "default_child_workspace", "default_child_ordering"} {
		if _, exists := body[k]; exists {
			t.Errorf("%s should not be present when flag is not set", k)
		}
	}
}

// TestTaskCreate_SharedGroupReportsUnsupported ensures legacy workspace policy
// flags produce one actionable error instead of contradictory validation.
func TestTaskCreate_SharedGroupReportsUnsupported(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{
			name: "missing",
			args: []string{"task", "create", "--title", "Bad task", "--workspace-mode", "shared_group"},
		},
		{
			name: "whitespace only",
			args: []string{"task", "create", "--title", "Bad task", "--workspace-mode", "shared_group", "--workspace-group-id", "   "},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("pipe: %v", err)
			}
			oldStderr := os.Stderr
			os.Stderr = w
			code := runKandevCLI(tc.args)
			_ = w.Close()
			os.Stderr = oldStderr
			output, readErr := io.ReadAll(r)
			_ = r.Close()
			if readErr != nil {
				t.Fatalf("read stderr: %v", readErr)
			}
			if code == 0 {
				t.Fatal("expected unsupported workspace mode to fail")
			}
			if !strings.Contains(string(output), "--workspace-mode is not supported by Office runtime task create") {
				t.Fatalf("stderr = %q, want unsupported workspace mode", output)
			}
		})
	}
}

func TestTaskCreate_RejectsBlankTitleBeforeRequest(t *testing.T) {
	original := http.DefaultTransport
	requestCount := 0
	http.DefaultTransport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(`{"task_id":"task-1"}`)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")

	code := runKandevCLI([]string{"task", "create", "--title", " \t "})
	if code == 0 {
		t.Fatal("expected non-zero exit for blank task title")
	}
	if requestCount != 0 {
		t.Fatalf("HTTP requests = %d, want 0 for blank title", requestCount)
	}
}

// --- Project Tests ---

func TestProjectsList_CallsRuntimeEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusOK, `{"projects":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"projects", "list"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != http.MethodGet {
		t.Errorf("expected GET, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/runtime/projects" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	assertAuthHeader(t, captured)
}

func TestProjectsCreate_SendsOptionsAndRepositories(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusCreated, `{"project":{"id":"project-1"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"projects", "create",
		"--name", "Platform",
		"--description", "Core platform work",
		"--repository", "https://github.com/acme/api.git",
		"--repository", "https://github.com/acme/web.git",
		"--lead-agent-profile-id", "lead-1",
		"--color", "#336699",
		"--budget-cents", "12500",
		"--executor-config", `{"type":"local_docker"}`,
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/runtime/projects" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	assertAuthHeader(t, captured)
	assertRunIDHeader(t, captured, "run-456")

	var body struct {
		Name               string   `json:"name"`
		Description        string   `json:"description"`
		Repositories       []string `json:"repositories"`
		LeadAgentProfileID string   `json:"lead_agent_profile_id"`
		Color              string   `json:"color"`
		BudgetCents        int      `json:"budget_cents"`
		ExecutorConfig     string   `json:"executor_config"`
	}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Name != "Platform" || body.Description != "Core platform work" {
		t.Errorf("unexpected project identity: name=%q description=%q", body.Name, body.Description)
	}
	wantRepositories := []string{"https://github.com/acme/api.git", "https://github.com/acme/web.git"}
	if len(body.Repositories) != len(wantRepositories) || body.Repositories[0] != wantRepositories[0] || body.Repositories[1] != wantRepositories[1] {
		t.Errorf("repositories = %v, want %v", body.Repositories, wantRepositories)
	}
	if body.LeadAgentProfileID != "lead-1" || body.Color != "#336699" || body.BudgetCents != 12500 {
		t.Errorf("unexpected project options: lead=%q color=%q budget=%d", body.LeadAgentProfileID, body.Color, body.BudgetCents)
	}
	if body.ExecutorConfig != `{"type":"local_docker"}` {
		t.Errorf("executor_config = %q", body.ExecutorConfig)
	}
}

func TestProjectsCreate_RequiresNonblankName(t *testing.T) {
	srv, captured := setupMockServer(t, http.StatusCreated, `{"project":{"id":"project-1"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"projects", "create", "--name", "   "})
	if code == 0 {
		t.Fatal("expected non-zero exit for blank project name")
	}
	if captured.Method != "" {
		t.Errorf("server must not be contacted for blank project name; got %s %s", captured.Method, captured.Path)
	}
}

func TestOfficeSetup_ProjectThenTaskUsesReturnedProjectAndWorkspace(t *testing.T) {
	var requests []capturedRequest
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requests = append(requests, capturedRequest{
			Method: req.Method,
			Path:   req.URL.Path,
			Body:   string(body),
			Header: req.Header.Clone(),
		})
		responseBody := `{"task":{"id":"task-1"}}`
		if req.URL.Path == "/api/v1/office/runtime/projects" {
			responseBody = `{"project":{"id":"project-1"}}`
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body:       io.NopCloser(strings.NewReader(responseBody)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_RUN_ID", "run-1")
	t.Setenv("KANDEV_WORKSPACE_ID", "ws-1")

	if code := runKandevCLI([]string{
		"projects", "create", "--name", "Alpha",
		"--repository", "https://github.com/acme/alpha",
	}); code != 0 {
		t.Fatalf("projects create exit = %d, want 0", code)
	}
	if code := runKandevCLI([]string{
		"task", "create", "--title", "Inspect Alpha", "--project", "project-1",
	}); code != 0 {
		t.Fatalf("task create exit = %d, want 0", code)
	}

	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].Method != http.MethodPost || requests[0].Path != "/api/v1/office/runtime/projects" {
		t.Fatalf("project request = %s %s", requests[0].Method, requests[0].Path)
	}
	if requests[1].Method != http.MethodPost || requests[1].Path != "/api/v1/office/runtime/tasks" {
		t.Fatalf("task request = %s %s", requests[1].Method, requests[1].Path)
	}
	var taskPayload map[string]interface{}
	if err := json.Unmarshal([]byte(requests[1].Body), &taskPayload); err != nil {
		t.Fatalf("decode task payload: %v", err)
	}
	if taskPayload["project_id"] != "project-1" {
		t.Fatalf("task payload = %#v, want project-1", taskPayload)
	}
	if _, ok := taskPayload["workspace_id"]; ok {
		t.Fatalf("task payload must not contain caller-selected workspace: %#v", taskPayload)
	}
	if requests[0].Header.Get("Authorization") != "Bearer signed-office-run-token" {
		t.Fatalf("project authorization = %q", requests[0].Header.Get("Authorization"))
	}
}

// --- Comment Tests ---

func TestCommentAdd_PostsComment(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"ok":true}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"comment", "add", "--body", "This is a test comment",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "POST" {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc/comments" {
		t.Errorf("unexpected path: %s", captured.Path)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["body"] != "This is a test comment" {
		t.Errorf("unexpected body: %s", body["body"])
	}
	if body["author_id"] != "agent-789" {
		t.Errorf("expected author_id=agent-789, got %s", body["author_id"])
	}
}

func TestCommentAdd_StdinMode(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"ok":true}`)
	setEnvVars(t, srv)

	// Replace stdin with a pipe containing test data.
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })

	_, _ = w.WriteString("multi-line\ncomment from stdin")
	_ = w.Close()

	code := runKandevCLI([]string{"comment", "add", "--body", "-"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["body"] != "multi-line\ncomment from stdin" {
		t.Errorf("unexpected body: %q", body["body"])
	}
}

func TestCommentList_GetsCommentsWithLimit(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"comments":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"comment", "list", "--limit", "5"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "GET" {
		t.Errorf("expected GET, got %s", captured.Method)
	}
	if !strings.Contains(captured.Query, "limit=5") {
		t.Errorf("expected limit=5 in query, got %s", captured.Query)
	}
}

func TestTasksMessage_PostsThroughSignedRuntimeScope(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"ok":true}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_RUN_ID", "run-456")
	t.Setenv("KANDEV_AGENT_ID", "spoofed-agent")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	code := runKandevCLI([]string{
		"tasks", "message", "--id", "task-allowed", "--prompt", "Status update",
	})
	if code != 0 {
		t.Fatalf("tasks message exit = %d, want 0", code)
	}
	if captured.Method != http.MethodPost || captured.Path != "/api/v1/office/runtime/comments" {
		t.Fatalf("request = %s %s, want POST /api/v1/office/runtime/comments", captured.Method, captured.Path)
	}
	assertRunIDHeader(t, captured, "run-456")

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["task_id"] != "task-allowed" || body["body"] != "Status update" {
		t.Fatalf("payload = %#v, want task_id and body", body)
	}
	for _, key := range []string{"author", "author_id", "author_type", "source"} {
		if _, ok := body[key]; ok {
			t.Errorf("payload must not contain caller-controlled %q: %#v", key, body)
		}
	}
}

func TestTasksMessage_PromptDashReadsStdin(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"ok":true}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = oldStdin
		_ = r.Close()
	})
	_, _ = w.WriteString("multiline\nagent update")
	_ = w.Close()

	if code := runKandevCLI([]string{"tasks", "message", "--prompt", "-"}); code != 0 {
		t.Fatalf("tasks message exit = %d, want 0", code)
	}
	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["body"] != "multiline\nagent update" {
		t.Fatalf("body = %q, want stdin content", body["body"])
	}
}

func TestTasksMessage_PropagatesRuntimeScopeDenial(t *testing.T) {
	captured := setupMockTransport(t, http.StatusForbidden, `{"error":"task outside run scope"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	code := runKandevCLI([]string{
		"tasks", "message", "--id", "task-forbidden", "--prompt", "should fail",
	})
	_ = w.Close()
	os.Stderr = oldStderr
	denial, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}
	if code != 1 {
		t.Fatalf("tasks message exit = %d, want 1 for HTTP 403", code)
	}
	if captured.Path != "/api/v1/office/runtime/comments" {
		t.Fatalf("request path = %q, want signed runtime comments endpoint", captured.Path)
	}
	if !strings.Contains(string(denial), "task outside run scope") {
		t.Fatalf("stderr = %q, want runtime denial body", denial)
	}
}

func TestTasksMove_FailsClosedWithoutHTTPRequest(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	code := runKandevCLI([]string{"tasks", "move", "--step", "review"})
	_ = w.Close()
	os.Stderr = oldStderr
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}

	if code != 1 {
		t.Fatalf("tasks move exit = %d, want 1", code)
	}
	if captured.Method != "" {
		t.Fatalf("tasks move contacted server: %s %s", captured.Method, captured.Path)
	}
	if !strings.Contains(string(output), "kandev task update --status") {
		t.Fatalf("stderr = %q, want signed status-update guidance", output)
	}
}

func TestTasksArchive_FailsClosedWithoutHTTPRequest(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-office-run-token")
	t.Setenv("KANDEV_TASK_ID", "task-current")

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	code := runKandevCLI([]string{"tasks", "archive"})
	_ = w.Close()
	os.Stderr = oldStderr
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}

	if code != 1 {
		t.Fatalf("tasks archive exit = %d, want 1", code)
	}
	if captured.Method != "" {
		t.Fatalf("tasks archive contacted server: %s %s", captured.Method, captured.Path)
	}
	if !strings.Contains(string(output), "human or admin") {
		t.Fatalf("stderr = %q, want human/admin archive guidance", output)
	}
}

// --- Agents Tests ---

func TestAgentsList_FiltersRoleAndStatus(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"agents":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"agents", "list", "--role", "worker", "--status", "idle"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/workspaces/ws-def/agents" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	if !strings.Contains(captured.Query, "role=worker") {
		t.Errorf("expected role=worker in query, got %s", captured.Query)
	}
	if !strings.Contains(captured.Query, "status=idle") {
		t.Errorf("expected status=idle in query, got %s", captured.Query)
	}
}

// TestAgentsList_SpecialCharsInFilterAreURLEncoded pins the cubic-dev-ai
// fix to `getWithParams`: the earlier raw string concat (`q += k + "="
// + v`) would have produced a malformed URL for any value containing
// `&`, `=`, or spaces. The new code routes through `url.Values.Encode()`.
// `TestAgentsList_FiltersRoleAndStatus` only exercises plain ASCII and
// would pass identically against the old buggy code; this test fails
// without the encoding fix because the captured query would contain a
// raw `&` that splits the value into a second parameter.
func TestAgentsList_SpecialCharsInFilterAreURLEncoded(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"agents":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"agents", "list", "--role", "a&b=c", "--status", "x y"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	// Standard query parsing must round-trip the values intact. If the
	// raw `&` from "a&b=c" leaked into the URL it would become a second
	// parameter named "b" with value "c" and `role` would be just "a".
	parsed, err := url.ParseQuery(captured.Query)
	if err != nil {
		t.Fatalf("captured query is not a valid query string: %v (raw=%q)", err, captured.Query)
	}
	if got := parsed.Get("role"); got != "a&b=c" {
		t.Errorf("role round-trip: got %q, want %q (raw query=%q)", got, "a&b=c", captured.Query)
	}
	if got := parsed.Get("status"); got != "x y" {
		t.Errorf("status round-trip: got %q, want %q (raw query=%q)", got, "x y", captured.Query)
	}
}

// --- Doc Tests ---

// TestDocCreate_FlagsAfterPositionals pins the cubic-dev-ai fix to
// `docCreate`'s argument parsing. The documented usage is
// `kandev doc create <task-id> <key> --type ... --title ... --content ...`,
// but Go's `flag.FlagSet.Parse` stops at the first non-flag argument —
// so the original implementation that called `fs.Parse(args)` left every
// flag at its default value. The fix peels the two required positionals
// off first and then parses `args[2:]`.
//
// A regression to the old behaviour would land defaults in the PATCH
// body (e.g. `type` = "custom" instead of "plan"); this test would catch
// it.
func TestDocCreate_FlagsAfterPositionals(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"ok":true}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"doc", "create", "task-abc", "plan",
		"--type", "plan", "--title", "My plan", "--content", "body",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "PUT" {
		t.Errorf("expected PUT, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/tasks/task-abc/documents/plan" {
		t.Errorf("unexpected path: %s", captured.Path)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["type"] != "plan" {
		t.Errorf("expected type=plan (flag parsed correctly), got %v — regression to flag.Parse stopping at the first positional", body["type"])
	}
	if body["title"] != "My plan" {
		t.Errorf("expected title='My plan', got %v", body["title"])
	}
	if body["content"] != "body" {
		t.Errorf("expected content='body', got %v", body["content"])
	}
}

// --- Memory Tests ---

func TestMemorySet_UpsertsEntry(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"ok":true}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"memory", "set", "--layer", "facts", "--key", "test-key", "--content", "test-value",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "PUT" {
		t.Errorf("expected PUT, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/agents/agent-789/memory" {
		t.Errorf("unexpected path: %s", captured.Path)
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	entries, ok := body["entries"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", body["entries"])
	}
	entry := entries[0].(map[string]any)
	if entry["layer"] != "facts" || entry["key"] != "test-key" || entry["content"] != "test-value" {
		t.Errorf("unexpected entry: %v", entry)
	}
}

func TestMemoryGet_QueriesByLayer(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"memory":[]}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"memory", "get", "--layer", "facts"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/agents/agent-789/memory" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
	if !strings.Contains(captured.Query, "layer=facts") {
		t.Errorf("expected layer=facts in query, got %s", captured.Query)
	}
}

func TestMemorySummary_CallsCorrectEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"count":5}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"memory", "summary"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/agents/agent-789/memory/summary" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
}

// --- Checkout Tests ---

func TestCheckout_CallsEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"ok":true}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"checkout"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "POST" {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc/checkout" {
		t.Errorf("unexpected path: %s", captured.Path)
	}
}

func TestCheckout_Handles409Conflict(t *testing.T) {
	srv, _ := setupMockServer(t, 409, `{"error":"already checked out by agent-other"}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"checkout"})
	if code != 1 {
		t.Fatalf("expected exit 1 on conflict, got %d", code)
	}
}

// --- Error Tests ---

func TestMissingEnv_ReturnsClearError(t *testing.T) {
	// Unset required env vars.
	t.Setenv("KANDEV_API_URL", "")
	t.Setenv("KANDEV_API_KEY", "")

	code := runKandevCLI([]string{"task", "get", "--id", "some-task"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestDefaultTaskID_UsesEnvVar(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"task":{"id":"task-abc"}}`)
	setEnvVars(t, srv)
	// Do not pass --id, should use KANDEV_TASK_ID.
	code := runKandevCLI([]string{"task", "get"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc" {
		t.Errorf("expected task-abc in path, got %s", captured.Path)
	}
}

func TestUnknownCommand_ReturnsError(t *testing.T) {
	code := runKandevCLI([]string{"invalid"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestNoArgs_ReturnsError(t *testing.T) {
	code := runKandevCLI(nil)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestNon2xxResponse_ReturnsError(t *testing.T) {
	srv, _ := setupMockServer(t, 500, `{"error":"internal server error"}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"task", "get"})
	if code != 1 {
		t.Fatalf("expected exit 1 on 500, got %d", code)
	}
}

func TestRunIDHeader_NotSetOnGet(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"task":{}}`)
	setEnvVars(t, srv)

	runKandevCLI([]string{"task", "get"})
	if captured.Header.Get("X-Kandev-Run-Id") != "" {
		t.Error("X-Kandev-Run-Id should not be set on GET requests")
	}
}

func TestMissingAgentID_ForMemory(t *testing.T) {
	srv, _ := setupMockServer(t, 200, `{}`)
	setEnvVars(t, srv)
	t.Setenv("KANDEV_AGENT_ID", "")

	code := runKandevCLI([]string{"memory", "get"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

func TestMissingWorkspaceID_ForAgents(t *testing.T) {
	srv, _ := setupMockServer(t, 200, `{}`)
	setEnvVars(t, srv)
	t.Setenv("KANDEV_WORKSPACE_ID", "")

	code := runKandevCLI([]string{"agents", "list"})
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
}

// --- Helpers ---

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupMockTransport(t *testing.T, status int, respBody string) *capturedRequest {
	t.Helper()
	captured := &capturedRequest{}
	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		captured.Method = req.Method
		captured.Path = req.URL.Path
		captured.Query = req.URL.RawQuery
		captured.Header = req.Header.Clone()
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			captured.Body = string(body)
		}
		return &http.Response{
			StatusCode: status,
			Body:       io.NopCloser(strings.NewReader(respBody)),
			Header:     make(http.Header),
		}, nil
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	return captured
}

func assertAuthHeader(t *testing.T, captured *capturedRequest) {
	t.Helper()
	auth := captured.Header.Get("Authorization")
	if auth != "Bearer test-key-123" {
		t.Errorf("expected Bearer test-key-123, got %s", auth)
	}
}

func assertRunIDHeader(t *testing.T, captured *capturedRequest, expected string) {
	t.Helper()
	runID := captured.Header.Get("X-Kandev-Run-Id")
	if runID != expected {
		t.Errorf("expected X-Kandev-Run-Id=%s, got %s", expected, runID)
	}
}
