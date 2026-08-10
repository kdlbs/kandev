package forgejo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func TestPATClientAuthenticatesWithForgejoToken(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/user" {
			t.Fatalf("path = %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "token token-value" {
			t.Fatal("Forgejo token header missing")
		}
		_ = json.NewEncoder(w).Encode(User{Login: "rob"})
	})
	t.Cleanup(server.Close)
	client, err := NewPATClient(server.URL+"/", "token-value")
	if err != nil {
		t.Fatal(err)
	}
	user, err := client.GetAuthenticatedUser(context.Background())
	if err != nil || user.Login != "rob" {
		t.Fatalf("user=%#v err=%v", user, err)
	}
}

func TestPATClientRepositoriesUsePagination(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("page") != "2" || request.URL.Query().Get("limit") != "25" {
			t.Fatalf("query = %s", request.URL.RawQuery)
		}
		w.Header().Set("x-total-count", "51")
		_, _ = w.Write([]byte(`[{"name":"rf-consulting","full_name":"botwork123/rf-consulting","default_branch":"main","owner":{"login":"botwork123"}}]`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	repos, total, err := client.ListRepositories(context.Background(), 2, 25)
	if err != nil || total != 51 || len(repos) != 1 || repos[0].FullName != "botwork123/rf-consulting" {
		t.Fatalf("repos=%#v total=%d err=%v", repos, total, err)
	}
}

func TestPATClientIssuesExcludePullRequests(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"number":1,"title":"Issue","state":"open"},{"number":2,"title":"PR","pull_request":{}}]`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	issues, _, err := client.ListIssues(context.Background(), "botwork123", "rf-consulting", 1, 30)
	if err != nil || len(issues) != 1 || issues[0].Number != 1 {
		t.Fatalf("issues=%#v err=%v", issues, err)
	}
}

func TestPATClientCreatePullRequest(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/v1/repos/owner/repo/pulls" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(request.Body).Decode(&body)
		if body["head"] != "feat/forgejo" || body["base"] != "main" {
			t.Fatalf("body=%#v", body)
		}
		_, _ = w.Write([]byte(`{"number":7,"title":"Forgejo integration","state":"open"}`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	pull, err := client.CreatePullRequest(context.Background(), CreatePullRequestInput{Owner: "owner", Repo: "repo", Title: "Forgejo integration", Head: "feat/forgejo", Base: "main"})
	if err != nil || pull.Number != 7 {
		t.Fatalf("pull=%#v err=%v", pull, err)
	}
}

func TestPATClientGetsPullRequest(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/repos/owner/repo/pulls/7" {
			t.Fatalf("path=%s", request.URL.Path)
		}
		_, _ = w.Write([]byte(`{"number":7,"title":"Forgejo integration","state":"open","html_url":"https://forgejo.example/owner/repo/pulls/7","head":{"ref":"feat/forgejo"},"base":{"ref":"main"}}`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	pull, err := client.GetPullRequest(context.Background(), "owner", "repo", 7)
	if err != nil || pull.Head != "feat/forgejo" || pull.Base != "main" {
		t.Fatalf("pull=%#v err=%v", pull, err)
	}
}

func TestPATClientGetsIssue(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/repos/owner/repo/issues/7" {
			t.Fatalf("path=%s", request.URL.Path)
		}
		_, _ = w.Write([]byte(`{"number":7,"title":"Issue","state":"open","html_url":"https://forgejo.example/owner/repo/issues/7"}`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	issue, err := client.GetIssue(context.Background(), "owner", "repo", 7)
	if err != nil || issue.Title != "Issue" {
		t.Fatalf("issue=%#v err=%v", issue, err)
	}
}

func TestPATClientListsPullRequests(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/repos/owner/repo/pulls" {
			t.Fatalf("path=%s", request.URL.Path)
		}
		w.Header().Set("x-total-count", "1")
		_, _ = w.Write([]byte(`[{"number":7,"title":"PR","state":"open","head":{"ref":"feat/a"},"base":{"ref":"main"}}]`))
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	pulls, total, err := client.ListPullRequests(context.Background(), "owner", "repo", 1, 30)
	if err != nil || total != 1 || len(pulls) != 1 || pulls[0].Head != "feat/a" {
		t.Fatalf("pulls=%#v total=%d err=%v", pulls, total, err)
	}
}

func TestPATClientListsPullRequestReviewResources(t *testing.T) {
	server := newServer(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/repos/owner/repo/pulls/7/commits":
			_, _ = w.Write([]byte(`[{"sha":"abc","commit":{"message":"Fix","author":{"name":"Alice"}}}]`))
		case "/api/v1/repos/owner/repo/pulls/7/files":
			_, _ = w.Write([]byte(`[{"filename":"a.go","status":"modified","additions":3,"deletions":1,"changes":4}]`))
		case "/api/v1/repos/owner/repo/pulls/7/comments":
			_, _ = w.Write([]byte(`[{"id":2,"body":"nit","path":"a.go","user":{"login":"bob"}}]`))
		case "/api/v1/repos/owner/repo/pulls/7/reviews":
			_, _ = w.Write([]byte(`[{"id":3,"state":"APPROVED","body":"LGTM","user":{"login":"carol"}}]`))
		default:
			http.NotFound(w, request)
		}
	})
	t.Cleanup(server.Close)
	client, _ := NewPATClient(server.URL, "token")
	commits, err := client.ListPullRequestCommits(context.Background(), "owner", "repo", 7)
	if err != nil || len(commits) != 1 || commits[0].Author != "Alice" {
		t.Fatalf("commits=%#v err=%v", commits, err)
	}
	files, err := client.ListPullRequestFiles(context.Background(), "owner", "repo", 7)
	if err != nil || len(files) != 1 || files[0].Changes != 4 {
		t.Fatalf("files=%#v err=%v", files, err)
	}
	comments, err := client.ListPullRequestComments(context.Background(), "owner", "repo", 7)
	if err != nil || len(comments) != 1 || comments[0].Author != "bob" {
		t.Fatalf("comments=%#v err=%v", comments, err)
	}
	reviews, err := client.ListPullRequestReviews(context.Background(), "owner", "repo", 7)
	if err != nil || len(reviews) != 1 || reviews[0].State != "APPROVED" {
		t.Fatalf("reviews=%#v err=%v", reviews, err)
	}
}
