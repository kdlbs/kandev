package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestSetMcpProvidersForSessionCallsLiveAgentctl(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/mcp/providers" {
			t.Errorf("request = %s %s, want PUT /api/v1/mcp/providers", r.Method, r.URL.Path)
		}
		var body struct {
			Providers []string `json:"mcp_providers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		got = body.Providers
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	mgr, execution := workspaceSourceTestManager(t, server.URL, nil)
	providers := []string{"github", "gitlab"}
	if err := mgr.SetMcpProvidersForSession(context.Background(), execution.SessionID, providers); err != nil {
		t.Fatalf("SetMcpProvidersForSession: %v", err)
	}
	if !reflect.DeepEqual(got, providers) {
		t.Fatalf("agentctl providers = %v, want %v", got, providers)
	}
}

type blockingRemoteRefresher struct {
	entered chan struct{}
	release chan struct{}
}

func (r *blockingRemoteRefresher) RefreshRemoteInstance(context.Context, *ExecutorInstance) (*RemoteInstanceRefresh, error) {
	close(r.entered)
	<-r.release
	return nil, nil
}

func TestSetMcpProvidersForSessionWaitsForRemoteRefresh(t *testing.T) {
	providerRequests := make(chan []string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Providers []string `json:"mcp_providers"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		providerRequests <- body.Providers
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	mgr, execution := workspaceSourceTestManager(t, server.URL, nil)
	profile := builtinDispatchProfile(mcpprofile.PackageCore)
	encoded, err := mcpprofile.MarshalBuiltinBinding(profile)
	require.NoError(t, err)
	execution.setMetadataValue(mcpprofile.BuiltinBindingMetadataKey, encoded)

	refresher := &blockingRemoteRefresher{entered: make(chan struct{}), release: make(chan struct{})}
	refreshDone := make(chan error, 1)
	var releaseOnce sync.Once
	releaseRefresh := func() { releaseOnce.Do(func() { close(refresher.release) }) }
	refreshFinished := false
	t.Cleanup(func() {
		releaseRefresh()
		if !refreshFinished {
			select {
			case <-refreshDone:
			case <-time.After(time.Second):
				t.Error("remote refresh did not finish after releasing its test barrier")
			}
		}
	})
	go func() {
		refreshDone <- mgr.refreshTrackedRemoteInstanceOnce(context.Background(), execution, refresher)
	}()
	<-refresher.entered

	updateDone := make(chan error, 1)
	updateStarted := make(chan struct{})
	go func() {
		close(updateStarted)
		updateDone <- mgr.SetMcpProvidersForSession(context.Background(), execution.SessionID, []string{"gitlab"})
	}()
	<-updateStarted
	select {
	case providers := <-providerRequests:
		t.Fatalf("provider update reached agentctl during remote refresh: %v", providers)
	case err := <-updateDone:
		t.Fatalf("provider update completed during remote refresh: %v", err)
	case <-time.After(time.Second):
	}

	releaseRefresh()
	require.NoError(t, <-refreshDone)
	refreshFinished = true
	require.NoError(t, <-updateDone)
	require.Equal(t, []string{"gitlab"}, <-providerRequests)
	value, present := execution.metadataValue(mcpprofile.BuiltinBindingMetadataKey)
	require.True(t, present)
	updated, err := mcpprofile.ParseBuiltinBindingMetadata(value)
	require.NoError(t, err)
	require.Equal(t, []string{"gitlab"}, updated.Providers)
}

func TestSetMcpProvidersForSession_NoExecutionIsNoOp(t *testing.T) {
	mgr := &Manager{executionStore: NewExecutionStore(), logger: newTestLogger()}
	if err := mgr.SetMcpProvidersForSession(context.Background(), "session-missing", []string{"gitlab"}); err != nil {
		t.Fatalf("SetMcpProvidersForSession without execution: %v", err)
	}
}

func TestSetPluginToolsForAllExecutionsCallsLiveAgentctl(t *testing.T) {
	got := make(chan plugintools.Snapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/v1/mcp/plugin-tools" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		var snapshot plugintools.Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			t.Error(err)
		}
		got <- snapshot
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	mgr, _ := workspaceSourceTestManager(t, server.URL, nil)
	want := plugintools.Snapshot{Generation: "g", Revision: 3}
	if err := mgr.SetPluginToolsForAllExecutions(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	if snapshot := <-got; snapshot.Generation != want.Generation || snapshot.Revision != want.Revision {
		t.Fatalf("snapshot = %#v, want %#v", snapshot, want)
	}
}

func TestSetPluginToolsForAllExecutionsProjectsCoordinatorSurface(t *testing.T) {
	got := make(chan plugintools.Snapshot, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var snapshot plugintools.Snapshot
		if err := json.NewDecoder(r.Body).Decode(&snapshot); err != nil {
			t.Error(err)
		}
		got <- snapshot
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	mgr, execution := workspaceSourceTestManager(t, server.URL, nil)
	execution.setMetadataValue(mcpprofile.CoordinatorToolPolicyMetadataKey, `{"version":2}`)

	want := plugintools.Snapshot{Generation: "g", Revision: 3, Tools: []plugintools.Definition{
		{PluginID: "provider", LocalName: "usage", ExposedName: "kandev_provider_usage", Description: "usage",
			Surfaces: []string{plugintools.SurfaceKanban, plugintools.SurfaceCoordinator}, ReadOnlyHint: true,
			InvocationScope: plugintools.InvocationScopeInstance, InputSchema: []byte(`{"type":"object"}`)},
	}}
	if err := mgr.SetPluginToolsForAllExecutions(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	snapshot := <-got
	if len(snapshot.Tools) != 1 || len(snapshot.Tools[0].Surfaces) != 1 || snapshot.Tools[0].Surfaces[0] != plugintools.SurfaceCoordinator {
		t.Fatalf("coordinator projection = %#v", snapshot.Tools)
	}
}
