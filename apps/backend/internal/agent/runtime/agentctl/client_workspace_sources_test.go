package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestMaterializeRepository_ReturnsTypedRemoteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		var body MaterializeRepositoryRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.BaseBranch != "main" || body.CheckoutBranch != "feature/work" {
			t.Fatalf("branches = base:%q checkout:%q", body.BaseBranch, body.CheckoutBranch)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"destination already exists"}`))
	}))
	defer server.Close()

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: server.URL, httpClient: server.Client(), logger: log}
	c.httpClient.Transport = &authTransport{token: "test-token", base: c.httpClient.Transport}
	_, err = c.MaterializeRepository(context.Background(), MaterializeRepositoryRequest{
		RepositoryURL:  "https://github.com/kdlbs/kandev.git",
		Destination:    "kandev",
		BaseBranch:     "main",
		CheckoutBranch: "feature/work",
	})

	var remoteErr *WorkspaceMaterializationError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("expected WorkspaceMaterializationError, got %v", err)
	}
	if remoteErr.StatusCode != http.StatusConflict || remoteErr.Message != "destination already exists" {
		t.Fatalf("error = %#v", remoteErr)
	}
}

func TestRemoveMaterializedRepository_UsesCleanupEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/materialize-repository/remove" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"removed":true}`))
	}))
	defer server.Close()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: server.URL, httpClient: server.Client(), logger: log}

	err = c.RemoveMaterializedRepository(context.Background(), RemoveMaterializedRepositoryRequest{
		RepositoryURL: "https://github.com/kdlbs/kandev.git",
		Destination:   "kandev",
	})
	if err != nil {
		t.Fatalf("RemoveMaterializedRepository: %v", err)
	}
}

func TestRescanWorkspace_UsesCurrentRootWhenWorkDirIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/rescan" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			WorkDir string `json:"work_dir"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.WorkDir != "" {
			t.Fatalf("work_dir = %q, want current root", body.WorkDir)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: server.URL, httpClient: server.Client(), logger: log}
	if err := c.RescanWorkspace(context.Background(), ""); err != nil {
		t.Fatalf("RescanWorkspace: %v", err)
	}
}

func TestReconcileWorkspace_UsesExactReconciliationEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/reconcile" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			WorkspaceSourceRoots []string `json:"workspace_source_roots"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.WorkspaceSourceRoots) != 1 || body.WorkspaceSourceRoots[0] != "/sources/original" {
			t.Fatalf("workspace_source_roots = %v", body.WorkspaceSourceRoots)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: server.URL, httpClient: server.Client(), logger: log}
	if err := c.ReconcileWorkspace(context.Background(), []string{"/sources/original"}); err != nil {
		t.Fatalf("ReconcileWorkspace: %v", err)
	}
}

func TestRebindWorkspace_UsesAuthenticatedRebindEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/workspace/rebind" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	c := &Client{baseURL: server.URL, httpClient: server.Client(), logger: log}
	if err := c.RebindWorkspace(context.Background(), "/workspace/task-1"); err != nil {
		t.Fatalf("RebindWorkspace: %v", err)
	}
}
