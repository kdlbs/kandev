package hostutility

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
)

func TestManagerProfileProbeAndModelResolutionUseScopedLaunchContext(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var requests []agentctlutil.ProbeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v1/inference/probe" {
			http.NotFound(w, r)
			return
		}
		var req agentctlutil.ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode probe request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		requests = append(requests, req)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"codex","name":"Codex"}],"config_options":[{"type":"select","id":"reasoning","name":"Reasoning","current_value":"medium"}]}`))
	}))
	defer server.Close()
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agents.NewClaudeACP()); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reg, host, port, nil, log)
	manager.instances["claude-acp"] = &instance{
		agentType: "claude-acp", instanceID: "test-instance", workDir: t.TempDir(),
		client: agentctlclient.NewClient(host, port, log),
	}
	profileContext := ProfileProbeContext{
		Scope:         "user-1:profile-1",
		Env:           map[string]string{"CODEX_PATH": "/profile/codex", "TOKEN": "secret-value"},
		CLIFlags:      []string{"--profile-flag", "ordered-value"},
		CommandPrefix: []string{"npx", "--"},
	}
	first, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{
		Context: profileContext, Refresh: true,
	})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities: %v", err)
	}
	if first.Capabilities.Status != StatusOK || first.ContextRevision == "" {
		t.Fatalf("profile probe = %#v", first)
	}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileContext}); err != nil {
		t.Fatalf("cached ProbeProfileCapabilities: %v", err)
	}
	resolution, err := manager.ResolveModelConfig(context.Background(), "claude-acp", ModelConfigResolutionRequest{
		Model: "codex", ProfileContext: &profileContext,
	})
	if err != nil {
		t.Fatalf("ResolveModelConfig: %v", err)
	}
	if resolution.Status != StatusOK || resolution.ContextRevision != first.ContextRevision || len(resolution.ConfigOptions) != 1 {
		t.Fatalf("profile model resolution = %#v", resolution)
	}
	manager.PublishCapabilities("claude-acp", AgentCapabilities{AgentType: "claude-acp", Status: StatusOK})
	refreshed, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileContext})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities after runtime publication: %v", err)
	}
	if refreshed.ContextRevision == first.ContextRevision {
		t.Fatal("runtime publication did not change the profile context generation")
	}
	rotatedContext := profileContext
	rotatedContext.Env = map[string]string{"CODEX_PATH": "/profile/codex", "TOKEN": "rotated-secret"}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: rotatedContext}); err != nil {
		t.Fatalf("ProbeProfileCapabilities after secret rotation: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 4 {
		t.Fatalf("provider requests = %d, want baseline, model options, runtime refresh, and secret rotation probes", len(requests))
	}
	for i, request := range requests {
		wantToken := "secret-value"
		if i == len(requests)-1 {
			wantToken = "rotated-secret"
		}
		if !request.ProfileContext || request.InferenceConfig.Env["CODEX_PATH"] != "/profile/codex" ||
			request.InferenceConfig.Env["TOKEN"] != wantToken ||
			!slices.Equal(request.InferenceConfig.CLIFlags, profileContext.CLIFlags) ||
			!slices.Equal(request.InferenceConfig.CommandPrefix, profileContext.CommandPrefix) {
			t.Fatalf("request %d omitted profile launch context: %#v", i, request)
		}
	}
	if requests[1].Model != "codex" {
		t.Fatalf("model probe selected %q, want codex", requests[1].Model)
	}
}

func TestProfileRefreshGenerationIsScopedToProfileContext(t *testing.T) {
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"codex","name":"Codex"}]}`))
	})
	defer server.Close()

	profileA := ProfileProbeContext{Scope: "user-1:profile-a", Env: map[string]string{"PROFILE": "a"}}
	profileB := ProfileProbeContext{Scope: "user-1:profile-b", Env: map[string]string{"PROFILE": "b"}}
	firstA, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileA, Refresh: true})
	if err != nil {
		t.Fatalf("refresh profile A: %v", err)
	}
	if _, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{Context: profileB, Refresh: true}); err != nil {
		t.Fatalf("refresh profile B: %v", err)
	}
	resolution, err := manager.ResolveModelConfig(context.Background(), "claude-acp", ModelConfigResolutionRequest{
		Model: "codex", ProfileContext: &profileA,
	})
	if err != nil {
		t.Fatalf("resolve model options for profile A: %v", err)
	}
	if resolution.ContextRevision != firstA.ContextRevision {
		t.Fatalf("profile B refresh changed profile A revision: got %q, want %q", resolution.ContextRevision, firstA.ContextRevision)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("probe count = %d, want 3 (A baseline, B baseline, A model options)", got)
	}
}

func TestProfileProbeDiscardsResultWhenRuntimeActivatesDuringProbe(t *testing.T) {
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseProbe) }) }
	t.Cleanup(release)
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
		if requests.Add(1) == 1 {
			close(probeStarted)
			<-releaseProbe
		}
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"codex","name":"Codex"}]}`))
	})
	defer server.Close()

	type probeResult struct {
		result ProfileCapabilityResult
		err    error
	}
	resultCh := make(chan probeResult, 1)
	go func() {
		result, err := manager.ProbeProfileCapabilities(context.Background(), "claude-acp", ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: "user-1:profile-a"}, Refresh: true,
		})
		resultCh <- probeResult{result: result, err: err}
	}()
	select {
	case <-probeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not reach the provider")
	}

	manager.PublishCapabilities("claude-acp", AgentCapabilities{AgentType: "claude-acp", Status: StatusOK})
	release()
	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Fatalf("obsolete profile probe returned success: %#v", result.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not finish after release")
	}
}

func TestProfileProbeRejectsCommandResolvedAcrossRuntimeActivation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	selectionReader := &blockingProfileProbeSelectionReader{
		started: make(chan struct{}), release: make(chan struct{}),
	}
	t.Cleanup(func() { selectionReader.releaseOnce.Do(func() { close(selectionReader.release) }) })
	var requests atomic.Int32
	manager, server := newProfileProbeTestManager(t, agents.NewOpenCodeACP(), selectionReader, func(w http.ResponseWriter, req agentctlutil.ProbeRequest) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"opencode","name":"OpenCode"}]}`))
	})
	defer server.Close()

	type probeResult struct {
		result ProfileCapabilityResult
		err    error
	}
	resultCh := make(chan probeResult, 1)
	go func() {
		result, err := manager.ProbeProfileCapabilities(context.Background(), "opencode-acp", ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: "user-1:profile-opencode"}, Refresh: true,
		})
		resultCh <- probeResult{result: result, err: err}
	}()
	select {
	case <-selectionReader.started:
	case <-time.After(5 * time.Second):
		t.Fatal("managed command resolution did not start")
	}

	manager.PublishCapabilities("opencode-acp", AgentCapabilities{AgentType: "opencode-acp", Status: StatusOK})
	selectionReader.releaseOnce.Do(func() { close(selectionReader.release) })
	select {
	case result := <-resultCh:
		if result.err == nil {
			t.Fatalf("probe returned success using a command snapshot that crossed runtime activation: %#v", result.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("profile probe did not finish after command resolution was released")
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("provider received %d probes for an obsolete command snapshot", got)
	}
}

type blockingProfileProbeSelectionReader struct {
	started     chan struct{}
	release     chan struct{}
	mu          sync.Mutex
	calls       int
	releaseOnce sync.Once
}

func (r *blockingProfileProbeSelectionReader) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call == 1 {
		close(r.started)
		<-r.release
		return managedruntime.Selection{Package: "opencode-ai", Version: "1.18.5"}, true, nil
	}
	return managedruntime.Selection{Package: "opencode-ai", Version: "1.19.0"}, true, nil
}

func newProfileProbeTestManager(
	t *testing.T,
	agent agents.Agent,
	selectionReader managedruntime.SelectionReader,
	handleProbe func(http.ResponseWriter, agentctlutil.ProbeRequest),
) (*Manager, *httptest.Server) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v1/inference/probe" {
			http.NotFound(w, r)
			return
		}
		var req agentctlutil.ProbeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode probe request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		handleProbe(w, req)
	}))
	t.Cleanup(server.Close)
	host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatal(err)
	}
	reg := registry.NewRegistry(log)
	if err := reg.Register(agent); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reg, host, port, nil, log)
	manager.managedRuntimeSelections = selectionReader
	manager.instances[agent.ID()] = &instance{
		agentType: agent.ID(), instanceID: "test-instance", workDir: t.TempDir(),
		client: agentctlclient.NewClient(host, port, log),
	}
	return manager, server
}
