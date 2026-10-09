package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

// @covers AC-PLUGINS-MANAGED-COORDINATION-006.2
func TestExactHumanInteraction(t *testing.T) {
	want := map[string]string{
		"RespondPermissionExact":   "host.v2.write:interactions",
		"AnswerClarificationExact": "host.v2.write:interactions",
	}
	got := make(map[string]string, len(exactHostMethods))
	for _, method := range exactHostMethods {
		got[method.method] = method.capability
	}
	for method, capability := range want {
		if got[method] != capability {
			t.Errorf("exact method %q capability = %q, want %q", method, got[method], capability)
		}
	}
}

func exactInteractionHost(t *testing.T, data *testDataHost, workspaceID string) (*pluginHost, *Service, string) {
	t.Helper()
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
	capabilities := manifest.Capabilities{APIRead: []string{"interactions"}, APIWrite: []string{"interactions"}}
	record := &pluginstore.Record{Manifest: manifest.Manifest{ID: "interaction-plugin", Capabilities: capabilities}, InstallationID: "interaction-installation"}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	svc.SetDataSources(data.tasks, data.workflows, data.steps, data.profiles, data.codeStats, data.messages, data.interactions, data.taskWriter)
	svc.SetInteractionResponder(data.responder)
	t.Cleanup(func() { _ = svc.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, workspaceID, 1, digest, []string{"host.v2.write:interactions"}, "human", "grant", "approval-interaction"); err != nil {
		t.Fatalf("grant exact interactions: %v", err)
	}
	return svc.hostForPlugin(record.ID).(*pluginHost), svc, digest
}

func TestExactHumanInteractionReceiptIsBoundAndSingleUse(t *testing.T) {
	data := newTestDataHost(readWriteCaps())
	interaction := pendingPermissionInteraction()
	withInteraction(data, interaction)
	data.tasks.tasksByID = map[string]*taskmodels.Task{interaction.TaskID: {ID: interaction.TaskID, WorkspaceID: "workspace-human", UpdatedAt: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)}}
	updated := *interaction
	updated.Status = taskmodels.InteractionStatusApproved
	data.interactions.afterWrite = &updated
	host, svc, digest := exactInteractionHost(t, data, "workspace-human")
	version := digestPublicValue(interactionModelToDTO(interaction))
	response := HumanInteractionResponse{Kind: string(taskmodels.InteractionKindPermission), OptionID: "allow"}
	if _, err := svc.IssueHumanInteractionResponseReceipt(context.Background(), "human-user", "workspace-human", interaction.ID, "sha256:stale", response); err != ErrHumanInteractionVersionChanged {
		t.Fatalf("stale receipt issue error = %v, want version conflict", err)
	}
	receipt, err := svc.IssueHumanInteractionResponseReceipt(context.Background(), "human-user", "workspace-human", interaction.ID, version, response)
	if err != nil {
		t.Fatalf("issue human receipt: %v", err)
	}
	manager, ok := pluginsdk.HostExactInteractionCommands(host)
	if !ok {
		t.Fatal("exact interaction commands are not exposed")
	}
	input := pluginsdk.ExactPermissionResponse{RequestID: "request-human", WorkspaceID: "workspace-human", InteractionID: interaction.ID, ExpectedResourceVersion: version, OptionID: "deny", HumanResponseReceiptID: receipt.ID, ApprovalRevision: 1, ManifestDigest: digest}
	forged, _, err := manager.RespondPermission(context.Background(), input)
	if err != nil || forged.Status != pluginsdk.CommandDenied || data.responder.writeCalled {
		t.Fatalf("forged response = %+v, wrote=%v, err=%v", forged, data.responder.writeCalled, err)
	}
	input.OptionID = "allow"
	result, got, err := manager.RespondPermission(context.Background(), input)
	if err != nil || result.Status != pluginsdk.CommandApplied || got.Status != pluginsdk.InteractionStatusApproved || !data.responder.writeCalled {
		t.Fatalf("human response = %+v interaction=%+v wrote=%v err=%v", result, got, data.responder.writeCalled, err)
	}
	replayed, _, err := manager.RespondPermission(context.Background(), input)
	if err != nil || replayed.Status != pluginsdk.CommandAlreadyApplied || data.responder.permission == nil {
		t.Fatalf("response replay = %+v err=%v", replayed, err)
	}
}

func TestExactClarificationRequiresHumanReceipt(t *testing.T) {
	data := newTestDataHost(readWriteCaps())
	interaction := pendingClarificationInteraction()
	withInteraction(data, interaction)
	data.tasks.tasksByID = map[string]*taskmodels.Task{interaction.TaskID: {ID: interaction.TaskID, WorkspaceID: "workspace-human", UpdatedAt: time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)}}
	updated := *interaction
	updated.Status = taskmodels.InteractionStatusAnswered
	data.interactions.afterWrite = &updated
	host, svc, digest := exactInteractionHost(t, data, "workspace-human")
	version := digestPublicValue(interactionModelToDTO(interaction))
	answers := []pluginsdk.ClarificationAnswer{{QuestionID: "q1", SelectedOptions: []string{"a"}, CustomText: "because"}}
	receipt, err := svc.IssueHumanInteractionResponseReceipt(context.Background(), "human-user", "workspace-human", interaction.ID, version, HumanInteractionResponse{Kind: string(taskmodels.InteractionKindClarification), Answers: answers})
	if err != nil {
		t.Fatalf("issue clarification receipt: %v", err)
	}
	manager, _ := pluginsdk.HostExactInteractionCommands(host)
	result, got, err := manager.AnswerClarification(context.Background(), pluginsdk.ExactClarificationResponse{RequestID: "request-clarification", WorkspaceID: "workspace-human", InteractionID: interaction.ID, ExpectedResourceVersion: version, Answers: answers, HumanResponseReceiptID: receipt.ID, ApprovalRevision: 1, ManifestDigest: digest})
	if err != nil || result.Status != pluginsdk.CommandApplied || got.Status != pluginsdk.InteractionStatusAnswered || data.responder.answered != interaction.ID || len(data.responder.answers) != 1 {
		t.Fatalf("clarification response = %+v interaction=%+v answered=%q answers=%+v err=%v", result, got, data.responder.answered, data.responder.answers, err)
	}
}
