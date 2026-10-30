package executor

import (
	"context"
	"errors"
	"testing"

	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type builtinWorkspaceManager struct {
	*mockAgentManager
	profile *p.Context
	err     error
}

func (m *builtinWorkspaceManager) PrepareBuiltinWorkspace(_ context.Context, executionID string, profile p.Context) error {
	if executionID != "execution" {
		return errors.New("wrong execution")
	}
	m.profile = &profile
	return m.err
}

func TestExistingWorkspacePreparesBuiltinPackagesBeforeStart(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "allowed", true: "denied"}[denied], func(t *testing.T) {
			manager := &builtinWorkspaceManager{mockAgentManager: &mockAgentManager{}}
			if denied {
				manager.err = errors.New("helper already used")
			}
			exec := newTestExecutor(t, manager, newMockRepository())
			profile := p.New(p.SurfaceKanbanTask, nil, nil)
			profile.BuiltinPackages = &p.BuiltinManifest{Version: 1, CatalogVersion: 1, WorkspaceID: "workspace", TaskID: "task", Surface: profile.Surface, Packages: []p.PackageID{p.PackageCore, p.PackageWalkthroughs}, Origins: map[p.PackageID]string{p.PackageCore: "required", p.PackageWalkthroughs: "workspace"}}
			err := exec.configureExistingWorkspace(context.Background(), &v1.Task{ID: "task", WorkspaceID: "workspace"}, &models.TaskSession{ID: "session", TaskID: "task"}, "execution", "", &LaunchAgentRequest{McpProfile: &profile})
			require.Equal(t, &profile, manager.profile)
			require.ErrorIs(t, err, manager.err)
		})
	}
}
