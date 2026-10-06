package backendapp

import (
	"context"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type managedAdmissionProfileLookup struct{}

func (managedAdmissionProfileLookup) GetAgentProfile(context.Context, string) (*settingsmodels.AgentProfile, error) {
	return &settingsmodels.AgentProfile{ID: "profile", Enabled: true}, nil
}

// @covers AC-PLUGINS-MANAGED-COORDINATION-002.1, AC-PLUGINS-MANAGED-COORDINATION-002.4
func TestManagedConversationAdmissionWiring(t *testing.T) {
	harness := newBootStateTestHarness(t)
	ctx := context.Background()
	require.NoError(t, harness.taskRepo.CreateWorkspace(ctx, &models.Workspace{ID: "managed-wiring", Name: "Managed"}))
	store, err := state.NewStore(db.NewPool(harness.db, harness.db))
	require.NoError(t, err)
	svc := NewAgentConversationService(harness.taskRepo, managedAdmissionProfileLookup{}, store, nil)
	spec := pluginsdk.ManagedAgentConversationSpec{WorkspaceID: "managed-wiring", InstanceKey: "lead", AgentProfileID: "profile", BasePrompt: "original", ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	first, _, err := svc.EnsureManaged(ctx, "plugin", "installation", spec, "create", "digest")
	require.NoError(t, err)
	require.NoError(t, harness.taskRepo.UpdateTaskSessionState(ctx, first.SessionID, models.TaskSessionStateRunning, ""))
	spec.ExpectedRevision, spec.BasePrompt = first.Revision, "rejected"
	_, _, err = svc.EnsureManaged(ctx, "plugin", "installation", spec, "update", "update-digest")
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	task, err := harness.taskRepo.GetTask(ctx, first.TaskID)
	require.NoError(t, err)
	require.Equal(t, "original", task.Metadata["kandev.base_prompt"])
}
