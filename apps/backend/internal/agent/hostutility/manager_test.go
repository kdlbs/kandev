package hostutility

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/usage"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
	"github.com/stretchr/testify/require"
)

func TestManagerExposesRefreshWithCommand(t *testing.T) {
	_, ok := reflect.TypeOf((*Manager)(nil)).MethodByName("RefreshWithCommand")
	require.True(t, ok, "managed runtime updates need a command-override refresh")
}

func TestBuildProbeRequestIncludesRuntimeEnv(t *testing.T) {
	const envKey = "HERMES_ACP_SKIP_CONFIGURED_MCP"
	ia := &installedInferenceAgent{
		id:         "hermes-acp",
		runtimeEnv: map[string]string{envKey: "1"},
	}
	req := buildProbeRequest(&instance{agentType: ia.id, workDir: t.TempDir()}, ia, false, agents.Command{})

	payload, err := json.Marshal(req)
	require.NoError(t, err)
	var decoded struct {
		InferenceConfig struct {
			Env map[string]string `json:"env"`
		} `json:"inference_config"`
	}
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, "1", decoded.InferenceConfig.Env[envKey])
}

func TestExecutePromptWithMCPIncludesRuntimeEnv(t *testing.T) {
	const envKey = "HERMES_ACP_SKIP_CONFIGURED_MCP"
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	ia := &installedInferenceAgent{
		id:         "hermes-acp",
		runtimeEnv: map[string]string{envKey: "1"},
	}
	require.NoError(t, reg.Register(ia))

	receivedEnv := make(chan map[string]string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/prompt":
			var request struct {
				InferenceConfig struct {
					Env map[string]string `json:"env"`
				} `json:"inference_config"`
			}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			receivedEnv <- request.InferenceConfig.Env
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.PromptResponse{
				Success:  true,
				Response: "ok",
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.instances[ia.id] = &instance{
		agentType: ia.id,
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}

	_, err := mgr.ExecutePromptWithMCP(context.Background(), ia.id, "", "", "test", nil)
	require.NoError(t, err)
	require.Equal(t, "1", (<-receivedEnv)[envKey])
}

func TestRefreshWithCommandUsesOverrideAndPreservesCacheOnAuthFailure(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "managed-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	var requestedCommand []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			var request agentctlutil.ProbeRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
			requestedCommand = append([]string(nil), request.InferenceConfig.Command...)
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success: false,
				Error:   "login required",
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)

	mgr := NewManager(reg, host, port, nil, log)
	mgr.cache.set(AgentCapabilities{
		AgentType:    agentType,
		AgentVersion: "1.0.0",
		Status:       StatusOK,
		Models:       []Model{{ID: "old-model", Name: "Old model"}},
	})
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}

	caps, err := mgr.RefreshWithCommand(
		context.Background(),
		agentType,
		agents.NewCommand("npx", "--yes", "--prefer-offline", "@example/managed-acp"),
	)
	require.NoError(t, err)
	require.Equal(t, StatusAuthRequired, caps.Status)
	require.Equal(t,
		[]string{"npx", "--yes", "--prefer-offline", "@example/managed-acp"},
		requestedCommand,
	)
	cached, ok := mgr.Get(agentType)
	require.True(t, ok)
	require.Equal(t, "1.0.0", cached.AgentVersion)
	require.Equal(t, "old-model", cached.Models[0].ID)
}

func TestRefreshWithCommandReplacesCacheAfterSuccessfulProbe(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "managed-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:      true,
				AgentVersion: "1.1.0",
				Models: []agentctlutil.ProbeModel{{
					ID:   "new-model",
					Name: "New model",
				}},
			}))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)
	mgr := NewManager(reg, host, port, nil, log)
	mgr.cache.set(AgentCapabilities{AgentType: agentType, AgentVersion: "1.0.0", Status: StatusOK})
	mgr.instances[agentType] = &instance{
		agentType: agentType,
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}

	_, err := mgr.RefreshWithCommand(
		context.Background(),
		agentType,
		agents.NewCommand("npx", "@example/managed-acp"),
	)
	require.NoError(t, err)
	cached, ok := mgr.Get(agentType)
	require.True(t, ok)
	require.Equal(t, "1.1.0", cached.AgentVersion)
	require.Equal(t, "new-model", cached.Models[0].ID)
}

func TestStopCancelsInFlightBootstrap(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)

	started := make(chan struct{})
	canceled := make(chan struct{})
	require.NoError(t, reg.Register(&blockingInferenceAgent{
		started:  started,
		canceled: canceled,
	}))

	mgr := NewManager(reg, "127.0.0.1", 1, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- mgr.Start(ctx)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("host utility bootstrap did not start installation check")
	}

	mgr.Stop(context.Background())

	select {
	case <-canceled:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("host utility Stop did not cancel in-flight bootstrap")
	}

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("host utility Start did not return after Stop")
	}
}

func TestStartLimitsConcurrentBootstraps(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	started := make(chan string, 8)
	release := make(chan struct{})
	var active atomic.Int32
	var peak atomic.Int32
	var attempted atomic.Int32

	for i := 0; i < maxConcurrentBootstraps+2; i++ {
		ag := &trackingInferenceAgent{
			installedInferenceAgent: installedInferenceAgent{id: "tracking-acp-" + strconv.Itoa(i)},
			started:                 started,
			release:                 release,
			active:                  &active,
			peak:                    &peak,
			attempted:               &attempted,
		}
		require.NoError(t, reg.Register(ag))
	}

	mgr := NewManager(reg, "127.0.0.1", 1, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()

	waitForStartedAgents(t, started, maxConcurrentBootstraps)
	require.LessOrEqual(t, peak.Load(), int32(maxConcurrentBootstraps))
	close(release)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("host utility bootstrap did not finish after releasing agents")
	}
	require.Equal(t, int32(maxConcurrentBootstraps+2), attempted.Load())
	require.LessOrEqual(t, peak.Load(), int32(maxConcurrentBootstraps))
	mgr.Stop(context.Background())
}

func TestStopCancelsQueuedBootstrap(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	started := make(chan string, 8)
	canceled := make(chan string, 8)
	var active atomic.Int32
	var peak atomic.Int32
	var attempted atomic.Int32

	for i := 0; i < maxConcurrentBootstraps+2; i++ {
		ag := &trackingInferenceAgent{
			installedInferenceAgent: installedInferenceAgent{id: "queued-acp-" + strconv.Itoa(i)},
			started:                 started,
			canceled:                canceled,
			active:                  &active,
			peak:                    &peak,
			attempted:               &attempted,
		}
		require.NoError(t, reg.Register(ag))
	}

	mgr := NewManager(reg, "127.0.0.1", 1, nil, log)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()

	waitForStartedAgents(t, started, maxConcurrentBootstraps)
	mgr.Stop(context.Background())

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("host utility Start did not return after queued cancellation")
	}
	cancel()
	require.Equal(t, int32(maxConcurrentBootstraps+2), attempted.Load())
	waitForCanceledAgents(t, canceled, maxConcurrentBootstraps+2)
	require.LessOrEqual(t, peak.Load(), int32(maxConcurrentBootstraps))
}

func waitForStartedAgents(t *testing.T, started <-chan string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for bootstrap %d/%d to start", i+1, count)
		}
	}
}

func waitForCanceledAgents(t *testing.T, canceled <-chan string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for bootstrap %d/%d to observe cancellation", i+1, count)
		}
	}
}

func TestProbePreservesConfigOptionDescriptions(t *testing.T) {
	log := newTestLogger(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/inference/probe" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"config_options": []any{map[string]any{
				"type":          "select",
				"id":            "reasoning_effort",
				"name":          "Reasoning effort",
				"description":   "Controls reasoning depth.",
				"current_value": "high",
				"options": []any{map[string]any{
					"value":       "high",
					"name":        "High",
					"description": "More thorough reasoning.",
				}},
			}},
		}); err != nil {
			t.Errorf("encode probe response: %v", err)
		}
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)
	mgr := NewManager(registry.NewRegistry(log), host, port, nil, log)

	caps := mgr.probe(context.Background(), &instance{
		agentType: "codex-acp",
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}, &installedInferenceAgent{id: "codex-acp"}, false)

	raw, err := json.Marshal(caps.ConfigOptions)
	require.NoError(t, err)
	var payload []map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Equal(t, "Controls reasoning depth.", payload[0]["description"])
	values := payload[0]["options"].([]any)
	require.Equal(t, "More thorough reasoning.", values[0].(map[string]any)["description"])
}

func TestDeleteInstanceRemovesWorkDirWithoutControlClient(t *testing.T) {
	log := newTestLogger(t)
	mgr := NewManager(registry.NewRegistry(log), "127.0.0.1", 1, nil, log)
	workDir := filepath.Join(t.TempDir(), "codex-acp")
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	mgr.deleteInstance(context.Background(), &instance{
		agentType: "codex-acp",
		workDir:   workDir,
	})

	_, err := os.Stat(workDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestHostUtilityDeleteContextIgnoresParentCancellation(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	ctx, cancel := hostUtilityDeleteContext(parent)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatal("delete context should not inherit parent cancellation")
	default:
	}
}

func TestExecutePromptRecreatesStaleCachedInstance(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "recreate-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	var promptCalls atomic.Int32
	instanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if got := r.Header.Get("X-Instance-ID"); got != "fresh-instance" {
				http.Error(w, "wrong instance id", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/prompt":
			if got := r.Header.Get("X-Instance-ID"); got != "fresh-instance" {
				http.Error(w, "wrong instance id", http.StatusNotFound)
				return
			}
			promptCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(agentctlutil.PromptResponse{
				Success:  true,
				Response: "summary",
				Model:    "test-model",
			})
			if err != nil {
				t.Errorf("encode prompt response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer instanceServer.Close()
	instanceHost, instancePort := serverHostPort(t, instanceServer)

	var createCalls atomic.Int32
	var deleteCalls atomic.Int32
	controlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
			createCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{
				ID:   "fresh-instance",
				Port: instancePort,
			})
			if err != nil {
				t.Errorf("encode create instance response: %v", err)
			}
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
			deleteCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer controlServer.Close()
	controlHost, controlPort := serverHostPort(t, controlServer)

	mgr := NewManager(reg, controlHost, controlPort, agentctlclient.NewControlClient(controlHost, controlPort, log), log)
	mgr.parentTmpDir = t.TempDir()

	staleWorkDir := filepath.Join(mgr.parentTmpDir, agentType)
	require.NoError(t, os.MkdirAll(staleWorkDir, 0o755))
	staleMarker := filepath.Join(staleWorkDir, "stale-marker")
	require.NoError(t, os.WriteFile(staleMarker, []byte("stale"), 0o644))
	mgr.instances[agentType] = &instance{
		agentType:  agentType,
		instanceID: "stale-instance",
		workDir:    staleWorkDir,
		client: agentctlclient.NewClient(instanceHost, instancePort, log,
			agentctlclient.WithExecutionID("stale-instance")),
	}

	result, err := mgr.ExecutePrompt(context.Background(), agentType, "test-model", "", "Summarize this")
	require.NoError(t, err)
	require.Equal(t, "summary", result.Response)
	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(1), deleteCalls.Load())
	require.Equal(t, int32(1), promptCalls.Load())

	mgr.mu.RLock()
	fresh := mgr.instances[agentType]
	mgr.mu.RUnlock()
	require.NotNil(t, fresh)
	require.Equal(t, "fresh-instance", fresh.instanceID)

	_, err = os.Stat(staleMarker)
	require.ErrorIs(t, err, os.ErrNotExist)

	mgr.Stop(context.Background())
	require.Equal(t, int32(2), deleteCalls.Load())
}

func TestRefreshRecreatesStaleCachedInstance(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "refresh-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	var probeCalls atomic.Int32
	var refreshRequested atomic.Bool
	instanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			if got := r.Header.Get("X-Instance-ID"); got != "fresh-refresh-instance" {
				http.Error(w, "wrong instance id", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			if got := r.Header.Get("X-Instance-ID"); got != "fresh-refresh-instance" {
				http.Error(w, "wrong instance id", http.StatusNotFound)
				return
			}
			probeCalls.Add(1)
			var request agentctlutil.ProbeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode probe request: %v", err)
			}
			refreshRequested.Store(request.Refresh)
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:        true,
				AgentName:      "Refresh ACP",
				AgentVersion:   "test",
				CurrentModelID: "test-model",
			})
			if err != nil {
				t.Errorf("encode probe response: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer instanceServer.Close()
	instanceHost, instancePort := serverHostPort(t, instanceServer)

	var createCalls atomic.Int32
	var deleteCalls atomic.Int32
	controlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
			createCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			err := json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{
				ID:   "fresh-refresh-instance",
				Port: instancePort,
			})
			if err != nil {
				t.Errorf("encode create instance response: %v", err)
			}
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
			deleteCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer controlServer.Close()
	controlHost, controlPort := serverHostPort(t, controlServer)

	mgr := NewManager(reg, controlHost, controlPort, agentctlclient.NewControlClient(controlHost, controlPort, log), log)
	mgr.parentTmpDir = t.TempDir()

	staleWorkDir := filepath.Join(mgr.parentTmpDir, agentType)
	require.NoError(t, os.MkdirAll(staleWorkDir, 0o755))
	mgr.instances[agentType] = &instance{
		agentType:  agentType,
		instanceID: "stale-refresh-instance",
		workDir:    staleWorkDir,
		client: agentctlclient.NewClient(instanceHost, instancePort, log,
			agentctlclient.WithExecutionID("stale-refresh-instance")),
	}

	caps, err := mgr.Refresh(context.Background(), agentType)
	require.NoError(t, err)
	require.Equal(t, StatusOK, caps.Status)
	require.Equal(t, "test-model", caps.CurrentModelID)
	require.Equal(t, int32(1), createCalls.Load())
	require.Equal(t, int32(1), deleteCalls.Load())
	require.Equal(t, int32(1), probeCalls.Load())
	require.True(t, refreshRequested.Load())

	mgr.mu.RLock()
	fresh := mgr.instances[agentType]
	mgr.mu.RUnlock()
	require.NotNil(t, fresh)
	require.Equal(t, "fresh-refresh-instance", fresh.instanceID)

	mgr.Stop(context.Background())
	require.Equal(t, int32(2), deleteCalls.Load())
}

func TestExecutePromptDoesNotRecreateWhenParentContextCanceled(t *testing.T) {
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	const agentType = "canceled-acp"
	require.NoError(t, reg.Register(&installedInferenceAgent{id: agentType}))

	instanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer instanceServer.Close()
	instanceHost, instancePort := serverHostPort(t, instanceServer)

	var createCalls atomic.Int32
	var deleteCalls atomic.Int32
	controlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
			createCalls.Add(1)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
			deleteCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer controlServer.Close()
	controlHost, controlPort := serverHostPort(t, controlServer)

	mgr := NewManager(reg, controlHost, controlPort, agentctlclient.NewControlClient(controlHost, controlPort, log), log)
	mgr.parentTmpDir = t.TempDir()
	cached := &instance{
		agentType:  agentType,
		instanceID: "cached-instance",
		workDir:    filepath.Join(mgr.parentTmpDir, agentType),
		client:     agentctlclient.NewClient(instanceHost, instancePort, log),
	}
	mgr.instances[agentType] = cached

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := mgr.ExecutePrompt(ctx, agentType, "test-model", "", "Summarize this")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, int32(0), createCalls.Load())
	require.Equal(t, int32(0), deleteCalls.Load())

	mgr.mu.RLock()
	current := mgr.instances[agentType]
	mgr.mu.RUnlock()
	require.Same(t, cached, current)
}

func serverHostPort(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	return host, port
}

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

type installedInferenceAgent struct {
	id         string
	runtimeEnv map[string]string
}

func (a *installedInferenceAgent) ID() string          { return a.id }
func (a *installedInferenceAgent) Name() string        { return "Installed ACP" }
func (a *installedInferenceAgent) DisplayName() string { return "Installed ACP" }
func (a *installedInferenceAgent) Description() string { return "installed test agent" }
func (a *installedInferenceAgent) Enabled() bool       { return true }
func (a *installedInferenceAgent) DisplayOrder() int   { return 1 }
func (a *installedInferenceAgent) Logo(agents.LogoVariant) []byte {
	return nil
}
func (a *installedInferenceAgent) IsInstalled(context.Context) (*agents.DiscoveryResult, error) {
	return &agents.DiscoveryResult{Available: true}, nil
}
func (a *installedInferenceAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return agents.NewCommand(a.id)
}
func (a *installedInferenceAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *installedInferenceAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{Protocol: agent.ProtocolACP, Env: a.runtimeEnv}
}
func (a *installedInferenceAgent) BillingType() usage.BillingType {
	return usage.BillingTypeSubscription
}
func (a *installedInferenceAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *installedInferenceAgent) InstallScript() string          { return "" }
func (a *installedInferenceAgent) InferenceConfig() *agents.InferenceConfig {
	return &agents.InferenceConfig{
		Supported: true,
		Command:   agents.NewCommand(a.id),
	}
}

type blockingInferenceAgent struct {
	once     sync.Once
	started  chan struct{}
	canceled chan struct{}
}

type trackingInferenceAgent struct {
	installedInferenceAgent
	started   chan<- string
	canceled  chan<- string
	release   <-chan struct{}
	active    *atomic.Int32
	peak      *atomic.Int32
	attempted *atomic.Int32
}

func (a *trackingInferenceAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.attempted.Add(1)
	current := a.active.Add(1)
	for {
		previous := a.peak.Load()
		if current <= previous || a.peak.CompareAndSwap(previous, current) {
			break
		}
	}
	a.started <- a.id
	defer a.active.Add(-1)

	if a.release == nil {
		<-ctx.Done()
		a.canceled <- a.id
		return nil, ctx.Err()
	}
	select {
	case <-a.release:
		return &agents.DiscoveryResult{Available: false}, nil
	case <-ctx.Done():
		a.canceled <- a.id
		return nil, ctx.Err()
	}
}

func (a *blockingInferenceAgent) ID() string          { return "blocking-acp" }
func (a *blockingInferenceAgent) Name() string        { return "Blocking ACP" }
func (a *blockingInferenceAgent) DisplayName() string { return "Blocking ACP" }
func (a *blockingInferenceAgent) Description() string { return "blocking test agent" }
func (a *blockingInferenceAgent) Enabled() bool       { return true }
func (a *blockingInferenceAgent) DisplayOrder() int   { return 1 }
func (a *blockingInferenceAgent) Logo(agents.LogoVariant) []byte {
	return nil
}
func (a *blockingInferenceAgent) IsInstalled(ctx context.Context) (*agents.DiscoveryResult, error) {
	a.once.Do(func() {
		close(a.started)
	})
	<-ctx.Done()
	close(a.canceled)
	return nil, ctx.Err()
}
func (a *blockingInferenceAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return agents.NewCommand("blocking-acp")
}
func (a *blockingInferenceAgent) PermissionSettings() map[string]agents.PermissionSetting {
	return nil
}
func (a *blockingInferenceAgent) Runtime() *agents.RuntimeConfig {
	return &agents.RuntimeConfig{Protocol: agent.ProtocolACP}
}
func (a *blockingInferenceAgent) BillingType() usage.BillingType {
	return usage.BillingTypeSubscription
}
func (a *blockingInferenceAgent) RemoteAuth() *agents.RemoteAuth { return nil }
func (a *blockingInferenceAgent) InstallScript() string          { return "" }
func (a *blockingInferenceAgent) InferenceConfig() *agents.InferenceConfig {
	return &agents.InferenceConfig{
		Supported: true,
		Command:   agents.NewCommand("blocking-acp"),
	}
}
