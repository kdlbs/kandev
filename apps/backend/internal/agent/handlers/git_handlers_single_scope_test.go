package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-EXECUTORS-PLUGIN-002.4, AC-PLATFORM-GIT-CAPTURE-RECOVERY-001.5
func TestWsCumulativeDiffSingleNonRootScopeFallback(t *testing.T) {
	const rootUnavailable = `{"success":false,"status_state":"unavailable","detail_state":"unavailable","error_code":"repository_unavailable"}`
	const rootReady = `{"success":true,"status_state":"ready","detail_state":"ready","base_commit":"root-base"}`
	const scopedReady = `{"success":true,"status_state":"ready","detail_state":"ready","base_commit":"scoped-base"}`

	tests := []struct {
		name               string
		rootBody           string
		scopesBody         string
		wantCumulativeDiff bool
		wantCumulativeBase string
		wantRequests       []string
	}{
		{
			name:     "root repository uses one detailed status request",
			rootBody: rootReady, wantCumulativeDiff: true, wantCumulativeBase: "root-base",
			wantRequests: []string{
				"/api/v1/git/status?fresh=true&details=wait",
				"/api/v1/git/cumulative-diff?base=root-base",
			},
		},
		{
			name:     "single non-root scope supplies the base commit",
			rootBody: rootUnavailable, scopesBody: `{"scopes":["owner-repo-main"]}`,
			wantCumulativeDiff: true, wantCumulativeBase: "scoped-base",
			wantRequests: []string{
				"/api/v1/git/status?fresh=true&details=wait",
				"/api/v1/git/turn-changes/scopes",
				"/api/v1/git/status?fresh=true&details=wait&repo=owner-repo-main",
				"/api/v1/git/cumulative-diff?base=scoped-base",
			},
		},
		{
			name:     "multiple scopes keep cumulative diff unavailable",
			rootBody: rootUnavailable, scopesBody: `{"scopes":["owner-repo-main","other-repo-main"]}`,
			wantRequests: []string{
				"/api/v1/git/status?fresh=true&details=wait",
				"/api/v1/git/turn-changes/scopes",
			},
		},
		{
			name:     "no scopes keep cumulative diff unavailable",
			rootBody: rootUnavailable, scopesBody: `{"scopes":[]}`,
			wantRequests: []string{
				"/api/v1/git/status?fresh=true&details=wait",
				"/api/v1/git/turn-changes/scopes",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan string, 8)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := r.URL.Path
				if r.URL.RawQuery != "" {
					request += "?" + r.URL.RawQuery
				}
				requests <- request
				status, body := http.StatusNotFound, `{"error":"unexpected request"}`
				switch r.URL.Path {
				case "/api/v1/git/status":
					if r.URL.Query().Get("repo") == "" {
						status, body = http.StatusOK, test.rootBody
					} else if r.URL.Query().Get("repo") == "owner-repo-main" {
						status, body = http.StatusOK, scopedReady
					}
				case "/api/v1/git/turn-changes/scopes":
					status, body = http.StatusOK, test.scopesBody
				case "/api/v1/git/cumulative-diff":
					status, body = http.StatusOK, `{}`
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)

			parsedURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			port, err := strconv.Atoi(parsedURL.Port())
			if err != nil {
				t.Fatal(err)
			}
			client := agentctl.NewClient(parsedURL.Hostname(), port, newTestLogger())
			t.Cleanup(client.Close)
			execution := &lifecycle.AgentExecution{ID: "execution", SessionID: "session", TaskID: "task"}
			execution.SetAgentCtlClientForTesting(client)
			lookup := &mockExecutionLookup{executions: map[string]*lifecycle.AgentExecution{"session": execution}}
			handler := NewGitHandlers(lookup, nil, newTestLogger())
			message, err := ws.NewRequest("request-1", ws.ActionSessionCumulativeDiff, CumulativeDiffRequest{SessionID: "session"})
			if err != nil {
				t.Fatal(err)
			}

			response, err := handler.wsCumulativeDiff(context.Background(), message)
			if err != nil {
				t.Fatalf("wsCumulativeDiff: %v", err)
			}
			var payload map[string]any
			if err := json.Unmarshal(response.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["cumulative_diff"] != nil; got != test.wantCumulativeDiff {
				t.Errorf("cumulative_diff present = %v, want %v", got, test.wantCumulativeDiff)
			}

			gotRequests := make([]string, 0, len(test.wantRequests))
			for range test.wantRequests {
				select {
				case request := <-requests:
					gotRequests = append(gotRequests, request)
				default:
					t.Fatalf("received %d requests, want %d", len(gotRequests), len(test.wantRequests))
				}
			}
			select {
			case extra := <-requests:
				t.Fatalf("unexpected request: %s", extra)
			default:
			}
			if !reflect.DeepEqual(gotRequests, test.wantRequests) {
				t.Errorf("requests = %v, want %v", gotRequests, test.wantRequests)
			}
			if test.wantCumulativeDiff {
				want := fmt.Sprintf("/api/v1/git/cumulative-diff?base=%s", test.wantCumulativeBase)
				if got := gotRequests[len(gotRequests)-1]; got != want {
					t.Errorf("cumulative diff request = %q, want %q", got, want)
				}
			}
		})
	}
}
