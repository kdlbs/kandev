package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchWorkspaceContentUsesDedicatedEndpointAndDecodesMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/workspace/content-search" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "hello world" {
			t.Fatalf("query = %q, want %q", got, "hello world")
		}
		if got := r.URL.Query().Get("limit_per_repo"); got != "12" {
			t.Fatalf("limit_per_repo = %q, want 12", got)
		}
		if legacy := r.URL.Query().Get("limit"); legacy != "" {
			t.Fatalf("legacy limit query unexpectedly set to %q", legacy)
		}
		_ = json.NewEncoder(w).Encode(WorkspaceContentSearchResponse{
			Results: []WorkspaceContentSearchResult{{
				RepositoryName: "backend",
				Path:           "main.go",
				Line:           9,
				Column:         4,
				Preview:        "say hello world",
				MatchRanges:    []WorkspaceContentMatchRange{{Start: 4, End: 15}},
			}},
		})
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client(), logger: newTestLogger()}
	response, err := client.SearchWorkspaceContent(context.Background(), "hello world", 12)
	if err != nil {
		t.Fatalf("SearchWorkspaceContent returned error: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].RepositoryName != "backend" {
		t.Fatalf("response = %#v", response)
	}
}

func TestSearchWorkspaceContentRejectsNonOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad query", http.StatusBadRequest)
	}))
	defer server.Close()

	client := &Client{baseURL: server.URL, httpClient: server.Client(), logger: newTestLogger()}
	if _, err := client.SearchWorkspaceContent(context.Background(), "needle", 10); err == nil {
		t.Fatal("SearchWorkspaceContent returned nil error for non-OK response")
	}
}
