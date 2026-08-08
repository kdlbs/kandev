package hostutility

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/stretchr/testify/require"
)

func TestModelConfigCacheKeyCanonicalizesOptionMap(t *testing.T) {
	first := ModelConfigResolutionRequest{
		Model: "model",
		Mode:  "build",
		ConfigOptions: map[string]string{
			"zeta":  "last",
			"alpha": "first",
		},
	}
	second := ModelConfigResolutionRequest{
		Model: "model",
		Mode:  "build",
		ConfigOptions: map[string]string{
			"alpha": "first",
			"zeta":  "last",
		},
	}

	require.Equal(t, modelConfigCacheKey("agent", first), modelConfigCacheKey("agent", second))
	require.NotEqual(t, modelConfigCacheKey("other-agent", first), modelConfigCacheKey("agent", second))
}

func TestModelConfigCacheExpiresAndClonesValues(t *testing.T) {
	cache := newModelConfigCache()
	now := time.Unix(100, 0)
	resolution := ModelConfigResolution{
		AgentType: "agent",
		Model:     "model",
		Status:    StatusOK,
		ConfigOptions: []ConfigOption{{
			ID:      "effort",
			Options: []ConfigOptionChoice{{Value: "high"}},
		}},
	}
	cache.set("key", resolution, now)

	got, ok := cache.get("key", now.Add(time.Second))
	require.True(t, ok)
	got.ConfigOptions[0].Options[0].Value = "mutated"

	again, ok := cache.get("key", now.Add(time.Second))
	require.True(t, ok)
	require.Equal(t, "high", again.ConfigOptions[0].Options[0].Value)

	_, ok = cache.get("key", now.Add(modelConfigCacheTTL))
	require.False(t, ok)
}

func TestModelConfigCacheSetSweepsExpiredEntriesAndCapsSize(t *testing.T) {
	cache := newModelConfigCache()
	now := time.Unix(100, 0)
	cache.set("expired", ModelConfigResolution{AgentType: "agent"}, now.Add(-modelConfigCacheTTL))
	cache.set("fresh", ModelConfigResolution{AgentType: "agent"}, now)

	cache.mu.RLock()
	if _, ok := cache.items["expired"]; ok {
		cache.mu.RUnlock()
		t.Fatal("expired entry remains after a set")
	}
	cache.mu.RUnlock()

	for i := 0; i < modelConfigCacheMaxEntries+1; i++ {
		cache.set("key-"+fmt.Sprint(i), ModelConfigResolution{AgentType: "agent"}, now)
	}

	cache.mu.RLock()
	defer cache.mu.RUnlock()
	if len(cache.items) > modelConfigCacheMaxEntries {
		t.Fatalf("cache size = %d, want at most %d", len(cache.items), modelConfigCacheMaxEntries)
	}
}

func TestResolveModelConfigSendsSelectedModel(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "dynamic-acp"
	agent := &installedInferenceAgent{id: agentType}
	require.NoError(t, reg.Register(agent))

	var requested agentctlutil.ProbeRequest
	probeCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			probeCalls++
			require.NoError(t, json.NewDecoder(r.Body).Decode(&requested))
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success: true,
				ConfigOptions: []agentctlutil.ProbeConfigOption{{
					ID:           "reasoning_effort",
					Name:         "Reasoning effort",
					Type:         "select",
					CurrentValue: "high",
				}},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctl.NewClient(host, port, log),
	}

	resolved, err := mgr.ResolveModelConfig(context.Background(), agentType, ModelConfigResolutionRequest{
		Model: "model-with-effort",
	})
	require.NoError(t, err)
	require.Equal(t, StatusOK, resolved.Status)
	require.Equal(t, "model-with-effort", resolved.Model)
	require.Len(t, resolved.ConfigOptions, 1)
	require.Equal(t, "reasoning_effort", resolved.ConfigOptions[0].ID)

	require.Equal(t, "model-with-effort", requested.Model)

	second, err := mgr.ResolveModelConfig(context.Background(), agentType, ModelConfigResolutionRequest{
		Model: "model-with-effort",
	})
	require.NoError(t, err)
	require.Equal(t, resolved.ConfigOptions[0].ID, second.ConfigOptions[0].ID)
	require.Equal(t, 1, probeCalls)
}

func TestResolveModelConfigDeduplicatesConcurrentProbes(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "concurrent-dynamic-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var probeCalls atomic.Int32
	var startOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			probeCalls.Add(1)
			startOnce.Do(func() { close(probeStarted) })
			<-releaseProbe
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success: true,
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctl.NewClient(host, port, log),
	}

	request := ModelConfigResolutionRequest{Model: "shared-model"}
	firstContext, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	results := make(chan ModelConfigResolution, 2)
	errors := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		resolved, err := mgr.ResolveModelConfig(firstContext, agentType, request)
		results <- resolved
		errors <- err
	}()

	<-probeStarted
	wg.Add(1)
	go func() {
		defer wg.Done()
		resolved, err := mgr.ResolveModelConfig(context.Background(), agentType, request)
		results <- resolved
		errors <- err
	}()
	cancelFirst()
	close(releaseProbe)
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	for range results {
	}
	require.Equal(t, int32(1), probeCalls.Load())
}

func TestResolveModelConfigRefreshStartsNewGenerationAndWinsOverOlderProbe(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "refresh-generation-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseSecond := make(chan struct{})
	var probeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			call := probeCalls.Add(1)
			switch call {
			case 1:
				close(firstStarted)
				<-releaseFirst
			case 2:
				close(secondStarted)
				<-releaseSecond
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success: true,
				ConfigOptions: []agentctlutil.ProbeConfigOption{{
					ID:           "generation",
					CurrentValue: fmt.Sprint(call),
				}},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctl.NewClient(host, port, log),
	}

	request := ModelConfigResolutionRequest{Model: "refresh-model"}
	firstResult := make(chan ModelConfigResolution, 1)
	firstError := make(chan error, 1)
	go func() {
		resolved, err := mgr.ResolveModelConfig(context.Background(), agentType, request)
		firstResult <- resolved
		firstError <- err
	}()
	<-firstStarted

	refreshResult := make(chan ModelConfigResolution, 1)
	refreshError := make(chan error, 1)
	go func() {
		resolved, err := mgr.ResolveModelConfig(context.Background(), agentType, ModelConfigResolutionRequest{
			Model:   request.Model,
			Refresh: true,
		})
		refreshResult <- resolved
		refreshError <- err
	}()

	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh joined the older in-flight probe")
	}
	close(releaseSecond)
	select {
	case err := <-refreshError:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("refresh probe did not finish")
	}
	refreshed := <-refreshResult
	require.Equal(t, "2", refreshed.ConfigOptions[0].CurrentValue)

	close(releaseFirst)
	require.NoError(t, <-firstError)
	<-firstResult

	cached, err := mgr.ResolveModelConfig(context.Background(), agentType, request)
	require.NoError(t, err)
	require.Equal(t, "2", cached.ConfigOptions[0].CurrentValue)
	require.Equal(t, int32(2), probeCalls.Load())
}

func TestResolveModelConfigDoesNotCacheFailures(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "failure-dynamic-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))
	var probeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			call := probeCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
					Success: false,
					Error:   "provider failed",
				}))
				return
			}
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{Success: true}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctl.NewClient(host, port, log),
	}
	request := ModelConfigResolutionRequest{Model: "retry-model"}

	first, err := mgr.ResolveModelConfig(context.Background(), agentType, request)
	require.NoError(t, err)
	require.Equal(t, StatusFailed, first.Status)
	second, err := mgr.ResolveModelConfig(context.Background(), agentType, request)
	require.NoError(t, err)
	require.Equal(t, StatusOK, second.Status)
	require.Equal(t, int32(2), probeCalls.Load())
}

func TestModelConfigCacheInvalidatesOneAgent(t *testing.T) {
	cache := newModelConfigCache()
	now := time.Unix(100, 0)
	cache.set("agent-key", ModelConfigResolution{AgentType: "agent", Status: StatusOK}, now)
	cache.set("other-key", ModelConfigResolution{AgentType: "other", Status: StatusOK}, now)

	cache.invalidateAgent("agent")
	_, ok := cache.get("agent-key", now.Add(time.Second))
	require.False(t, ok)
	_, ok = cache.get("other-key", now.Add(time.Second))
	require.True(t, ok)
}
