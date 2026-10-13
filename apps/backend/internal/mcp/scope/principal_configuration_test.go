package scope

import (
	"context"
	"testing"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestScopePrincipalConfigurationRequiresStoredSessionPurpose(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		taskMetadata, sessionMetadata map[string]interface{}
		origin                        string
		want                          mcpprofile.Surface
	}{
		{name: "configuration session", sessionMetadata: map[string]interface{}{"config_mode": true}, want: mcpprofile.SurfaceConfiguration},
		{name: "task marker alone", taskMetadata: map[string]interface{}{"config_mode": true}, want: mcpprofile.SurfaceKanbanTask},
		{name: "string marker", sessionMetadata: map[string]interface{}{"config_mode": "true"}, want: mcpprofile.SurfaceKanbanTask},
		{name: "automation stays restricted", origin: models.TaskOriginAutomationRun, taskMetadata: map[string]interface{}{"automation_id": "automation"}, sessionMetadata: map[string]interface{}{"config_mode": true}, want: mcpprofile.SurfaceAutomation},
		{name: "coordinator stays restricted", origin: models.TaskOriginCoordinator, sessionMetadata: map[string]interface{}{"config_mode": true}, want: mcpprofile.SurfaceCoordinator},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resolver := &Resolver{
				tasks: principalLookup{
					task:      &models.Task{ID: "task", WorkspaceID: "workspace", Origin: tt.origin, Metadata: tt.taskMetadata},
					workspace: &models.Workspace{ID: "workspace"},
					session:   &models.TaskSession{ID: "session", TaskID: "task", Metadata: tt.sessionMetadata},
				},
				coordinators: fakeCoordinatorLookup{coordinatorID: "coordinator", ok: true},
			}
			ctx, err := resolver.ScopePrincipal(context.Background(), "task", "session")
			require.NoError(t, err)
			principal, ok := PrincipalFromContext(ctx)
			require.True(t, ok)
			require.Equal(t, tt.want, principal.Surface)
		})
	}
}

func TestScopePrincipalConfigurationCannotOverrideInvalidManagedPolicy(t *testing.T) {
	resolver := &Resolver{tasks: principalLookup{
		task:      &models.Task{ID: "task", WorkspaceID: "workspace", Metadata: map[string]interface{}{models.MetaKeyManagedRetained: true}},
		workspace: &models.Workspace{ID: "workspace"},
		session:   &models.TaskSession{ID: "session", TaskID: "task", Metadata: map[string]interface{}{"config_mode": true}},
	}}
	_, err := resolver.ScopePrincipal(context.Background(), "task", "session")
	require.Error(t, err)
}
