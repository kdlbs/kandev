package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
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
		true,
		agent.ManagedNPMRuntime().ACPCommand(""),
		initial,
	)

	if response != initial {
		t.Fatalf("response = %#v, want initial failure after cancellation", response)
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
