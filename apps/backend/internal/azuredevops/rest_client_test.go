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
		case "/_apis/connectionData":
			_, _ = w.Write([]byte(`{"authenticatedUser":{"id":"user-1","providerDisplayName":"Ada","properties":{"Account":{"$value":"ada@example.com"}}}}`))
		case "/_apis/projects":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"project-1","name":"Platform","url":"https://api/project-1"}]}`))
		case "/project-1/_apis/git/repositories":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"repo-1","name":"widgets","defaultBranch":"refs/heads/main","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewRESTClient(server.URL, pat, server.Client())
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
}

func TestRESTClientQueryWIQLBatchesAndPreservesOrder(t *testing.T) {
	var mu sync.Mutex
	var batches [][]int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/project-1/_apis/wit/wiql":
			refs := make([]map[string]int, 0, 205)
			for id := 1; id <= 205; id++ {
				refs = append(refs, map[string]int{"id": id})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"workItems": refs})
		case "/project-1/_apis/wit/workitemsbatch":
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

	client := NewRESTClient(server.URL, "pat", server.Client())
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
		case "/project-1/_apis/git/repositories/repo-1/pullrequests":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"pullRequestId":42,"title":"Ship it","status":"active","isDraft":true,"sourceRefName":"refs/heads/feature","targetRefName":"refs/heads/main","url":"https://api/pr/42","createdBy":{"id":"u1","displayName":"Ada"},"repository":{"id":"repo-1","name":"widgets","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}}]}`))
		case "/project-1/_apis/git/repositories/repo-1/pullrequests/42":
			_, _ = w.Write([]byte(`{"pullRequestId":42,"title":"Ship it","status":"active","sourceRefName":"refs/heads/feature","targetRefName":"refs/heads/main","createdBy":{"id":"u1","displayName":"Ada"},"repository":{"id":"repo-1","name":"widgets","webUrl":"https://dev.azure.com/acme/Platform/_git/widgets","project":{"id":"project-1","name":"Platform"}}}`))
		case "/project-1/_apis/git/repositories/repo-1/pullrequests/42/reviewers":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"u2","displayName":"Grace","vote":10,"isRequired":true}]}`))
		case "/project-1/_apis/git/repositories/repo-1/pullrequests/42/threads":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":7,"status":"active","comments":[{"id":8,"content":"Please add a test","author":{"id":"u2","displayName":"Grace"},"commentType":"text"}]}]}`))
		case "/project-1/_apis/git/repositories/repo-1/pullrequests/42/workitems":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"id":"101","url":"https://api/workitems/101"}]}`))
		case "/project-1/_apis/policy/evaluations":
			_, _ = w.Write([]byte(`{"count":1,"value":[{"evaluationId":"eval-1","status":"approved","configuration":{"isBlocking":true,"type":{"displayName":"Build"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	client := NewRESTClient(server.URL, "pat", server.Client())
	prs, err := client.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "project-1", RepositoryID: "repo-1", Status: "active", ReviewerID: "me"})
	if err != nil || len(prs.Items) != 1 || prs.Items[0].SourceBranch != "feature" || prs.Items[0].WebURL == "" {
		t.Fatalf("ListPullRequests = %+v, %v", prs, err)
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

	_, err := NewRESTClient(server.URL, pat, server.Client()).ListProjects(context.Background())
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

	_, err := NewRESTClient(server.URL, "pat", server.Client()).ListProjects(context.Background())
	if err == nil || err.Error() != "azure devops response exceeded size limit" {
		t.Fatalf("error = %v, want response size limit", err)
	}
}
