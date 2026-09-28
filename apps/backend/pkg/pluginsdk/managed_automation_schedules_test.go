package pluginsdk

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
)

type managedScheduleManagerFixture struct {
	listed  []ManagedConversationSchedule
	created ManagedConversationScheduleCreate
	updated ManagedConversationScheduleUpdate
	enabled ManagedConversationScheduleEnabled
	deleted ManagedConversationScheduleDelete
}

func (f *managedScheduleManagerFixture) List(context.Context, ManagedConversationScheduleQuery) ([]ManagedConversationSchedule, error) {
	return f.listed, nil
}
func (f *managedScheduleManagerFixture) Create(_ context.Context, input ManagedConversationScheduleCreate) (*CommandResult, ManagedConversationSchedule, error) {
	f.created = input
	return &CommandResult{Status: CommandApplied}, input.Schedule, nil
}
func (f *managedScheduleManagerFixture) Update(_ context.Context, input ManagedConversationScheduleUpdate) (*CommandResult, ManagedConversationSchedule, error) {
	f.updated = input
	return &CommandResult{Status: CommandApplied}, input.Schedule, nil
}
func (f *managedScheduleManagerFixture) SetEnabled(_ context.Context, input ManagedConversationScheduleEnabled) (*CommandResult, ManagedConversationSchedule, error) {
	f.enabled = input
	return &CommandResult{Status: CommandApplied}, ManagedConversationSchedule{ID: input.AutomationID, Enabled: input.Enabled}, nil
}
func (f *managedScheduleManagerFixture) Delete(_ context.Context, input ManagedConversationScheduleDelete) (*CommandResult, error) {
	f.deleted = input
	return &CommandResult{Status: CommandApplied}, nil
}

type managedScheduleHostFixture struct {
	*exactHostFixture
	manager ManagedConversationScheduleManager
}

func (h *managedScheduleHostFixture) ManagedConversationSchedules() ManagedConversationScheduleManager {
	return h.manager
}

func TestManagedConversationSchedulesHostRoundTrip(t *testing.T) {
	ctx := context.Background()
	schedule := ManagedConversationSchedule{
		ID: "automation-1", WorkspaceID: "workspace-1", Name: "Daily brief", Description: "Summarize blockers",
		Prompt: "Review open tasks", PluginID: "coordinator", InstanceKey: "daily-brief",
		DestinationRevision: 8, ResourceRevision: 3, Enabled: true,
		Triggers: []ManagedConversationScheduleTrigger{{Type: "scheduled", Config: json.RawMessage(`{"cron_expression":"0 9 * * *"}`), Enabled: true}},
	}
	manager := &managedScheduleManagerFixture{listed: []ManagedConversationSchedule{schedule}}
	author := &fakeAuthorPlugin{}
	serverHost := &managedScheduleHostFixture{exactHostFixture: &exactHostFixture{}, manager: manager}
	plugin := &GRPCPlugin{Impl: author, Host: serverHost, HostDialTimeout: 5 * time.Second}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{PluginMapKey: plugin})
	t.Cleanup(func() {
		server.Stop()
		_ = client.Close()
	})

	_, err := client.Dispense(PluginMapKey)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return author.Host() != nil }, 5*time.Second, 10*time.Millisecond)
	api, ok := HostManagedConversationSchedules(author.Host())
	require.True(t, ok)

	listed, err := api.List(ctx, ManagedConversationScheduleQuery{WorkspaceID: "workspace-1", ApprovalRevision: 4, ManifestDigest: "manifest"})
	require.NoError(t, err)
	require.Equal(t, schedule, listed[0])

	create := ManagedConversationScheduleCreate{RequestID: "request-create", IdempotencyKey: "create-1", ApprovalRevision: 4, ManifestDigest: "manifest", Schedule: schedule}
	result, created, err := api.Create(ctx, create)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, schedule, created)
	require.Equal(t, create, manager.created)

	update := ManagedConversationScheduleUpdate{RequestID: "request-update", IdempotencyKey: "update-1", AutomationID: schedule.ID, ExpectedResourceRevision: 3, ApprovalRevision: 4, ManifestDigest: "manifest", Schedule: schedule}
	result, _, err = api.Update(ctx, update)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, update, manager.updated)

	enabled := ManagedConversationScheduleEnabled{RequestID: "request-enable", IdempotencyKey: "enable-1", WorkspaceID: schedule.WorkspaceID, AutomationID: schedule.ID, ExpectedResourceRevision: 3, Enabled: false, ApprovalRevision: 4, ManifestDigest: "manifest"}
	result, paused, err := api.SetEnabled(ctx, enabled)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.False(t, paused.Enabled)
	require.Equal(t, enabled, manager.enabled)

	remove := ManagedConversationScheduleDelete{RequestID: "request-delete", IdempotencyKey: "delete-1", WorkspaceID: schedule.WorkspaceID, AutomationID: schedule.ID, ExpectedResourceRevision: 3, ApprovalRevision: 4, ManifestDigest: "manifest"}
	result, err = api.Delete(ctx, remove)
	require.NoError(t, err)
	require.Equal(t, CommandApplied, result.Status)
	require.Equal(t, remove, manager.deleted)
}
