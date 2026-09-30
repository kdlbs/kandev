package controller

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/settings/dto"
)

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.1
func TestRuntimeStatusCoversManagedManualAndVirtualAgents(t *testing.T) {
	c := newTestController(map[string]agents.Agent{
		"claude-acp":       agents.NewClaudeACP(),
		"codex-app-server": agents.NewCodexAppServer(false),
		"auggie":           agents.NewAuggie(),
		"cursor-acp":       agents.NewCursorACP(),
		"dynamic":          agents.NewDynamicAgent(),
		"mock-agent":       agents.NewMockAgent(),
	})
	c.SetRuntimeUpdateStatusResolver(func(context.Context, string) (string, error) { return "9.0.0", nil })
	resp, err := c.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Statuses) != 6 {
		t.Fatalf("registered status count = %d, want 6", len(resp.Statuses))
	}
	byAgent := map[string]map[string]any{}
	for _, status := range resp.Statuses {
		raw, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		fields := map[string]any{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		byAgent[status.AgentName] = fields
	}
	for id, management := range map[string]string{"claude-acp": "managed", "codex-app-server": "managed", "auggie": "manual", "cursor-acp": "manual", "dynamic": "unsupported", "mock-agent": "unsupported"} {
		if got := byAgent[id]["management"]; got != management {
			t.Errorf("%s management = %v, want %s", id, got, management)
		}
	}
	if byAgent["cursor-acp"]["check_state"] != "unknown" {
		t.Error("opaque native latest must remain unknown")
	}
	if byAgent["codex-app-server"]["enabled"] != false {
		t.Error("disabled native protocol agent must remain explicitly disabled")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-002.2
func TestNativeOpenCodeDoesNotAdvertiseManagedActivation(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, testExecutableName("opencode")), []byte("native"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ag := agents.NewOpenCodeACP()
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(&recoveryRuntimeUpdater{
		currentFound: true, current: hostutility.AgentCapabilities{AgentVersion: "1.0.0"},
		metadata: RuntimeVersionMetadata{Latest: "1.1.0", Versions: []string{"1.0.0", "1.1.0"}},
	})
	resp, err := c.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	status := resp.Statuses[0]
	if status.Management != "manual" || status.Owner != "external" || status.EffectiveVersion != "1.0.0" || status.AutoUpdateSupported {
		t.Fatalf("native ownership: %+v", status)
	}
	if c.buildRuntimeUpdateDTO(context.Background(), ag, true) != nil {
		t.Error("external native runtime advertises Kandev-managed activation")
	}
}

// @covers AC-AGENTS-RUNTIME-NOTIFY-001.1
func TestUnmanagedAdapterDoesNotCompareUnmatchedVendorVersion(t *testing.T) {
	ag := agents.NewAmpACP()
	c := newTestController(map[string]agents.Agent{ag.ID(): ag})
	c.SetRuntimeUpdater(&recoveryRuntimeUpdater{
		currentFound: true, current: hostutility.AgentCapabilities{AgentVersion: "1.7.0"},
		metadata: RuntimeVersionMetadata{Latest: "0.3.0", Versions: []string{"0.2.0", "0.3.0"}},
	})
	response, err := c.ListAgentUpdateStatuses(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	status := response.Statuses[0]
	if status.CheckState != dto.AgentUpdateCheckStateUnknown || status.EffectiveVersion != "" {
		t.Fatalf("vendor observation must not identify the npm adapter version: %+v", status)
	}
	if status.CurrentVersion != "1.7.0" || status.LatestVersion != "0.3.0" {
		t.Fatalf("independent observation and release metadata lost: %+v", status)
	}
}
