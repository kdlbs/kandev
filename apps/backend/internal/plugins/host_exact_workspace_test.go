package plugins

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

type workspaceAdminRecordingWriter struct {
	inputs []ExactWorkspaceAdminInput
}

func (w *workspaceAdminRecordingWriter) ApplyWorkspaceAdministrationExact(_ context.Context, input ExactWorkspaceAdminInput) (ExactWorkspaceAdminResult, error) {
	w.inputs = append(w.inputs, input)
	return ExactWorkspaceAdminResult{
		TargetID:        input.TargetID,
		ResourceVersion: "2026-09-26T12:00:00Z",
	}, nil
}

func TestExactWorkspaceAdministration(t *testing.T) {
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "workspace-admin-plugin", Capabilities: manifest.Capabilities{
			APIWrite: []string{"workspaces", "workflows", "repositories"},
		}},
		InstallationID: "workspace-admin-installation",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	const workspaceID = "workspace-admin"
	capabilities := []string{
		"host.v2.write:workspaces", "host.v2.write:workflows", "host.v2.write:repositories",
	}
	if _, err := svc.approvalGrant(record.InstallationID, workspaceID, 1,
		ManifestCapabilityDigest(record.Manifest), capabilities, "human", "grant", "workspace-admin-approval"); err != nil {
		t.Fatalf("grant capabilities: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.commandStore = commandStore
	unsupported, err := host.GetCapabilityContext(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("GetCapabilityContext without writer: %v", err)
	}
	for _, operation := range unsupported.Operations {
		if isExactWorkspaceAdminMethod(operation.Method) && (operation.Supported || operation.Authorized || operation.UnavailableReason != "workspace_administration_unavailable") {
			t.Errorf("workspace operation without writer = %+v; want unavailable", operation)
		}
	}

	writer := &workspaceAdminRecordingWriter{}
	host.workspaceAdminWriter = writer
	capabilityContext, err := host.GetCapabilityContext(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("GetCapabilityContext with writer: %v", err)
	}
	operations := make(map[string]pluginsdk.CapabilityOperation, len(capabilityContext.Operations))
	for _, operation := range capabilityContext.Operations {
		operations[operation.Method] = operation
	}
	for method, capability := range map[string]string{
		"UpdateWorkspaceDefaultsExact": "host.v2.write:workspaces",
		"CreateWorkflowExact":          "host.v2.write:workflows",
		"UpdateWorkflowExact":          "host.v2.write:workflows",
		"ReorderWorkflowsExact":        "host.v2.write:workflows",
		"CreateWorkflowStepExact":      "host.v2.write:workflows",
		"UpdateWorkflowStepExact":      "host.v2.write:workflows",
		"ReorderWorkflowStepsExact":    "host.v2.write:workflows",
		"RegisterRepositoryExact":      "host.v2.write:repositories",
		"UpdateRepositoryExact":        "host.v2.write:repositories",
	} {
		operation, found := operations[method]
		if !found || !operation.Supported || !operation.Authorized || operation.CapabilityID != capability {
			t.Errorf("capability operation %q = %+v, found=%v; want supported, approved %s", method, operation, found, capability)
		}
	}
	for _, method := range []string{
		"UpdateRepositoryScriptsExact", "UpdateRepositorySecretBindingsExact", "DeleteWorkflowExact",
		"DeleteWorkflowStepExact", "DetachRepositoryExact",
	} {
		if _, found := operations[method]; found {
			t.Errorf("human-only or destructive operation %q must not be exposed", method)
		}
	}

	manager, ok := pluginsdk.HostWorkspaceAdministration(host)
	if !ok {
		t.Fatal("plugin Host does not expose workspace administration commands")
	}
	name := "Coordinator workspace"
	command := pluginsdk.WorkspaceAdminCommand{
		RequestID: "workspace-defaults-request", WorkspaceID: workspaceID,
		IdempotencyKey: "workspace-defaults-1", ApprovalRevision: 1,
		ManifestDigest: ManifestCapabilityDigest(record.Manifest),
		UpdateDefaults: &pluginsdk.WorkspaceDefaultsUpdate{
			ExpectedResourceVersion: "2026-09-26T11:00:00Z",
			Name:                    &name,
		},
	}
	result, err := manager.Apply(context.Background(), command)
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied || result.Receipt == nil {
		t.Fatalf("apply workspace defaults = result:%+v err:%v", result, err)
	}
	if len(writer.inputs) != 1 || writer.inputs[0].OperationKind != pluginsdk.WorkspaceAdminUpdateDefaults ||
		writer.inputs[0].OperationID == "" || writer.inputs[0].TargetID != workspaceID || writer.inputs[0].PayloadDigest == "" {
		t.Fatalf("workspace writer admission = %+v", writer.inputs)
	}
	replayed, err := manager.Apply(context.Background(), command)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandAlreadyApplied || len(writer.inputs) != 1 {
		t.Fatalf("replayed workspace defaults = result:%+v calls:%d err:%v", replayed, len(writer.inputs), err)
	}
	invalidOrder, err := manager.Apply(context.Background(), pluginsdk.WorkspaceAdminCommand{
		RequestID: "workflow-reorder-missing-version", WorkspaceID: workspaceID,
		IdempotencyKey: "workflow-reorder-missing-version-1", ApprovalRevision: 1,
		ManifestDigest: ManifestCapabilityDigest(record.Manifest),
		ReorderWorkflows: &pluginsdk.WorkspaceWorkflowReorder{
			WorkflowIDs: []string{"workflow-1"}, WorkflowResourceVersions: []string{"2026-09-26T11:00:00Z"},
		},
	})
	if err != nil || invalidOrder == nil || invalidOrder.Status != pluginsdk.CommandInvalid || len(writer.inputs) != 1 {
		t.Fatalf("reorder without workspace version = result:%+v calls:%d err:%v", invalidOrder, len(writer.inputs), err)
	}
}
