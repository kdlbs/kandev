package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

type gitStatusFallbackRequest struct {
	path    string
	fresh   string
	details string
	repo    string
}

func TestGetGitStatusWithDetailsSingleNonRootScopeFallback(t *testing.T) {
	const rootUnavailable = `{"success":false,"status_state":"unavailable","detail_state":"unavailable","error_code":"repository_unavailable"}`
	const rootReady = `{"success":true,"status_state":"ready","detail_state":"ready","base_commit":"root-base"}`
	const scopedReady = `{"success":true,"status_state":"ready","detail_state":"ready","repository_name":"owner/repo","base_commit":"scoped-base"}`
	const scopedUnavailable = `{"success":false,"status_state":"unavailable","detail_state":"unavailable","error_code":"details_unavailable"}`

	tests := []struct {
		name             string
		rootStatus       int
		rootBody         string
		scopesStatus     int
		scopesBody       string
		scopedStatus     int
		scopedBody       string
		wantRequests     []gitStatusFallbackRequest
		wantBaseCommit   string
		wantErrorCode    string
		wantErrorMessage string
	}{
		{
			name:       "root repository succeeds with one request",
			rootStatus: http.StatusOK, rootBody: rootReady,
			wantRequests:   []gitStatusFallbackRequest{{path: "/api/v1/git/status", fresh: "true", details: "wait"}},
			wantBaseCommit: "root-base",
		},
		{
			name:       "single non-root scope succeeds",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusOK, scopesBody: `{"scopes":["owner-repo-main"]}`,
			scopedStatus: http.StatusOK, scopedBody: scopedReady,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
				{path: "/api/v1/git/status", fresh: "true", details: "wait", repo: "owner-repo-main"},
			},
			wantBaseCommit: "scoped-base",
		},
		{
			name:       "scoped details error is preserved",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusOK, scopesBody: `{"scopes":["owner-repo-main"]}`,
			scopedStatus: http.StatusOK, scopedBody: scopedUnavailable,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
				{path: "/api/v1/git/status", fresh: "true", details: "wait", repo: "owner-repo-main"},
			},
			wantErrorCode: "details_unavailable", wantErrorMessage: "details_unavailable",
		},
		{
			name:       "no scopes preserves root result",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusOK, scopesBody: `{"scopes":[]}`,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
			},
			wantErrorCode: "repository_unavailable", wantErrorMessage: "repository_unavailable",
		},
		{
			name:       "multiple scopes preserve root result",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusOK, scopesBody: `{"scopes":["owner-repo-main","other-repo-main"]}`,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
			},
			wantErrorCode: "repository_unavailable", wantErrorMessage: "repository_unavailable",
		},
		{
			name:       "empty scope preserves root result",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusOK, scopesBody: `{"scopes":[""]}`,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
			},
			wantErrorCode: "repository_unavailable", wantErrorMessage: "repository_unavailable",
		},
		{
			name:       "scope endpoint not found preserves root result",
			rootStatus: http.StatusOK, rootBody: rootUnavailable,
			scopesStatus: http.StatusNotFound, scopesBody: `{"error":"not found"}`,
			wantRequests: []gitStatusFallbackRequest{
				{path: "/api/v1/git/status", fresh: "true", details: "wait"},
				{path: "/api/v1/git/turn-changes/scopes"},
			},
			wantErrorCode: "repository_unavailable", wantErrorMessage: "repository_unavailable",
		},
		{
			name:       "other root detail error does not query scopes",
			rootStatus: http.StatusOK, rootBody: scopedUnavailable,
			wantRequests:  []gitStatusFallbackRequest{{path: "/api/v1/git/status", fresh: "true", details: "wait"}},
			wantErrorCode: "details_unavailable", wantErrorMessage: "details_unavailable",
		},
		{
			name:       "root HTTP error does not query scopes",
			rootStatus: http.StatusInternalServerError, rootBody: `{"error":"server failure"}`,
			wantRequests:     []gitStatusFallbackRequest{{path: "/api/v1/git/status", fresh: "true", details: "wait"}},
			wantErrorMessage: "status 500",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := make(chan gitStatusFallbackRequest, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				request := gitStatusFallbackRequest{
					path: r.URL.Path, fresh: r.URL.Query().Get("fresh"),
					details: r.URL.Query().Get("details"), repo: r.URL.Query().Get("repo"),
				}
				requests <- request
				status, body := http.StatusNotFound, `{"error":"unexpected request"}`
				switch {
				case r.URL.Path == "/api/v1/git/status" && request.repo == "":
					status, body = test.rootStatus, test.rootBody
				case r.URL.Path == "/api/v1/git/turn-changes/scopes":
					status, body = test.scopesStatus, test.scopesBody
				case r.URL.Path == "/api/v1/git/status" && request.repo != "":
					status, body = test.scopedStatus, test.scopedBody
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)

			result, err := newHTTPOnlyClient(server.URL).GetGitStatusWithDetails(context.Background())
			if test.wantErrorMessage == "" && err != nil {
				t.Fatalf("GetGitStatusWithDetails() error = %v", err)
			}
			if test.wantErrorMessage != "" && (err == nil || !containsText(err.Error(), test.wantErrorMessage)) {
				t.Fatalf("GetGitStatusWithDetails() error = %v, want message containing %q", err, test.wantErrorMessage)
			}
			if result == nil {
				t.Fatal("GetGitStatusWithDetails() returned nil result")
			}
			if result.BaseCommit != test.wantBaseCommit {
				t.Errorf("base_commit = %q, want %q", result.BaseCommit, test.wantBaseCommit)
			}
			if result.ErrorCode != test.wantErrorCode {
				t.Errorf("error_code = %q, want %q", result.ErrorCode, test.wantErrorCode)
			}

			gotRequests := make([]gitStatusFallbackRequest, 0, len(test.wantRequests))
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
				t.Fatalf("unexpected extra request: %+v", extra)
			default:
			}
			if !reflect.DeepEqual(gotRequests, test.wantRequests) {
				t.Errorf("requests = %+v, want %+v", gotRequests, test.wantRequests)
			}
		})
	}
}

func containsText(value, fragment string) bool {
	return fragment == "" || strings.Contains(value, fragment)
}
