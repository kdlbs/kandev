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

func TestTaskUpdate_SendsPatchWithRunIDHeader(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{"ok":true}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"task", "update", "--status", "done", "--comment", "finished",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "PATCH" {
		t.Errorf("expected PATCH, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/office/tasks/task-abc" {
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

// TestTaskUpdate_EmptyPayload_ReturnsError pins the guard added in
// `taskUpdate` after the cubic-dev-ai review: invoking `task update`
// with neither `--status` nor `--comment` would otherwise send an
// empty PATCH, which is never the intent. The fix returns exit 1
// before contacting the server; the assertion below would have caught
// a regression to the old "send empty body" behaviour.
func TestTaskUpdate_EmptyPayload_ReturnsError(t *testing.T) {
	srv, captured := setupMockServer(t, 200, `{}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{"task", "update"})
	if code == 0 {
		t.Fatal("expected non-zero exit when no update fields provided, got 0")
	}
	if captured.Method != "" {
		t.Errorf("server must not be contacted on empty payload; got %s %s", captured.Method, captured.Path)
	}
}

func TestTaskCreate_PostsToTasksEndpoint(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"task":{"id":"new-1"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"task", "create", "--title", "New task", "--parent", "parent-1", "--assignee", "agent-2",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if captured.Method != "POST" {
		t.Errorf("expected POST, got %s", captured.Method)
	}
	if captured.Path != "/api/v1/tasks" {
		t.Errorf("unexpected path: %s", captured.Path)
	}

	var body map[string]string
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body["title"] != "New task" {
		t.Errorf("expected title='New task', got %s", body["title"])
	}
	if body["parent_id"] != "parent-1" {
		t.Errorf("expected parent_id='parent-1', got %s", body["parent_id"])
	}
}

func TestTaskCreate_BlockedBy(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"task":{"id":"new-2"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"task", "create",
		"--title", "Staged task",
		"--blocked-by", "task-1,task-2",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	blockedBy, ok := body["blocked_by"].([]interface{})
	if !ok {
		t.Fatalf("expected blocked_by to be []interface{}, got %T: %v", body["blocked_by"], body["blocked_by"])
	}
	if len(blockedBy) != 2 {
		t.Fatalf("expected 2 blocked_by entries, got %d", len(blockedBy))
	}
	if blockedBy[0] != "task-1" || blockedBy[1] != "task-2" {
		t.Errorf("unexpected blocked_by values: %v", blockedBy)
	}
}

func TestTaskCreate_BlockedByTrimsSpaces(t *testing.T) {
	srv, captured := setupMockServer(t, 201, `{"task":{"id":"new-3"}}`)
	setEnvVars(t, srv)

	code := runKandevCLI([]string{
		"task", "create",
		"--title", "Space task",
		"--blocked-by", "task-a, task-b , task-c",
	})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal([]byte(captured.Body), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}

	blockedBy, ok := body["blocked_by"].([]interface{})
	if !ok {
		t.Fatalf("expected blocked_by to be []interface{}, got %T", body["blocked_by"])
	}
	if len(blockedBy) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(blockedBy))
	}
	if blockedBy[0] != "task-a" || blockedBy[1] != "task-b" || blockedBy[2] != "task-c" {
		t.Errorf("unexpected blocked_by values: %v", blockedBy)
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
