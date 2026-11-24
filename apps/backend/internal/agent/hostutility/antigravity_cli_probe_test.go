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
	cli := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo 'agy 1.5.15'; exit 0; fi\nexit 1\n"
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
	manager, _ := newProfileProbeTestManager(t, agents.NewAntigravityACP(), nil,
		func(w http.ResponseWriter, request agentctlutil.ProbeRequest) {
			selectedCommand = slices.Clone(request.InferenceConfig.Command)
			if slices.Equal(selectedCommand, []string{cliPath, "acp"}) {
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
	result, err := manager.ProbeProfileCapabilities(context.Background(), agents.AntigravityACPAgentID,
		ProfileCapabilityRequest{
			Context: ProfileProbeContext{Scope: "user-1:antigravity"},
			Refresh: true,
		})
	if err != nil {
		t.Fatalf("ProbeProfileCapabilities: %v", err)
	}
	if result.Capabilities.Status != StatusOK || len(result.Capabilities.Models) != 1 || result.Capabilities.Models[0].ID != "agy-gemini-3" {
		t.Fatalf("capabilities = %#v, selected command = %v; want signed-in CLI models", result.Capabilities, selectedCommand)
	}
	if result.Capabilities.RuntimeVariant != "cli" {
		t.Fatalf("runtime variant = %q, want cli", result.Capabilities.RuntimeVariant)
	}
	if !slices.Equal(selectedCommand, []string{cliPath, "acp"}) {
		t.Fatalf("selected command = %v, want %v", selectedCommand, []string{cliPath, "acp"})
	}
}
