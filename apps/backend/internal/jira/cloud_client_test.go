package jira

import (
	"context"
	"encoding/base64"
	"errors"
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
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
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
