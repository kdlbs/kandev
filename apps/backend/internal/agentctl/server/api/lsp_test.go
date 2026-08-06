package api

import (
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
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("task LSP stop status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestTaskLSPWatchDisconnectIsNonOwning(t *testing.T) {
	server := newTestServer(t)
	httpServer := httptest.NewServer(server.router)
	t.Cleanup(httpServer.Close)
	watchURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/lsp/languages/kotlin/watch"
	conn, _, err := websocket.DefaultDialer.Dial(watchURL, nil)
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
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"phase":"off"`) {
		t.Fatalf("snapshot after watch close status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLSPAttachRouteIsRegisteredAndRequiresReadyGeneration(t *testing.T) {
	server := newTestServer(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/lsp/languages/kotlin/attach?generation=1", nil)
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
	response := httptest.NewRecorder()
	server.router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"kotlin"`) {
		t.Fatalf("LSP discovery status=%d body=%s", response.Code, response.Body.String())
	}
	if processes := server.procMgr.ListProcesses(""); len(processes) != 0 {
		t.Fatalf("discovery started processes: %#v", processes)
	}
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
