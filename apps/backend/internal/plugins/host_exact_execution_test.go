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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// @covers AC-PLUGINS-COORDINATION-006.1
func TestExactExecutionControls(t *testing.T) {
	want := map[string]string{
		"EnsureTaskRunExact":               "host.v2.write:execution",
		"StopTaskRunExact":                 "host.v2.write:execution",
		"RecoverSessionExact":              "host.v2.write:execution",
		"CancelPendingTaskTransitionExact": "host.v2.write:execution",
		"GetSessionModeContextExact":       "host.v2.read:sessions",
		"SetSessionModeExact":              "host.v2.write:execution",
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

type fakeExactExecutionController struct {
	ensureCalls, stopCalls, recoverCalls, cancelCalls, modeCalls int
	activeExecutionID                                            string
	ensureErr, recoverErr                                        error
	callerInstallations                                          []string
	ensureInput                                                  pluginsdk.ExactTaskRunCommand
	stopInput                                                    pluginsdk.ExactTaskExecutionCommand
	recoverInput                                                 pluginsdk.ExactSessionRecoveryCommand
	cancelInput                                                  pluginsdk.ExactPendingTransitionCommand
	modeReadInput                                                pluginsdk.ExactTaskExecutionCommand
	modeSetInput                                                 pluginsdk.ExactSessionModeCommand
}

func (f *fakeExactExecutionController) EnsureTaskRun(_ context.Context, installationID string, in pluginsdk.ExactTaskRunCommand) (pluginsdk.ExactTaskRunResult, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.ensureInput = in
	f.ensureCalls++
	if f.ensureErr != nil {
		return pluginsdk.ExactTaskRunResult{}, f.ensureErr
	}
	return pluginsdk.ExactTaskRunResult{SessionID: "session-exact", SessionState: "RUNNING", ExecutionID: "exec-exact"}, nil
}

func (f *fakeExactExecutionController) StopTaskRun(_ context.Context, installationID string, in pluginsdk.ExactTaskExecutionCommand) (bool, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.stopInput = in
	f.stopCalls++
	if in.ExpectedExecutionID != f.activeExecutionID {
		return false, status.Error(codes.Aborted, "replacement execution")
	}
	return true, nil
}

func (f *fakeExactExecutionController) RecoverSession(_ context.Context, installationID string, in pluginsdk.ExactSessionRecoveryCommand) (pluginsdk.ExactTaskRunResult, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.recoverInput = in
	f.recoverCalls++
	if f.recoverErr != nil {
		return pluginsdk.ExactTaskRunResult{}, f.recoverErr
	}
	return pluginsdk.ExactTaskRunResult{SessionID: "session-exact", ExecutionID: "exec-recovered", SessionState: "RUNNING"}, nil
}

func (f *fakeExactExecutionController) CancelPendingTaskTransition(_ context.Context, installationID string, in pluginsdk.ExactPendingTransitionCommand) (bool, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.cancelInput = in
	f.cancelCalls++
	return in.TransitionID == "transition-exact", nil
}

func (f *fakeExactExecutionController) GetSessionModeContext(_ context.Context, installationID string, in pluginsdk.ExactTaskExecutionCommand) (pluginsdk.SessionModeContext, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.modeReadInput = in
	return pluginsdk.SessionModeContext{SessionID: "session-exact", ExecutionID: f.activeExecutionID, AvailableModes: []pluginsdk.SessionModeOption{{ID: safeManagedSessionModeID}, {ID: "bypassPermissions"}}}, nil
}

func (f *fakeExactExecutionController) SetSessionMode(_ context.Context, installationID string, in pluginsdk.ExactSessionModeCommand) (string, error) {
	f.callerInstallations = append(f.callerInstallations, installationID)
	f.modeSetInput = in
	f.modeCalls++
	if in.ExpectedExecutionID != f.activeExecutionID {
		return "", status.Error(codes.Aborted, "replacement execution")
	}
	return in.ModeID, nil
}

func newExactExecutionHost(t *testing.T, controller *fakeExactExecutionController) (*pluginHost, string) {
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
	record := &pluginstore.Record{Manifest: manifest.Manifest{ID: "execution-plugin", Capabilities: manifest.Capabilities{APIRead: []string{"sessions"}, APIWrite: []string{"execution"}}}, InstallationID: "execution-installation"}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	svc.SetExactExecutionController(controller)
	t.Cleanup(func() { _ = svc.Close() })
	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-execution", 1, digest,
		[]string{"host.v2.read:sessions", "host.v2.write:execution"}, "human", "grant", "approval-execution"); err != nil {
		t.Fatalf("grant exact execution: %v", err)
	}
	return svc.hostForPlugin(record.ID).(*pluginHost), digest
}

func TestExactExecutionControlsGenerationAndReplay(t *testing.T) {
	controller := &fakeExactExecutionController{activeExecutionID: "exec-current"}
	host, digest := newExactExecutionHost(t, controller)
	host.taskData = &fakeTaskDataSource{workspaces: []*taskmodels.Workspace{{ID: "workspace-execution"}}}
	manager, ok := pluginsdk.HostExecutionCommands(host)
	if !ok {
		t.Fatal("execution commands are not exposed")
	}
	ensure := pluginsdk.ExactTaskRunCommand{RequestID: "request-ensure", WorkspaceID: "workspace-execution", TaskID: "task-exact", IdempotencyKey: "ensure-1", ExpectedTaskResourceVersion: "2026-09-26T09:00:00Z", ApprovalRevision: 1, ManifestDigest: digest}
	ensure.ManagementInstanceKey, ensure.ExpectedClaimGeneration = "delivery-lead", 4
	first, run, err := manager.EnsureTaskRun(context.Background(), ensure)
	if err != nil || first.Status != pluginsdk.CommandApplied || run.SessionID != "session-exact" {
		t.Fatalf("ensure = %+v, run=%+v, err=%v", first, run, err)
	}
	replayed, replayRun, err := manager.EnsureTaskRun(context.Background(), ensure)
	if err != nil || replayed.Status != pluginsdk.CommandAlreadyApplied || replayRun.SessionID != run.SessionID ||
		replayRun.SessionState != run.SessionState || replayRun.ExecutionID != run.ExecutionID || controller.ensureCalls != 1 {
		t.Fatalf("ensure replay = %+v, run=%+v, calls=%d, err=%v", replayed, replayRun, controller.ensureCalls, err)
	}
	base := pluginsdk.ExactTaskExecutionCommand{RequestID: "request-stop", WorkspaceID: "workspace-execution", TaskID: "task-exact", SessionID: "session-exact", ExpectedSessionResourceVersion: "2026-09-26T09:01:00Z", ExpectedExecutionID: "exec-old", IdempotencyKey: "stop-old", ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 4, ApprovalRevision: 1, ManifestDigest: digest}
	stale, stopped, err := manager.StopTaskRun(context.Background(), base)
	if err != nil || stale.Status != pluginsdk.CommandConflict || stopped {
		t.Fatalf("stale stop = %+v stopped=%v err=%v", stale, stopped, err)
	}
	base.ExpectedExecutionID, base.IdempotencyKey = "exec-current", "stop-current"
	stoppedResult, stopped, err := manager.StopTaskRun(context.Background(), base)
	if err != nil || stoppedResult.Status != pluginsdk.CommandApplied || !stopped {
		t.Fatalf("exact stop = %+v stopped=%v err=%v", stoppedResult, stopped, err)
	}
	replayedStop, stopped, err := manager.StopTaskRun(context.Background(), base)
	if err != nil || replayedStop.Status != pluginsdk.CommandAlreadyApplied || !stopped || controller.stopCalls != 2 {
		t.Fatalf("stop replay = %+v stopped=%v calls=%d err=%v", replayedStop, stopped, controller.stopCalls, err)
	}
	recoverInput := pluginsdk.ExactSessionRecoveryCommand{ExactTaskExecutionCommand: base, Action: "fresh_start"}
	recovered, _, err := manager.RecoverSession(context.Background(), recoverInput)
	if err != nil || recovered.Status != pluginsdk.CommandInvalid || controller.recoverCalls != 0 {
		t.Fatalf("unsafe recovery = %+v, calls=%d, err=%v", recovered, controller.recoverCalls, err)
	}
	recoverInput.Action, recoverInput.IdempotencyKey = "resume", "recover-resume"
	recovered, run, err = manager.RecoverSession(context.Background(), recoverInput)
	if err != nil || recovered.Status != pluginsdk.CommandApplied || run.ExecutionID != "exec-recovered" {
		t.Fatalf("resume recovery = %+v, run=%+v, err=%v", recovered, run, err)
	}
	replayedRecovery, replayRun, err := manager.RecoverSession(context.Background(), recoverInput)
	if err != nil || replayedRecovery.Status != pluginsdk.CommandAlreadyApplied || replayRun.SessionID != run.SessionID ||
		replayRun.SessionState != run.SessionState || replayRun.ExecutionID != run.ExecutionID || controller.recoverCalls != 1 {
		t.Fatalf("recovery replay = %+v run=%+v calls=%d err=%v", replayedRecovery, replayRun, controller.recoverCalls, err)
	}
	cancelInput := pluginsdk.ExactPendingTransitionCommand{RequestID: "request-cancel", WorkspaceID: "workspace-execution", TaskID: "task-exact", TransitionID: "transition-exact", ExpectedResourceVersion: "sha256:transition-version", IdempotencyKey: "cancel-transition", ManagementInstanceKey: "delivery-lead", ExpectedClaimGeneration: 4, ApprovalRevision: 1, ManifestDigest: digest}
	cancel, err := manager.CancelPendingTaskTransition(context.Background(), cancelInput)
	if err != nil || cancel.Status != pluginsdk.CommandApplied || controller.cancelCalls != 1 {
		t.Fatalf("cancel transition = %+v, calls=%d, err=%v", cancel, controller.cancelCalls, err)
	}
	modeRead := base
	modeRead.RequestID = "request-mode-read"
	modeRead.IdempotencyKey = ""
	modeRead.ExpectedExecutionID = ""
	modeContext, err := manager.GetSessionModeContext(context.Background(), modeRead)
	if err != nil || modeContext.SessionID != "session-exact" {
		t.Fatalf("mode context = %+v, err=%v", modeContext, err)
	}
	modeSet := pluginsdk.ExactSessionModeCommand{ExactTaskExecutionCommand: base, ModeID: safeManagedSessionModeID}
	modeSet.RequestID = "request-mode-set"
	modeSet.IdempotencyKey = "mode-set"
	modeSetResult, modeID, err := manager.SetSessionMode(context.Background(), modeSet)
	if err != nil || modeSetResult.Status != pluginsdk.CommandApplied || modeID != safeManagedSessionModeID {
		t.Fatalf("mode set = %+v mode=%q err=%v", modeSetResult, modeID, err)
	}
	for _, unsafeMode := range []string{"acceptEdits", "default", "bypassPermissions", "Plan"} {
		unsafeInput := modeSet
		unsafeInput.RequestID = "request-mode-unsafe-" + unsafeMode
		unsafeInput.IdempotencyKey = "mode-unsafe-" + unsafeMode
		unsafeInput.ModeID = unsafeMode
		unsafeResult, _, unsafeErr := manager.SetSessionMode(context.Background(), unsafeInput)
		if unsafeErr != nil || unsafeResult.Status != pluginsdk.CommandInvalid || controller.modeCalls != 1 {
			t.Fatalf("unsafe mode %q = %+v calls=%d err=%v", unsafeMode, unsafeResult, controller.modeCalls, unsafeErr)
		}
	}
	if got := safeSessionModes([]pluginsdk.SessionModeOption{
		{ID: safeManagedSessionModeID}, {ID: "acceptEdits"}, {ID: "default"}, {ID: "bypassPermissions"}, {ID: "dont-ask"},
		{ID: "fast", Name: "Auto approve all permissions"},
	}); len(got) != 1 || got[0].ID != safeManagedSessionModeID {
		t.Fatalf("safe modes = %+v", got)
	}
	if controller.ensureInput.ManagementInstanceKey != "delivery-lead" || controller.ensureInput.ExpectedClaimGeneration != 4 ||
		controller.stopInput.ManagementInstanceKey != "delivery-lead" || controller.stopInput.ExpectedClaimGeneration != 4 ||
		controller.recoverInput.ManagementInstanceKey != "delivery-lead" || controller.recoverInput.ExpectedClaimGeneration != 4 ||
		controller.cancelInput.ManagementInstanceKey != "delivery-lead" || controller.cancelInput.ExpectedClaimGeneration != 4 ||
		controller.modeReadInput.ManagementInstanceKey != "delivery-lead" || controller.modeReadInput.ExpectedClaimGeneration != 4 ||
		controller.modeSetInput.ManagementInstanceKey != "delivery-lead" || controller.modeSetInput.ExpectedClaimGeneration != 4 {
		t.Fatalf("Host did not preserve observed task claim fences: ensure=%+v stop=%+v recover=%+v cancel=%+v modeRead=%+v modeSet=%+v",
			controller.ensureInput, controller.stopInput, controller.recoverInput, controller.cancelInput, controller.modeReadInput, controller.modeSetInput)
	}
	for _, installationID := range controller.callerInstallations {
		if installationID != "execution-installation" {
			t.Fatalf("controller caller installation = %q, want host-derived installation %q", installationID, "execution-installation")
		}
	}
}

func TestExactExecutionPendingReplaysReturnUncertainWithoutReceipt(t *testing.T) {
	controller := &fakeExactExecutionController{}
	host, digest := newExactExecutionHost(t, controller)
	manager, ok := pluginsdk.HostExecutionCommands(host)
	if !ok {
		t.Fatal("execution commands are not exposed")
	}
	ctx := context.Background()

	ensure := pluginsdk.ExactTaskRunCommand{
		RequestID: "request-ensure-pending", WorkspaceID: "workspace-execution", TaskID: "task-ensure-pending",
		IdempotencyKey: "ensure-pending", ExpectedTaskResourceVersion: "2026-09-26T09:00:00Z",
		ApprovalRevision: 1, ManifestDigest: digest,
	}
	ensureAdmission, result := host.admitExecutionCommand(ctx, exactEnsureTaskRunMethod, "host.v2.write:execution",
		ensure.RequestID, ensure.WorkspaceID, ensure.TaskID, ensure.IdempotencyKey,
		ensure.ExpectedTaskResourceVersion, ensure.ApprovalRevision, ensure.ManifestDigest, exactCommandDigest(exactEnsureTaskRunMethod, ensure))
	if result != nil || ensureAdmission == nil {
		t.Fatalf("admit pending ensure: result=%+v admission=%+v", result, ensureAdmission)
	}
	ensureAdmission.unlock()
	ensureReplay, ensureRun, err := manager.EnsureTaskRun(ctx, ensure)
	if err != nil || ensureReplay.Status != pluginsdk.CommandUnavailable || ensureReplay.Reason != "operation_outcome_uncertain" ||
		ensureReplay.Receipt != nil || ensureRun != (pluginsdk.ExactTaskRunResult{}) || controller.ensureCalls != 0 {
		t.Fatalf("pending ensure replay = %+v run=%+v calls=%d err=%v", ensureReplay, ensureRun, controller.ensureCalls, err)
	}

	recoverInput := pluginsdk.ExactSessionRecoveryCommand{
		ExactTaskExecutionCommand: pluginsdk.ExactTaskExecutionCommand{
			RequestID: "request-recover-pending", WorkspaceID: "workspace-execution", TaskID: "task-recover-pending",
			SessionID: "session-recover-pending", ExpectedSessionResourceVersion: "2026-09-26T09:01:00Z",
			ExpectedExecutionID: "exec-pending", IdempotencyKey: "recover-pending", ApprovalRevision: 1, ManifestDigest: digest,
		},
		Action: "resume",
	}
	base := recoverInput.ExactTaskExecutionCommand
	recoverAdmission, result := host.admitExecutionCommand(ctx, exactRecoverSessionMethod, "host.v2.write:execution",
		base.RequestID, base.WorkspaceID, base.SessionID, base.IdempotencyKey,
		base.ExpectedSessionResourceVersion, base.ApprovalRevision, base.ManifestDigest, exactCommandDigest(exactRecoverSessionMethod, recoverInput))
	if result != nil || recoverAdmission == nil {
		t.Fatalf("admit pending recovery: result=%+v admission=%+v", result, recoverAdmission)
	}
	recoverAdmission.unlock()
	recoverReplay, recoverRun, err := manager.RecoverSession(ctx, recoverInput)
	if err != nil || recoverReplay.Status != pluginsdk.CommandUnavailable || recoverReplay.Reason != "operation_outcome_uncertain" ||
		recoverReplay.Receipt != nil || recoverRun != (pluginsdk.ExactTaskRunResult{}) || controller.recoverCalls != 0 {
		t.Fatalf("pending recovery replay = %+v run=%+v calls=%d err=%v", recoverReplay, recoverRun, controller.recoverCalls, err)
	}

	modeInput := pluginsdk.ExactSessionModeCommand{
		ExactTaskExecutionCommand: pluginsdk.ExactTaskExecutionCommand{
			RequestID: "request-mode-pending", WorkspaceID: "workspace-execution", TaskID: "task-mode-pending",
			SessionID: "session-mode-pending", ExpectedSessionResourceVersion: "2026-09-26T09:02:00Z",
			ExpectedExecutionID: "exec-mode-pending", IdempotencyKey: "mode-pending", ApprovalRevision: 1, ManifestDigest: digest,
		},
		ModeID: safeManagedSessionModeID,
	}
	modeBase := modeInput.ExactTaskExecutionCommand
	modeAdmission, result := host.admitExecutionCommand(ctx, exactSetSessionModeMethod, "host.v2.write:execution",
		modeBase.RequestID, modeBase.WorkspaceID, modeBase.SessionID, modeBase.IdempotencyKey,
		modeBase.ExpectedSessionResourceVersion, modeBase.ApprovalRevision, modeBase.ManifestDigest, exactCommandDigest(exactSetSessionModeMethod, modeInput))
	if result != nil || modeAdmission == nil {
		t.Fatalf("admit pending session mode update: result=%+v admission=%+v", result, modeAdmission)
	}
	modeAdmission.unlock()
	modeReplay, currentMode, err := manager.SetSessionMode(ctx, modeInput)
	if err != nil || modeReplay.Status != pluginsdk.CommandUnavailable || modeReplay.Reason != "operation_outcome_uncertain" ||
		modeReplay.Receipt != nil || currentMode != "" || controller.modeCalls != 0 {
		t.Fatalf("pending session mode replay = %+v current=%q calls=%d err=%v", modeReplay, currentMode, controller.modeCalls, err)
	}
}

func TestExactExecutionResourceVersionValidation(t *testing.T) {
	valid := pluginsdk.ExactTaskRunCommand{RequestID: "r", WorkspaceID: "w", TaskID: "t", IdempotencyKey: "k", ExpectedTaskResourceVersion: time.Now().UTC().Format(time.RFC3339Nano), ApprovalRevision: 1, ManifestDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if !validTaskRunCommand(valid) {
		t.Fatal("valid exact task run command was rejected")
	}
	valid.ExpectedTaskResourceVersion = "not-a-version"
	if validTaskRunCommand(valid) {
		t.Fatal("malformed task version was accepted")
	}
}
