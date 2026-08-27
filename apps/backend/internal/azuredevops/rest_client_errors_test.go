package azuredevops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestRESTClientSurfacesUpstreamEndpointForEveryRead pins the exact Azure REST
// path each client method targets. The failing server records the path it was
// asked for, and the returned APIError must carry that same path back.
func TestRESTClientSurfacesUpstreamEndpointForEveryRead(t *testing.T) {
	const failureBody = `{"message":"upstream is down"}`
	var requested string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = r.URL.Path
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(failureBody))
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	ctx := context.Background()
	column := "column-active"
	assign := assignCurrentUserAction
	tests := []struct {
		name     string
		endpoint string
		call     func() error
	}{
		{"TestAuth", "/_apis/connectionData", func() error {
			_, err := client.TestAuth(ctx)
			return err
		}},
		{"ListProjects", "/_apis/projects", func() error {
			_, err := client.ListProjects(ctx)
			return err
		}},
		{"ListRepositories", "/project-1/_apis/git/repositories", func() error {
			_, err := client.ListRepositories(ctx, "project-1")
			return err
		}},
		{"ListBranches", "/project-1/_apis/git/repositories/repo-1/refs", func() error {
			_, err := client.ListBranches(ctx, "project-1", "repo-1")
			return err
		}},
		{"QueryWIQL", "/project-1/_apis/wit/wiql", func() error {
			_, err := client.QueryWIQL(ctx, "project-1", "SELECT [System.Id] FROM WorkItems", 0)
			return err
		}},
		{"GetWorkItem", "/project-1/_apis/wit/workitems/101", func() error {
			_, err := client.GetWorkItem(ctx, "project-1", 101)
			return err
		}},
		{"GetWorkItemDetail", "/project-1/_apis/wit/workitems/101", func() error {
			_, err := client.GetWorkItemDetail(ctx, "project-1", 101)
			return err
		}},
		{"ListWorkItemComments", "/project-1/_apis/wit/workItems/101/comments", func() error {
			_, err := client.ListWorkItemComments(ctx, "project-1", 101, "")
			return err
		}},
		{"GetCurrentIdentity", "/_apis/connectionData", func() error {
			_, err := client.GetCurrentIdentity(ctx)
			return err
		}},
		{"ListTeams", "/_apis/projects/project-1/teams", func() error {
			_, err := client.ListTeams(ctx, "project-1")
			return err
		}},
		{"ListBoards", "/project-1/team-1/_apis/work/backlogs", func() error {
			_, err := client.ListBoards(ctx, "project-1", "team-1")
			return err
		}},
		{"GetBoardSnapshot", "/project-1/team-1/_apis/work/boards/board-1", func() error {
			_, err := client.GetBoardSnapshot(ctx, "project-1", "team-1", "board-1")
			return err
		}},
		{"UpdateBoardWorkItem", "/project-1/team-1/_apis/work/boards/board-1", func() error {
			_, err := client.UpdateBoardWorkItem(ctx, "project-1", "team-1", "board-1", 101,
				BoardWorkItemUpdateRequest{Revision: 7, ColumnID: &column})
			return err
		}},
		{"UpdateWorkItem", "/project-1/_apis/wit/workitems/101", func() error {
			_, err := client.UpdateWorkItem(ctx, "project-1", 101, WorkItemAssignmentRequest{
				Revision: 7, AssigneeAction: &assign,
				resolvedAssignee: "ada@example.com", hasResolvedAssignee: true,
			})
			return err
		}},
		{"ListPullRequests", "/project-1/_apis/git/repositories/repo-1/pullrequests", func() error {
			_, err := client.ListPullRequests(ctx, PullRequestFilter{ProjectID: "project-1", RepositoryID: "repo-1"})
			return err
		}},
		{"ListPullRequestsForProject", "/project-1/_apis/git/pullrequests", func() error {
			_, err := client.ListPullRequests(ctx, PullRequestFilter{ProjectID: "project-1"})
			return err
		}},
		{"GetPullRequest", "/project-1/_apis/git/repositories/repo-1/pullrequests/42", func() error {
			_, err := client.GetPullRequest(ctx, "project-1", "repo-1", 42)
			return err
		}},
		{"ListReviewers", "/project-1/_apis/git/repositories/repo-1/pullrequests/42/reviewers", func() error {
			_, err := client.ListReviewers(ctx, "project-1", "repo-1", 42)
			return err
		}},
		{"ListThreads", "/project-1/_apis/git/repositories/repo-1/pullrequests/42/threads", func() error {
			_, err := client.ListThreads(ctx, "project-1", "repo-1", 42)
			return err
		}},
		{"ListLinkedWorkItems", "/project-1/_apis/git/repositories/repo-1/pullrequests/42/workitems", func() error {
			_, err := client.ListLinkedWorkItems(ctx, "project-1", "repo-1", 42)
			return err
		}},
		{"ListPolicyEvaluations", "/project-1/_apis/policy/evaluations", func() error {
			_, err := client.ListPolicyEvaluations(ctx, "project-1", 42)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requested = ""
			err := tt.call()
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want 500", apiErr.StatusCode)
			}
			if apiErr.Endpoint != tt.endpoint {
				t.Errorf("APIError.Endpoint = %q, want %q", apiErr.Endpoint, tt.endpoint)
			}
			if requested != "/acme"+tt.endpoint {
				t.Errorf("requested path = %q, want %q", requested, "/acme"+tt.endpoint)
			}
			if apiErr.Body != failureBody {
				t.Errorf("APIError.Body = %q, want %q", apiErr.Body, failureBody)
			}
		})
	}
}

func TestAPIErrorMessageIncludesEndpointStatusAndBody(t *testing.T) {
	err := &APIError{StatusCode: http.StatusForbidden, Endpoint: "/_apis/projects", Body: "no access"}
	want := "azure devops API /_apis/projects returned 403: no access"
	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	var target *APIError
	if !AsAPIError(fmt.Errorf("wrapped: %w", err), &target) || target != err {
		t.Fatalf("AsAPIError did not unwrap the API error")
	}
}

// TestRESTClientPropagatesWorkItemHydrationFailure covers the second upstream
// hop: the ID query succeeds and the batch hydration is what fails.
func TestRESTClientPropagatesWorkItemHydrationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/acme/project-1/_apis/wit/wiql":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"workItems":[{"id":1}]}`))
		case "/acme/project-1/team-1/_apis/work/boards/board-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"board-1","fields":{"columnField":{"referenceName":"System.BoardColumn"}}}`))
		case "/acme/project-1/team-1/_apis/work/backlogs/board-1/workItems":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"workItems":[{"target":{"id":1}}]}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("batch unavailable"))
		}
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	for _, tt := range []struct {
		name string
		call func() error
	}{
		{"QueryWIQL", func() error {
			_, err := client.QueryWIQL(context.Background(), "project-1", "SELECT", 10)
			return err
		}},
		{"GetBoardSnapshot", func() error {
			_, err := client.GetBoardSnapshot(context.Background(), "project-1", "team-1", "board-1")
			return err
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var apiErr *APIError
			if err := tt.call(); !errors.As(err, &apiErr) {
				t.Fatalf("error = %v, want *APIError", err)
			}
			if apiErr.Endpoint != "/project-1/_apis/wit/workitemsbatch" || apiErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("hydration error = %+v", apiErr)
			}
		})
	}
}

// TestRESTClientBoardSnapshotPropagatesBacklogItemFailure covers the board
// read succeeding while the backlog work-item reference read fails.
func TestRESTClientBoardSnapshotPropagatesBacklogItemFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/acme/project-1/team-1/_apis/work/boards/board-1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"board-1","name":"Stories"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no backlog"))
	}))
	t.Cleanup(server.Close)

	_, err := newTestRESTClient(t, server, "pat").GetBoardSnapshot(context.Background(), "project-1", "team-1", "board-1")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.Endpoint != "/project-1/team-1/_apis/work/backlogs/board-1/workItems" {
		t.Fatalf("error = %v (%+v), want backlog work-item endpoint", err, apiErr)
	}
}

func TestRESTClientWrapsTransportFailure(t *testing.T) {
	transportErr := errors.New("dial tcp: connection refused")
	client := NewRESTClient("https://dev.azure.com/acme", "pat", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, transportErr }),
	})

	_, err := client.ListProjects(context.Background())
	if err == nil || !strings.Contains(err.Error(), "azure devops request /_apis/projects") {
		t.Fatalf("error = %v, want endpoint-tagged transport failure", err)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatalf("transport failure reported as APIError: %+v", apiErr)
	}
}

type failingBody struct{ err error }

func (b failingBody) Read([]byte) (int, error) { return 0, b.err }
func (b failingBody) Close() error             { return nil }

func TestRESTClientWrapsResponseBodyReadFailure(t *testing.T) {
	client := NewRESTClient("https://dev.azure.com/acme", "pat", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       failingBody{err: errors.New("connection reset")},
			}, nil
		}),
	})

	_, err := client.ListProjects(context.Background())
	if err == nil || !strings.Contains(err.Error(), "read azure devops response") ||
		!strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("error = %v, want wrapped body read failure", err)
	}
}

func TestRESTClientAcceptsEmptySuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	projects, err := newTestRESTClient(t, server, "pat").ListProjects(context.Background())
	if err != nil || len(projects) != 0 {
		t.Fatalf("ListProjects = %+v, %v; want empty result and no error", projects, err)
	}
}

func TestRESTClientRejectsUndecodableSuccessBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"value": not-json}`))
	}))
	t.Cleanup(server.Close)

	_, err := newTestRESTClient(t, server, "pat").ListProjects(context.Background())
	if err == nil || !strings.Contains(err.Error(), "decode azure devops response") {
		t.Fatalf("error = %v, want decode failure", err)
	}
}

// TestRESTClientNormalizesBranchFiltersIntoRefNames pins the search criteria
// actually sent upstream for branch-scoped pull-request queries.
func TestRESTClientNormalizesBranchFiltersIntoRefNames(t *testing.T) {
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"value":[]}`))
	}))
	t.Cleanup(server.Close)

	page, err := newTestRESTClient(t, server, "pat").ListPullRequests(context.Background(), PullRequestFilter{
		ProjectID: "project-1", RepositoryID: "repo-1",
		SourceBranch: "feature/azure", TargetBranch: "refs/heads/main",
		Skip: 25, Top: 500,
	})
	if err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	if got := query.Get("searchCriteria.sourceRefName"); got != "refs/heads/feature/azure" {
		t.Errorf("sourceRefName = %q, want refs/heads/feature/azure", got)
	}
	if got := query.Get("searchCriteria.targetRefName"); got != "refs/heads/main" {
		t.Errorf("targetRefName = %q, want the already-qualified ref unchanged", got)
	}
	if query.Get("$skip") != "25" || query.Get("$top") != "50" {
		t.Errorf("paging = skip %q top %q, want 25 and the clamped default 50", query.Get("$skip"), query.Get("$top"))
	}
	if page.Skip != 25 || page.Top != defaultPRPageSize || page.Count != 0 || len(page.Items) != 0 {
		t.Errorf("page = %+v", page)
	}
}

// TestRESTClientOmitsUnsetPullRequestSearchCriteria proves empty filters are
// left off the query string rather than sent as empty values.
func TestRESTClientOmitsUnsetPullRequestSearchCriteria(t *testing.T) {
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"count":0,"value":[]}`))
	}))
	t.Cleanup(server.Close)

	if _, err := newTestRESTClient(t, server, "pat").ListPullRequests(context.Background(),
		PullRequestFilter{ProjectID: "project-1", RepositoryID: "repo-1", Top: 10}); err != nil {
		t.Fatalf("ListPullRequests: %v", err)
	}
	for _, key := range []string{
		"searchCriteria.status", "searchCriteria.creatorId", "searchCriteria.reviewerId",
		"searchCriteria.sourceRefName", "searchCriteria.targetRefName", "$skip",
	} {
		if _, present := query[key]; present {
			t.Errorf("query carries unset filter %q = %q", key, query.Get(key))
		}
	}
	if query.Get("$top") != "10" {
		t.Errorf("$top = %q, want the requested 10", query.Get("$top"))
	}
}

func TestPlanningFieldsSkipsAbsentAndBlankValues(t *testing.T) {
	fields := map[string]any{
		"Microsoft.VSTS.Scheduling.Effort":           3,
		"Microsoft.VSTS.Scheduling.StoryPoints":      "   ",
		"Microsoft.VSTS.Scheduling.Size":             nil,
		"Microsoft.VSTS.Scheduling.RemainingWork":    "2.5",
		"Microsoft.VSTS.Scheduling.OriginalEstimate": 0,
		"System.Title": "not a planning field",
	}
	got := planningFields(fields)
	want := []PlanningField{
		{ReferenceName: "Microsoft.VSTS.Scheduling.Effort", Label: "Effort", Value: "3"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.RemainingWork", Label: "Remaining work", Value: "2.5"},
		{ReferenceName: "Microsoft.VSTS.Scheduling.OriginalEstimate", Label: "Original estimate", Value: "0"},
	}
	if len(got) != len(want) {
		t.Fatalf("planningFields = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("planningFields[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBoardItemPlacementReadsStringAndBooleanDoneFields(t *testing.T) {
	board := Board{
		Columns: []BoardColumn{{ID: "column-done", Name: "Done"}},
		Fields: BoardFields{
			ColumnField: FieldReference{ReferenceName: "System.BoardColumn"},
			DoneField:   FieldReference{ReferenceName: "System.BoardColumnDone"},
		},
	}
	tests := []struct {
		name     string
		fields   map[string]any
		columnID string
		done     bool
	}{
		{"string true", map[string]any{"System.BoardColumn": "Done", "System.BoardColumnDone": "True"}, "column-done", true},
		{"string false", map[string]any{"System.BoardColumn": "Done", "System.BoardColumnDone": "no"}, "column-done", false},
		{"boolean true", map[string]any{"System.BoardColumn": "Done", "System.BoardColumnDone": true}, "column-done", true},
		{"unknown column", map[string]any{"System.BoardColumn": "Triage"}, "", false},
		{"unsupported done type", map[string]any{"System.BoardColumnDone": 1}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columnID, done := boardItemPlacement(board, WorkItem{Fields: tt.fields})
			if columnID != tt.columnID || done != tt.done {
				t.Fatalf("boardItemPlacement = (%q, %t), want (%q, %t)", columnID, done, tt.columnID, tt.done)
			}
		})
	}
}

func TestWorkItemAssignmentPatchRemovesAssigneeAndRejectsUnresolvedRequests(t *testing.T) {
	unassign := unassignAction
	patch, err := workItemAssignmentPatch(WorkItemAssignmentRequest{
		Revision: 4, AssigneeAction: &unassign, hasResolvedAssignee: true,
	})
	if err != nil {
		t.Fatalf("workItemAssignmentPatch: %v", err)
	}
	if len(patch) != 2 || patch[0]["op"] != "test" || patch[0]["value"] != 4 ||
		patch[1]["op"] != "remove" || patch[1]["path"] != "/fields/System.AssignedTo" {
		t.Fatalf("unassign patch = %#v", patch)
	}

	assign := assignCurrentUserAction
	if _, err := workItemAssignmentPatch(WorkItemAssignmentRequest{
		Revision: 4, AssigneeAction: &assign,
	}); err == nil || !strings.Contains(err.Error(), "resolved server-side") {
		t.Fatalf("unresolved assignee error = %v", err)
	}
	if _, err := workItemAssignmentPatch(WorkItemAssignmentRequest{Revision: 0}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid revision error = %v, want ErrInvalidConfig", err)
	}
}

type testBoard = struct {
	Columns []BoardColumn `json:"columns"`
	Fields  BoardFields   `json:"fields"`
}

func TestBoardWorkItemPatchRejectsUnusableBoardMetadata(t *testing.T) {
	column := "column-active"
	done := true
	assign := assignCurrentUserAction
	withColumn := testBoard{Columns: []BoardColumn{{ID: column, Name: "Active"}}}
	tests := []struct {
		name    string
		board   testBoard
		request BoardWorkItemUpdateRequest
		wantErr string
	}{
		{
			name:    "unknown column",
			board:   testBoard{Columns: []BoardColumn{{ID: "column-todo", Name: "To Do"}}},
			request: BoardWorkItemUpdateRequest{Revision: 7, ColumnID: &column},
			wantErr: "unknown board column",
		},
		{
			name:    "board has no column field",
			board:   withColumn,
			request: BoardWorkItemUpdateRequest{Revision: 7, ColumnID: &column},
			wantErr: "board column field is unavailable",
		},
		{
			name:    "board has no done field",
			board:   withColumn,
			request: BoardWorkItemUpdateRequest{Revision: 7, ColumnDone: &done},
			wantErr: "board done field is unavailable",
		},
		{
			name:    "unresolved assignee action",
			board:   withColumn,
			request: BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &assign},
			wantErr: "resolved server-side",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := boardWorkItemPatch(tt.board, tt.request); err == nil ||
				!strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("boardWorkItemPatch error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestBoardWorkItemPatchBuildsColumnAndDoneOperations(t *testing.T) {
	column := "column-active"
	done := false
	board := testBoard{
		Columns: []BoardColumn{{ID: column, Name: "Active"}},
		Fields: BoardFields{
			ColumnField: FieldReference{ReferenceName: "System.BoardColumn"},
			DoneField:   FieldReference{ReferenceName: "System.BoardColumnDone"},
		},
	}
	patch, err := boardWorkItemPatch(board, BoardWorkItemUpdateRequest{Revision: 9, ColumnID: &column, ColumnDone: &done})
	if err != nil {
		t.Fatalf("boardWorkItemPatch: %v", err)
	}
	if len(patch) != 3 ||
		patch[1]["op"] != "replace" || patch[1]["path"] != "/fields/System.BoardColumn" || patch[1]["value"] != "Active" ||
		patch[2]["path"] != "/fields/System.BoardColumnDone" || patch[2]["value"] != false {
		t.Fatalf("patch = %#v", patch)
	}
}

// TestRESTClientUpdateBoardWorkItemFallsBackToRequestedPlacement covers the
// branch where Azure echoes no board fields back and the client keeps the
// caller's requested column and done flag.
func TestRESTClientUpdateBoardWorkItemFallsBackToRequestedPlacement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/project-1/team-1/_apis/work/boards/board-1":
			_, _ = w.Write([]byte(`{"fields":{"columnField":{"referenceName":"System.BoardColumn"},"doneField":{"referenceName":"System.BoardColumnDone"}},"columns":[{"id":"column-active","name":"Active"}]}`))
		case "/acme/project-1/_apis/wit/workitems/101":
			_, _ = w.Write([]byte(`{"id":101,"rev":9,"fields":{"System.Title":"Moved"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	column := "column-active"
	done := true
	item, err := newTestRESTClient(t, server, "pat").UpdateBoardWorkItem(
		context.Background(), "project-1", "team-1", "board-1", 101,
		BoardWorkItemUpdateRequest{Revision: 8, ColumnID: &column, ColumnDone: &done},
	)
	if err != nil {
		t.Fatalf("UpdateBoardWorkItem: %v", err)
	}
	if item.ID != 101 || item.Revision != 9 || item.Title != "Moved" ||
		item.ColumnID != column || !item.ColumnDone {
		t.Fatalf("item = %+v, want requested placement retained", item)
	}
}

func TestRESTClientRejectsUpdateBoardWorkItemWithoutRevision(t *testing.T) {
	called := false
	client := NewRESTClient("https://dev.azure.com/acme", "pat", &http.Client{
		Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			called = true
			return nil, errors.New("must not be reached")
		}),
	})
	_, err := client.UpdateBoardWorkItem(context.Background(), "project-1", "team-1", "board-1", 101,
		BoardWorkItemUpdateRequest{})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("error = %v, want ErrInvalidConfig", err)
	}
	if called {
		t.Fatal("missing revision reached the HTTP transport")
	}
}

func TestRESTClientListLinkedWorkItemsSkipsNonNumericIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"value":[{"id":"101","url":"https://api/101"},{"id":"vstfs:///artifact","url":"https://api/x"},{"id":"7","url":"https://api/7"}]}`))
	}))
	t.Cleanup(server.Close)

	refs, err := newTestRESTClient(t, server, "pat").ListLinkedWorkItems(context.Background(), "project-1", "repo-1", 42)
	if err != nil {
		t.Fatalf("ListLinkedWorkItems: %v", err)
	}
	want := []WorkItemRef{{ID: 101, URL: "https://api/101"}, {ID: 7, URL: "https://api/7"}}
	if len(refs) != len(want) {
		t.Fatalf("refs = %+v, want %+v", refs, want)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs[%d] = %+v, want %+v", i, refs[i], want[i])
		}
	}
}
