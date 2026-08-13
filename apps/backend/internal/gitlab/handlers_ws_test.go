package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// --- Shared WS handler fixture ---
//
// The WS surface in handlers.go is the legacy, workspace-unscoped GitLab
// action set: every handler closes over the package Service and translates a
// ws.Message into one service call. These helpers drive a handler directly
// and decode the envelope it returns, so the tests can assert the wire
// contract (error code, reason string, payload keys) rather than merely
// executing the closure.

func newWSFixture(t *testing.T, client Client) (*Service, *Store, *logger.Logger) {
	t.Helper()
	log := newTestLogger(t)
	store := newTestStore(t)
	svc := NewService(DefaultHost, client, AuthMethodPAT, nil, log)
	svc.SetStore(store)
	return svc, store, log
}

// wsRequest builds a request envelope with payload marshalled as JSON.
func wsRequest(t *testing.T, action string, payload interface{}) *ws.Message {
	t.Helper()
	msg, err := ws.NewRequest("req-42", action, payload)
	if err != nil {
		t.Fatalf("build %s request: %v", action, err)
	}
	return msg
}

// wsRawRequest builds a request envelope carrying a verbatim payload, used to
// exercise the malformed-payload branches that a marshalled Go value cannot
// produce.
func wsRawRequest(action, rawPayload string) *ws.Message {
	return &ws.Message{
		ID:      "req-42",
		Type:    ws.MessageTypeRequest,
		Action:  action,
		Payload: json.RawMessage(rawPayload),
	}
}

// wsOK asserts a success envelope that echoes the request's correlation ID and
// action, then decodes the payload into out.
func wsOK(t *testing.T, resp *ws.Message, err error, action string, out interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned a transport error: %v", err)
	}
	if resp == nil {
		t.Fatal("handler returned a nil message")
	}
	if resp.Type != ws.MessageTypeResponse {
		t.Fatalf("type = %q, want %q (payload: %s)", resp.Type, ws.MessageTypeResponse, resp.Payload)
	}
	if resp.ID != "req-42" {
		t.Errorf("id = %q, want the request id echoed back", resp.ID)
	}
	if resp.Action != action {
		t.Errorf("action = %q, want %q", resp.Action, action)
	}
	if out == nil {
		return
	}
	if decodeErr := json.Unmarshal(resp.Payload, out); decodeErr != nil {
		t.Fatalf("decode payload %s: %v", resp.Payload, decodeErr)
	}
}

// wsErr asserts an error envelope with the expected code and reason and
// returns the decoded error payload.
func wsErr(t *testing.T, resp *ws.Message, err error, action, code, reason string) ws.ErrorPayload {
	t.Helper()
	if err != nil {
		t.Fatalf("handler returned a transport error: %v", err)
	}
	if resp == nil {
		t.Fatal("handler returned a nil message")
	}
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("type = %q, want %q (payload: %s)", resp.Type, ws.MessageTypeError, resp.Payload)
	}
	if resp.ID != "req-42" {
		t.Errorf("id = %q, want the request id echoed back", resp.ID)
	}
	if resp.Action != action {
		t.Errorf("action = %q, want %q", resp.Action, action)
	}
	var payload ws.ErrorPayload
	if decodeErr := json.Unmarshal(resp.Payload, &payload); decodeErr != nil {
		t.Fatalf("decode error payload %s: %v", resp.Payload, decodeErr)
	}
	if payload.Code != code {
		t.Errorf("error code = %q, want %q", payload.Code, code)
	}
	if reason != "" && payload.Message != reason {
		t.Errorf("error message = %q, want %q", payload.Message, reason)
	}
	return payload
}

// failingSecretProvider makes Service.GetStatus fail at its findTokenSecret
// step, which is the only internal-error path wsStatus has.
type failingSecretProvider struct{ err error }

func (f *failingSecretProvider) List(context.Context) ([]*SecretListItem, error) {
	return nil, f.err
}

func (f *failingSecretProvider) Reveal(context.Context, string) (string, error) {
	return "", f.err
}

// --- Status ---

func TestWSStatusReturnsServiceStatus(t *testing.T) {
	client := NewMockClient(DefaultHost)
	client.SetUser("alice")
	svc, _, log := newWSFixture(t, client)

	resp, err := wsStatus(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabStatus, nil))

	var status Status
	wsOK(t, resp, err, ws.ActionGitLabStatus, &status)
	if !status.Authenticated {
		t.Error("authenticated = false, want true for the mock client")
	}
	if status.Username != "alice" {
		t.Errorf("username = %q, want alice", status.Username)
	}
	if status.AuthMethod != AuthMethodPAT {
		t.Errorf("auth_method = %q, want %q", status.AuthMethod, AuthMethodPAT)
	}
	if status.Host != DefaultHost {
		t.Errorf("host = %q, want %q", status.Host, DefaultHost)
	}
}

func TestWSStatusMapsSecretLookupFailureToInternalError(t *testing.T) {
	log := newTestLogger(t)
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT,
		&failingSecretProvider{err: errors.New("vault offline")}, log)

	resp, err := wsStatus(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabStatus, nil))

	payload := wsErr(t, resp, err, ws.ActionGitLabStatus, ws.ErrorCodeInternalError, "")
	if payload.Message == "" {
		t.Error("internal error message is empty; the cause must reach the client")
	}
}

// --- Task MR listing ---

func TestWSListTaskMRsByTaskReturnsOnlyThatTasksRows(t *testing.T) {
	svc, store, log := newWSFixture(t, NewMockClient(DefaultHost))
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-a", "ws-1")
	seedTask(t, store, "task-b", "ws-1")
	if err := store.UpsertTaskMR(ctx, newTestMR("task-a", "repo-1", "group/a", 11)); err != nil {
		t.Fatalf("seed task-a MR: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, newTestMR("task-b", "repo-1", "group/b", 22)); err != nil {
		t.Fatalf("seed task-b MR: %v", err)
	}

	resp, err := wsListTaskMRs(svc, log)(ctx,
		wsRequest(t, ws.ActionGitLabTaskMRsList, map[string]string{"task_id": "task-a"}))

	var body struct {
		TaskMRs []*TaskMR `json:"task_mrs"`
	}
	wsOK(t, resp, err, ws.ActionGitLabTaskMRsList, &body)
	if len(body.TaskMRs) != 1 {
		t.Fatalf("task_mrs = %d rows, want exactly task-a's single row", len(body.TaskMRs))
	}
	if body.TaskMRs[0].ProjectPath != "group/a" || body.TaskMRs[0].MRIID != 11 {
		t.Errorf("row = %s!%d, want group/a!11", body.TaskMRs[0].ProjectPath, body.TaskMRs[0].MRIID)
	}
}

func TestWSListTaskMRsByWorkspaceGroupsRowsByTask(t *testing.T) {
	svc, store, log := newWSFixture(t, NewMockClient(DefaultHost))
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-a", "ws-1")
	seedTask(t, store, "task-b", "ws-1")
	for _, mr := range []*TaskMR{
		newTestMR("task-a", "repo-1", "group/a", 11),
		newTestMR("task-b", "repo-1", "group/b", 22),
	} {
		if err := store.UpsertTaskMR(ctx, mr); err != nil {
			t.Fatalf("seed %s MR: %v", mr.TaskID, err)
		}
	}

	resp, err := wsListTaskMRs(svc, log)(ctx,
		wsRequest(t, ws.ActionGitLabTaskMRsList, map[string]string{"workspace_id": "ws-1"}))

	var body TaskMRsResponse
	wsOK(t, resp, err, ws.ActionGitLabTaskMRsList, &body)
	if len(body.TaskMRs) != 2 {
		t.Fatalf("grouped tasks = %d, want 2 (%v)", len(body.TaskMRs), body.TaskMRs)
	}
	if got := body.TaskMRs["task-a"]; len(got) != 1 || got[0].MRIID != 11 {
		t.Errorf("task-a group = %+v, want the single !11 row", got)
	}
}

// TestWSListTaskMRsRejectsMalformedPayload pins the comment in handlers.go: a
// payload that fails to parse must not fall through to the "list everything in
// the workspace" branch, which would return more rows than the caller asked
// for.
func TestWSListTaskMRsRejectsMalformedPayload(t *testing.T) {
	svc, store, log := newWSFixture(t, NewMockClient(DefaultHost))
	seedWorkspace(t, store, "ws-1")

	resp, err := wsListTaskMRs(svc, log)(context.Background(),
		wsRawRequest(ws.ActionGitLabTaskMRsList, `{"task_id":12345}`))

	wsErr(t, resp, err, ws.ActionGitLabTaskMRsList, ws.ErrorCodeBadRequest, errMsgInvalidPayload)
}

func TestWSListTaskMRsRequiresWorkspaceOrTask(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsListTaskMRs(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabTaskMRsList, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabTaskMRsList, ws.ErrorCodeBadRequest,
		"workspace_id or task_id required")
}

func TestWSGetTaskMRRequiresTaskID(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	for name, msg := range map[string]*ws.Message{
		"absent":    wsRequest(t, ws.ActionGitLabTaskMRGet, map[string]string{}),
		"malformed": wsRawRequest(ws.ActionGitLabTaskMRGet, `{"task_id":[]}`),
	} {
		t.Run(name, func(t *testing.T) {
			resp, err := wsGetTaskMR(svc, log)(context.Background(), msg)
			wsErr(t, resp, err, ws.ActionGitLabTaskMRGet, ws.ErrorCodeBadRequest, "task_id required")
		})
	}
}

func TestWSGetTaskMRReturnsRowsForTask(t *testing.T) {
	svc, store, log := newWSFixture(t, NewMockClient(DefaultHost))
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-a", "ws-1")
	if err := store.UpsertTaskMR(ctx, newTestMR("task-a", "repo-1", "group/a", 7)); err != nil {
		t.Fatalf("seed MR: %v", err)
	}

	resp, err := wsGetTaskMR(svc, log)(ctx,
		wsRequest(t, ws.ActionGitLabTaskMRGet, map[string]string{"task_id": "task-a"}))

	var body struct {
		TaskMRs []*TaskMR `json:"task_mrs"`
	}
	wsOK(t, resp, err, ws.ActionGitLabTaskMRGet, &body)
	if len(body.TaskMRs) != 1 || body.TaskMRs[0].MRIID != 7 {
		t.Fatalf("task_mrs = %+v, want the single !7 row", body.TaskMRs)
	}
}

// --- MR feedback / files / commits ---

func TestWSGetMRFeedbackValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))
	handler := wsGetMRFeedback(svc, log)

	cases := map[string]*ws.Message{
		"missing project": wsRequest(t, ws.ActionGitLabMRFeedbackGet, map[string]interface{}{"iid": 3}),
		"zero iid":        wsRequest(t, ws.ActionGitLabMRFeedbackGet, map[string]interface{}{"project": "g/p", "iid": 0}),
		"negative iid":    wsRequest(t, ws.ActionGitLabMRFeedbackGet, map[string]interface{}{"project": "g/p", "iid": -1}),
		"malformed":       wsRawRequest(ws.ActionGitLabMRFeedbackGet, `{"project":true}`),
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			resp, err := handler(context.Background(), msg)
			wsErr(t, resp, err, ws.ActionGitLabMRFeedbackGet, ws.ErrorCodeBadRequest, "project and iid required")
		})
	}
}

func TestWSGetMRFeedbackReturnsClientFeedback(t *testing.T) {
	client := NewMockClient(DefaultHost)
	client.SetUser("alice")
	iid := client.SeedMR("group/p", &MR{Title: "feat: x", State: "opened"})
	client.SeedApprovals("group/p", iid, []MRApproval{{Username: "bob"}}, 1)
	svc, _, log := newWSFixture(t, client)

	resp, err := wsGetMRFeedback(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRFeedbackGet, map[string]interface{}{"project": "group/p", "iid": iid}))

	var feedback MRFeedback
	wsOK(t, resp, err, ws.ActionGitLabMRFeedbackGet, &feedback)
	if len(feedback.Approvals) != 1 || feedback.Approvals[0].Username != "bob" {
		t.Errorf("approvals = %+v, want bob's single approval", feedback.Approvals)
	}
}

func TestWSGetMRFilesReturnsSeededFiles(t *testing.T) {
	client := NewMockClient(DefaultHost)
	iid := client.SeedMR("group/p", &MR{Title: "feat: x"})
	client.SeedFiles("group/p", iid, []MRFile{{Filename: "a.go", Additions: 3}})
	svc, _, log := newWSFixture(t, client)

	resp, err := wsGetMRFiles(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRFilesGet, map[string]interface{}{"project": "group/p", "iid": iid}))

	var body struct {
		Files []MRFile `json:"files"`
	}
	wsOK(t, resp, err, ws.ActionGitLabMRFilesGet, &body)
	if len(body.Files) != 1 || body.Files[0].Filename != "a.go" {
		t.Fatalf("files = %+v, want the single a.go entry", body.Files)
	}
}

func TestWSGetMRFilesValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsGetMRFiles(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRFilesGet, map[string]interface{}{"project": "", "iid": 1}))

	wsErr(t, resp, err, ws.ActionGitLabMRFilesGet, ws.ErrorCodeBadRequest, "project and iid required")
}

func TestWSGetMRCommitsReturnsSeededCommits(t *testing.T) {
	client := NewMockClient(DefaultHost)
	iid := client.SeedMR("group/p", &MR{Title: "feat: x"})
	client.SeedCommits("group/p", iid, []MRCommitInfo{{SHA: "abc123", Message: "feat: x"}})
	svc, _, log := newWSFixture(t, client)

	resp, err := wsGetMRCommits(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRCommitsGet, map[string]interface{}{"project": "group/p", "iid": iid}))

	var body struct {
		Commits []MRCommitInfo `json:"commits"`
	}
	wsOK(t, resp, err, ws.ActionGitLabMRCommitsGet, &body)
	if len(body.Commits) != 1 || body.Commits[0].SHA != "abc123" {
		t.Fatalf("commits = %+v, want the single abc123 entry", body.Commits)
	}
}

func TestWSGetMRCommitsValidatesProjectAndIID(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsGetMRCommits(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabMRCommitsGet, map[string]interface{}{"project": "g/p"}))

	wsErr(t, resp, err, ws.ActionGitLabMRCommitsGet, ws.ErrorCodeBadRequest, "project and iid required")
}

// --- Sync ---

func TestWSSyncTaskMRRequiresTaskID(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsSyncTaskMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabTaskMRSync, map[string]string{}))

	wsErr(t, resp, err, ws.ActionGitLabTaskMRSync, ws.ErrorCodeBadRequest, "task_id required")
}

func TestWSSyncTaskMRReportsMissingStoreAsInternalError(t *testing.T) {
	log := newTestLogger(t)
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), AuthMethodPAT, nil, log)

	resp, err := wsSyncTaskMR(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabTaskMRSync, map[string]string{"task_id": "task-a"}))

	wsErr(t, resp, err, ws.ActionGitLabTaskMRSync, ws.ErrorCodeInternalError,
		"gitlab store not configured")
}

func TestWSSyncTaskMRReturnsRefreshedRows(t *testing.T) {
	client := NewMockClient(DefaultHost)
	iid := client.SeedMR("group/p", &MR{Title: "feat: refreshed", State: "opened"})
	svc, store, log := newWSFixture(t, client)
	ctx := context.Background()
	seedWorkspace(t, store, "ws-1")
	seedTask(t, store, "task-a", "ws-1")
	row := newTestMR("task-a", "repo-1", "group/p", iid)
	row.MRTitle = "stale title"
	if err := store.UpsertTaskMR(ctx, row); err != nil {
		t.Fatalf("seed MR row: %v", err)
	}

	resp, err := wsSyncTaskMR(svc, log)(ctx,
		wsRequest(t, ws.ActionGitLabTaskMRSync, map[string]string{"task_id": "task-a"}))

	var body struct {
		TaskMRs []*TaskMR `json:"task_mrs"`
	}
	wsOK(t, resp, err, ws.ActionGitLabTaskMRSync, &body)
	if len(body.TaskMRs) != 1 {
		t.Fatalf("task_mrs = %d rows, want 1", len(body.TaskMRs))
	}
	if body.TaskMRs[0].MRTitle != "feat: refreshed" {
		t.Errorf("title = %q, want the upstream title to have replaced the stale one",
			body.TaskMRs[0].MRTitle)
	}
}

// --- Stats ---

// statsRecordingClient records the filters GetStats sends upstream so the test
// can assert the three distinct queries, not just the aggregate numbers.
type statsRecordingClient struct {
	*MockClient
	mrFilters []string
}

func (c *statsRecordingClient) SearchMRsPaged(ctx context.Context, filter, customQuery string, page, perPage int) (*MRSearchPage, error) {
	c.mrFilters = append(c.mrFilters, filter)
	return c.MockClient.SearchMRsPaged(ctx, filter, customQuery, page, perPage)
}

func TestWSGetStatsReturnsCountsAndQueriesEachScope(t *testing.T) {
	client := &statsRecordingClient{MockClient: NewMockClient(DefaultHost)}
	client.SetUser("alice")
	client.SeedMR("group/p", &MR{Title: "one", State: "opened"})
	client.SeedMR("group/q", &MR{Title: "two", State: "opened"})
	client.SeedIssue("group/p", &Issue{IID: 1, Title: "bug", State: "opened"})
	svc, _, log := newWSFixture(t, client)

	resp, err := wsGetStats(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabStats, nil))

	var stats Stats
	wsOK(t, resp, err, ws.ActionGitLabStats, &stats)
	if stats.OpenMRs != 2 {
		t.Errorf("open_mrs = %d, want 2", stats.OpenMRs)
	}
	if stats.MRsAwaitingMyReview != 2 {
		t.Errorf("mrs_awaiting_my_review = %d, want 2", stats.MRsAwaitingMyReview)
	}
	if stats.OpenIssuesAssignedMe != 1 {
		t.Errorf("open_issues_assigned_me = %d, want 1", stats.OpenIssuesAssignedMe)
	}
	want := []string{"scope=created_by_me&state=opened", "reviewer_username=alice&state=opened"}
	if len(client.mrFilters) != len(want) {
		t.Fatalf("MR search filters = %q, want %q", client.mrFilters, want)
	}
	for i, filter := range want {
		if client.mrFilters[i] != filter {
			t.Errorf("MR search filter[%d] = %q, want %q", i, client.mrFilters[i], filter)
		}
	}
}

// TestWSGetStatsDegradesToZeroWithoutAuthenticatedUser pins the documented
// behavior of Service.GetStats: an unresolvable user yields empty counts
// rather than an error envelope, so the dashboard still renders.
func TestWSGetStatsDegradesToZeroWithoutAuthenticatedUser(t *testing.T) {
	svc, _, log := newWSFixture(t, NewMockClient(DefaultHost))

	resp, err := wsGetStats(svc, log)(context.Background(),
		wsRequest(t, ws.ActionGitLabStats, nil))

	var stats Stats
	wsOK(t, resp, err, ws.ActionGitLabStats, &stats)
	if stats.OpenMRs != 0 || stats.OpenIssuesAssignedMe != 0 {
		t.Errorf("stats = %+v, want zeroed counts when no user resolves", stats)
	}
}
