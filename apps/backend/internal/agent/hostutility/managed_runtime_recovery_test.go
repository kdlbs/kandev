package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.8
func TestManagedRuntimeProbeRecoveryPreservesSharedTree(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	const version = "1.18.29"
	agent := agents.NewOpenCodeACP()
	var commands [][]string
	var repairSpecs []string
	cacheRoot := t.TempDir()
	packageSpec := agent.ManagedNPMRuntime().PackageSpec(version)
	sentinel := filepath.Join(cacheRoot, "_npx", managedruntime.NpxExecutionCacheKey(packageSpec), "node_modules", "healthy-sibling", "sentinel")
	writeSentinel := func() {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(sentinel), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sentinel, []byte("sibling package tree"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeSentinel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inference/probe":
			var request agentctlutil.ProbeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			commands = append(commands, request.InferenceConfig.Command)
			if len(commands) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success":      false,
					"error":        "ACP initialize failed: peer disconnected before response",
					"failure_code": "managed_runtime_npm_resolution",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:      true,
				AgentVersion: version,
				Models:       []agentctlutil.ProbeModel{{ID: "opencode/model", Name: "Recovered model"}},
			})
		case "/api/v1/agent/managed-runtime/cache-repair":
			var request agentctlclient.RepairManagedRuntimeCacheRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			repairSpecs = append(repairSpecs, request.PackageSpec)
			if err := managedruntime.RemoveNpxExecutionTree(cacheRoot, request.PackageSpec); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	client := agentctlclient.NewClient(host, port, newTestLogger(t))
	t.Cleanup(client.Close)
	if err := client.RepairManagedRuntimeCache(context.Background(), packageSpec); err != nil {
		t.Fatalf("positive control repair: %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("positive control sentinel stat error = %v, want deletion", err)
	}
	repairSpecs = nil
	writeSentinel()

	manager := &Manager{
		log: newTestLogger(t),
		managedRuntimeSelections: managedRuntimeSelectionReader{
			selection: managedruntime.Selection{Package: agent.ManagedNPMRuntime().Package, Version: version},
			found:     true,
		},
	}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    client,
	}

	caps := manager.probe(context.Background(), inst, agent, true)

	if caps.Status != StatusOK {
		t.Fatalf("probe status = %q, want %q (error: %s)", caps.Status, StatusOK, caps.Error)
	}
	wantCommands := [][]string{
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, false).Args(),
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, true).Args(),
	}
	if !equalStringSlices(commands, wantCommands) {
		t.Fatalf("probe commands = %#v, want %#v", commands, wantCommands)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("automatic probe recovery removed the shared sibling tree: %v", err)
	}
	if len(repairSpecs) != 0 {
		t.Fatalf("automatic probe recovery called cache repair: %#v", repairSpecs)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "opencode/model" {
		t.Fatalf("models = %#v, want recovered model", caps.Models)
	}
}

func TestManagerProbeRecoversCodexAppServerETarget(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	const version = "0.154.0"
	agent := agents.NewCodexAppServer(true)
	var commands [][]string
	var repairSpecs []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inference/probe":
			var request agentctlutil.ProbeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			commands = append(commands, request.InferenceConfig.Command)
			if len(commands) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"success":      false,
					"error":        "initialize Codex app-server: read app-server frame: EOF",
					"failure_code": "managed_runtime_npm_resolution",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:      true,
				AgentVersion: version,
				Models:       []agentctlutil.ProbeModel{{ID: "gpt-6-astra", Name: "GPT-6-Astra"}},
			})
		case "/api/v1/agent/managed-runtime/cache-repair":
			var request agentctlclient.RepairManagedRuntimeCacheRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			repairSpecs = append(repairSpecs, request.PackageSpec)
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)

	manager := &Manager{
		log: newTestLogger(t),
		managedRuntimeSelections: managedRuntimeSelectionReader{
			selection: managedruntime.Selection{Package: agent.ManagedNPMRuntime().Package, Version: version},
			found:     true,
		},
	}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, manager.log),
	}

	caps := manager.probe(context.Background(), inst, agent, true)

	if caps.Status != StatusOK {
		t.Fatalf("probe status = %q, want %q (error: %s)", caps.Status, StatusOK, caps.Error)
	}
	wantCommands := [][]string{
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, false).Args(),
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, true).Args(),
	}
	if !equalStringSlices(commands, wantCommands) {
		t.Fatalf("probe commands = %#v, want %#v", commands, wantCommands)
	}
	if len(repairSpecs) != 0 {
		t.Fatalf("automatic probe recovery called cache repair: %#v", repairSpecs)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "gpt-6-astra" {
		t.Fatalf("models = %#v, want recovered model", caps.Models)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
func TestManagerProbeDoesNotRetryManagedRuntimeRecoveryTwice(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	agent := agents.NewOpenCodeACP()
	var probes, repairs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inference/probe":
			probes++
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:     false,
				Error:       "ACP initialize failed",
				FailureCode: agentctlutil.ProbeFailureManagedRuntimeNPMResolution,
			})
		case "/api/v1/agent/managed-runtime/cache-repair":
			repairs++
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	manager := &Manager{log: newTestLogger(t)}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, manager.log),
	}

	caps := manager.probe(context.Background(), inst, agent, true)

	if caps.Status != StatusFailed {
		t.Fatalf("probe status = %q, want %q", caps.Status, StatusFailed)
	}
	if probes != 2 || repairs != 0 {
		t.Fatalf("attempts = (%d probes, %d repairs), want (2, 0)", probes, repairs)
	}
}

func TestManagedRuntimeReleaseAgePolicySkipsCacheRepair(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	agent := agents.NewOpenCodeACP()
	var probes, repairs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inference/probe":
			probes++
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:     false,
				Error:       "ACP initialize failed",
				FailureCode: agentctlutil.ProbeFailureCode("managed_runtime_npm_policy"),
			})
		case "/api/v1/agent/managed-runtime/cache-repair":
			repairs++
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	manager := &Manager{log: newTestLogger(t)}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, manager.log),
	}

	caps := manager.probe(context.Background(), inst, agent, true)
	if caps.Status != StatusFailed {
		t.Fatalf("probe status = %q, want %q", caps.Status, StatusFailed)
	}
	if probes != 1 || repairs != 0 {
		t.Fatalf("attempts = (%d probes, %d repairs), want (1, 0)", probes, repairs)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
func TestManagedRuntimeProbeRecoveryStopsOnCancellation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	agent := agents.NewOpenCodeACP()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cancelled recovery must not reach agentctl")
		http.Error(w, "unexpected request", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	manager := &Manager{log: newTestLogger(t)}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, manager.log),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	initial := &agentctlutil.ProbeResponse{
		Success:     false,
		Error:       "ACP initialize failed",
		FailureCode: agentctlutil.ProbeFailureManagedRuntimeNPMResolution,
	}

	response := manager.recoverManagedRuntimeProbe(
		ctx,
		inst,
		agent,
		agent.ManagedNPMRuntime().ACPCommand("1.18.29"),
		buildProbeRequest(
			inst, agent, true, agent.ManagedNPMRuntime().ACPCommand("1.18.29"),
		),
		initial,
	)

	if response != initial {
		t.Fatalf("response = %#v, want initial failure after cancellation", response)
	}
}

func TestResolveModelConfigRecoversManagedRuntimeETarget(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	const version = "1.18.29"
	agent := agents.NewOpenCodeACP()
	log := newTestLogger(t)
	reg := registry.NewRegistry(log)
	if err := reg.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	var probes []agentctlutil.ProbeRequest
	var repairs int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/inference/probe":
			var request agentctlutil.ProbeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			probes = append(probes, request)
			if len(probes) == 1 {
				_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
					Success:     false,
					Error:       "ACP initialize failed",
					FailureCode: agentctlutil.ProbeFailureManagedRuntimeNPMResolution,
				})
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success: true,
				ConfigOptions: []agentctlutil.ProbeConfigOption{{
					ID:           "reasoning_effort",
					CurrentValue: "high",
				}},
			})
		case "/api/v1/agent/managed-runtime/cache-repair":
			repairs++
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	manager := NewManager(reg, host, port, nil, log)
	manager.managedRuntimeSelections = managedRuntimeSelectionReader{
		selection: managedruntime.Selection{Package: agent.ManagedNPMRuntime().Package, Version: version},
		found:     true,
	}
	manager.instances[agent.ID()] = &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}

	resolved, err := manager.ResolveModelConfig(context.Background(), agent.ID(), ModelConfigResolutionRequest{
		Model: "mock-fast",
	})
	if err != nil {
		t.Fatalf("ResolveModelConfig: %v", err)
	}
	if resolved.Status != StatusOK || len(resolved.ConfigOptions) != 1 {
		t.Fatalf("resolution = %#v, want recovered config options", resolved)
	}
	if len(probes) != 2 || repairs != 0 {
		t.Fatalf("attempts = (%d probes, %d repairs), want (2, 0)", len(probes), repairs)
	}
	if probes[1].Model != "mock-fast" {
		t.Fatalf("retry model = %q, want selected model", probes[1].Model)
	}
	wantRetry := agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, true).Args()
	if !equalStrings(probes[1].InferenceConfig.Command, wantRetry) {
		t.Fatalf("retry command = %#v, want %#v", probes[1].InferenceConfig.Command, wantRetry)
	}
}

func TestManagedRuntimeProbeRecoveryWaitsForConcurrentProbe(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	agent := agents.NewOpenCodeACP()
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	retryStarted := make(chan struct{})
	var repairs atomic.Int32
	var probeCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/inference/probe":
			call := probeCalls.Add(1)
			if call == 1 {
				close(blockerStarted)
				<-releaseBlocker
				_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{Success: true})
				return
			}
			if call == 2 {
				_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
					Success:     false,
					Error:       "ACP initialize failed",
					FailureCode: agentctlutil.ProbeFailureManagedRuntimeNPMResolution,
				})
				return
			}
			close(retryStarted)
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{Success: true})
		case "/api/v1/agent/managed-runtime/cache-repair":
			repairs.Add(1)
			_ = json.NewEncoder(w).Encode(agentctlclient.RepairManagedRuntimeCacheResponse{Success: true})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	host, port := serverHostPort(t, server)
	manager := &Manager{log: newTestLogger(t)}
	inst := &instance{
		agentType: agent.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, manager.log),
	}
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		_ = manager.probe(context.Background(), inst, agent, true)
	}()
	<-blockerStarted
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		_ = manager.probe(context.Background(), inst, agent, true)
	}()

	startedBeforeRelease := false
	select {
	case <-retryStarted:
		startedBeforeRelease = true
	case <-time.After(250 * time.Millisecond):
	}
	close(releaseBlocker)
	select {
	case <-retryStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("recovery probe did not start after the concurrent probe completed")
	}
	<-firstDone
	<-secondDone
	if startedBeforeRelease {
		t.Fatal("recovery probe raced a concurrent probe")
	}
	if repairs.Load() != 0 {
		t.Fatalf("automatic recovery called cache repair %d times", repairs.Load())
	}
}

func TestManagedRuntimeProbeRetryRejectsUntrustedCommands(t *testing.T) {
	spec := agents.NewOpenCodeACP().ManagedNPMRuntime()
	for _, command := range []agents.Command{
		agents.NewCommand("opencode", "acp"),
		agents.NewCommand("npx", "--yes", "--prefer-online", spec.PackageSpec("1.18.29"), "acp"),
		agents.NewCommand("npx", "--yes", "--prefer-offline", "other-agent@1.18.29", "acp"),
		agents.NewCommand("npx", "--yes", "--prefer-offline", spec.PackageSpec("1.18.29"), "different-args"),
	} {
		if retry, packageSpec, ok := managedRuntimeProbeRetry(command, spec); ok {
			t.Fatalf("command %#v produced retry %#v for %q", command.Args(), retry.Args(), packageSpec)
		}
	}
}

func equalStringSlices(left, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !equalStrings(left[i], right[i]) {
			return false
		}
	}
	return true
}
