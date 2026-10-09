package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

type sourceWritebackTaskData struct {
	taskDataSource
	workspaces []*taskmodels.Workspace
}

func (s sourceWritebackTaskData) ListWorkspaces(context.Context) ([]*taskmodels.Workspace, error) {
	return s.workspaces, nil
}

type sourceIssueControllerFake struct {
	capabilities *pluginsdk.SourceIssueCapabilities
	readErr      error
	readCalls    int
	apply        func(func() error) (string, error)
	sends        int
}

func (f *sourceIssueControllerFake) GetCapabilities(context.Context, string, string) (*pluginsdk.SourceIssueCapabilities, error) {
	f.readCalls++
	if f.capabilities == nil {
		return nil, f.readErr
	}
	copy := *f.capabilities
	copy.Transitions = append([]pluginsdk.SourceIssueTransition(nil), f.capabilities.Transitions...)
	return &copy, f.readErr
}

func (f *sourceIssueControllerFake) ApplyWriteback(_ context.Context, _ SourceIssueWritebackInput, beforeSend func() error) (string, error) {
	if f.apply != nil {
		return f.apply(beforeSend)
	}
	if err := beforeSend(); err != nil {
		return "", err
	}
	f.sends++
	return "provider-comment-1", nil
}

func TestSourceWritebackReceipts(t *testing.T) {
	ctx := context.Background()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	commandStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "source-writer", Capabilities: manifest.Capabilities{
			APIRead: []string{"source_issues"}, APIWrite: []string{"source_issues"},
		}},
		InstallationID: "source-writer-installation",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	const workspaceID = "source-writeback-workspace"
	if _, err := svc.approvalGrant(record.InstallationID, workspaceID, 1, ManifestCapabilityDigest(record.Manifest),
		[]string{"host.v2.read:source_issues", "host.v2.write:source_issues"}, "human", "grant", "source-writeback-approval"); err != nil {
		t.Fatalf("grant source issue capabilities: %v", err)
	}
	controller := &sourceIssueControllerFake{capabilities: &pluginsdk.SourceIssueCapabilities{
		WorkspaceID: workspaceID, TaskID: "task-source-1", TaskResourceVersion: "task-v1",
		Provider: "jira", SourceID: "jira-issue-42", Identifier: "ENG-42",
		URL: "https://jira.example/browse/ENG-42", Title: "Coordinate release",
		StatusID: "status-open", StatusName: "Open", ResourceVersion: "source-v1",
		Transitions: []pluginsdk.SourceIssueTransition{{TargetID: "transition-review", TargetName: "In Review"}},
	}}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.commandStore = commandStore
	host.sourceIssueController = controller
	host.taskData = sourceWritebackTaskData{workspaces: []*taskmodels.Workspace{{ID: workspaceID}}}
	manager, ok := pluginsdk.HostSourceIssueWriteback(host)
	if !ok {
		t.Fatal("plugin Host does not expose source issue writeback")
	}

	observation, capabilities, readReceipt, err := manager.GetCapabilities(ctx, pluginsdk.SourceIssueCapabilitiesQuery{
		RequestID: "source-read-request", WorkspaceID: workspaceID, TaskID: "task-source-1",
	})
	if err != nil || observation == nil || observation.Status != pluginsdk.CommandApplied || capabilities == nil ||
		capabilities.SourceID != "jira-issue-42" || readReceipt == nil || readReceipt.InstallationID != record.InstallationID {
		t.Fatalf("source capabilities = result:%+v item:%+v receipt:%+v err:%v", observation, capabilities, readReceipt, err)
	}

	comment := pluginsdk.SourceIssueWritebackCommand{
		RequestID: "source-comment-request", WorkspaceID: workspaceID, TaskID: "task-source-1",
		IdempotencyKey: "source-comment-1", ExpectedTaskResourceVersion: "task-v1",
		ExpectedSourceResourceVersion: "source-v1", ApprovalRevision: 1,
		ManifestDigest: ManifestCapabilityDigest(record.Manifest), Body: "Release is ready for review.",
	}
	result, receipt, err := manager.Comment(ctx, comment)
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied || receipt == nil ||
		receipt.InstallationID != record.InstallationID || receipt.TaskID != comment.TaskID ||
		receipt.Provider != "jira" || receipt.SourceID != "jira-issue-42" || receipt.Operation != "comment" ||
		receipt.ProviderReceiptID != "provider-comment-1" || receipt.State != "completed" || controller.sends != 1 {
		t.Fatalf("source comment = result:%+v receipt:%+v sends:%d err:%v", result, receipt, controller.sends, err)
	}
	replayed, replayReceipt, err := manager.Comment(ctx, comment)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandAlreadyApplied || replayReceipt == nil ||
		replayReceipt.OperationID != receipt.OperationID || controller.sends != 1 {
		t.Fatalf("source comment replay = result:%+v receipt:%+v sends:%d err:%v", replayed, replayReceipt, controller.sends, err)
	}

	uncertain := comment
	uncertain.RequestID = "source-comment-timeout"
	uncertain.IdempotencyKey = "source-comment-timeout-1"
	controller.apply = func(beforeSend func() error) (string, error) {
		if err := beforeSend(); err != nil {
			return "", err
		}
		controller.sends++
		return "", errors.New("connection closed after provider accepted the comment")
	}
	unknown, unknownReceipt, err := manager.Comment(ctx, uncertain)
	if err != nil || unknown == nil || unknown.Status != pluginsdk.CommandUncertain || unknownReceipt == nil ||
		unknownReceipt.State != "uncertain" || unknownReceipt.Reason != "provider_outcome_unknown" || controller.sends != 2 {
		t.Fatalf("uncertain source comment = result:%+v receipt:%+v sends:%d err:%v", unknown, unknownReceipt, controller.sends, err)
	}
	unknownReplay, unknownReplayReceipt, err := manager.Comment(ctx, uncertain)
	if err != nil || unknownReplay == nil || unknownReplay.Status != pluginsdk.CommandUncertain || unknownReplayReceipt == nil ||
		unknownReplayReceipt.OperationID != unknownReceipt.OperationID || controller.sends != 2 {
		t.Fatalf("uncertain source comment replay = result:%+v receipt:%+v sends:%d err:%v", unknownReplay, unknownReplayReceipt, controller.sends, err)
	}

	stale := comment
	stale.RequestID = "source-comment-stale"
	stale.IdempotencyKey = "source-comment-stale-1"
	controller.apply = func(func() error) (string, error) {
		return "", &SourceIssueError{Kind: SourceIssueErrorStale}
	}
	staleResult, staleReceipt, err := manager.Comment(ctx, stale)
	if err != nil || staleResult == nil || staleResult.Status != pluginsdk.CommandConflict || staleReceipt == nil ||
		staleReceipt.Reason != "source_link_changed" || controller.sends != 2 {
		t.Fatalf("stale source link = result:%+v receipt:%+v sends:%d err:%v", staleResult, staleReceipt, controller.sends, err)
	}

	limited := comment
	limited.RequestID = "source-comment-limited"
	limited.IdempotencyKey = "source-comment-limited-1"
	controller.apply = func(beforeSend func() error) (string, error) {
		if err := beforeSend(); err != nil {
			return "", err
		}
		controller.sends++
		return "", &SourceIssueError{Kind: SourceIssueErrorRateLimited}
	}
	limitedResult, limitedReceipt, err := manager.Comment(ctx, limited)
	if err != nil || limitedResult == nil || limitedResult.Status != pluginsdk.CommandRateLimited || limitedReceipt == nil ||
		limitedReceipt.Reason != "provider_rate_limited" || controller.sends != 3 {
		t.Fatalf("rate-limited source write = result:%+v receipt:%+v sends:%d err:%v", limitedResult, limitedReceipt, controller.sends, err)
	}
	limitedReplay, _, err := manager.Comment(ctx, limited)
	if err != nil || limitedReplay == nil || limitedReplay.Status != pluginsdk.CommandRateLimited || controller.sends != 3 {
		t.Fatalf("rate-limited source write replay = result:%+v sends:%d err:%v", limitedReplay, controller.sends, err)
	}

	restartCommand := comment
	restartCommand.RequestID = "source-comment-restart"
	restartCommand.IdempotencyKey = "source-comment-restart-1"
	restartDigest, err := sourceIssueWritebackDigest("comment", restartCommand)
	if err != nil {
		t.Fatal(err)
	}
	commandRecord, _, err := commandStore.Admit(ctx, state.CommandIntent{
		InstallationID: record.InstallationID, WorkspaceID: workspaceID, RequestID: restartCommand.RequestID,
		IdempotencyKey: restartCommand.IdempotencyKey, Method: exactSourceIssueCommentMethod,
		CapabilityID: "host.v2.write:source_issues", PayloadDigest: restartDigest, TargetID: restartCommand.TaskID,
		ExpectedResourceVersion: restartCommand.ExpectedTaskResourceVersion + "/" + restartCommand.ExpectedSourceResourceVersion,
		ApprovalRevision:        restartCommand.ApprovalRevision, ManifestDigest: restartCommand.ManifestDigest,
	})
	if err != nil {
		t.Fatalf("admit pre-restart source write: %v", err)
	}
	prepared, _, err := commandStore.PrepareSourceWriteback(ctx, state.SourceWritebackIntent{
		OperationID: commandRecord.Intent.OperationID, InstallationID: record.InstallationID,
		WorkspaceID: workspaceID, TaskID: restartCommand.TaskID, Provider: "jira", SourceID: "jira-issue-42",
		Operation: "comment", ExpectedSourceVersion: restartCommand.ExpectedSourceResourceVersion, PayloadDigest: restartDigest,
	})
	if err != nil {
		t.Fatalf("prepare pre-restart source write: %v", err)
	}
	if _, started, err := commandStore.StartSourceWriteback(ctx, prepared.OperationID); err != nil || !started {
		t.Fatalf("start pre-restart source write = started %v, err %v", started, err)
	}
	reopenedStore, err := state.NewCommandStore(db.NewPool(connection, connection))
	if err != nil {
		t.Fatalf("reopen source writeback store: %v", err)
	}
	host.commandStore = reopenedStore
	controller.capabilities = nil
	controller.readErr = &SourceIssueError{Kind: SourceIssueErrorNotFound}
	readsBeforeReplay := controller.readCalls
	uncertainAfterRestart, restartReceipt, err := manager.Comment(ctx, restartCommand)
	if err != nil || uncertainAfterRestart == nil || uncertainAfterRestart.Status != pluginsdk.CommandUncertain ||
		restartReceipt == nil || restartReceipt.State != "uncertain" || controller.readCalls != readsBeforeReplay || controller.sends != 3 {
		t.Fatalf("recovered in-flight source write = result:%+v receipt:%+v reads:%d sends:%d err:%v",
			uncertainAfterRestart, restartReceipt, controller.readCalls, controller.sends, err)
	}
}
