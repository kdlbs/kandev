package plugins

import (
	"context"
	"errors"
	"strings"
	"sync"
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

func TestPluginHost_GetCapabilityContextReportsApprovedExactMethods(t *testing.T) {
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
		Manifest: manifest.Manifest{
			ID:           "plugin-a",
			Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}},
		},
		InstallationID: "installation-a",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })

	digest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-a", 1, digest,
		[]string{"host.v2.write:tasks"}, "human", "grant", "approval-1"); err != nil {
		t.Fatalf("grant capability: %v", err)
	}

	boundHost := svc.hostForPlugin(record.ID).(*pluginHost)
	boundHost.taskWriter = &exactClaimCapabilityWriter{exactAdmissionWriter: &exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}}}
	host, ok := pluginsdk.HostV2(boundHost)
	if !ok {
		t.Fatal("plugin Host does not expose the additive ExactHost contract")
	}
	got, err := host.GetCapabilityContext(context.Background(), "workspace-a")
	if err != nil {
		t.Fatalf("GetCapabilityContext: %v", err)
	}
	if got.InstallationID != record.InstallationID || got.ApprovalRevision != 1 {
		t.Fatalf("capability context identity/revision = %q/%d, want %q/1", got.InstallationID, got.ApprovalRevision, record.InstallationID)
	}
	if len(got.Operations) != len(exactHostMethods)+len(exactReadMethods) || got.Operations[1].Method != "UpdateTaskExact" || !got.Operations[1].Authorized {
		t.Fatalf("capability operations = %+v, want registered operations and approved UpdateTaskExact", got.Operations)
	}
	for method, capability := range map[string]string{
		"ListManagedConversationSchedulesExact":      "host.v2.read:automations",
		"CreateManagedConversationScheduleExact":     "host.v2.write:automations",
		"UpdateManagedConversationScheduleExact":     "host.v2.write:automations",
		"SetManagedConversationScheduleEnabledExact": "host.v2.write:automations",
		"DeleteManagedConversationScheduleExact":     "host.v2.write:automations",
	} {
		var found *pluginsdk.CapabilityOperation
		for i := range got.Operations {
			if got.Operations[i].Method == method {
				found = &got.Operations[i]
				break
			}
		}
		if found == nil {
			t.Errorf("capability context omitted managed schedule method %q", method)
			continue
		}
		if found.CapabilityID != capability || found.Authorized || found.Supported || found.UnavailableReason != "managed_automation_schedule_service_unavailable" {
			t.Errorf("unwired schedule operation = %+v; want capability %q and unavailable service", *found, capability)
		}
	}
	for _, method := range []string{
		exactTaskManagementClaimAcquireMethod,
		exactTaskManagementClaimReleaseMethod,
		exactTaskManagementClaimTransferMethod,
	} {
		var found *pluginsdk.CapabilityOperation
		for i := range got.Operations {
			if got.Operations[i].Method == method {
				found = &got.Operations[i]
				break
			}
		}
		if found == nil || found.CapabilityID != "host.v2.write:tasks" || !found.Supported || !found.Authorized {
			t.Errorf("claim capability operation %q = %+v, want supported and approved task write", method, found)
		}
	}
	for _, operation := range got.Operations {
		if strings.Contains(operation.Method, "ManagedAgent") && (operation.Supported || operation.Authorized || operation.UnavailableReason != "managed_conversation_service_unavailable") {
			t.Fatalf("unwired managed conversation operation = %+v, want unavailable", operation)
		}
	}
}

func TestExactTaskManagementClaimAcquireUsesCallingInstallationAndInstance(t *testing.T) {
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
		Manifest: manifest.Manifest{
			ID:           "plugin-a",
			Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}},
		},
		InstallationID: "installation-a",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-a", 1, ManifestCapabilityDigest(record.Manifest),
		[]string{"host.v2.write:tasks"}, "human", "grant", "approval-claim-acquire"); err != nil {
		t.Fatalf("grant capability: %v", err)
	}

	writer := &exactClaimCapabilityWriter{exactAdmissionWriter: &exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}}}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.taskWriter = writer
	command, claim, err := host.AcquireTaskManagementClaimExact(context.Background(), pluginsdk.ExactTaskManagementClaimAcquire{
		ExactTaskManagementClaimCommand: pluginsdk.ExactTaskManagementClaimCommand{
			RequestID: "request-claim", WorkspaceID: "workspace-a", TaskID: "task-a",
			ExpectedTaskResourceVersion: "2026-09-26T00:00:00Z", IdempotencyKey: "claim-a",
			ApprovalRevision: 1, ManifestDigest: ManifestCapabilityDigest(record.Manifest),
		},
		InstanceKey: "coordinator-main",
	})
	if err != nil {
		t.Fatalf("AcquireTaskManagementClaimExact: %v", err)
	}
	if command == nil || command.Status != pluginsdk.CommandApplied {
		t.Fatalf("command result = %+v, want applied", command)
	}
	if writer.claimChange.InstallationID != record.InstallationID || writer.claimChange.InstanceKey != "coordinator-main" {
		t.Fatalf("claim owner identity = %q/%q, want %q/coordinator-main", writer.claimChange.InstallationID, writer.claimChange.InstanceKey, record.InstallationID)
	}
	if claim == nil || claim.InstallationID != record.InstallationID || claim.InstanceKey != "coordinator-main" {
		t.Fatalf("claim = %+v, want caller installation and instance", claim)
	}
}

type exactClaimCapabilityWriter struct {
	*exactAdmissionWriter
	claimChange taskmodels.TaskManagementClaimChange
}

func (f *exactClaimCapabilityWriter) ChangeTaskManagementClaim(_ context.Context, _ string, change taskmodels.TaskManagementClaimChange) (*taskmodels.TaskManagementClaim, error) {
	f.claimChange = change
	return &taskmodels.TaskManagementClaim{
		TaskID: change.TaskID, WorkspaceID: change.WorkspaceID, OwnerKind: "plugin",
		InstallationID: change.InstallationID, InstanceKey: change.InstanceKey, Generation: 1,
		ResourceVersion: "claim-version-1", UpdatedAt: time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC),
	}, nil
}

type exactAdmissionWriter struct {
	*fakeTaskWriter
	exactCalls int
	lastInput  ExactTaskUpdateInput
	recoverAck bool
	committed  *taskmodels.Task
}

func (f *exactAdmissionWriter) UpdateTaskExact(_ context.Context, input ExactTaskUpdateInput) (*taskmodels.Task, bool, error) {
	f.exactCalls++
	f.lastInput = input
	if f.recoverAck && f.exactCalls == 1 {
		f.committed = &taskmodels.Task{
			ID: input.TaskID, WorkspaceID: input.WorkspaceID, Title: dereference(input.Title),
			UpdatedAt: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		}
		return nil, false, errors.New("simulated lost response after the domain commit")
	}
	if f.recoverAck {
		return f.committed, true, nil
	}
	return &taskmodels.Task{
		ID: input.TaskID, WorkspaceID: input.WorkspaceID, Title: dereference(input.Title),
		UpdatedAt: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
	}, false, nil
}

type blockingExactAdmissionWriter struct {
	exactAdmissionWriter
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (f *blockingExactAdmissionWriter) UpdateTaskExact(ctx context.Context, input ExactTaskUpdateInput) (*taskmodels.Task, bool, error) {
	f.once.Do(func() {
		close(f.entered)
		<-f.release
	})
	return f.exactAdmissionWriter.UpdateTaskExact(ctx, input)
}

func TestExactHostRecoversAcceptedReceiptAfterDomainCommit(t *testing.T) {
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
		Manifest:       manifest.Manifest{ID: "plugin-recovery", Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}}},
		InstallationID: "installation-recovery",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	manifestDigest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-recovery", 1, manifestDigest,
		[]string{"host.v2.write:tasks"}, "human", "grant", "approval-recovery"); err != nil {
		t.Fatalf("grant capability: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	writer := &exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}, recoverAck: true}
	host.taskWriter = writer
	title := "committed before restart"
	input := pluginsdk.ExactTaskUpdate{
		RequestID: "request-recovery", WorkspaceID: "workspace-recovery", TaskID: "task-recovery",
		IdempotencyKey: "update-recovery", ExpectedResourceVersion: "2026-09-25T10:00:00Z",
		ApprovalRevision: 1, ManifestDigest: manifestDigest, Title: &title,
	}
	first, _, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil || first.Status != pluginsdk.CommandUnavailable {
		t.Fatalf("first UpdateTaskExact = %+v, err=%v, want unavailable after receipt gap", first, err)
	}
	second, task, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil {
		t.Fatalf("retry UpdateTaskExact: %v", err)
	}
	if second.Status != pluginsdk.CommandAlreadyApplied || second.Receipt == nil || task == nil || writer.exactCalls != 2 {
		t.Fatalf("recovered result = %+v, task=%+v, writer calls=%d", second, task, writer.exactCalls)
	}
}

func TestExactHostAdmissionSerializesApprovalRevocation(t *testing.T) {
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
		Manifest:       manifest.Manifest{ID: "plugin-race", Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}}},
		InstallationID: "installation-race",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	manifestDigest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-race", 1, manifestDigest,
		[]string{"host.v2.write:tasks"}, "human", "grant", "approval-race"); err != nil {
		t.Fatalf("grant capability: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	writer := &blockingExactAdmissionWriter{
		exactAdmissionWriter: exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}},
		entered:              make(chan struct{}),
		release:              make(chan struct{}),
	}
	host.taskWriter = writer
	title := "serialized mutation"
	input := pluginsdk.ExactTaskUpdate{
		RequestID: "request-race", WorkspaceID: "workspace-race", TaskID: "task-race",
		IdempotencyKey: "update-race", ExpectedResourceVersion: "2026-09-25T10:00:00Z",
		ApprovalRevision: 1, ManifestDigest: manifestDigest, Title: &title,
	}
	updateDone := make(chan *pluginsdk.CommandResult, 1)
	updateErr := make(chan error, 1)
	go func() {
		result, _, err := host.UpdateTaskExact(context.Background(), input)
		updateDone <- result
		updateErr <- err
	}()
	<-writer.entered
	if svc.approvalEffectMu.TryLock() {
		svc.approvalEffectMu.Unlock()
		t.Fatal("exact task mutation did not hold the approval-effect guard")
	}
	revokeStarted := make(chan struct{})
	revokeDone := make(chan error, 1)
	go func() {
		close(revokeStarted)
		_, err := svc.RevokeCapabilityApproval(record.InstallationID, "workspace-race", 1, "human", "revoke", "approval-race-revoke")
		revokeDone <- err
	}()
	<-revokeStarted
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation completed while an admitted mutation was in flight: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(writer.release)
	if err := <-updateErr; err != nil {
		t.Fatalf("UpdateTaskExact: %v", err)
	}
	if result := <-updateDone; result.Status != pluginsdk.CommandApplied {
		t.Fatalf("update result = %+v, want APPLIED before revocation", result)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("RevokeCapabilityApproval: %v", err)
	}

	input.IdempotencyKey = "update-after-revoke"
	denied, _, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil || denied.Status != pluginsdk.CommandDenied || writer.exactCalls != 1 {
		t.Fatalf("post-revocation result = %+v, err=%v, exact calls=%d", denied, err, writer.exactCalls)
	}
}

func TestExactHostAdmission(t *testing.T) {
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
		Manifest: manifest.Manifest{
			ID:           "plugin-a",
			Capabilities: manifest.Capabilities{APIWrite: []string{"tasks"}},
		},
		InstallationID: "installation-a",
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	t.Cleanup(func() { _ = svc.Close() })
	manifestDigest := ManifestCapabilityDigest(record.Manifest)
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-a", 1, manifestDigest,
		[]string{"host.v2.write:tasks"}, "human", "grant", "approval-1"); err != nil {
		t.Fatalf("grant capability: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	writer := &exactAdmissionWriter{fakeTaskWriter: &fakeTaskWriter{}}
	host.taskWriter = writer
	title := "Exact update"
	input := pluginsdk.ExactTaskUpdate{
		RequestID: "request-1", WorkspaceID: "workspace-a", TaskID: "task-1",
		IdempotencyKey: "update-1", ExpectedResourceVersion: "2026-09-25T11:00:00Z",
		ApprovalRevision: 1, ManifestDigest: manifestDigest, Title: &title,
	}
	result, task, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil {
		t.Fatalf("UpdateTaskExact: %v", err)
	}
	if result.Status != pluginsdk.CommandApplied || result.Receipt == nil || task == nil || task.Title != title {
		t.Fatalf("first result = %+v, task = %+v, want applied update", result, task)
	}
	if writer.exactCalls != 1 || writer.lastInput.OperationID == "" {
		t.Fatalf("exact writer calls = %d, input = %+v", writer.exactCalls, writer.lastInput)
	}

	// Replaying a completed Host receipt returns the original outcome without
	// asking the domain service to apply the same mutation again.
	replayed, _, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil {
		t.Fatalf("replayed UpdateTaskExact: %v", err)
	}
	if replayed.Status != pluginsdk.CommandApplied || replayed.Receipt.ID != result.Receipt.ID || writer.exactCalls != 1 {
		t.Fatalf("replay = %+v, writer calls = %d, want original receipt and one domain call", replayed, writer.exactCalls)
	}

	changedTitle := "different payload"
	input.Title = &changedTitle
	conflict, _, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil {
		t.Fatalf("payload-conflict UpdateTaskExact: %v", err)
	}
	if conflict.Status != pluginsdk.CommandConflict || writer.exactCalls != 1 {
		t.Fatalf("payload conflict = %+v, writer calls = %d", conflict, writer.exactCalls)
	}

	if _, err := svc.RevokeCapabilityApproval(record.InstallationID, "workspace-a", 1, "human", "revoke", "approval-2"); err != nil {
		t.Fatalf("revoke capability: %v", err)
	}
	input.IdempotencyKey = "update-after-revoke"
	denied, _, err := host.UpdateTaskExact(context.Background(), input)
	if err != nil {
		t.Fatalf("revoked UpdateTaskExact: %v", err)
	}
	if denied.Status != pluginsdk.CommandDenied || writer.exactCalls != 1 {
		t.Fatalf("revoked update = %+v, writer calls = %d", denied, writer.exactCalls)
	}
}

func dereference(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
