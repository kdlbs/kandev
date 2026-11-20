package sysprompt

import (
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-AGENT-TASK-REFERENCE-001.7
func TestTaskReferenceGuidance(t *testing.T) {
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: "workspace", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore}}
	prompts := map[string]string{
		"task":         FormatKandevContext("task", "session", false),
		"bound task":   FormatKandevContextWithOptions("task", "session", KandevContextOptions{BuiltinPackages: manifest}),
		"office":       FormatOfficeContext("task", "session"),
		"bound office": FormatOfficeContextWithPackages("task", "session", false, manifest),
	}
	for name, prompt := range prompts {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, prompt, "[Task title](/t/task-id)")
			require.Contains(t, prompt, "Do not guess an ID.")
			require.Contains(t, prompt, "A task reference does not authorize creating or changing a task.")
			require.Contains(t, prompt, "Kandev Task ID: task")
		})
	}
}
