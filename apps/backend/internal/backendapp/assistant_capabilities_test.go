package backendapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	settings "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	shared "github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type capabilityTestProfiles struct {
	settingsstore.Repository
	rows   []*settings.AgentProfile
	config *settings.AgentProfileMcpConfig
}

func (p *capabilityTestProfiles) ListAgents(context.Context) ([]*settings.Agent, error) {
	return []*settings.Agent{{ID: "example"}}, nil
}
func (p *capabilityTestProfiles) ListAgentProfiles(context.Context, string) ([]*settings.AgentProfile, error) {
	return p.rows, nil
}
func (p *capabilityTestProfiles) GetAgentProfile(_ context.Context, id string) (*settings.AgentProfile, error) {
	for _, row := range p.rows {
		if row.ID == id {
			return row, nil
		}
	}
	return nil, fmt.Errorf("missing")
}
func (p *capabilityTestProfiles) GetAgentProfileMcpConfig(_ context.Context, id string) (*settings.AgentProfileMcpConfig, error) {
	if p.config != nil && p.config.ProfileID == id {
		return p.config, nil
	}
	return nil, sql.ErrNoRows
}

type capabilityTestPlugins struct{ snapshot plugintools.Snapshot }

func (p *capabilityTestPlugins) AgentToolCatalog() (plugintools.Snapshot, error) {
	return p.snapshot, nil
}

func TestAssistantCapabilitiesScopeAndRedaction(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	profiles := &capabilityTestProfiles{rows: []*settings.AgentProfile{
		{ID: "own", Name: "Example", Enabled: true, EnvVars: []taskmodels.ProfileEnvVar{{Key: "TOKEN", Value: "SYNTHETIC_ENV_SECRET"}}},
		{ID: "foreign", WorkspaceID: "other", Name: "FOREIGN_PROFILE"},
	}}
	profiles.config = &settings.AgentProfileMcpConfig{ProfileID: "own", Enabled: true, Servers: map[string]any{"example-mcp": map[string]any{"env": map[string]string{"TOKEN": "SYNTHETIC_MCP_SECRET"}}}}
	plugins := &capabilityTestPlugins{snapshot: plugintools.Snapshot{Generation: "one", Revision: 1, Tools: []plugintools.Definition{
		{ExposedName: "kandev_example_inspect", Surfaces: []string{"conversation"}, ReadOnlyHint: true, InputSchema: []byte(`{"type":"object","properties":{"query":{"type":"string","default":"SYNTHETIC_SCHEMA_SECRET"}},"x-config":"SYNTHETIC_EXTENSION_SECRET"}`)},
		{ExposedName: "legacy_task_tool", Surfaces: []string{"kanban-task"}},
	}}}
	reader := &assistantCapabilityReader{tasks: svc, profiles: profiles, executors: adapter.taskRepo, plugins: plugins}
	q := shared.CapabilityQuery{OwnerID: "owner", WorkspaceID: "ws-1", Limit: 100}
	page, err := reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	raw, err := json.Marshal(page)
	require.NoError(t, err)
	for _, canary := range []string{"SYNTHETIC_ENV_SECRET", "SYNTHETIC_MCP_SECRET", "SYNTHETIC_SCHEMA_SECRET", "SYNTHETIC_EXTENSION_SECRET", "FOREIGN_PROFILE", "legacy_task_tool"} {
		require.NotContains(t, string(raw), canary)
	}
	for _, entry := range page.Entries {
		require.False(t, entry.InspectAllowed)
		require.False(t, entry.Attached)
	}
	require.Contains(t, string(raw), "example-mcp")
	require.Contains(t, string(raw), "schema_partial")
	_, err = reader.ReadCapabilities(context.Background(), q)
	require.Error(t, err)
	q.OwnerID = "foreign"
	_, err = reader.ReadCapabilities(ctx, q)
	require.Error(t, err)
	require.NoError(t, adapter.taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "private-foreign", Name: "Foreign", OwnerID: "another-owner"}))
	q.OwnerID = "owner"
	q.WorkspaceID = "private-foreign"
	_, err = reader.ReadCapabilities(ctx, q)
	require.Error(t, err)
}

func TestAssistantCapabilitiesNativePaginationAndRevocation(t *testing.T) {
	_, svc := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	profiles := &capabilityTestProfiles{}
	for i := 0; i < 205; i++ {
		profiles.rows = append(profiles.rows, &settings.AgentProfile{ID: fmt.Sprintf("profile-%03d", i), Enabled: true})
	}
	reader := &assistantCapabilityReader{tasks: svc, profiles: profiles}
	q := shared.CapabilityQuery{OwnerID: "owner", WorkspaceID: "ws-1", Limit: 1000, Kind: "profile"}
	page, err := reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.Len(t, page.Entries, 100)
	q.After, q.Generation = page.After, page.Generation
	page, err = reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.Len(t, page.Entries, 100)
	require.Equal(t, "profile/profile-100", page.Entries[0].ID)
	q.After = page.After
	page, err = reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.Len(t, page.Entries, 5)
	require.Empty(t, page.After)
	require.Equal(t, "profile/profile-200", page.Entries[0].ID)
	profiles.rows[0].Enabled = false
	_, err = reader.ReadCapabilities(ctx, q)
	require.ErrorIs(t, err, shared.ErrCapabilityGeneration)
}

func TestAssistantCapabilitiesSessionAttachmentRevocation(t *testing.T) {
	adapter, svc := newOfficeTaskAdapterHarness(t)
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleMember})
	require.NoError(t, adapter.taskRepo.CreateTask(ctx, &taskmodels.Task{ID: "chat", WorkspaceID: "ws-1", Title: "Example", State: "CREATED"}))
	now := time.Now().UTC()
	history := streams.MCPAttachmentHistory{Version: 1, Current: streams.MCPAttachmentAttempt{AttemptID: "attempt", TaskID: "chat", SessionID: "session", ExecutionID: "execution", AgentProfileID: "own", StartedAt: now, UpdatedAt: now, Servers: []streams.MCPServerAttachment{{Name: "example", Status: streams.MCPAttachmentStatusActive, ConfiguredAt: &now, Tools: []streams.MCPToolSummary{{Name: "lookup", InputSchema: []byte(`{"type":"object"}`)}}}}}}
	session := &taskmodels.TaskSession{ID: "session", TaskID: "chat", AgentProfileID: "own", AgentExecutionID: "execution", State: taskmodels.TaskSessionStateRunning, Metadata: map[string]any{taskmodels.SessionMetaKeyMCPAttachmentState: history}}
	require.NoError(t, adapter.taskRepo.CreateTaskSession(ctx, session))
	require.NoError(t, adapter.taskRepo.UpsertExecutorRunning(ctx, &taskmodels.ExecutorRunning{ID: "running", SessionID: session.ID, TaskID: session.TaskID, AgentExecutionID: "execution", ExecutionProfileID: "own", Status: "running"}))
	profiles := &capabilityTestProfiles{rows: []*settings.AgentProfile{{ID: "own", Enabled: true}}}
	reader := &assistantCapabilityReader{tasks: svc, profiles: profiles}
	q := shared.CapabilityQuery{OwnerID: "owner", WorkspaceID: "ws-1", ConversationID: "chat", SessionID: "session", Kind: "mcp", Limit: 50}
	page, err := reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.Len(t, page.Entries, 2)
	require.True(t, page.Entries[0].Attached)
	profiles.rows[0].Enabled = false
	page, err = reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.False(t, page.Entries[0].Attached)
	profiles.rows[0].Enabled = true
	history.Current.Servers[0].DisconnectedAt = &now
	require.NoError(t, adapter.taskRepo.SetSessionMetadataKey(ctx, session.ID, taskmodels.SessionMetaKeyMCPAttachmentState, history))
	page, err = reader.ReadCapabilities(ctx, q)
	require.NoError(t, err)
	require.Equal(t, "disconnected", page.Entries[0].Health)
	q.ConversationID = "foreign"
	_, err = reader.ReadCapabilities(ctx, q)
	require.Error(t, err)
}
