package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/plugins/instances"
)

func TestCanvasPublishAuthorityCapturesPlacementAndDataScope(t *testing.T) {
	item := &canvas.Canvas{
		PluginInstanceID: "instance-1",
		ScopeKind:        canvas.ScopeTask,
		DataScopeKind:    instances.ScopeWorkspace,
		WorkspaceID:      "workspace-1",
		TaskID:           "task-1",
		Status:           canvas.StatusActive,
		GrantGeneration:  4,
	}

	authority := canvasPublishAuthority(item)
	if authority.ScopeKind != canvas.ScopeTask || authority.DataScopeKind != instances.ScopeWorkspace {
		t.Fatalf("publish authority scopes = placement %q, data %q; want task placement and workspace data", authority.ScopeKind, authority.DataScopeKind)
	}
	if authority.InstanceID != item.PluginInstanceID || authority.WorkspaceID != item.WorkspaceID || authority.TaskID != item.TaskID || authority.Status != item.Status || authority.GrantGeneration != item.GrantGeneration {
		t.Fatalf("publish authority = %+v, missing current canvas state", authority)
	}
}
