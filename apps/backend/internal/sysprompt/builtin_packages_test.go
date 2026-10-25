package sysprompt

import (
	"strings"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinGuidanceFollowsBoundManifest(t *testing.T) {
	for _, selected := range []bool{false, true} {
		manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: "workspace", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
		if selected {
			for _, id := range []p.PackageID{p.PackageConfiguration, p.PackageRichOutput, p.PackageWalkthroughs, p.PackageDiagnostics} {
				manifest.Packages = append(manifest.Packages, id)
				manifest.Origins[id] = "task"
			}
		}
		text := FormatKandevContextWithOptions("task", "session", KandevContextOptions{BuiltinPackages: manifest, RequiresCompletionSignal: true, IncludeTaskTitleTool: true, IncludeUserQuestionTool: true})
		for _, required := range []string{"Kandev Task ID: task", "Kandev Session ID: session", "step_complete_kandev", "set_task_title_kandev", "ask_user_question_kandev", "PLAN EDITING:", "DELEGATION POLICY:"} {
			require.Contains(t, text, required)
		}
		for _, optional := range []string{"call show_rich_output_kandev", "- show_rich_output_kandev:", "- show_walkthrough_kandev", "kandev-configuration", "kandev-diagnostics"} {
			require.Equal(t, selected, strings.Contains(text, optional), optional)
		}
		require.Contains(t, text, "supersedes earlier optional-tool guidance")
		require.NotContains(t, text, "Selected tools from \"kandev\".")
		require.NotContains(t, text, "Use callable tools from \"kandev\" with")
		if selected {
			require.Contains(t, text, "shared resources")
			require.Contains(t, text, "cannot change built-in package policy")
		}
	}
}

func TestBuiltinConfigurationDiscoveryAllowsPackageServers(t *testing.T) {
	text := FormatConfigContext("session")
	require.NotContains(t, text, "from the \"kandev\" server")
	require.Contains(t, text, "client-specific callable names")
}

func TestBuiltinOfficeGuidanceKeepsRequiredTools(t *testing.T) {
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: "workspace", Surface: p.SurfaceOfficeTask, Packages: []p.PackageID{p.PackageCore}, Origins: map[p.PackageID]string{p.PackageCore: "required"}}
	text := FormatOfficeContextWithPackages("task", "session", true, manifest)
	require.NotContains(t, text, "show_rich_output_kandev")
	require.Contains(t, text, "step_complete_kandev")
	require.Contains(t, text, "write_task_document_kandev")
	require.Contains(t, text, "transfer_task_kandev")
	require.Contains(t, text, "$KANDEV_CLI")
	manifest.Packages = append(manifest.Packages, p.PackageRichOutput)
	manifest.Origins[p.PackageRichOutput] = "workspace"
	selected := FormatOfficeContextWithPackages("task", "session", true, manifest)
	require.Contains(t, selected, "show_rich_output_kandev")
	require.NotContains(t, selected, "from the \"kandev\" server")
	require.Contains(t, selected, "client-specific callable names")
	require.Contains(t, FormatOfficeContext("task", "session"), "from the \"kandev\" server")
}
