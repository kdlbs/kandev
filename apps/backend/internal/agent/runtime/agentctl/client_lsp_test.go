package client

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

func TestTaskLSPControlUsesLanguageRouteAndServerOwnedBody(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotInstanceID string
	var gotBody map[string]any
	want := sharedlsp.RuntimeSnapshot{Language: "python", Generation: 4, Phase: sharedlsp.PhaseReady}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotAuth, gotInstanceID = r.Header.Get("Authorization"), r.Header.Get("X-Instance-ID")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(want)
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger(), WithAuthToken("secret-token"), WithExecutionID("exec-1"))

	got, err := client.StartTaskLSP(context.Background(), sharedlsp.TaskHostStartRequest{
		Language: "python", Generation: 4, AutoInstall: true,
		Configuration: json.RawMessage(`{"python":{"analysis":{"typeCheckingMode":"strict"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/lsp/languages/python/start" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer secret-token" || gotInstanceID != "exec-1" {
		t.Fatalf("headers auth=%q instance=%q", gotAuth, gotInstanceID)
	}
	if gotBody["language"] != nil || gotBody["task_id"] != nil || gotBody["session_id"] != nil {
		t.Fatalf("ownership leaked into body: %v", gotBody)
	}
	if gotBody["generation"] != float64(4) || gotBody["auto_install"] != true {
		t.Fatalf("control body = %v", gotBody)
	}
	if got.Language != want.Language || got.Generation != want.Generation || got.Phase != want.Phase {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestTaskLSPConfigurationUsesTaskHostGenerationRoute(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(sharedlsp.RuntimeSnapshot{
			Language: "kotlin", Generation: 7, Phase: sharedlsp.PhaseReady,
		})
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger())

	got, err := client.UpdateTaskLSPConfiguration(context.Background(), sharedlsp.TaskHostConfigurationRequest{
		Language: "kotlin", Generation: 7,
		Configuration: json.RawMessage(`{"kotlin":{"compiler":{"jvmTarget":"21"}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/lsp/languages/kotlin/configuration" || gotBody["generation"] != float64(7) {
		t.Fatalf("request path=%q body=%v", gotPath, gotBody)
	}
	if gotBody["task_id"] != nil || gotBody["session_id"] != nil || got.Generation != 7 {
		t.Fatalf("ownership leaked or response wrong: body=%v snapshot=%#v", gotBody, got)
	}
}

func TestDialTaskLSPAttachUsesGenerationAndAuthHeaders(t *testing.T) {
	var gotPath, gotGeneration, gotAuth, gotInstanceID string
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotGeneration = r.URL.Path, r.URL.Query().Get("generation")
		gotAuth, gotInstanceID = r.Header.Get("Authorization"), r.Header.Get("X-Instance-ID")
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		_ = conn.Close()
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger(), WithAuthToken("secret-token"), WithExecutionID("exec-1"))

	conn, _, err := client.DialTaskLSPAttach(context.Background(), "kotlin", 9)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if gotPath != "/api/v1/lsp/languages/kotlin/attach" || gotGeneration != "9" {
		t.Fatalf("attach path=%q generation=%q", gotPath, gotGeneration)
	}
	if gotAuth != "Bearer secret-token" || gotInstanceID != "exec-1" {
		t.Fatalf("headers auth=%q instance=%q", gotAuth, gotInstanceID)
	}
}

func TestWatchTaskLSPStreamsSnapshotsUntilContextCancellation(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/lsp/languages/go/watch" {
			t.Errorf("watch path = %q", r.URL.Path)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteJSON(sharedlsp.RuntimeSnapshot{Language: "go", Generation: 2, Phase: sharedlsp.PhaseInitializing})
		_, _, _ = conn.ReadMessage()
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger(), WithAuthToken("secret"))
	ctx, cancel := context.WithCancel(context.Background())
	updates := make(chan sharedlsp.RuntimeSnapshot, 1)
	done := make(chan error, 1)
	go func() {
		done <- client.WatchTaskLSP(ctx, "go", func(snapshot sharedlsp.RuntimeSnapshot) error {
			updates <- snapshot
			cancel()
			return nil
		})
	}()
	select {
	case snapshot := <-updates:
		if snapshot.Generation != 2 || snapshot.Phase != sharedlsp.PhaseInitializing {
			t.Fatalf("snapshot = %#v", snapshot)
		}
	case <-time.After(time.Second):
		t.Fatal("watch snapshot timed out")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error = %v", err)
	}
}

func TestDiscoverLSPUsesReadOnlyTaskHostRouteWithAuthHeaders(t *testing.T) {
	var gotMethod, gotPath, gotAuth, gotInstanceID string
	want := sharedlsp.DiscoveryResult{
		Languages: []string{"go", "kotlin"},
		State:     sharedlsp.DetectionComplete,
		ScannedAt: time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotInstanceID = r.Header.Get("X-Instance-ID")
		_ = json.NewEncoder(w).Encode(want)
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger(),
		WithAuthToken("secret-token"),
		WithExecutionID("exec-1"),
	)

	got, err := client.DiscoverLSP(context.Background())
	if err != nil {
		t.Fatalf("DiscoverLSP: %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/lsp/discovery" {
		t.Fatalf("discovery request = %s %s", gotMethod, gotPath)
	}
	if gotAuth != "Bearer secret-token" || gotInstanceID != "exec-1" {
		t.Fatalf("discovery headers auth=%q instance=%q", gotAuth, gotInstanceID)
	}
	if !slices.Equal(got.Languages, want.Languages) || got.State != want.State ||
		!got.ScannedAt.Equal(want.ScannedAt) {
		t.Fatalf("discovery response = %#v", got)
	}
}

func TestRefreshTaskLSPWorkspaceUsesTaskHostRoute(t *testing.T) {
	var gotMethod, gotPath string
	want := sharedlsp.WorkspaceUpdateResult{
		DynamicLanguages: []string{"go"},
		WorkspaceFolders: []sharedlsp.WorkspaceFolder{{URI: "file:///task/repo", Name: "repo"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewEncoder(w).Encode(want)
	}))
	t.Cleanup(server.Close)
	host, port := testServerAddress(t, server.URL)
	client := NewClient(host, port, newTestLogger(), WithAuthToken("secret"))

	got, err := client.RefreshTaskLSPWorkspace(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/lsp/workspace/refresh" ||
		!slices.Equal(got.DynamicLanguages, want.DynamicLanguages) {
		t.Fatalf("request=%s %s result=%#v", gotMethod, gotPath, got)
	}
}

func testServerAddress(t *testing.T, rawURL string) (string, int) {
	t.Helper()
	hostPort := strings.TrimPrefix(rawURL, "http://")
	host, portString, err := net.SplitHostPort(hostPort)
	if err != nil {
		t.Fatalf("parse test server host: %v", err)
	}
	port, err := strconv.Atoi(portString)
	if err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return host, port
}
