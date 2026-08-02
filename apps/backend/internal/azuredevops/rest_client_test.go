package azuredevops

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRESTClientAuthAndDiscovery(t *testing.T) {
	const pat = "top-secret-pat"
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != wantAuth {
			t.Errorf("Authorization = %q, want Basic PAT", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/_apis/connectionData":
			_, _ = w.Write([]byte(`{"authenticatedUser":{"id":"user-1","providerDisplayName":"Ada","properties":{"Account":{"$value":"ada@example.com"}}}}`))
		case "/acme/_apis/projects":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"project-1","name":"Platform","url":"https://api/project-1"}]}`))
		case "/acme/project-1/_apis/git/repositories":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"repo-1","name":"widgets","defaultBranch":"refs/heads/main","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}]}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/refs":
			if got := r.URL.Query().Get("$top"); got != "1000" {
				t.Errorf("branch $top = %q, want 1000", got)
			}
			_, _ = w.Write([]byte(`{"count":1,"value":[{"name":"refs/heads/main"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, pat)
	auth, err := client.TestAuth(context.Background())
	if err != nil || !auth.OK || auth.ID != "user-1" || auth.Email != "ada@example.com" {
		t.Fatalf("TestAuth = %+v, %v", auth, err)
	}
	projects, err := client.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].Name != "Platform" {
		t.Fatalf("ListProjects = %+v, %v", projects, err)
	}
	repos, err := client.ListRepositories(context.Background(), "project-1")
	if err != nil || len(repos) != 1 || repos[0].DefaultBranch != "main" {
		t.Fatalf("ListRepositories = %+v, %v", repos, err)
	}
	branches, err := client.ListBranches(context.Background(), "project-1", "repo-1")
	if err != nil || len(branches) != 1 || branches[0].Name != "main" {
		t.Fatalf("ListBranches = %+v, %v", branches, err)
	}
}

func TestRESTClientBoardReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/_apis/projects/project-1/teams":
			_, _ = w.Write([]byte(`{"value":[{"id":"team-1","name":"Platform Team","projectId":"project-1","projectName":"Platform"}]}`))
		case "/acme/project-1/team-1/_apis/work/backlogs":
			_, _ = w.Write([]byte(`{"value":[{"id":"Microsoft.RequirementCategory","name":"Stories","isHidden":false},{"id":"Microsoft.TaskCategory","name":"Tasks","isHidden":true}]}`))
		case "/acme/project-1/team-1/_apis/work/boards/Microsoft.RequirementCategory":
			_, _ = w.Write([]byte(`{"id":"board-1","name":"Stories","fields":{"columnField":{"referenceName":"System.BoardColumn"},"doneField":{"referenceName":"System.BoardColumnDone"}},"columns":[{"id":"column-todo","name":"To Do","columnType":"incoming","itemLimit":10},{"id":"column-active","name":"Active","columnType":"inProgress"},{"id":"column-done","name":"Done","columnType":"outgoing"}]}`))
		case "/acme/project-1/team-1/_apis/work/backlogs/Microsoft.RequirementCategory/workItems":
			_, _ = w.Write([]byte(`{"workItems":[{"target":{"id":2}},{"target":{"id":1}},{"target":{"id":3}}]}`))
		case "/acme/project-1/_apis/wit/workitemsbatch":
			_, _ = w.Write([]byte(`{"value":[{"id":1,"rev":4,"fields":{"System.Title":"First","System.WorkItemType":"User Story","System.AssignedTo":"Ada","System.Tags":"one; two","System.BoardColumn":"To Do","System.BoardColumnDone":false}},{"id":2,"rev":5,"fields":{"System.Title":"Second","System.WorkItemType":"Bug","System.BoardColumn":"Done","System.BoardColumnDone":true}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	teams, err := client.ListTeams(context.Background(), "project-1")
	if err != nil || len(teams) != 1 || teams[0].Name != "Platform Team" || teams[0].ProjectID != "project-1" || teams[0].ProjectName != "Platform" {
		t.Fatalf("ListTeams = %+v, %v", teams, err)
	}
	boards, err := client.ListBoards(context.Background(), "project-1", "team-1")
	if err != nil || len(boards) != 1 || boards[0].ID != "Microsoft.RequirementCategory" {
		t.Fatalf("ListBoards = %+v, %v", boards, err)
	}
	snapshot, err := client.GetBoardSnapshot(context.Background(), "project-1", "team-1", boards[0].ID)
	if err != nil {
		t.Fatalf("GetBoardSnapshot: %v", err)
	}
	if len(snapshot.Board.Columns) != 3 || len(snapshot.Items) != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if snapshot.Items[0].ID != 2 || snapshot.Items[0].ColumnID != "column-done" || !snapshot.Items[0].ColumnDone {
		t.Fatalf("first board item = %+v", snapshot.Items[0])
	}
	if snapshot.Items[1].ID != 1 || snapshot.Items[1].ColumnID != "column-todo" || len(snapshot.Items[1].Tags) != 2 {
		t.Fatalf("second board item = %+v", snapshot.Items[1])
	}
}

func TestRESTClientGetWorkItemDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/project-1/_apis/wit/workitems/101" || r.URL.Query().Get("$expand") != "all" {
			t.Fatalf("detail request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":101,"rev":8,"url":"https://api/work-items/101","_links":{"html":{"href":"https://dev.azure.com/acme/project/_workitems/edit/101"}},"fields":{"System.Title":"Ship detail","System.WorkItemType":"User Story","System.State":"Active","System.AssignedTo":{"displayName":"Ada"},"System.Tags":"ui; azure","System.Description":"<p>Useful <strong>detail</strong></p><script>alert('xss')</script>","Microsoft.VSTS.Scheduling.Effort":5,"Microsoft.VSTS.Scheduling.StoryPoints":"3","Microsoft.VSTS.Scheduling.RemainingWork":2.5}}`))
	}))
	t.Cleanup(server.Close)

	detail, err := newTestRESTClient(t, server, "pat").GetWorkItemDetail(t.Context(), "project-1", 101)
	if err != nil {
		t.Fatalf("GetWorkItemDetail: %v", err)
	}
	if detail.ID != 101 || detail.Revision != 8 || detail.AssignedTo != "Ada" || detail.WebURL == "" {
		t.Fatalf("detail core fields = %+v", detail)
	}
	if strings.Contains(detail.Description, "script") || !strings.Contains(detail.Description, "Useful detail") {
		t.Fatalf("description = %q, want sanitized useful content", detail.Description)
	}
	if len(detail.PlanningFields) != 3 || detail.PlanningFields[0].ReferenceName != "Microsoft.VSTS.Scheduling.Effort" || detail.PlanningFields[2].Value != "2.5" {
		t.Fatalf("planning fields = %+v", detail.PlanningFields)
	}
}

func TestRESTClientListWorkItemComments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/project-1/_apis/wit/workItems/101/comments" || r.URL.Query().Get("continuationToken") != "opaque-token" || r.URL.Query().Get("$top") != "100" {
			t.Fatalf("comments request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-ms-continuationtoken", "next-opaque-token")
		_, _ = w.Write([]byte(`{"comments":[{"id":1,"text":"Older","createdDate":"2026-07-20T10:00:00Z","createdBy":{"id":"u1","displayName":"Ada"}},{"id":2,"text":"Deleted","isDeleted":true,"createdDate":"2026-07-21T10:00:00Z"},{"id":3,"text":"Newest","createdDate":"2026-07-22T10:00:00Z"}]}`))
	}))
	t.Cleanup(server.Close)

	page, err := newTestRESTClient(t, server, "pat").ListWorkItemComments(t.Context(), "project-1", 101, "opaque-token")
	if err != nil {
		t.Fatalf("ListWorkItemComments: %v", err)
	}
	if page.ContinuationToken != "next-opaque-token" || len(page.Comments) != 2 || page.Comments[0].ID != 3 || page.Comments[1].Author.DisplayName != "Ada" {
		t.Fatalf("comment page = %+v", page)
	}
}

func TestRESTClientGetCurrentIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/_apis/connectionData" {
			t.Fatalf("identity request = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authenticatedUser":{"id":"u1","providerDisplayName":"Ada","properties":{"Account":{"$value":"ada@example.com"}}}}`))
	}))
	t.Cleanup(server.Close)

	identity, err := newTestRESTClient(t, server, "pat").GetCurrentIdentity(t.Context())
	if err != nil || identity.ID != "u1" || identity.DisplayName != "Ada" || identity.UniqueName != "ada@example.com" {
		t.Fatalf("GetCurrentIdentity = %+v, %v", identity, err)
	}
}

func TestBoardWorkItemPatchAssignCurrentUser(t *testing.T) {
	action := "assign_current_user"
	patch, err := boardWorkItemPatch(struct {
		Columns []BoardColumn `json:"columns"`
		Fields  BoardFields   `json:"fields"`
	}{}, BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &action, resolvedAssignee: "ada@example.com", hasResolvedAssignee: true})
	if err != nil {
		t.Fatalf("boardWorkItemPatch: %v", err)
	}
	if len(patch) != 2 || patch[0]["op"] != "test" || patch[0]["path"] != "/rev" || patch[1]["op"] != "add" || patch[1]["path"] != "/fields/System.AssignedTo" || patch[1]["value"] != "ada@example.com" {
		t.Fatalf("patch = %#v", patch)
	}
}

func TestBoardWorkItemPatchUnassign(t *testing.T) {
	action := "unassign"
	patch, err := boardWorkItemPatch(struct {
		Columns []BoardColumn `json:"columns"`
		Fields  BoardFields   `json:"fields"`
	}{}, BoardWorkItemUpdateRequest{Revision: 7, AssigneeAction: &action, hasResolvedAssignee: true})
	if err != nil || len(patch) != 2 || patch[1]["op"] != "remove" || patch[1]["path"] != "/fields/System.AssignedTo" {
		t.Fatalf("patch = %#v, %v", patch, err)
	}
}

func TestBoardWorkItemPatchRejectsUnsupportedFields(t *testing.T) {
	if _, err := boardWorkItemPatch(struct {
		Columns []BoardColumn `json:"columns"`
		Fields  BoardFields   `json:"fields"`
	}{}, BoardWorkItemUpdateRequest{Revision: 7}); err == nil {
		t.Fatal("boardWorkItemPatch accepted an unsupported empty mutation")
	}
}

func TestRESTClientUpdateBoardWorkItem(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/project-1/team-1/_apis/work/boards/board-1":
			_, _ = w.Write([]byte(`{"id":"board-1","name":"Stories","fields":{"columnField":{"referenceName":"System.BoardColumn"},"doneField":{"referenceName":"System.BoardColumnDone"}},"columns":[{"id":"column-active","name":"Active"}]}`))
		case "/acme/project-1/_apis/wit/workitems/101":
			if r.Method != http.MethodPatch || r.Header.Get("Content-Type") != "application/json-patch+json" {
				t.Fatalf("request method/content type = %s/%q", r.Method, r.Header.Get("Content-Type"))
			}
			var patch []map[string]any
			if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
				t.Fatalf("decode patch: %v", err)
			}
			if len(patch) != 3 || patch[0]["op"] != "test" || patch[0]["path"] != "/rev" || patch[0]["value"] != float64(7) || patch[1]["path"] != "/fields/System.BoardColumn" || patch[2]["path"] != "/fields/System.BoardColumnDone" {
				t.Fatalf("patch = %#v", patch)
			}
			_, _ = w.Write([]byte(`{"id":101,"rev":8,"fields":{"System.Title":"Updated","System.AssignedTo":"Grace","System.Tags":"one; two","System.BoardColumn":"Active","System.BoardColumnDone":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newTestRESTClient(t, server, "pat")
	column := "column-active"
	done := true
	item, err := client.UpdateBoardWorkItem(context.Background(), "project-1", "team-1", "board-1", 101, BoardWorkItemUpdateRequest{Revision: 7, ColumnID: &column, ColumnDone: &done})
	if err != nil || item == nil || item.Revision != 8 || item.ColumnID != "column-active" || !item.ColumnDone {
		t.Fatalf("updated item = %+v, %v", item, err)
	}
}

func TestRESTClientUpdateWorkItemAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/acme/project-1/_apis/wit/workitems/101" || r.Method != http.MethodPatch {
			t.Fatalf("assignment request = %s %s", r.Method, r.URL.Path)
		}
		var patch []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			t.Fatalf("decode patch: %v", err)
		}
		if len(patch) != 2 || patch[0]["path"] != "/rev" || patch[0]["value"] != float64(7) || patch[1]["path"] != "/fields/System.AssignedTo" || patch[1]["value"] != "ada@example.com" {
			t.Fatalf("patch = %#v", patch)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":101,"rev":8,"fields":{"System.Title":"Fix build","System.AssignedTo":"Ada"}}`))
	}))
	t.Cleanup(server.Close)
	action := assignCurrentUserAction
	item, err := newTestRESTClient(t, server, "pat").UpdateWorkItem(context.Background(), "project-1", 101, WorkItemAssignmentRequest{Revision: 7, AssigneeAction: &action, resolvedAssignee: "ada@example.com", hasResolvedAssignee: true})
	if err != nil || item == nil || item.Revision != 8 || item.AssignedTo != "Ada" {
		t.Fatalf("updated item = %+v, %v", item, err)
	}
}

func TestRESTClientQueryWIQLBatchesAndPreservesOrder(t *testing.T) {
	var mu sync.Mutex
	var batches [][]int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/project-1/_apis/wit/wiql":
			refs := make([]map[string]int, 0, 205)
			for id := 1; id <= 205; id++ {
				refs = append(refs, map[string]int{"id": id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"workItems": refs})
		case "/acme/project-1/_apis/wit/workitemsbatch":
			var req struct {
				IDs []int `json:"ids"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode batch: %v", err)
			}
			mu.Lock()
			batches = append(batches, append([]int(nil), req.IDs...))
			mu.Unlock()
			items := make([]map[string]any, 0, len(req.IDs))
			for i := len(req.IDs) - 1; i >= 0; i-- {
				id := req.IDs[i]
				if id == 3 { // Azure may omit an inaccessible or deleted item.
					continue
				}
				items = append(items, map[string]any{
					"id": id,
					"fields": map[string]any{
						"System.Title":        fmt.Sprintf("Item %d", id),
						"System.State":        "Active",
						"System.WorkItemType": "Task",
					},
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"value": items})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	result, err := client.QueryWIQL(context.Background(), "project-1", "SELECT [System.Id] FROM WorkItems", 205)
	if err != nil {
		t.Fatalf("QueryWIQL: %v", err)
	}
	if len(batches) != 2 || len(batches[0]) != 200 || len(batches[1]) != 5 {
		t.Fatalf("batches = %v", batches)
	}
	if len(result.Items) != 204 || result.Items[0].ID != 1 || result.Items[1].ID != 2 || result.Items[2].ID != 4 || result.Items[len(result.Items)-1].ID != 205 {
		t.Fatalf("result order/omission = first %+v, last %+v, len %d", result.Items[:3], result.Items[len(result.Items)-1], len(result.Items))
	}
}

func TestRESTClientPullRequestReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"pullRequestId":42,"title":"Ship it","status":"active","isDraft":true,"sourceRefName":"refs/heads/feature","targetRefName":"refs/heads/main","url":"https://api/pr/42","createdBy":{"id":"u1","displayName":"Ada"},"repository":{"id":"repo-1","name":"widgets","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}}]}`))
		case "/acme/project-1/_apis/git/pullrequests":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"pullRequestId":42,"title":"Ship it","status":"active","repository":{"id":"repo-1","name":"widgets","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}}]}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests/42":
			_, _ = w.Write([]byte(`{"pullRequestId":42,"title":"Ship it","status":"active","sourceRefName":"refs/heads/feature","targetRefName":"refs/heads/main","createdBy":{"id":"u1","displayName":"Ada"},"repository":{"id":"repo-1","name":"widgets","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests/42/reviewers":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"u2","displayName":"Grace","vote":10,"isRequired":true}]}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests/42/threads":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":7,"status":"active","comments":[{"id":8,"content":"Please add a test","author":{"id":"u2","displayName":"Grace"},"commentType":"text","publishedDate":"2026-07-17T10:00:00Z","lastUpdatedDate":"2026-07-17T11:30:00Z"}]}]}`))
		case "/acme/project-1/_apis/git/repositories/repo-1/pullrequests/42/workitems":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"101","url":"https://api/workitems/101"}]}`))
		case "/acme/project-1/_apis/policy/evaluations":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"evaluationId":"eval-1","status":"approved","configuration":{"isBlocking":true,"type":{"displayName":"Build"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := newTestRESTClient(t, server, "pat")
	prs, err := client.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "project-1", RepositoryID: "repo-1", Status: "active", ReviewerID: "me"})
	if err != nil || len(prs.Items) != 1 || prs.Items[0].SourceBranch != "feature" || prs.Items[0].WebURL == "" {
		t.Fatalf("ListPullRequests = %+v, %v", prs, err)
	}
	projectPRs, err := client.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "project-1", Status: "active", Top: 10})
	if err != nil || len(projectPRs.Items) != 1 || projectPRs.Items[0].RepositoryID != "repo-1" {
		t.Fatalf("project ListPullRequests = %+v, %v", projectPRs, err)
	}
	pr, err := client.GetPullRequest(context.Background(), "project-1", "repo-1", 42)
	if err != nil || pr.ID != 42 {
		t.Fatalf("GetPullRequest = %+v, %v", pr, err)
	}
	reviewers, err := client.ListReviewers(context.Background(), "project-1", "repo-1", 42)
	if err != nil || len(reviewers) != 1 || reviewers[0].Vote != 10 {
		t.Fatalf("ListReviewers = %+v, %v", reviewers, err)
	}
	threads, err := client.ListThreads(context.Background(), "project-1", "repo-1", 42)
	if err != nil || len(threads) != 1 || threads[0].Comments[0].Content != "Please add a test" {
		t.Fatalf("ListThreads = %+v, %v", threads, err)
	}
	comment := threads[0].Comments[0]
	if !comment.PublishedAt.Equal(time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)) ||
		!comment.UpdatedAt.Equal(time.Date(2026, 7, 17, 11, 30, 0, 0, time.UTC)) {
		t.Fatalf("comment timestamps = published %v, updated %v", comment.PublishedAt, comment.UpdatedAt)
	}
	refs, err := client.ListLinkedWorkItems(context.Background(), "project-1", "repo-1", 42)
	if err != nil || len(refs) != 1 || refs[0].ID != 101 {
		t.Fatalf("ListLinkedWorkItems = %+v, %v", refs, err)
	}
	policies, err := client.ListPolicyEvaluations(context.Background(), "project-1", 42)
	if err != nil || len(policies) != 1 || policies[0].Status != "approved" {
		t.Fatalf("ListPolicyEvaluations = %+v, %v", policies, err)
	}
}

func TestRESTClientErrorIsBoundedAndRedacted(t *testing.T) {
	const pat = "never-echo-this-pat"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(pat + strings.Repeat("x", 6000)))
	}))
	t.Cleanup(server.Close)

	_, err := newTestRESTClient(t, server, pat).ListProjects(context.Background())
	var apiErr *APIError
	if err == nil || !AsAPIError(err, &apiErr) {
		t.Fatalf("error = %v, want APIError", err)
	}
	if strings.Contains(apiErr.Body, pat) || len(apiErr.Body) > 4096 {
		t.Fatalf("unsafe API error body length=%d body=%q", len(apiErr.Body), apiErr.Body)
	}
}

func TestRESTClientSuccessResponseIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBodyBytes+1)))
	}))
	t.Cleanup(server.Close)

	_, err := newTestRESTClient(t, server, "pat").ListProjects(context.Background())
	if err == nil || err.Error() != "azure devops response exceeded size limit" {
		t.Fatalf("error = %v, want response size limit", err)
	}
}

func TestRESTClientRejectsNonCanonicalOrganizationURLBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	t.Cleanup(server.Close)

	_, err := NewRESTClient(server.URL, "pat", server.Client()).ListProjects(context.Background())
	if err == nil || !strings.Contains(err.Error(), "invalid azure devops organization URL") {
		t.Fatalf("ListProjects error = %v, want invalid organization URL", err)
	}
	if called {
		t.Fatal("invalid organization URL reached the HTTP transport")
	}
}

func newTestRESTClient(t *testing.T, server *httptest.Server, pat string) *RESTClient {
	t.Helper()
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		response := httptest.NewRecorder()
		server.Config.Handler.ServeHTTP(response, req)
		return response.Result(), nil
	})}
	return NewRESTClient("https://dev.azure.com/acme", pat, httpClient)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
