package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestTaskLSPControlSnapshotRouteIsRegistered(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/languages/kotlin", nil)
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("task LSP snapshot status = %d, want 200", response.Code)
	}
}

func TestTaskLSPControlRoutesRejectTransportOwnershipFields(t *testing.T) {
	server := newTestServer(t)
	for _, action := range []string{"start", "restart", "configuration"} {
		request := httptest.NewRequest(
			http.MethodPost, "/api/v1/lsp/languages/kotlin/"+action,
			strings.NewReader(`{"generation":1,"task_id":"untrusted"}`),
		)
		request.Header.Set("Content-Type", "application/json")
		setTaskLSPTaskHeader(request)
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s injected ownership status = %d, want 400", action, response.Code)
		}
	}
}

func TestTaskLSPConfigurationRouteValidatesLiveGeneration(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/lsp/languages/kotlin/configuration",
		strings.NewReader(`{"generation":1,"configuration":{"kotlin":{"compiler":{"jvmTarget":"21"}}}}`),
	)
	request.Header.Set("Content-Type", "application/json")
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("task LSP configuration status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestTaskLSPStopRouteIsRegistered(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/lsp/languages/kotlin/stop", strings.NewReader(`{"reason":"user"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("task LSP stop status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestTaskLSPPurgeRouteClearsOnlyTrustedTaskState(t *testing.T) {
	server := newTestServer(t)
	taskWorkspace := filepath.Join(server.cfg.WorkDir, "borrower")
	if _, err := server.lspManager.UpdateWorkspaceForTask("task-1", taskWorkspace, nil); err != nil {
		t.Fatal(err)
	}
	if got := server.lspManager.SnapshotForTask("task-1", "kotlin").WorkspacePath; got != taskWorkspace {
		t.Fatalf("task workspace before purge = %q", got)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/lsp/task", nil)
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("task LSP purge status = %d body=%s", response.Code, response.Body.String())
	}
	if got := server.lspManager.SnapshotForTask("task-1", "kotlin").WorkspacePath; got != server.cfg.WorkDir {
		t.Fatalf("task workspace after purge = %q, want default %q", got, server.cfg.WorkDir)
	}
}

func TestTaskLSPWatchDisconnectIsNonOwning(t *testing.T) {
	server := newTestServer(t)
	httpServer := httptest.NewServer(server.router)
	t.Cleanup(httpServer.Close)
	watchURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/lsp/languages/kotlin/watch"
	headers := http.Header{}
	headers.Set(taskLSPTaskIDHeader, "task-1")
	conn, _, err := websocket.DefaultDialer.Dial(watchURL, headers)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, snapshot, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(snapshot), `"phase":"off"`) {
		t.Fatalf("watch snapshot = %s", snapshot)
	}
	_ = conn.Close()

	request := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/languages/kotlin", nil)
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"phase":"off"`) {
		t.Fatalf("snapshot after watch close status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLSPAttachRouteIsRegisteredAndRequiresReadyGeneration(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/languages/kotlin/attach?generation=1", nil)
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("task LSP attach status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestLSPDiscoveryRouteDetectsWithoutStartingProcess(t *testing.T) {
	server := newTestServer(t)
	if err := os.WriteFile(filepath.Join(server.cfg.WorkDir, "Main.kt"), []byte("class Main"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/discovery", nil)
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kotlin"`) {
		t.Fatalf("LSP discovery status=%d body=%s", response.Code, response.Body.String())
	}
	if processes := server.procMgr.ListProcesses(""); len(processes) != 0 {
		t.Fatalf("discovery started processes: %#v", processes)
	}
}

func TestTaskLSPWorkspaceRefreshScopesTaskWithoutRebindingPhysicalTrackers(t *testing.T) {
	server := newTestServer(t)
	t.Cleanup(func() { _ = server.procMgr.Stop(context.Background()) })
	for _, name := range []string{"repo-a", "repo-b"} {
		initWorkspaceGitRepo(t, filepath.Join(server.cfg.WorkDir, name))
	}
	if got := server.procMgr.RepoSubpaths(); len(got) != 0 {
		t.Fatalf("initial repository subpaths = %v, want none", got)
	}

	payload, err := json.Marshal(map[string]any{
		"workspace_path": server.cfg.WorkDir,
		"workspace_roots": []string{
			filepath.Join(server.cfg.WorkDir, "repo-a"),
			filepath.Join(server.cfg.WorkDir, "repo-b"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost, "/api/v1/lsp/workspace/refresh", strings.NewReader(string(payload)),
	)
	request.Header.Set("Content-Type", "application/json")
	setTaskLSPTaskHeader(request)
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("workspace refresh status=%d body=%s", response.Code, response.Body.String())
	}
	if got := server.procMgr.RepoSubpaths(); len(got) != 0 {
		t.Fatalf("physical repository trackers changed during task LSP refresh: %v", got)
	}
	snapshot := server.lspManager.SnapshotForTask("task-1", "kotlin")
	if len(snapshot.WorkspaceFolders) != 2 || snapshot.WorkspaceFolders[0].Name != "repo-a" ||
		snapshot.WorkspaceFolders[1].Name != "repo-b" {
		t.Fatalf("task-scoped LSP workspace = %#v", snapshot.WorkspaceFolders)
	}
}

func TestTaskLSPRoutesRequireTrustedTaskIdentityHeader(t *testing.T) {
	server := newTestServer(t)
	for _, test := range []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/lsp/languages/kotlin"},
		{method: http.MethodDelete, path: "/api/v1/lsp/task"},
	} {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		server.router.ServeHTTP(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("%s %s missing task identity status = %d, want 400", test.method, test.path, response.Code)
		}
	}
}

func setTaskLSPTaskHeader(request *http.Request) {
	request.Header.Set(taskLSPTaskIDHeader, "task-1")
}

func TestWorkspaceFileURI(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{path: "/task root/A#B?100%.kt", want: "file:///task%20root/A%23B%3F100%25.kt"},
		{path: "/task:root/A!B(C):D.kt", want: "file:///task%3Aroot/A%21B%28C%29%3AD.kt"},
		{path: `C:\Task Root\src\Main.kt`, want: "file:///C:/Task%20Root/src/Main.kt"},
		{path: `\\build-server\work share\Main.kt`, want: "file://build-server/work%20share/Main.kt"},
		{path: `/task\name/Main.kt`, want: "file:///task%5Cname/Main.kt"},
	}
	for _, test := range tests {
		if got := workspaceFileURI(test.path); got != test.want {
			t.Fatalf("workspaceFileURI(%q)=%q want=%q", test.path, got, test.want)
		}
	}
}

func TestTaskLSPWorkspaceFoldersAreOrderedContainedRepositoryRoots(t *testing.T) {
	folders := taskLSPWorkspaceFolders("/workspace/task", "repo-b", "../escape", "repo-a", "repo-b")
	if len(folders) != 2 || folders[0].Name != "repo-b" || folders[1].Name != "repo-a" {
		t.Fatalf("workspace folders = %#v", folders)
	}
	for _, folder := range folders {
		if strings.Contains(folder.URI, "escape") {
			t.Fatalf("workspace folder escaped task root: %#v", folders)
		}
	}
}
