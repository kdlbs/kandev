package plugins

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAgentToolCatalogIncludesActiveTools(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	destructive := false
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID:          "task-tags.v1",
			DisplayName: "Task tags",
			AgentTools: []manifest.AgentTool{{
				Name:        "add_tag",
				Description: "Add a tag to a task",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{"tag": map[string]any{"type": "string"}},
					"required":   []any{"tag"},
				},
				Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly, DestructiveHint: &destructive},
			}},
		},
		Status: store.StatusActive,
	})
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "disabled.v1",
			AgentTools: []manifest.AgentTool{{
				Name:        "hidden",
				Description: "Hidden tool",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		Status: store.StatusDisabled,
	})

	snapshot, err := svc.AgentToolCatalog()
	if err != nil {
		t.Fatalf("AgentToolCatalog() error: %v", err)
	}
	if snapshot.Generation == "" || snapshot.Revision == 0 {
		t.Fatalf("snapshot identity = %#v, want generation and revision", snapshot)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("snapshot tools = %d, want 1", len(snapshot.Tools))
	}
	tool := snapshot.Tools[0]
	if tool.ExposedName != plugintools.ExposedName("task-tags.v1", "add_tag") {
		t.Fatalf("exposed name = %q", tool.ExposedName)
	}
	if !tool.ReadOnlyHint || tool.DestructiveHint {
		t.Fatalf("annotations = %#v, want read-only true and destructive false", tool)
	}
	again, err := svc.AgentToolCatalog()
	if err != nil {
		t.Fatalf("second AgentToolCatalog() error: %v", err)
	}
	if again.Revision != snapshot.Revision {
		t.Fatalf("revision changed without catalog change: %d -> %d", snapshot.Revision, again.Revision)
	}
}

func TestAgentToolCatalogUsesConservativeAnnotationDefaults(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "defaults", AgentTools: []manifest.AgentTool{{
				Name: "echo", Description: "Echo input.",
				Surfaces:    []string{manifest.AgentToolSurfaceKanban},
				InputSchema: map[string]any{"type": "object"},
			}},
		},
		Status: store.StatusActive,
	})

	snapshot, err := svc.AgentToolCatalog()
	require.NoError(t, err)
	require.Len(t, snapshot.Tools, 1)
	tool := snapshot.Tools[0]
	require.False(t, tool.ReadOnlyHint)
	require.True(t, tool.DestructiveHint)
	require.False(t, tool.IdempotentHint)
	require.True(t, tool.OpenWorldHint)
}

func TestInvokeAgentToolLogsOnlyBoundMetadata(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc := NewService(store.NewFSStore(t.TempDir()), NewRegistry(), nil, log)

	_, err = svc.InvokeAgentTool(context.Background(), "missing", "echo", map[string]any{
		"secret": "do-not-log",
	}, AgentToolInvocationContext{
		InvocationID: "invoke-1", TaskID: "task-1", SessionID: "session-1",
	})
	require.Error(t, err)

	entries := observed.FilterMessage("plugin agent tool invocation").All()
	require.Len(t, entries, 1)
	fields := entries[0].ContextMap()
	require.Equal(t, "invoke-1", fields["invocation_id"])
	require.Equal(t, "missing", fields["plugin_id"])
	require.Equal(t, "echo", fields["local_name"])
	require.Equal(t, "task-1", fields["task_id"])
	require.Equal(t, "session-1", fields["session_id"])
	require.Equal(t, "error", fields["outcome"])
	require.NotContains(t, fields, "arguments")
	require.NotContains(t, fields, "result")
}

func TestInvokeManagedAgentToolRequiresCurrentApprovedSelection(t *testing.T) {
	registry := NewRegistry()
	core, observed := observer.New(zap.InfoLevel)
	managedLogger, err := logger.NewFromZap(zap.New(core))
	require.NoError(t, err)
	svc := NewService(store.NewFSStore(t.TempDir()), registry, nil, managedLogger)
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID:           "coordinator.v1",
			Capabilities: manifest.Capabilities{APIWrite: []string{"managed_agent_tools"}},
			AgentTools: []manifest.AgentTool{{
				Name: "read_task", Description: "Read one task.",
				Surfaces: []string{manifest.AgentToolSurfaceManaged}, InputSchema: map[string]any{
					"type": "object", "properties": map[string]any{"secret": map[string]any{"type": "string"}},
				},
			}},
		},
		InstallationID: "installation-1", Status: StatusActive,
	}
	registry.Add(record)
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.GrantCapabilityApproval("installation-1", "workspace-1", 1, digest,
		[]string{"host.v2.write:managed_agent_tools"}, "human", "grant", "grant-1"); err != nil {
		t.Fatalf("grant managed tool capability: %v", err)
	}
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: record.ID, InstallationID: record.InstallationID, WorkspaceID: "workspace-1", InstanceKey: "lead",
		ConversationRevision: 4, ApprovalRevision: 1, ManifestDigest: digest, AgentToolNames: []string{"read_task"},
	}
	invocation := AgentToolInvocationContext{
		InvocationID: "invocation-1", TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		Surface: manifest.AgentToolSurfaceManaged, ExecutionID: "execution-1", ManagedToolPolicy: &policy,
	}
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "read_task", map[string]any{"secret": "do-not-log"}, invocation)
	if err == nil || !strings.Contains(err.Error(), "is not running") {
		t.Fatalf("approved selected tool error = %v, want dispatch to the active plugin boundary", err)
	}

	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "unselected", map[string]any{}, invocation)
	if err == nil || !strings.Contains(err.Error(), "policy denied") {
		t.Fatalf("unselected tool error = %v, want policy denial", err)
	}

	forgedWorkspace := invocation
	forgedWorkspace.WorkspaceID = "workspace-other"
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "read_task", map[string]any{}, forgedWorkspace)
	if err == nil || !strings.Contains(err.Error(), "policy denied") {
		t.Fatalf("forged workspace error = %v, want policy denial", err)
	}

	if _, err := svc.RevokeCapabilityApproval(record.InstallationID, "workspace-1", 1, "human", "revoke", "revoke-1"); err != nil {
		t.Fatalf("revoke managed tool capability: %v", err)
	}
	_, err = svc.InvokeAgentTool(context.Background(), record.ID, "read_task", map[string]any{}, invocation)
	if err == nil || !strings.Contains(err.Error(), "capability denied") {
		t.Fatalf("revoked tool error = %v, want current approval denial", err)
	}
	entries := observed.FilterMessage("managed plugin agent tool invocation").All()
	if len(entries) != 4 {
		t.Fatalf("managed tool audit entries = %d, want one for each call", len(entries))
	}
	for _, entry := range entries {
		fields := entry.ContextMap()
		if _, found := fields["arguments"]; found {
			t.Fatalf("managed tool audit logged arguments: %#v", fields)
		}
		if _, found := fields["execution_id"]; found {
			t.Fatalf("managed tool audit logged the execution handle: %#v", fields)
		}
		if strings.Contains(entry.Message, "do-not-log") {
			t.Fatalf("managed tool audit logged tool input: %s", entry.Message)
		}
	}
}

type callbackManagedAgentToolRemote struct {
	call func(context.Context, *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error)
}

func (r callbackManagedAgentToolRemote) InvokeAgentTool(ctx context.Context, request *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error) {
	type outcome struct {
		result *pluginsdk.AgentToolResult
		err    error
	}
	completed := make(chan outcome, 1)
	go func() {
		result, err := r.call(ctx, request)
		completed <- outcome{result: result, err: err}
	}()
	select {
	case response := <-completed:
		return response.result, response.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestManagedAgentToolCanCallBackIntoRealHostAuthorizationReadAndWrite(t *testing.T) {
	connection, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	require.NoError(t, err)

	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "coordinator.v1",
			Capabilities: manifest.Capabilities{
				APIRead:  []string{"tasks"},
				APIWrite: []string{"tasks", "managed_agent_tools"},
			},
			AgentTools: []manifest.AgentTool{{
				Name: "read_task", Description: "Read and update the selected task.",
				Surfaces:    []string{manifest.AgentToolSurfaceManaged},
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			}},
		},
		InstallationID: "installation-callback", Status: StatusActive,
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(store.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	require.NoError(t, svc.SetPluginsDir(t.TempDir()))
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	grantedCapabilities := []string{
		"host.v2.read:tasks", "host.v2.write:tasks", "host.v2.write:managed_agent_tools",
	}
	_, err = svc.approvalGrant(record.InstallationID, "workspace-callback", 1, digest,
		grantedCapabilities, "human", "grant", "approval-callback")
	require.NoError(t, err)

	host := svc.hostForPlugin(record.ID).(*pluginHost)
	writer := &exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}}
	host.taskWriter = writer
	host.taskData = &fakeTaskDataSource{tasksByID: map[string]*taskmodels.Task{
		"task-callback": {
			ID: "task-callback", WorkspaceID: "workspace-callback", Title: "before",
			UpdatedAt: time.Date(2026, 9, 25, 11, 0, 0, 0, time.UTC),
		},
	}, workspaces: []*taskmodels.Workspace{{ID: "workspace-callback"}}}
	policy := mcpprofile.ManagedToolPolicy{
		PluginID: record.ID, InstallationID: record.InstallationID, WorkspaceID: "workspace-callback", InstanceKey: "lead",
		ConversationRevision: 1, ApprovalRevision: 1, ManifestDigest: digest, AgentToolNames: []string{"read_task"},
	}
	invocation := AgentToolInvocationContext{
		InvocationID: "callback-invocation", TaskID: "task-callback", SessionID: "session-callback",
		WorkspaceID: "workspace-callback", Surface: manifest.AgentToolSurfaceManaged,
		ExecutionID: "execution-callback", ManagedToolPolicy: &policy,
	}

	callbackFinished := make(chan struct{})
	remote := callbackManagedAgentToolRemote{call: func(ctx context.Context, _ *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error) {
		defer close(callbackFinished)
		capability, err := host.GetCapabilityContext(ctx, "workspace-callback")
		if err != nil || capability.ApprovalRevision != 1 {
			return nil, err
		}
		observedTask, _, err := host.GetTask(ctx, pluginsdk.ExactTaskGetQuery{
			RequestID: "callback-read", WorkspaceID: "workspace-callback", TaskID: "task-callback",
		})
		if err != nil || observedTask.Task.Title != "before" {
			return nil, err
		}
		title := "after callback"
		updated, _, err := host.UpdateTaskExact(ctx, pluginsdk.ExactTaskUpdate{
			RequestID: "callback-write", WorkspaceID: "workspace-callback", TaskID: "task-callback",
			IdempotencyKey: "callback-update", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
			ApprovalRevision: 1, ManifestDigest: digest, Title: &title,
		})
		if err != nil || updated.Status != pluginsdk.CommandApplied || writer.exactCalls != 1 {
			return nil, err
		}
		return &pluginsdk.AgentToolResult{Text: "updated task"}, nil
	}}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	result, err := svc.invokeManagedAgentToolWithRemote(ctx, record.ID, "read_task", map[string]any{}, invocation,
		func() (managedAgentToolRemote, error) { return remote, nil })
	if err != nil {
		select {
		case <-callbackFinished:
		case <-time.After(time.Second):
			t.Fatal("remote Host callback did not finish after tool RPC returned")
		}
		t.Fatalf("managed tool callback returned %v", err)
	}
	require.Equal(t, "updated task", result.Text)
	select {
	case <-callbackFinished:
	case <-time.After(time.Second):
		t.Fatal("remote Host callback did not finish")
	}
	require.Equal(t, 1, writer.exactCalls)
}
