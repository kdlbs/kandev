package client

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types"
)

type workspaceRootFilesLister interface {
	ListWorkspaceRootFiles(context.Context) (*types.WorkspaceRootFiles, error)
}

func TestListWorkspaceRootFilesRequestsAndDecodesListing(t *testing.T) {
	srv, request := captureServer(t, jsonResponder(http.StatusOK, `{
		"supported":true,
		"files":[{"path":"dashboard.json","size":2,"modified_at":"2026-11-19T10:00:00Z"}],
		"truncated":false,
		"destinations":[{"repository_name":"frontend","path":"frontend"}]
	}`))
	lister, ok := any(newHTTPOnlyClient(srv.URL)).(workspaceRootFilesLister)
	if !ok {
		t.Fatal("client does not expose workspace root file listing")
	}
	listing, err := lister.ListWorkspaceRootFiles(context.Background())
	if err != nil {
		t.Fatalf("ListWorkspaceRootFiles: %v", err)
	}
	if request.Method != http.MethodGet || request.Path != "/api/v1/workspace/root-files" || request.RawQuery != "" {
		t.Fatalf("request = %s %s?%s, want GET /api/v1/workspace/root-files", request.Method, request.Path, request.RawQuery)
	}
	if !listing.Supported || len(listing.Files) != 1 || listing.Files[0].Path != "dashboard.json" || len(listing.Destinations) != 1 {
		t.Fatalf("listing = %+v, want dashboard.json and one destination", listing)
	}
}

func TestListWorkspaceRootFilesReportsHTTPFailure(t *testing.T) {
	srv, _ := captureServer(t, jsonResponder(http.StatusServiceUnavailable, `{"error":"unavailable"}`))
	lister, ok := any(newHTTPOnlyClient(srv.URL)).(workspaceRootFilesLister)
	if !ok {
		t.Fatal("client does not expose workspace root file listing")
	}
	_, err := lister.ListWorkspaceRootFiles(context.Background())
	if err == nil || !strings.Contains(err.Error(), "503") {
		t.Fatalf("ListWorkspaceRootFiles error = %v, want status 503", err)
	}
}
