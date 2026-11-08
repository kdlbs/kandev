package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kdlbs/kandev/pluginsdk"
)

func TestAgentToolCatalogTransportsScopeAndBindingIdentity(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-provider-usage",
			AgentTools: []manifest.AgentTool{{
				Name: "get_provider_usage", Description: "Read cached usage.",
				Surfaces:        []string{manifest.AgentToolSurfaceKanban, manifest.AgentToolSurfaceCoordinator, manifest.AgentToolSurfaceExternal},
				InvocationScope: manifest.AgentToolInvocationScopeInstance,
				InputSchema:     map[string]any{"type": "object"},
				Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
			}},
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
	if tool.InvocationScope != "instance" || !tool.IsInstanceRead() {
		t.Fatalf("descriptor scope/read-only = %q/%v", tool.InvocationScope, tool.ReadOnlyHint)
	}
	if tool.InstallationID != "installation-1" || tool.PolicyVersion != "policy-1" {
		t.Fatalf("descriptor binding identity = %q/%q", tool.InstallationID, tool.PolicyVersion)
	}

	bindings, err := svc.CoordinatorPluginBindings()
	if err != nil {
		t.Fatalf("CoordinatorPluginBindings: %v", err)
	}
	if len(bindings) != 1 || bindings[0].PluginID != "kandev-provider-usage" ||
		bindings[0].LocalName != "get_provider_usage" ||
		bindings[0].InstallationID != "installation-1" || bindings[0].PolicyVersion != "policy-1" {
		t.Fatalf("bindings = %#v", bindings)
	}
}

func TestCoordinatorPluginBindingsExcludeNonEligibleDeclarations(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	notReadOnly := false
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-eligibility",
			AgentTools: []manifest.AgentTool{
				{
					Name: "task_scoped", Description: "Task scoped.",
					Surfaces: []string{manifest.AgentToolSurfaceCoordinator}, InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
				},
				{
					Name: "not_read_only", Description: "Not read only.", InvocationScope: manifest.AgentToolInvocationScopeInstance,
					Surfaces: []string{manifest.AgentToolSurfaceCoordinator}, InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &notReadOnly},
				},
				{
					Name: "kanban_only", Description: "Kanban only.", InvocationScope: manifest.AgentToolInvocationScopeInstance,
					Surfaces: []string{manifest.AgentToolSurfaceKanban}, InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
				},
			},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	})

	bindings, err := svc.CoordinatorPluginBindings()
	if err != nil {
		t.Fatalf("CoordinatorPluginBindings: %v", err)
	}
	if len(bindings) != 0 {
		t.Fatalf("bindings = %#v, want none", bindings)
	}
}

func TestValidateCoordinatorPluginToolRejectsStaleState(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-provider-usage",
			AgentTools: []manifest.AgentTool{{
				Name: "get_provider_usage", Description: "Read cached usage.",
				Surfaces:        []string{manifest.AgentToolSurfaceCoordinator},
				InvocationScope: manifest.AgentToolInvocationScopeInstance,
				InputSchema:     map[string]any{"type": "object"},
				Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
			}},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
	svc.registry.Add(record)
	if err := svc.ValidateCoordinatorPluginTool("kandev-provider-usage", "get_provider_usage", "installation-1", "policy-1"); err != nil {
		t.Fatalf("current reference rejected: %v", err)
	}
	if err := svc.ValidateCoordinatorPluginTool("kandev-provider-usage", "get_provider_usage", "installation-2", "policy-1"); err == nil {
		t.Fatal("stale installation must be rejected")
	}
	if err := svc.ValidateCoordinatorPluginTool("kandev-provider-usage", "get_provider_usage", "installation-1", "policy-2"); err == nil {
		t.Fatal("stale policy version must be rejected")
	}
	disabled := *record
	disabled.Status = store.StatusDisabled
	svc.registry.Add(&disabled)
	if err := svc.ValidateCoordinatorPluginTool("kandev-provider-usage", "get_provider_usage", "installation-1", "policy-1"); err == nil {
		t.Fatal("disabled installation must be rejected")
	}
}

func TestInvokeInstanceSurfaceRevalidatesDeclaration(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	notReadOnly := false
	svc.registry.Add(&store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-provider-usage",
			AgentTools: []manifest.AgentTool{
				{
					Name: "task_scoped", Description: "Task scoped.",
					Surfaces: []string{manifest.AgentToolSurfaceExternal}, InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
				},
				{
					Name: "write_tool", Description: "Not read only.", InvocationScope: manifest.AgentToolInvocationScopeInstance,
					Surfaces: []string{manifest.AgentToolSurfaceExternal}, InputSchema: map[string]any{"type": "object"},
					Annotations: manifest.AgentToolAnnotations{ReadOnlyHint: &notReadOnly},
				},
			},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	})

	for name, want := range map[string]string{
		"task_scoped": "requires instance or workspace invocation scope",
		"write_tool":  "requires a true read-only hint",
	} {
		_, err := svc.InvokeAgentTool(context.Background(), "kandev-provider-usage", name, map[string]any{},
			AgentToolInvocationContext{InvocationID: "invocation-1", Surface: manifest.AgentToolSurfaceExternal})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s invocation error = %v, want declaration rejection %q", name, err, want)
		}
	}
}

func TestInvokeAgentToolRejectsUnsupportedSurface(t *testing.T) {
	svc, _, _ := newTestService(t)
	_, err := svc.InvokeAgentTool(context.Background(), "kandev-provider-usage", "get_provider_usage", map[string]any{},
		AgentToolInvocationContext{InvocationID: "invocation-1", Surface: "configuration"})
	if err == nil {
		t.Fatal("configuration surface must fail before plugin lookup")
	}
}

func TestInvokeCoordinatorToolRevalidatesPinnedPolicyUnderDispatch(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-provider-usage",
			AgentTools: []manifest.AgentTool{{
				Name: "get_provider_usage", Description: "Read cached usage.",
				Surfaces:        []string{manifest.AgentToolSurfaceCoordinator},
				InvocationScope: manifest.AgentToolInvocationScopeInstance,
				InputSchema:     map[string]any{"type": "object"},
				Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
			}},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
	svc.registry.Add(record)

	authorization := &plugintools.CoordinatorToolAuthorization{
		CoordinatorID: "coordinator-1", WorkspaceID: "workspace-1",
		ConversationTaskID: "task-1", PluginID: "kandev-provider-usage",
		LocalName: "get_provider_usage", InstallationID: "installation-1", PolicyVersion: "policy-1",
	}
	invocation := AgentToolInvocationContext{
		InvocationID: "invocation-1", TaskID: "task-1", SessionID: "session-1",
		WorkspaceID: "workspace-1", Surface: manifest.AgentToolSurfaceCoordinator,
		CoordinatorToolAuthorization: authorization,
	}

	// A policy rotation after the handler admitted the call must fail the
	// in-lease record check before any remote lookup.
	rotated := *record
	rotated.Status = store.StatusDisabled
	svc.registry.Add(&rotated)
	_, err := svc.InvokeAgentTool(context.Background(), "kandev-provider-usage", "get_provider_usage", map[string]any{}, invocation)
	if err == nil || !strings.Contains(err.Error(), "not active") {
		t.Fatalf("stale coordinator invocation error = %v, want in-lease rejection", err)
	}

	svc.registry.Add(record)
	_, err = svc.InvokeAgentTool(context.Background(), "kandev-provider-usage", "get_provider_usage", map[string]any{}, invocation)
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("current coordinator invocation error = %v, want remote lookup after authorization", err)
	}
}

func TestInvokeCoordinatorToolRejectsRotationInsideDispatchGap(t *testing.T) {
	svc, _, _ := newTestService(t)
	readOnly := true
	record := &store.Record{
		Manifest: manifest.Manifest{
			ID: "kandev-provider-usage",
			AgentTools: []manifest.AgentTool{{
				Name: "get_provider_usage", Description: "Read cached usage.",
				Surfaces:        []string{manifest.AgentToolSurfaceCoordinator},
				InvocationScope: manifest.AgentToolInvocationScopeInstance,
				InputSchema:     map[string]any{"type": "object"},
				Annotations:     manifest.AgentToolAnnotations{ReadOnlyHint: &readOnly},
			}},
		},
		Status:                 store.StatusActive,
		InstallationID:         "installation-1",
		AgentToolPolicyVersion: "policy-1",
	}
	svc.registry.Add(record)
	invocation := AgentToolInvocationContext{
		InvocationID: "invocation-1", TaskID: "task-1", SessionID: "session-1",
		WorkspaceID: "workspace-1", Surface: manifest.AgentToolSurfaceCoordinator,
		CoordinatorToolAuthorization: &plugintools.CoordinatorToolAuthorization{
			CoordinatorID: "coordinator-1", WorkspaceID: "workspace-1",
			ConversationTaskID: "task-1", PluginID: "kandev-provider-usage",
			LocalName: "get_provider_usage", InstallationID: "installation-1", PolicyVersion: "policy-1",
		},
	}

	lease := svc.dispatchLocks.lockFor("kandev-provider-usage")
	admitted := make(chan struct{})
	release := make(chan struct{})
	lease.onReadAdmission = func() {
		close(admitted)
		<-release
	}
	defer func() { lease.onReadAdmission = nil }()
	lease.Lock()
	started := make(chan struct{})
	results := make(chan error, 1)
	go func() {
		close(started)
		_, err := svc.InvokeAgentTool(context.Background(), "kandev-provider-usage", "get_provider_usage", map[string]any{}, invocation)
		results <- err
	}()
	<-started
	lease.Unlock()
	<-admitted
	rotated := *record
	rotated.AgentToolPolicyVersion = "policy-2"
	svc.registry.Add(&rotated)
	close(release)

	err := <-results
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("rotation inside the dispatch gap error = %v, want stale policy rejection", err)
	}
}

type capturingAgentToolRemote struct {
	context pluginsdk.AgentToolContext
}

func (r *capturingAgentToolRemote) InvokeAgentTool(_ context.Context, req *pluginsdk.AgentToolRequest) (*pluginsdk.AgentToolResult, error) {
	r.context = req.Context
	return &pluginsdk.AgentToolResult{Text: "ok"}, nil
}

func TestInvokeRemoteAgentToolSuppliesCoordinatorInstallation(t *testing.T) {
	remote := &capturingAgentToolRemote{}
	_, err := invokeRemoteAgentTool(context.Background(), remote, "get_provider_usage", map[string]any{},
		AgentToolInvocationContext{
			InvocationID: "invocation-1", Surface: manifest.AgentToolSurfaceCoordinator,
			CoordinatorToolAuthorization: &plugintools.CoordinatorToolAuthorization{
				InstallationID: "installation-1", PolicyVersion: "policy-1",
			},
		})
	if err != nil {
		t.Fatalf("invokeRemoteAgentTool: %v", err)
	}
	if remote.context.InstallationID != "installation-1" {
		t.Fatalf("installation context = %q, want installation-1", remote.context.InstallationID)
	}
}
