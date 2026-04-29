package linear

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMockServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	return s
}

// pointTo overrides the GraphQL endpoint on a freshly-built client so tests
// hit the test server without needing a mockable URL on the production
// constructor.
func pointTo(c *GraphQLClient, url string) *GraphQLClient {
	c.endpoint = url
	return c
}

// readReq drains a request body into a generic JSON map so tests can assert on
// the GraphQL query+variables payload.
func readReq(t *testing.T, r *http.Request) map[string]interface{} {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return m
}

func TestGraphQLClient_AuthHeader(t *testing.T) {
	var gotAuth string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"u","name":"Alice","displayName":"Alice","email":"a@x"},"organization":{"urlKey":"acme","name":"Acme"}}}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "lin_api_xyz"), ts.URL)
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	if gotAuth != "lin_api_xyz" {
		t.Errorf("auth header = %q, want bare API key", gotAuth)
	}
	if res.OrgSlug != "acme" {
		t.Errorf("expected orgSlug=acme, got %q", res.OrgSlug)
	}
	if res.DisplayName != "Alice" {
		t.Errorf("expected displayName=Alice, got %q", res.DisplayName)
	}
}

func TestGraphQLClient_TestAuth_BadCreds_ReportsErrorInResult(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid api key"}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "bad"), ts.URL)
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth should not error on 401, got %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if !strings.Contains(res.Error, "401") {
		t.Errorf("expected 401 in error, got %q", res.Error)
	}
}

func TestGraphQLClient_TestAuth_GraphQLErrors(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"message":"Authentication required","extensions":{"type":"AUTHENTICATION_ERROR"}}]}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false on GraphQL error")
	}
	if !strings.Contains(res.Error, "Authentication required") {
		t.Errorf("expected GraphQL error message preserved, got %q", res.Error)
	}
}

func TestGraphQLClient_ListTeams(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := readReq(t, r)
		if !strings.Contains(body["query"].(string), "teams(") {
			t.Errorf("expected teams query, got %q", body["query"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"teams":{"nodes":[{"id":"t1","key":"ENG","name":"Engineering"}]}}}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	teams, err := c.ListTeams(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(teams) != 1 || teams[0].Key != "ENG" {
		t.Errorf("teams = %+v", teams)
	}
}

func TestGraphQLClient_ListStates_RequiresTeamKey(t *testing.T) {
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), "http://nope")
	if _, err := c.ListStates(context.Background(), ""); err == nil {
		t.Error("expected error for empty team key")
	}
}

func TestGraphQLClient_GetIssue_AttachesStates(t *testing.T) {
	calls := 0
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := readReq(t, r)
		q, _ := body["query"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "issue(id:"):
			calls++
			_, _ = w.Write([]byte(`{"data":{"issue":{"id":"i1","identifier":"ENG-1","title":"Hi","description":"body","url":"https://linear.app/acme/issue/ENG-1","priority":2,"priorityLabel":"High","state":{"id":"s1","name":"In Progress","type":"started","color":"#fff"},"team":{"id":"t1","key":"ENG"},"assignee":null,"creator":null}}}`))
		case strings.Contains(q, "workflowStates"):
			calls++
			_, _ = w.Write([]byte(`{"data":{"workflowStates":{"nodes":[{"id":"s1","name":"In Progress","type":"started","color":"#fff","position":2}]}}}`))
		default:
			t.Errorf("unexpected query: %q", q)
		}
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	issue, err := c.GetIssue(context.Background(), "ENG-1")
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if issue.Identifier != "ENG-1" || issue.StateName != "In Progress" {
		t.Errorf("issue mismatch: %+v", issue)
	}
	if issue.StateCategory != "indeterminate" {
		t.Errorf("expected category=indeterminate for started, got %q", issue.StateCategory)
	}
	if len(issue.States) != 1 {
		t.Errorf("expected 1 state attached, got %d", len(issue.States))
	}
	if calls != 2 {
		t.Errorf("expected 2 GraphQL calls (issue + states), got %d", calls)
	}
}

func TestGraphQLClient_SearchIssues_PaginationCursor(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := readReq(t, r)
		vars, _ := body["variables"].(map[string]interface{})
		if got, _ := vars["after"].(string); got != "cursor-1" {
			t.Errorf("expected after=cursor-1, got %v", vars["after"])
		}
		if got, _ := vars["first"].(float64); got != 25 {
			t.Errorf("expected first=25, got %v", vars["first"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[{"id":"i1","identifier":"ENG-1","title":"a","description":"","url":"u","priority":0,"priorityLabel":"None","state":{"id":"s","name":"S","type":"backlog","color":""},"team":{"id":"t","key":"ENG"}}],"pageInfo":{"hasNextPage":true,"endCursor":"cursor-2"}}}}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	res, err := c.SearchIssues(context.Background(), SearchFilter{Query: "auth"}, "cursor-1", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.IsLast {
		t.Error("expected IsLast=false when hasNextPage=true")
	}
	if res.NextPageToken != "cursor-2" {
		t.Errorf("expected next cursor, got %q", res.NextPageToken)
	}
	if len(res.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(res.Issues))
	}
}

func TestGraphQLClient_SetIssueState(t *testing.T) {
	var seenVars map[string]interface{}
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		body := readReq(t, r)
		seenVars, _ = body["variables"].(map[string]interface{})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issueUpdate":{"success":true}}}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	if err := c.SetIssueState(context.Background(), "ENG-1", "state-id"); err != nil {
		t.Fatalf("set state: %v", err)
	}
	if seenVars["id"] != "ENG-1" || seenVars["stateId"] != "state-id" {
		t.Errorf("vars = %+v", seenVars)
	}
}

func TestGraphQLClient_SetIssueState_FailureFlag(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issueUpdate":{"success":false}}}`))
	})
	c := pointTo(NewGraphQLClient(&LinearConfig{}, "x"), ts.URL)
	err := c.SetIssueState(context.Background(), "ENG-1", "state-id")
	if err == nil {
		t.Error("expected error when success=false")
	}
}

func TestStateCategoryMapping(t *testing.T) {
	cases := map[string]string{
		"backlog":   "new",
		"unstarted": "new",
		"triage":    "new",
		"started":   "indeterminate",
		"completed": "done",
		"canceled":  "done",
		"weird":     "new",
	}
	for input, want := range cases {
		if got := stateCategory(input); got != want {
			t.Errorf("stateCategory(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestBuildIssueFilter_DropsEmpty(t *testing.T) {
	if got := buildIssueFilter(SearchFilter{}); got != nil {
		t.Errorf("expected nil filter for empty input, got %+v", got)
	}
	got := buildIssueFilter(SearchFilter{Query: "auth", TeamKey: "ENG", Assigned: "me"})
	if _, ok := got["searchableContent"]; !ok {
		t.Error("expected searchableContent filter")
	}
	if _, ok := got["team"]; !ok {
		t.Error("expected team filter")
	}
	if _, ok := got["assignee"]; !ok {
		t.Error("expected assignee filter")
	}
}
