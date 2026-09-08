package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
func TestManagerProbeRecoversManagedRuntimeETarget(t *testing.T) {
	const version = "1.18.29"
	agent := agents.NewOpenCodeACP()
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
	packageSpec := agent.ManagedNPMRuntime().PackageSpec(version)
	wantCommands := [][]string{
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, false).Args(),
		agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, true).Args(),
	}
	if !equalStringSlices(commands, wantCommands) {
		t.Fatalf("probe commands = %#v, want %#v", commands, wantCommands)
	}
	if len(repairSpecs) != 1 || repairSpecs[0] != packageSpec {
		t.Fatalf("repair specs = %#v, want [%q]", repairSpecs, packageSpec)
	}
	if len(caps.Models) != 1 || caps.Models[0].ID != "opencode/model" {
		t.Fatalf("models = %#v, want recovered model", caps.Models)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
func TestManagerProbeDoesNotRetryManagedRuntimeRecoveryTwice(t *testing.T) {
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
	if probes != 2 || repairs != 1 {
		t.Fatalf("attempts = (%d probes, %d repairs), want (2, 1)", probes, repairs)
	}
}

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-001.6
func TestManagedRuntimeProbeRecoveryStopsOnCancellation(t *testing.T) {
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
	if len(probes) != 2 || repairs != 1 {
		t.Fatalf("attempts = (%d probes, %d repairs), want (2, 1)", len(probes), repairs)
	}
	if probes[1].Model != "mock-fast" {
		t.Fatalf("retry model = %q, want selected model", probes[1].Model)
	}
	wantRetry := agent.ManagedNPMRuntime().ACPCommandWithNpmPreference(version, true).Args()
	if !equalStrings(probes[1].InferenceConfig.Command, wantRetry) {
		t.Fatalf("retry command = %#v, want %#v", probes[1].InferenceConfig.Command, wantRetry)
	}
}

func TestManagedRuntimeRepairWaitsForConcurrentProbe(t *testing.T) {
	agent := agents.NewOpenCodeACP()
	blockerStarted := make(chan struct{})
	releaseBlocker := make(chan struct{})
	repairStarted := make(chan struct{})
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
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{Success: true})
		case "/api/v1/agent/managed-runtime/cache-repair":
			close(repairStarted)
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

	raced := false
	select {
	case <-repairStarted:
		raced = true
	case <-time.After(250 * time.Millisecond):
	}
	close(releaseBlocker)
	select {
	case <-repairStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("cache repair did not start after the concurrent probe completed")
	}
	<-firstDone
	<-secondDone
	if raced {
		t.Fatal("cache repair raced a concurrent probe")
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
