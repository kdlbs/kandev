package jira

import (
	"context"
	"encoding/base64"
	"errors"
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

func clientTo(ts *httptest.Server, method, secret string) *CloudClient {
	cfg := &JiraConfig{
		SiteURL:    ts.URL,
		Email:      "user@example.com",
		AuthMethod: method,
	}
	return NewCloudClient(cfg, secret)
}

func TestCloudClient_AuthHeaders_APIToken(t *testing.T) {
	var gotAuth string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"a","displayName":"d","emailAddress":"e"}`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "secrettoken")

	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if !res.OK {
		t.Fatalf("expected OK=true, got %+v", res)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:secrettoken"))
	if gotAuth != want {
		t.Errorf("auth header = %q, want %q", gotAuth, want)
	}
}

func TestCloudClient_AuthHeaders_SessionCookie_BareJWT(t *testing.T) {
	var gotCookie string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotCookie = r.Header.Get("Cookie")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"accountId":"a"}`))
	})
	// New flow: user pastes just the value of cloud.session.token /
	// tenant.session.token from DevTools → Application → Cookies. The client
	// wraps it under both names so a single paste works for password accounts
	// and SSO tenants alike.
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.sig"
	c := clientTo(ts, AuthMethodSessionCookie, jwt)
	if _, err := c.TestAuth(context.Background()); err != nil {
		t.Fatalf("test auth: %v", err)
	}
	want := "cloud.session.token=" + jwt + "; tenant.session.token=" + jwt
	if gotCookie != want {
		t.Errorf("cookie header = %q, want %q", gotCookie, want)
	}
}

func TestCloudClient_TestAuth_BadCreds_ReportsError(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["bad creds"]}`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "bad")
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth should not error on 401, got %v", err)
	}
	if res.OK {
		t.Fatalf("expected OK=false, got %+v", res)
	}
	if !strings.Contains(res.Error, "401") {
		t.Errorf("expected 401 in error, got %q", res.Error)
	}
}

func TestCloudClient_GetTicket_ParsesADFDescription(t *testing.T) {
	issueBody := `{
		"key": "PROJ-42",
		"fields": {
			"summary": "Fix the thing",
			"description": {
				"type": "doc",
				"content": [
					{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}]},
					{"type":"paragraph","content":[{"type":"text","text":"line two"}]}
				]
			},
			"status": {"id":"3","name":"In Progress"},
			"project": {"key":"PROJ"},
			"issuetype": {"name":"Bug"}
		}
	}`
	transitionsBody := `{"transitions":[{"id":"11","name":"Start","to":{"id":"3","name":"In Progress"}}]}`

	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-42/transitions"):
			_, _ = w.Write([]byte(transitionsBody))
		case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/PROJ-42"):
			_, _ = w.Write([]byte(issueBody))
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	})

	c := clientTo(ts, AuthMethodAPIToken, "tok")
	ticket, err := c.GetTicket(context.Background(), "PROJ-42")
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if ticket.Summary != "Fix the thing" {
		t.Errorf("summary: got %q", ticket.Summary)
	}
	if !strings.Contains(ticket.Description, "Hello world") {
		t.Errorf("description missing first line: %q", ticket.Description)
	}
	if !strings.Contains(ticket.Description, "line two") {
		t.Errorf("description missing second paragraph: %q", ticket.Description)
	}
	if ticket.StatusName != "In Progress" {
		t.Errorf("status name: got %q", ticket.StatusName)
	}
	if len(ticket.Transitions) != 1 || ticket.Transitions[0].Name != "Start" {
		t.Errorf("transitions: %+v", ticket.Transitions)
	}
	if ticket.URL != ts.URL+"/browse/PROJ-42" {
		t.Errorf("url: got %q", ticket.URL)
	}
}

func TestCloudClient_GetTicket_PlainStringDescription(t *testing.T) {
	body := `{"key":"X-1","fields":{"summary":"s","description":"plain","status":{},"project":{},"issuetype":{}}}`
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/X-1/transitions") {
			_, _ = w.Write([]byte(`{"transitions":[]}`))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	ticket, err := c.GetTicket(context.Background(), "X-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ticket.Description != "plain" {
		t.Errorf("description: got %q", ticket.Description)
	}
}

func TestCloudClient_GetTicket_NonOK_ReturnsAPIError(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["not found"]}`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	_, err := c.GetTicket(context.Background(), "NONE-1")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("status: got %d", apiErr.StatusCode)
	}
}

func TestCloudClient_DoTransition_PostsBody(t *testing.T) {
	var gotPath, gotBody, gotMethod string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		// io.ReadAll over a sized r.Body.Read: chunked transfer encoding
		// sets ContentLength=-1 (panicking on make), and a single Read may
		// return partial bytes even when the length is known.
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(http.StatusNoContent)
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	if err := c.DoTransition(context.Background(), "PROJ-1", "21"); err != nil {
		t.Fatalf("transition: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method: got %q", gotMethod)
	}
	if gotPath != "/rest/api/3/issue/PROJ-1/transitions" {
		t.Errorf("path: got %q", gotPath)
	}
	if !strings.Contains(gotBody, `"id":"21"`) {
		t.Errorf("body missing transition id: %q", gotBody)
	}
}

func TestCloudClient_ListProjects(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/rest/api/3/project/search") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"values":[{"id":"1","key":"A","name":"Alpha"},{"id":"2","key":"B","name":"Beta"}]}`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	projects, err := c.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(projects) != 2 || projects[0].Key != "A" || projects[1].Name != "Beta" {
		t.Errorf("unexpected projects: %+v", projects)
	}
}

func TestCloudClient_ListProjectStatuses_FlattensAndDedupes(t *testing.T) {
	var gotPath string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		// Two issue types share the "In Development" status (id 10001); the
		// flattened result must contain it exactly once.
		_, _ = w.Write([]byte(`[
			{"name":"Bug","statuses":[
				{"id":"10001","name":"In Development","statusCategory":{"key":"indeterminate"}},
				{"id":"3","name":"Done","statusCategory":{"key":"done"}}
			]},
			{"name":"Story","statuses":[
				{"id":"10001","name":"In Development","statusCategory":{"key":"indeterminate"}},
				{"id":"1","name":"To Do","statusCategory":{"key":"new"}}
			]}
		]`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	statuses, err := c.ListProjectStatuses(context.Background(), "PROJ")
	if err != nil {
		t.Fatalf("list statuses: %v", err)
	}
	if gotPath != "/rest/api/3/project/PROJ/statuses" {
		t.Errorf("path: got %q", gotPath)
	}
	if len(statuses) != 3 {
		t.Fatalf("expected 3 de-duped statuses, got %d: %+v", len(statuses), statuses)
	}
	if statuses[0].ID != "10001" || statuses[0].Name != "In Development" || statuses[0].StatusCategory != "indeterminate" {
		t.Errorf("first status unexpected: %+v", statuses[0])
	}
}

func TestCloudClient_ListProjectStatuses_ServerMode_UsesV2(t *testing.T) {
	var gotPath string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`[{"name":"Task","statuses":[{"id":"1","name":"Open","statusCategory":{"key":"new"}}]}]`))
	})
	c := serverClient(ts, AuthMethodPAT, "tok")
	statuses, err := c.ListProjectStatuses(context.Background(), "P")
	if err != nil {
		t.Fatalf("list statuses: %v", err)
	}
	if gotPath != "/rest/api/2/project/P/statuses" {
		t.Errorf("path: got %q, want v2", gotPath)
	}
	if len(statuses) != 1 || statuses[0].Name != "Open" {
		t.Errorf("unexpected statuses: %+v", statuses)
	}
}

func TestCloudClient_ListProjectStatuses_NonOK_ReturnsAPIError(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errorMessages":["No project"]}`))
	})
	c := clientTo(ts, AuthMethodAPIToken, "t")
	_, err := c.ListProjectStatuses(context.Background(), "NONE")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("status: got %d", apiErr.StatusCode)
	}
}

func TestCloudClient_SiteURLTrailingSlash_Stripped(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		// If trailing slash wasn't stripped, we'd see "//rest/..."
		if strings.HasPrefix(r.URL.Path, "//") {
			t.Errorf("double slash in path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{}`))
	})
	cfg := &JiraConfig{SiteURL: ts.URL + "/", Email: "e", AuthMethod: AuthMethodAPIToken}
	c := NewCloudClient(cfg, "t")
	if _, err := c.TestAuth(context.Background()); err != nil {
		t.Fatalf("test: %v", err)
	}
}

// --- Server / Data Center mode ---

// serverClient builds a CloudClient configured for a self-hosted Server/DC
// instance against the test server.
func serverClient(ts *httptest.Server, method, secret string) *CloudClient {
	cfg := &JiraConfig{
		SiteURL:      ts.URL,
		Email:        "user@example.com",
		AuthMethod:   method,
		InstanceType: InstanceTypeServer,
	}
	return NewCloudClient(cfg, secret)
}

func TestCloudClient_ServerMode_UsesV2Paths(t *testing.T) {
	var gotPath string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accountId":"a"}`))
	})
	c := serverClient(ts, AuthMethodPAT, "pat-token")
	if _, err := c.TestAuth(context.Background()); err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if gotPath != "/rest/api/2/myself" {
		t.Errorf("path: got %q, want /rest/api/2/myself", gotPath)
	}
}

func TestCloudClient_PAT_SendsBearer(t *testing.T) {
	var gotAuth string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	})
	c := serverClient(ts, AuthMethodPAT, "pat-token-value")
	if _, err := c.TestAuth(context.Background()); err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if gotAuth != "Bearer pat-token-value" {
		t.Errorf("auth header = %q, want Bearer pat-token-value", gotAuth)
	}
}

func TestCloudClient_ServerMode_SearchTickets_StartAtPagination(t *testing.T) {
	var gotPath string
	var gotBody string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"startAt": 50,
			"maxResults": 50,
			"total": 120,
			"issues": [
				{"key":"P-51","fields":{"summary":"a","status":{},"project":{},"issuetype":{}}},
				{"key":"P-52","fields":{"summary":"b","status":{},"project":{},"issuetype":{}}}
			]
		}`))
	})
	c := serverClient(ts, AuthMethodPAT, "tok")
	res, err := c.SearchTickets(context.Background(), "project = P", "50", 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if gotPath != "/rest/api/2/search" {
		t.Errorf("path: got %q, want /rest/api/2/search", gotPath)
	}
	if !strings.Contains(gotBody, `"startAt":50`) {
		t.Errorf("body missing startAt=50: %q", gotBody)
	}
	if res.IsLast {
		t.Error("expected IsLast=false (52 of 120)")
	}
	if res.NextPageToken != "52" {
		t.Errorf("next page token = %q, want 52", res.NextPageToken)
	}
	if len(res.Tickets) != 2 || res.Tickets[0].Key != "P-51" {
		t.Errorf("tickets: %+v", res.Tickets)
	}
}

func TestCloudClient_SearchTickets_PreservesImmutableIssueID(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"issues": [{
				"id": "10042",
				"key": "ENG-42",
				"fields": {"summary": "Fix auth", "status": {}, "project": {}, "issuetype": {}}
			}],
			"isLast": true
		}`))
	})

	result, err := clientTo(ts, AuthMethodAPIToken, "token").SearchTickets(
		context.Background(), "summary ~ auth", "", 5,
	)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Tickets) != 1 || result.Tickets[0].ID != "10042" {
		t.Fatalf("tickets = %#v, want immutable issue ID", result.Tickets)
	}
}

// Jira Server can return fewer issues than maxResults on a non-terminal page
// (rate limits, filtered results). The pager must still advance — IsLast is
// driven by total, not by len(issues) < maxResults.
func TestCloudClient_ServerMode_SearchTickets_PartialBatchAdvances(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"startAt": 100,
			"maxResults": 50,
			"total": 110,
			"issues": [
				{"key":"P-101","fields":{"summary":"x","status":{},"project":{},"issuetype":{}}}
			]
		}`))
	})
	c := serverClient(ts, AuthMethodPAT, "tok")
	res, err := c.SearchTickets(context.Background(), "x", "100", 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if res.IsLast {
		t.Error("expected IsLast=false: startAt+len=101 < total=110")
	}
	if res.NextPageToken != "101" {
		t.Errorf("next page token = %q, want 101", res.NextPageToken)
	}
}

func TestCloudClient_ServerMode_SearchTickets_TerminalPage(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"startAt": 100,
			"maxResults": 50,
			"total": 102,
			"issues": [
				{"key":"P-101","fields":{"summary":"x","status":{},"project":{},"issuetype":{}}},
				{"key":"P-102","fields":{"summary":"y","status":{},"project":{},"issuetype":{}}}
			]
		}`))
	})
	c := serverClient(ts, AuthMethodPAT, "tok")
	res, err := c.SearchTickets(context.Background(), "x", "100", 50)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !res.IsLast {
		t.Error("expected IsLast=true on terminal page")
	}
	if res.NextPageToken != "" {
		t.Errorf("expected empty NextPageToken on terminal page, got %q", res.NextPageToken)
	}
}

func TestCloudClient_ServerMode_GetTicket_UsesV2(t *testing.T) {
	var paths []string
	ts := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/transitions"):
			_, _ = w.Write([]byte(`{"transitions":[]}`))
		default:
			_, _ = w.Write([]byte(`{"key":"P-1","fields":{"summary":"s","description":"plain text","status":{},"project":{},"issuetype":{}}}`))
		}
	})
	c := serverClient(ts, AuthMethodPAT, "tok")
	ticket, err := c.GetTicket(context.Background(), "P-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if ticket.Description != "plain text" {
		t.Errorf("description: %q", ticket.Description)
	}
	wantPrefix := "/rest/api/2/issue/P-1"
	for _, p := range paths {
		if !strings.HasPrefix(p, wantPrefix) {
			t.Errorf("path %q does not use v2 base", p)
		}
	}
}

func TestCloudClient_ServerMode_HtmlAuthHint_MentionsPAT(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		// Set headers before WriteHeader — once WriteHeader fires the response
		// headers are committed and any later Header().Set is silently dropped.
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<html><body><form action="/login.jsp"></form></body></html>`))
	})
	c := serverClient(ts, AuthMethodPAT, "bad")
	res, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatalf("test auth: %v", err)
	}
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Error, "Personal Access Token") {
		t.Errorf("expected PAT hint, got %q", res.Error)
	}
}

func TestCloudClient_CloudMode_APITokenOnServer_HintFlagsMismatch(t *testing.T) {
	ts := newMockServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`<html>login</html>`))
	})
	// Force the misconfiguration: instance=server but auth=api_token (which is
	// what existing users saw before this change). The hint should call out
	// the mismatch and direct them to switch to PAT.
	cfg := &JiraConfig{
		SiteURL:      ts.URL,
		Email:        "u@x",
		AuthMethod:   AuthMethodAPIToken,
		InstanceType: InstanceTypeServer,
	}
	c := NewCloudClient(cfg, "tok")
	res, _ := c.TestAuth(context.Background())
	if res.OK {
		t.Fatal("expected OK=false")
	}
	if !strings.Contains(res.Error, "Cloud-only") || !strings.Contains(res.Error, "PAT") {
		t.Errorf("hint should explain mismatch + suggest PAT, got %q", res.Error)
	}
}

func TestExtractDescription_Nil(t *testing.T) {
	if got := extractDescription(nil); got != "" {
		t.Errorf("nil → %q", got)
	}
}

func TestExtractDescription_UnknownShape(t *testing.T) {
	if got := extractDescription(42); got != "" {
		t.Errorf("int → %q", got)
	}
}
