package lifecycle

import (
	"slices"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/acpcompat"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

func TestAntigravityContextResetRestoresSavedMCPSurface(t *testing.T) {
	profile := mcpprofile.New(mcpprofile.SurfaceKanbanTask, nil, nil)
	metadata := buildLaunchMetadata(&LaunchRequest{
		Metadata: map[string]interface{}{
			mcpprofile.SurfaceMetadataKey: string(mcpprofile.SurfaceCoordinator),
		},
		McpProfile: &profile,
	}, "", "", "")
	if got := metadata[mcpprofile.SurfaceMetadataKey]; got != string(mcpprofile.SurfaceKanbanTask) {
		t.Fatalf("persisted profile surface = %v, want typed launch surface", got)
	}

	restored := restoreMcpProfileContext(metadata)
	if restored == nil || restored.Surface != mcpprofile.SurfaceKanbanTask {
		t.Fatalf("restored MCP profile = %+v, want kanban task surface", restored)
	}
	selection := agents.LocalRuntimeSelection{
		Variant: acpcompat.AntigravityRuntimeCLI, SupportsDangerouslySkipPermissions: true,
	}
	args := filterAntigravitySkipPermissions(
		[]string{"agy", "acp", "--dangerously-skip-permissions"}, selection, &LaunchRequest{McpProfile: restored},
	)
	if !slices.Contains(args, "--dangerously-skip-permissions") {
		t.Fatalf("restored ordinary task omitted the supported CLI flag: %q", args)
	}
}

func TestAntigravityContextResetRejectsMissingOrInvalidSavedMCPSurface(t *testing.T) {
	metadataCases := []map[string]interface{}{
		{},
		{mcpprofile.SurfaceMetadataKey: "unknown-surface"},
		{
			mcpprofile.SurfaceMetadataKey:           string(mcpprofile.SurfaceKanbanTask),
			mcpprofile.ManagedToolPolicyMetadataKey: map[string]interface{}{"invalid": true},
		},
	}
	selection := agents.LocalRuntimeSelection{
		Variant: acpcompat.AntigravityRuntimeCLI, SupportsDangerouslySkipPermissions: true,
	}
	for _, metadata := range metadataCases {
		restored := restoreMcpProfileContext(metadata)
		if antigravitySkipPermissionsAllowed(selection, &LaunchRequest{McpProfile: restored}) {
			t.Fatalf("invalid stored profile metadata enabled skip permissions: %v", metadata)
		}
	}
}
