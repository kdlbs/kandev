package mcp

import (
	"sort"
	"strings"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

var allCapabilities = []mcpprofile.Capability{
	mcpprofile.CapabilityUserQuestion, mcpprofile.CapabilityParentQuestion, mcpprofile.CapabilityTaskTitle,
	mcpprofile.CapabilityGitHubPR, mcpprofile.CapabilityGitLabMR, mcpprofile.CapabilityCanvas,
}

func catalogueNames(t *testing.T, surface mcpprofile.Surface, caps []mcpprofile.Capability, providers []string) []string {
	t.Helper()
	log := newTestLogger(t)
	backend := NewChannelBackendClient(log)
	t.Cleanup(backend.Close)
	s := New(backend, "session-1", "task-1", 10005, log, "", false, ModeTask)
	s.SetProfile(mcpprofile.New(surface, caps, providers))
	var names []string
	for name := range s.mcpServer.ListTools() {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func TestGuardedTableCoversEveryTaskCatalogueTool(t *testing.T) {
	var missing []string
	seen := map[string]bool{}
	for _, surface := range []mcpprofile.Surface{mcpprofile.SurfaceKanbanTask, mcpprofile.SurfaceOfficeTask} {
		for _, providers := range [][]string{nil, {"github"}, {"gitlab"}, {"github", "gitlab"}} {
			for _, caps := range [][]mcpprofile.Capability{nil, allCapabilities} {
				for _, name := range catalogueNames(t, surface, caps, providers) {
					bare, ok := strings.CutSuffix(name, "_kandev")
					if !ok {
						bare = name
					}
					if _, listed := mcpprofile.GuardedToolByName(bare); !listed && !seen[bare] {
						missing = append(missing, bare)
					}
					seen[bare] = true
				}
			}
		}
	}
	require.Empty(t, missing, "catalogue tools without a classification row")
}

func TestGuardedParityCataloguesAreNonTrivial(t *testing.T) {
	kanban := catalogueNames(t, mcpprofile.SurfaceKanbanTask, allCapabilities, []string{"github", "gitlab"})
	office := catalogueNames(t, mcpprofile.SurfaceOfficeTask, allCapabilities, nil)
	require.Contains(t, kanban, "create_task_kandev")
	require.Contains(t, kanban, "update_task_change_request_automation_kandev")
	require.Greater(t, len(kanban), 30)
	require.NotEmpty(t, office)
	bare := catalogueNames(t, mcpprofile.SurfaceKanbanTask, nil, nil)
	require.Less(t, len(bare), len(kanban), "capabilities and providers must widen the catalogue")
}

// Plugin tools have dynamic names, so the table cannot list them; they reach
// the backend only through the invoke action, which the table denies.
func TestGuardedTableDeniesPluginToolDispatch(t *testing.T) {
	for _, action := range []string{ws.ActionMCPInvokePluginTool, ws.ActionMCPListPluginTools} {
		entry, ok := mcpprofile.GuardedToolByAction(action)
		require.True(t, ok, action)
		require.Equal(t, mcpprofile.ClassDenied, entry.Class, action)
	}
}
