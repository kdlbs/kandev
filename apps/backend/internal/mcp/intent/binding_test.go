package intent

import (
	"context"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuiltinIntentDispatchRequiresMatchingExecution(t *testing.T) {
	manifest := &p.BuiltinManifest{Version: 1, CatalogVersion: 1, TaskID: "task", WorkspaceID: "workspace", Surface: p.SurfaceKanbanTask, Packages: []p.PackageID{p.PackageCore, p.PackageWalkthroughs}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageWalkthroughs: "task"}}
	for _, tc := range []struct {
		name                     string
		binding                  Binding
		task, session, execution string
		manifest                 *p.BuiltinManifest
		allowed                  bool
	}{
		{"selected", Binding{Walkthrough, "task", "session", "execution"}, "task", "session", "execution", manifest, true},
		{"rotation", Binding{Walkthrough, "task", "session", "old"}, "task", "session", "execution", manifest, false},
		{"foreign-task", Binding{Walkthrough, "other", "session", "execution"}, "task", "session", "execution", manifest, false},
		{"foreign-session", Binding{Walkthrough, "task", "other", "execution"}, "task", "session", "execution", manifest, false},
		{"fresh-start", Binding{Walkthrough, "task", "session", ""}, "task", "session", "execution", manifest, true},
		{"fresh-legacy-unknown", Binding{Walkthrough, "task", "session", ""}, "task", "session", "execution", nil, false},
		{"observed-legacy", Binding{Walkthrough, "task", "session", "execution"}, "task", "session", "execution", nil, true},
		{"empty-execution", Binding{Walkthrough, "task", "session", ""}, "task", "session", "", manifest, false},
		{"invalid-intent", Binding{"unknown", "task", "session", "execution"}, "task", "session", "execution", manifest, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateExecution(Bind(context.Background(), tc.binding), tc.task, tc.session, tc.execution, tc.manifest)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
	ctx := Bind(context.Background(), Binding{Walkthrough, "task", "session", "execution"})
	excluded := manifest.Clone()
	excluded.Packages = []p.PackageID{p.PackageCore}
	delete(excluded.Origins, p.PackageWalkthroughs)
	require.Error(t, ValidateExecution(ctx, "task", "session", "execution", excluded))
	foreign := manifest.Clone()
	foreign.TaskID = "other"
	require.Error(t, ValidateExecution(ctx, "task", "session", "execution", foreign))
	require.NoError(t, ValidateExecution(context.Background(), "task", "session", "execution", nil))
}
