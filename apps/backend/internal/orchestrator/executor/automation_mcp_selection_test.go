package executor

import (
	"context"
	"encoding/json"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
)

type automationSnapshotReader struct {
	*builtinSnapshotReader
	binding *p.BuiltinManifest
}

func (r *automationSnapshotReader) GetAutomationMCPBinding(context.Context, string, string) (*p.BuiltinManifest, error) {
	return r.binding.Clone(), r.err
}

// @covers AC-OFFICE-AUTOMATION-MCP-003.1, AC-OFFICE-AUTOMATION-MCP-003.2
func TestAutomationMCPBindingLifecycle(t *testing.T) {
	repo := newMockRepository()
	repo.tasks["task"] = &models.Task{ID: "task", WorkspaceID: "workspace", Origin: models.TaskOriginAutomationRun}
	session := &models.TaskSession{ID: "session", TaskID: "task"}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	reader := &automationSnapshotReader{builtinSnapshotReader: &builtinSnapshotReader{}, binding: &p.BuiltinManifest{Version: 2, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: p.SurfaceAutomation, Packages: []p.PackageID{p.PackageCore, p.PackageRichOutput}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageRichOutput: "automation"}}}
	// Supply new wire fields through JSON so the old reader still compiles.
	require.NoError(t, json.Unmarshal([]byte(`{"version":2,"catalog_version":1,"workspace_id":"workspace","task_id":"task","surface":"automation","automation_id":"automation","automation_revision":1,"packages":["core","rich-output"],"origins":{"core":"required","rich-output":"automation"}}`), reader.binding))
	exec.SetBuiltinPolicyReader(reader)
	ctx, err := exec.PrepareMCPLaunch(context.Background(), "task", session, false)
	require.NoError(t, err)
	prepared := PreparedMCPProfile(ctx, "task", "session")
	require.NotNil(t, prepared.BuiltinPackages)
	require.Contains(t, prepared.BuiltinPackages.Packages, p.PackageRichOutput)
	reader.binding.Packages = []p.PackageID{p.PackageCore}
	reader.binding.Origins = map[p.PackageID]string{p.PackageCore: "required"}
	same, err := exec.PrepareTaskSessionMCPProfile(ctx, "task", session, false)
	require.NoError(t, err)
	require.Contains(t, same.BuiltinPackages.Packages, p.PackageRichOutput)
	fresh, err := exec.PrepareTaskSessionMCPProfile(context.Background(), "task", session, false)
	require.NoError(t, err)
	require.NotContains(t, fresh.BuiltinPackages.Packages, p.PackageRichOutput)
}
