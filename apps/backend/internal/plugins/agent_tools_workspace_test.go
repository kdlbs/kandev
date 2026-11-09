package plugins

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
)

func workspaceReadDeclaration(name string, surfaces ...string) manifest.AgentTool {
	readOnly := true
	return manifest.AgentTool{
		Name:            name,
		Description:     "Read workspace-owned data.",
		Surfaces:        surfaces,
		InvocationScope: manifest.AgentToolInvocationScopeWorkspace,
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"key": map[string]any{"type": "string"}},
		},
		Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
	}
}

func TestAgentToolCatalogTransportsWorkspaceScope(t *testing.T) {
	svc, _, _ := newTestService(t)
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-workspace-reader",
			AgentTools: []manifest.AgentTool{
				workspaceReadDeclaration("read_workspace_data",
					manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceExternal),
			},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	})

	snapshot, err := svc.AgentToolCatalog()
	if err != nil {
		t.Fatalf("AgentToolCatalog: %v", err)
	}
	if len(snapshot.Tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(snapshot.Tools))
	}
	tool := snapshot.Tools[0]
	if tool.InvocationScope != plugintools.InvocationScopeWorkspace || !tool.IsWorkspaceRead() {
		t.Fatalf("descriptor scope/read-only = %q/%v", tool.InvocationScope, tool.ReadOnlyHint)
	}
	if tool.IsInstanceRead() {
		t.Fatal("a workspace descriptor must not report instance-read eligibility")
	}
	bindings, err := svc.CoordinatorPluginBindings()
	if err != nil {
		t.Fatalf("CoordinatorPluginBindings: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("bindings = %#v, want none for a workspace tool", bindings)
	}
}

func TestExternalDispatchGateScopeWorkspaceConsistency(t *testing.T) {
	readOnly := true
	notReadOnly := false
	instanceTool := manifest.AgentTool{
		Name: "instance_read", Description: "Read instance data.",
		Surfaces:        []string{manifest.AgentToolSurfaceExternal},
		InvocationScope: manifest.AgentToolInvocationScopeInstance,
		InputSchema:     map[string]any{"type": "object"},
		Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
	}
	workspaceTool := workspaceReadDeclaration("workspace_read", manifest.AgentToolSurfaceExternal)
	taskTool := manifest.AgentTool{
		Name: "task_read", Description: "Read task data.",
		Surfaces: []string{manifest.AgentToolSurfaceExternal}, InputSchema: map[string]any{"type": "object"},
		Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
	}
	notReadOnlyWorkspaceTool := workspaceTool
	notReadOnlyWorkspaceTool.Annotations = manifest.AgentToolAnnotations{ReadOnlyHint: &notReadOnly}

	tests := []struct {
		name        string
		tool        manifest.AgentTool
		workspaceID string
		wantErr     string
	}{
		{name: "instance read without a workspace", tool: instanceTool},
		{name: "workspace read with a workspace", tool: workspaceTool, workspaceID: "ws-1"},
		{
			name: "instance read with a workspace", tool: instanceTool, workspaceID: "ws-1",
			wantErr: "does not accept a workspace",
		},
		{
			name: "workspace read without a workspace", tool: workspaceTool,
			wantErr: "requires a verified workspace",
		},
		{
			name: "workspace read without the read-only hint", tool: notReadOnlyWorkspaceTool, workspaceID: "ws-1",
			wantErr: "read-only hint",
		},
		{
			name: "task scope cannot dispatch externally", tool: taskTool, workspaceID: "ws-1",
			wantErr: "requires instance or workspace invocation scope",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDispatchSurfaceDeclaration(tc.tool, AgentToolInvocationContext{
				Surface:     manifest.AgentToolSurfaceExternal,
				WorkspaceID: tc.workspaceID,
			})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDispatchSurfaceDeclaration() unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateDispatchSurfaceDeclaration() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

// workspaceRecordingPlugin records the exact request the host sends over the
// real plugin RPC boundary.
type workspaceRecordingPlugin struct {
	pluginsdk.UnimplementedPlugin

	mu      sync.Mutex
	request *pluginsdk.AgentToolRequest
}

func (p *workspaceRecordingPlugin) InvokeAgentTool(
	_ context.Context, req *pluginsdk.AgentToolRequest,
) (*pluginsdk.AgentToolResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.request = req
	return &pluginsdk.AgentToolResult{Text: "workspace data"}, nil
}

func (p *workspaceRecordingPlugin) recorded() *pluginsdk.AgentToolRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.request
}

func newWorkspaceToolService(t *testing.T, tool manifest.AgentTool) (*Service, *workspaceRecordingPlugin) {
	t.Helper()
	svc, _, rt := newTestService(t)
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-workspace-reader", AgentTools: []manifest.AgentTool{tool},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
	svc.registry.Add(record)

	impl := &workspaceRecordingPlugin{}
	// A real Host keeps the go-plugin broker connection alive; the plugin
	// side dials it at startup and would otherwise retry until test teardown.
	gp := &pluginsdk.GRPCPlugin{
		Impl:            impl,
		Host:            newTestDataHost(manifest.Capabilities{}).host,
		HostDialTimeout: 5 * time.Second,
	}
	client, server := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: gp,
	})
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(server.Stop)
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	if err != nil {
		t.Fatalf("Dispense: %v", err)
	}
	remote, ok := raw.(*pluginsdk.RemotePlugin)
	if !ok {
		t.Fatalf("Dispense returned %T, want *pluginsdk.RemotePlugin", raw)
	}
	deadline := time.Now().Add(5 * time.Second)
	for impl.Host() == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if impl.Host() == nil {
		t.Fatal("plugin-side Host was not injected over the broker")
	}
	rt.mu.Lock()
	rt.running[record.ID] = true
	rt.remote = remote
	rt.mu.Unlock()
	return svc, impl
}

func TestWorkspaceAgentToolReceivesVerifiedContext(t *testing.T) {
	tool := workspaceReadDeclaration("read_workspace_data", manifest.AgentToolSurfaceExternal)
	svc, plugin := newWorkspaceToolService(t, tool)
	arguments := map[string]any{"key": "value"}

	result, err := svc.InvokeAgentTool(context.Background(), "kandev-workspace-reader", tool.Name, arguments,
		AgentToolInvocationContext{InvocationID: "invocation-1", Surface: manifest.AgentToolSurfaceExternal, WorkspaceID: "ws-1"})
	if err != nil {
		t.Fatalf("InvokeAgentTool: %v", err)
	}
	if result.Text != "workspace data" {
		t.Fatalf("result text = %q", result.Text)
	}
	request := plugin.recorded()
	if request == nil {
		t.Fatal("plugin did not receive the invocation")
	}
	if request.Context.WorkspaceID != "ws-1" || request.Context.Surface != manifest.AgentToolSurfaceExternal {
		t.Fatalf("plugin context = %#v", request.Context)
	}
	if request.Context.TaskID != "" || request.Context.SessionID != "" || request.Context.ExecutionID != "" {
		t.Fatalf("workspace read carried task identity: %#v", request.Context)
	}
	if len(request.Arguments) != 1 || request.Arguments["key"] != "value" {
		t.Fatalf("plugin arguments = %#v", request.Arguments)
	}
}

func TestExternalWorkspaceToolScopeMismatchFailsBeforeRPC(t *testing.T) {
	instanceTool := manifest.AgentTool{
		Name: "instance_read", Description: "Read instance data.",
		Surfaces:        []string{manifest.AgentToolSurfaceExternal},
		InvocationScope: manifest.AgentToolInvocationScopeInstance,
		InputSchema:     map[string]any{"type": "object"},
		Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: boolPointer(true)},
	}
	tests := []struct {
		name        string
		tool        manifest.AgentTool
		workspaceID string
		wantErr     string
	}{
		{name: "workspace scope without a workspace", tool: workspaceReadDeclaration("read_workspace_data", manifest.AgentToolSurfaceExternal), wantErr: "verified workspace"},
		{name: "instance scope with a workspace", tool: instanceTool, workspaceID: "ws-1", wantErr: "does not accept a workspace"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, plugin := newWorkspaceToolService(t, tc.tool)
			_, err := svc.InvokeAgentTool(context.Background(), "kandev-workspace-reader", tc.tool.Name, map[string]any{},
				AgentToolInvocationContext{InvocationID: "invocation-1", Surface: manifest.AgentToolSurfaceExternal, WorkspaceID: tc.workspaceID})
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("InvokeAgentTool error = %v, want substring %q", err, tc.wantErr)
			}
			if plugin.recorded() != nil {
				t.Fatal("scope mismatch reached the plugin RPC")
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
