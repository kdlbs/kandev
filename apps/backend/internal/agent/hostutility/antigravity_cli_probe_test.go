package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

// TestAntigravityLocalProbePrefersCLIWithStandaloneInstalled covers
// AC-AGENTS-ANTIGRAVITY-ACP-003.14 and AC-AGENTS-ANTIGRAVITY-ACP-003.15.
func TestAntigravityLocalProbePrefersCLIWithStandaloneInstalled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agy executable uses a POSIX script")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, "agy")
	cli := hostUtilityAntigravityCLIScript("1.5.15")
	if err := os.WriteFile(cliPath, []byte(cli), 0o755); err != nil {
		t.Fatal(err)
	}
	standalonePaths := []string{
		filepath.Join(binDir, "agy_acp_server.par"),
		filepath.Join(binDir, "localharness_external"),
	}
	for _, path := range standalonePaths {
		if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", binDir)

	var selectedCommand []string
	var selectedRuntime string
	manager, _ := newProfileProbeTestManagerWithCapabilities(t, agents.NewAntigravityACP(), nil,
		[]string{"launch-wrapper.v1", acpcompat.AntigravityCLIRuntimeCapability},
		func(w http.ResponseWriter, request agentctlutil.ProbeRequest) {
			selectedCommand = slices.Clone(request.InferenceConfig.Command)
			if request.InferenceConfig.AuthPolicy != nil {
				selectedRuntime = request.InferenceConfig.AuthPolicy.RuntimeVariant
			}
			if slices.Equal(selectedCommand, []string{cliPath, "acp", "--process-per-session"}) {
				_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
					Success: true,
					Models:  []agentctlutil.ProbeModel{{ID: "agy-gemini-3", Name: "Gemini 3"}},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:     false,
				Error:       "standalone oauth-personal consent is required",
				FailureCode: agentctlutil.ProbeFailureAuthenticationRequired,
			})
		})
	result, err := manager.Refresh(context.Background(), agents.AntigravityACPAgentID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Status != StatusOK || len(result.Models) != 1 || result.Models[0].ID != "agy-gemini-3" {
		t.Fatalf("capabilities = %#v, selected command = %v; want signed-in CLI models", result, selectedCommand)
	}
	if result.RuntimeVariant != "cli" {
		t.Fatalf("runtime variant = %q, want cli", result.RuntimeVariant)
	}
	if !slices.Equal(selectedCommand, []string{cliPath, "acp", "--process-per-session"}) {
		t.Fatalf("selected command = %v, want %v", selectedCommand, []string{cliPath, "acp", "--process-per-session"})
	}
	if selectedRuntime != acpcompat.AntigravityRuntimeCLI {
		t.Fatalf("probe auth runtime = %q, want %q", selectedRuntime, acpcompat.AntigravityRuntimeCLI)
	}
}

func TestAntigravityFailedCLIProbeKeepsSelectedRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake agy executable uses a POSIX script")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KANDEV_HOME_DIR", t.TempDir())
	binDir := t.TempDir()
	cliPath := filepath.Join(binDir, "agy")
	if err := os.WriteFile(cliPath, []byte(hostUtilityAntigravityCLIScript("1.5.15")), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	manager, _ := newProfileProbeTestManagerWithCapabilities(t, agents.NewAntigravityACP(), nil,
		[]string{"launch-wrapper.v1", acpcompat.AntigravityCLIRuntimeCapability},
		func(w http.ResponseWriter, _ agentctlutil.ProbeRequest) {
			_ = json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
				Success:     false,
				Error:       "Google sign-in required",
				FailureCode: agentctlutil.ProbeFailureAuthenticationRequired,
			})
		})
	result, err := manager.Refresh(context.Background(), agents.AntigravityACPAgentID)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if result.Status != StatusAuthRequired || result.Error != "Google sign-in required" {
		t.Fatalf("failed probe = %#v, want visible auth-required status", result)
	}
	if result.RuntimeVariant != acpcompat.AntigravityRuntimeCLI {
		t.Fatalf("failed probe runtime variant = %q, want cli", result.RuntimeVariant)
	}
}

func hostUtilityAntigravityCLIScript(version string) string {
	return "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'agy " + version + "'; exit 0; fi\n" +
		"if [ \"$1\" = \"acp\" ] && [ \"$2\" = \"--help\" ]; then printf 'Usage: agy acp [flags]\\n\\nFlags:\\n  --process-per-session  Serve each session from an isolated worker process\\n'; exit 0; fi\nexit 1\n"
}
