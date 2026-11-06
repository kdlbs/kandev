package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.2
// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchPollingEventuallyReadsEveryFilter(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc, store := setupWatchServiceTest(t)
				t.Cleanup(svc.Stop)
				var calls []string
				client := watchProgressClient(&calls)
				configureTestWorkspaceAuth(t, svc, client, "ws-1")
				repos := []RepoFilter{{Owner: "acme", Name: "first"}, {Owner: "acme", Name: "second"}}
				issue := &IssueWatch{ID: "issue", WorkspaceID: "ws-1", Repos: repos, CustomQuery: "is:issue", Enabled: true}
				review := &ReviewWatch{ID: "review", WorkspaceID: "ws-1", Repos: repos, CustomQuery: "is:pr", Enabled: true}
				if err := store.CreateIssueWatch(context.Background(), issue); err != nil {
					t.Fatal(err)
				}
				if err := store.CreateReviewWatch(context.Background(), review); err != nil {
					t.Fatal(err)
				}
				var results []string
				for cycle := 0; cycle < 2; cycle++ {
					items, completed := checkProgressWatch(t, svc, kind, issue, review)
					results = append(results, items...)
					if cycle == 0 && completed {
						t.Error("deferred sweep claimed a completed poll")
					}
					time.Sleep(5 * time.Minute)
				}
				want := []string{"first", "second"}
				if !reflect.DeepEqual(calls, want) {
					t.Fatalf("provider reads=%v, want %v", calls, want)
				}
				if !reflect.DeepEqual(results, want) {
					t.Fatalf("discoveries=%v, want %v", results, want)
				}
			})
		})
	}
}

func watchProgressClient(calls *[]string) *PATClient {
	client := NewPATClient("fixture")
	client.httpClient.Transport = legacyIdentityRoundTripper(func(req *http.Request) (*http.Response, error) {
		body := `{}`
		if req.URL.Path == "/search/issues" {
			repo := ""
			for _, part := range strings.Fields(req.URL.Query().Get("q")) {
				if strings.HasPrefix(part, "repo:acme/") {
					repo = strings.TrimPrefix(part, "repo:acme/")
				}
			}
			*calls = append(*calls, repo)
			body = fmt.Sprintf(`{"items":[{"id":1,"number":1,"title":"fixture","state":"open","html_url":"https://github.com/acme/%s/issues/1","repository_url":"https://api.github.com/repos/acme/%s","user":{"login":"alice"}}]}`, repo, repo)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return client
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchPollingSharesCapacityAcrossWatches(t *testing.T) {
	for _, kind := range []string{"issue", "review"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				poller, svc, _, store := setupPollerTest(t)
				t.Cleanup(svc.Stop)
				var calls []string
				configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws1")
				for _, name := range []string{"first", "second"} {
					repos := []RepoFilter{{Owner: "acme", Name: name}}
					var err error
					if kind == "issue" {
						err = store.CreateIssueWatch(context.Background(), &IssueWatch{ID: name, WorkspaceID: "ws1", Repos: repos, Enabled: true})
					} else {
						err = store.CreateReviewWatch(context.Background(), &ReviewWatch{ID: name, WorkspaceID: "ws1", Repos: repos, CustomQuery: "is:pr", Enabled: true})
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 2; i++ {
					if kind == "issue" {
						poller.checkIssueWatches(pollerContext(context.Background()))
					} else {
						poller.checkReviewWatches(pollerContext(context.Background()))
					}
					time.Sleep(5 * time.Minute)
				}
				if len(calls) != 2 || calls[0] == calls[1] {
					t.Fatalf("polling did not distribute reads across watches: %v", calls)
				}
			})
		})
	}
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-002.7
func TestWatchPollingSharesCapacityAcrossIssueAndReviewLoops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		poller, svc, _, store := setupPollerTest(t)
		t.Cleanup(svc.Stop)
		var calls []string
		configureTestWorkspaceAuth(t, svc, watchProgressClient(&calls), "ws1")
		if err := store.CreateIssueWatch(context.Background(), &IssueWatch{ID: "issue", WorkspaceID: "ws1", Repos: []RepoFilter{{Owner: "acme", Name: "issue"}}, Enabled: true}); err != nil {
			t.Fatal(err)
		}
		if err := store.CreateReviewWatch(context.Background(), &ReviewWatch{ID: "review", WorkspaceID: "ws1", Repos: []RepoFilter{{Owner: "acme", Name: "review"}}, CustomQuery: "is:pr", Enabled: true}); err != nil {
			t.Fatal(err)
		}
		// Run the production loop owner, including initial checks and timer ticks.
		poller.Start(context.Background())
		t.Cleanup(poller.Stop)
		time.Sleep(20 * time.Minute)
		poller.Stop()
		seen := map[string]bool{}
		for _, call := range calls {
			seen[call] = true
		}
		if !seen["issue"] || !seen["review"] {
			t.Fatalf("one watch kind starved: %v", calls)
		}
	})
}

func checkProgressWatch(t *testing.T, svc *Service, kind string, issue *IssueWatch, review *ReviewWatch) ([]string, bool) {
	t.Helper()
	var names []string
	ctx := pollerContext(context.Background())
	if kind == "issue" {
		items, err := svc.CheckIssueWatch(ctx, issue)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range items {
			names = append(names, item.RepoName)
		}
		return names, issue.LastPolledAt != nil
	}
	items, err := svc.CheckReviewWatch(ctx, review)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		names = append(names, item.RepoName)
	}
	return names, review.LastPolledAt != nil
}
