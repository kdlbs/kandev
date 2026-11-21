package plugins

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kdlbs/kandev/pluginsdk"
	_ "github.com/mattn/go-sqlite3"
)

type exactChangeRequestTaskData struct {
	taskDataSource
	task *taskmodels.Task
}

func (d *exactChangeRequestTaskData) GetTask(_ context.Context, taskID string) (*taskmodels.Task, error) {
	if d.task == nil || d.task.ID != taskID {
		return nil, repoerrors.ErrTaskNotFound
	}
	task := *d.task
	return &task, nil
}

func newExactChangeRequestHost(t *testing.T, providerIDs []string) *pluginHost {
	t.Helper()
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	pool := db.NewPool(connection, connection)
	commandStore, err := state.NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	changeRequests, err := state.NewChangeRequestStore(pool)
	if err != nil {
		t.Fatalf("NewChangeRequestStore: %v", err)
	}
	record := &pluginstore.Record{
		Manifest: manifest.Manifest{ID: "change-request-plugin", RepositoryProviders: providerIDs,
			Capabilities: manifest.Capabilities{APIWrite: []string{"task_change_requests"}}},
		InstallationID: "change-request-installation",
		Status:         StatusActive,
	}
	registry := NewRegistry()
	registry.Add(record)
	svc := NewService(pluginstore.NewFSStore(t.TempDir()), registry, nil, testLogger(t))
	if err := svc.SetPluginsDir(t.TempDir()); err != nil {
		t.Fatalf("SetPluginsDir: %v", err)
	}
	svc.SetExactCommandStore(commandStore)
	svc.SetChangeRequestStore(changeRequests)
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := svc.approvalGrant(record.InstallationID, "workspace-change-requests", 1,
		ManifestCapabilityDigest(record.Manifest), []string{"host.v2.write:task_change_requests"}, "human", "grant", "approval-change-requests"); err != nil {
		t.Fatalf("grant change requests: %v", err)
	}
	host := svc.hostForPlugin(record.ID).(*pluginHost)
	host.commandStore = commandStore
	host.taskData = &exactChangeRequestTaskData{task: &taskmodels.Task{
		ID: "task-1", WorkspaceID: "workspace-change-requests", UpdatedAt: time.Now().UTC(),
	}}
	return host
}

func changeRequestManifestDigest(t *testing.T, host *pluginHost) string {
	t.Helper()
	return ManifestCapabilityDigest(host.service.installedRecordByInstallationID(host.installationID).Manifest)
}

func TestExactChangeRequestReportAndRemove(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	mergedAt := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	report := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1",
		Number: 7, URL: "https://forge.example.test/owner/repo/pulls/7", Title: "Fix it",
		State: "merged", HeadBranch: "fix", BaseBranch: "main",
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano), MergedAt: mergedAt,
		IdempotencyKey: "report-1", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	result, reported, err := manager.Report(context.Background(), report)
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("Report = result:%+v change:%+v err:%v", result, reported, err)
	}
	if reported.MergedAt == nil {
		t.Fatalf("reported = %+v, want forgejo number 7 with merged_at", reported)
	}
	parsedMergedAt, parseErr := time.Parse(time.RFC3339Nano, *reported.MergedAt)
	expectedMergedAt, _ := time.Parse(time.RFC3339Nano, mergedAt)
	if reported.ProviderID != "forgejo" || reported.Number != 7 || parseErr != nil || !parsedMergedAt.Equal(expectedMergedAt) {
		t.Fatalf("reported = %+v, want forgejo number 7 with merged_at", reported)
	}
	remove := pluginsdk.ExactTaskChangeRequestRemove{
		RequestID: "request-remove", WorkspaceID: "workspace-change-requests",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 7,
		IdempotencyKey: "remove-1", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	removeResult, err := manager.Remove(context.Background(), remove)
	if err != nil || removeResult == nil || removeResult.Status != pluginsdk.CommandApplied {
		t.Fatalf("Remove = result:%+v err:%v", removeResult, err)
	}
	replayed, replayedChange, err := manager.Report(context.Background(), report)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied || replayedChange.ID != "" {
		t.Fatalf("Report replay after remove = result:%+v change:%+v err:%v, want recorded APPLIED", replayed, replayedChange, err)
	}
}

func TestExactChangeRequestRejectsUnownedProvider(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	report := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "gitea", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "report-1", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	result, _, err := manager.Report(context.Background(), report)
	if err != nil {
		t.Fatalf("Report unowned provider err: %v", err)
	}
	if result == nil || result.Status != pluginsdk.CommandDenied {
		t.Fatalf("Report unowned provider = %+v, want DENIED", result)
	}
	replayed, _, err := manager.Report(context.Background(), report)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandDenied ||
		replayed.Reason != result.Reason {
		t.Fatalf("replayed denied Report = %+v err=%v, want the recorded denial", replayed, err)
	}
}

func TestExactChangeRequestRejectsMergedWithoutTimestamp(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	report := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "merged",
		IdempotencyKey: "report-1", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	result, _, err := manager.Report(context.Background(), report)
	if err != nil {
		t.Fatalf("Report merged without timestamp err: %v", err)
	}
	if result == nil || result.Status != pluginsdk.CommandInvalid {
		t.Fatalf("Report merged without timestamp = %+v, want INVALID", result)
	}
}

func TestExactChangeRequestRejectsMissingTask(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	host.taskData = &exactChangeRequestTaskData{}
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	report := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-report", WorkspaceID: "workspace-change-requests", TaskID: "task-missing",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "report-1", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	result, _, err := manager.Report(context.Background(), report)
	if err != nil {
		t.Fatalf("Report missing task err: %v", err)
	}
	if result == nil || result.Status != pluginsdk.CommandNotFound {
		t.Fatalf("Report missing task = %+v, want NOT_FOUND", result)
	}
}

func TestExactChangeRequestReportIsIdempotentAndRejectsPayloadMismatch(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	report := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-idempotent", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 9, URL: "https://forge.example.test/pulls/9",
		Title: "Fix it", State: "open", IdempotencyKey: "report-idempotent",
		ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	}
	first, firstChange, err := manager.Report(context.Background(), report)
	if err != nil || first == nil || first.Status != pluginsdk.CommandApplied || firstChange.ID == "" {
		t.Fatalf("first Report = result:%+v change:%+v err:%v", first, firstChange, err)
	}
	replayed, replayedChange, err := manager.Report(context.Background(), report)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied || replayedChange.ID != firstChange.ID {
		t.Fatalf("replayed Report = result:%+v change:%+v err:%v", replayed, replayedChange, err)
	}
	report.Title = "Changed payload"
	conflict, _, err := manager.Report(context.Background(), report)
	if err != nil || conflict == nil || conflict.Status != pluginsdk.CommandConflict {
		t.Fatalf("payload mismatch Report = result:%+v err:%v, want CONFLICT", conflict, err)
	}
}

func TestExactChangeRequestRejectsArchivedTask(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	archived := time.Now().UTC()
	host.taskData = &exactChangeRequestTaskData{task: &taskmodels.Task{
		ID: "task-1", WorkspaceID: "workspace-change-requests", ArchivedAt: &archived, UpdatedAt: archived,
	}}
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	result, _, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "request-archived", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "report-archived", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	})
	if err != nil || result == nil || result.Status != pluginsdk.CommandConflict {
		t.Fatalf("archived task Report = result:%+v err:%v, want CONFLICT", result, err)
	}
}

func TestExactChangeRequestWireRoundTrip(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	wire := dialPluginHostOverWire(t, host)
	manager, ok := pluginsdk.HostTaskChangeRequests(wire)
	if !ok {
		t.Fatal("wire Host does not expose exact task change requests")
	}
	result, change, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "wire-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1", Number: 11,
		URL: "https://forge.example.test/pulls/11", Title: "Wire", State: "open",
		IdempotencyKey: "wire-report", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	})
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied ||
		change.ProviderID != "forgejo" || change.Number != 11 || change.ResourceVersion == "" {
		t.Fatalf("wire Report = result:%+v change:%+v err:%v", result, change, err)
	}
	removeResult, err := manager.Remove(context.Background(), pluginsdk.ExactTaskChangeRequestRemove{
		RequestID: "wire-remove", WorkspaceID: "workspace-change-requests",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 11,
		IdempotencyKey: "wire-remove", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	})
	if err != nil || removeResult == nil || removeResult.Status != pluginsdk.CommandApplied {
		t.Fatalf("wire Remove = result:%+v err:%v", removeResult, err)
	}
}

func TestExactChangeRequestRejectsStaleApprovalRevision(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	digest := changeRequestManifestDigest(t, host)
	if _, err := host.service.approvalGrant(host.installationID, "workspace-change-requests", 2, digest,
		[]string{"host.v2.write:task_change_requests"}, "human", "upgrade", "approval-upgrade"); err != nil {
		t.Fatalf("grant revision 2: %v", err)
	}
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	stale, _, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "stale-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "stale-report", ApprovalRevision: 1, ManifestDigest: digest,
	})
	if err != nil || stale == nil || stale.Status != pluginsdk.CommandDenied {
		t.Fatalf("stale Report = result:%+v err:%v, want DENIED", stale, err)
	}
	current, _, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "current-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "current-report", ApprovalRevision: 2, ManifestDigest: digest,
	})
	if err != nil || current == nil || current.Status != pluginsdk.CommandApplied {
		t.Fatalf("current Report = result:%+v err:%v, want APPLIED", current, err)
	}
}

func TestExactChangeRequestRejectsDisabledInstallation(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	records := host.service.registry.List()
	if len(records) != 1 {
		t.Fatalf("registry records = %d, want 1", len(records))
	}
	if _, ok := host.service.registry.SetStatus(records[0].ID, StatusDisabled); !ok {
		t.Fatal("could not disable the test installation")
	}
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	result, _, err := manager.Report(context.Background(), pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "disabled-report", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "open",
		IdempotencyKey: "disabled-report", ApprovalRevision: 1, ManifestDigest: changeRequestManifestDigest(t, host),
	})
	if err != nil || result == nil || result.Status != pluginsdk.CommandDenied || result.Reason != "plugin_not_active" {
		t.Fatalf("disabled Report = result:%+v err:%v, want DENIED plugin_not_active", result, err)
	}
}

func TestExactChangeRequestNewKeyUpdatesExistingRow(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	digest := changeRequestManifestDigest(t, host)
	base := pluginsdk.ExactTaskChangeRequestReport{
		WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 21,
		URL: "https://forge.example.test/pulls/21", Title: "Initial", State: "open",
		ApprovalRevision: 1, ManifestDigest: digest,
	}
	base.RequestID, base.IdempotencyKey = "initial-report", "initial-report"
	if result, _, err := manager.Report(context.Background(), base); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("initial Report = result:%+v err:%v", result, err)
	}
	mergedAt := time.Now().UTC().Format(time.RFC3339Nano)
	next := base
	next.RequestID, next.IdempotencyKey = "merged-report", "merged-report"
	next.State, next.Title, next.MergedAt = "merged", "Merged", mergedAt
	result, change, err := manager.Report(context.Background(), next)
	if err != nil || result == nil || result.Status != pluginsdk.CommandApplied || change.State != "merged" || change.Title != "Merged" {
		t.Fatalf("second Report under a new key = result:%+v change:%+v err:%v", result, change, err)
	}
}

func TestExactChangeRequestReplayDoesNotUndoNewerOperations(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	digest := changeRequestManifestDigest(t, host)
	report := pluginsdk.ExactTaskChangeRequestReport{
		WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 31,
		URL: "https://forge.example.test/pulls/31", Title: "Initial", State: "open",
		ApprovalRevision: 1, ManifestDigest: digest,
	}
	report.RequestID, report.IdempotencyKey = "initial", "initial"
	if result, _, err := manager.Report(context.Background(), report); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("initial Report = %+v err=%v", result, err)
	}
	merged := report
	merged.RequestID, merged.IdempotencyKey = "merged", "merged"
	merged.State, merged.Title, merged.MergedAt = "merged", "Merged", time.Now().UTC().Format(time.RFC3339Nano)
	if result, _, err := manager.Report(context.Background(), merged); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("merged Report = %+v err=%v", result, err)
	}
	replayed, change, err := manager.Report(context.Background(), report)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandApplied || change.State != "merged" {
		t.Fatalf("initial replay after merge = result:%+v change:%+v err=%v, want current merged row", replayed, change, err)
	}

	remove := pluginsdk.ExactTaskChangeRequestRemove{
		RequestID: "remove", WorkspaceID: "workspace-change-requests",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 31,
		IdempotencyKey: "remove", ApprovalRevision: 1, ManifestDigest: digest,
	}
	if result, err := manager.Remove(context.Background(), remove); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("Remove = %+v err=%v", result, err)
	}
	newer := merged
	newer.RequestID, newer.IdempotencyKey = "newer", "newer"
	newer.Title, newer.State, newer.MergedAt = "Newer", "merged", time.Now().UTC().Format(time.RFC3339Nano)
	if result, _, err := manager.Report(context.Background(), newer); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("newer Report = %+v err=%v", result, err)
	}
	if result, err := manager.Remove(context.Background(), remove); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("Remove replay after newer report = %+v err=%v", result, err)
	}
	rows, err := host.changeRequestLedger().ListChangeRequestsByTask(context.Background(), host.installationID, "workspace-change-requests", "task-1")
	if err != nil {
		t.Fatalf("list rows after remove replay: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != 31 || rows[0].Title != "Newer" {
		t.Fatalf("rows after remove replay = %+v, want the newer row", rows)
	}
}

func TestExactChangeRequestUncertainRetryDoesNotRewrite(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	digestManifest := changeRequestManifestDigest(t, host)
	input := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "uncertain", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 41,
		URL: "https://forge.example.test/pulls/41", Title: "Old", State: "open",
		IdempotencyKey: "uncertain", ApprovalRevision: 1, ManifestDigest: digestManifest,
	}
	payloadDigest, err := exactTaskChangeRequestReportDigest(input)
	if err != nil {
		t.Fatalf("payload digest: %v", err)
	}
	if _, _, err := host.commandStore.Admit(context.Background(), state.CommandIntent{
		InstallationID: host.installationID, WorkspaceID: input.WorkspaceID,
		RequestID: input.RequestID, IdempotencyKey: input.IdempotencyKey,
		Method: exactTaskChangeRequestReportMethod, CapabilityID: exactTaskChangeRequestCapability,
		PayloadDigest: payloadDigest, TargetID: input.TaskID,
		ExpectedResourceVersion: changeRequestAdmissionVersion(input.TaskID),
		ApprovalRevision:        1, ManifestDigest: digestManifest,
	}); err != nil {
		t.Fatalf("admit accepted receipt: %v", err)
	}
	first, _, err := manager.Report(context.Background(), input)
	if err != nil || first == nil || first.Status != pluginsdk.CommandUncertain {
		t.Fatalf("uncertain Report = result:%+v err:%v", first, err)
	}
	newer := input
	newer.RequestID, newer.IdempotencyKey = "newer", "newer"
	newer.Title, newer.State, newer.MergedAt = "Newer", "merged", time.Now().UTC().Format(time.RFC3339Nano)
	if result, _, err := manager.Report(context.Background(), newer); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("newer Report = result:%+v err:%v", result, err)
	}
	replayed, change, err := manager.Report(context.Background(), input)
	if err != nil || replayed == nil || replayed.Status != pluginsdk.CommandUncertain || change.ID != "" {
		t.Fatalf("uncertain replay = result:%+v change:%+v err:%v", replayed, change, err)
	}
	rows, err := host.changeRequestLedger().ListChangeRequestsByTask(context.Background(), host.installationID, input.WorkspaceID, input.TaskID)
	if err != nil {
		t.Fatalf("list rows: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Newer" || rows[0].State != "merged" {
		t.Fatalf("rows after uncertain replay = %+v, want the newer row", rows)
	}
}

func TestExactChangeRequestFailedCompleteThenNewerReportIsNotRewritten(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	pool := db.NewPool(connection, connection)
	commandStore, err := state.NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	ledger, err := state.NewChangeRequestStore(pool)
	if err != nil {
		t.Fatalf("NewChangeRequestStore: %v", err)
	}
	host.commandStore = commandStore
	host.changeRequestsDep = func() *state.ChangeRequestStore { return ledger }

	if _, err := connection.Exec(`
		CREATE TRIGGER fail_change_request_receipt
		BEFORE UPDATE ON plugin_host_command_receipts
		WHEN NEW.operation_id IN (
			SELECT operation_id FROM plugin_host_command_intents WHERE idempotency_key = 'uncertain'
		)
		BEGIN
			SELECT RAISE(ABORT, 'injected receipt completion failure');
		END;
	`); err != nil {
		t.Fatalf("create receipt failure trigger: %v", err)
	}

	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	digestManifest := changeRequestManifestDigest(t, host)
	old := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "uncertain", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 51,
		URL: "https://forge.example.test/pulls/51", Title: "Old", State: "open",
		IdempotencyKey: "uncertain", ApprovalRevision: 1, ManifestDigest: digestManifest,
	}
	failed, _, err := manager.Report(context.Background(), old)
	if err != nil || failed == nil || failed.Status != pluginsdk.CommandUncertain {
		t.Fatalf("failed Report = result:%+v err:%v, want UNCERTAIN", failed, err)
	}
	if _, err := connection.Exec(`DROP TRIGGER fail_change_request_receipt`); err != nil {
		t.Fatalf("drop receipt failure trigger: %v", err)
	}

	newer := old
	newer.RequestID, newer.IdempotencyKey = "newer", "newer"
	newer.Title, newer.State, newer.MergedAt = "Newer", "merged", time.Now().UTC().Format(time.RFC3339Nano)
	if result, _, err := manager.Report(context.Background(), newer); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("newer Report = result:%+v err:%v", result, err)
	}
	replayed, change, err := manager.Report(context.Background(), old)
	if err != nil || replayed == nil {
		t.Fatalf("old replay = result:%+v change:%+v err:%v", replayed, change, err)
	}
	rows, err := ledger.ListChangeRequestsByTask(context.Background(), host.installationID, old.WorkspaceID, old.TaskID)
	if err != nil {
		t.Fatalf("list after old replay: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Newer" || rows[0].State != "merged" {
		t.Fatalf("rows after old replay = %+v, want the newer merged row", rows)
	}
	if replayed.Status != pluginsdk.CommandUncertain || change.ID != "" {
		t.Fatalf("old replay = result:%+v change:%+v, want UNCERTAIN with no row", replayed, change)
	}
}

func TestWorkspaceDeletedRemovesOnlyThatWorkspaceChangeRequests(t *testing.T) {
	eventBus := bus.NewMemoryEventBus(testLogger(t))
	ledger := newChangeRequestEvidenceStore(t)
	service := NewService(nil, NewRegistry(), eventBus, testLogger(t))
	service.SetChangeRequestStore(ledger)
	service.subscribeWorkspaceDeletedEvents()
	t.Cleanup(service.closeWorkspaceDeletedEvents)

	for _, record := range []state.ChangeRequestRecord{
		{InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1", ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: state.ChangeRequestStateOpen},
		{InstallationID: "inst-1", WorkspaceID: "ws-2", TaskID: "task-2", ProviderID: "forgejo", RepositoryID: "repo-1", Number: 2, State: state.ChangeRequestStateOpen},
	} {
		if _, _, err := ledger.ReportChangeRequest(context.Background(), record); err != nil {
			t.Fatalf("report %+v: %v", record, err)
		}
	}
	event := bus.NewEvent(events.WorkspaceDeleted, "task-service", map[string]interface{}{"id": "ws-1"})
	if err := eventBus.Publish(context.Background(), events.WorkspaceDeleted, event); err != nil {
		t.Fatalf("publish workspace.deleted: %v", err)
	}
	deleted, err := ledger.ListChangeRequestsByTask(context.Background(), "inst-1", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list deleted workspace: %v", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("ws-1 rows = %d, want 0", len(deleted))
	}
	remaining, err := ledger.ListChangeRequestsByTask(context.Background(), "inst-1", "ws-2", "task-2")
	if err != nil {
		t.Fatalf("list remaining workspace: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("ws-2 rows = %d, want 1", len(remaining))
	}
}

func TestExactChangeRequestFailedRemoveCompleteThenNewerReportIsNotDeleted(t *testing.T) {
	host := newExactChangeRequestHost(t, []string{"forgejo"})
	connection, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open command database: %v", err)
	}
	connection.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = connection.Close() })
	pool := db.NewPool(connection, connection)
	commandStore, err := state.NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	ledger, err := state.NewChangeRequestStore(pool)
	if err != nil {
		t.Fatalf("NewChangeRequestStore: %v", err)
	}
	host.commandStore = commandStore
	host.changeRequestsDep = func() *state.ChangeRequestStore { return ledger }
	manager, ok := pluginsdk.HostTaskChangeRequests(host)
	if !ok {
		t.Fatal("plugin Host does not expose exact task change requests")
	}
	digestManifest := changeRequestManifestDigest(t, host)
	seed := pluginsdk.ExactTaskChangeRequestReport{
		RequestID: "seed", WorkspaceID: "workspace-change-requests", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 61,
		URL: "https://forge.example.test/pulls/61", Title: "Seed", State: "open",
		IdempotencyKey: "seed", ApprovalRevision: 1, ManifestDigest: digestManifest,
	}
	if result, _, err := manager.Report(context.Background(), seed); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("seed Report = result:%+v err:%v", result, err)
	}
	if _, err := connection.Exec(`
		CREATE TRIGGER fail_remove_receipt
		BEFORE UPDATE ON plugin_host_command_receipts
		WHEN NEW.operation_id IN (
			SELECT operation_id FROM plugin_host_command_intents WHERE idempotency_key = 'uncertain-remove'
		)
		BEGIN
			SELECT RAISE(ABORT, 'injected remove receipt completion failure');
		END;
	`); err != nil {
		t.Fatalf("create remove failure trigger: %v", err)
	}
	remove := pluginsdk.ExactTaskChangeRequestRemove{
		RequestID: "uncertain-remove", WorkspaceID: "workspace-change-requests",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 61,
		IdempotencyKey: "uncertain-remove", ApprovalRevision: 1, ManifestDigest: digestManifest,
	}
	failed, err := manager.Remove(context.Background(), remove)
	if err != nil || failed == nil || failed.Status != pluginsdk.CommandUncertain {
		t.Fatalf("failed Remove = result:%+v err:%v, want UNCERTAIN", failed, err)
	}
	if _, err := connection.Exec(`DROP TRIGGER fail_remove_receipt`); err != nil {
		t.Fatalf("drop remove failure trigger: %v", err)
	}
	newer := seed
	newer.RequestID, newer.IdempotencyKey = "newer", "newer"
	newer.Title, newer.State, newer.MergedAt = "Newer", "merged", time.Now().UTC().Format(time.RFC3339Nano)
	if result, _, err := manager.Report(context.Background(), newer); err != nil || result == nil || result.Status != pluginsdk.CommandApplied {
		t.Fatalf("newer Report = result:%+v err:%v", result, err)
	}
	replayed, err := manager.Remove(context.Background(), remove)
	if err != nil || replayed == nil {
		t.Fatalf("old Remove replay = result:%+v err:%v", replayed, err)
	}
	rows, err := ledger.ListChangeRequestsByTask(context.Background(), host.installationID, seed.WorkspaceID, seed.TaskID)
	if err != nil {
		t.Fatalf("list after old Remove replay: %v", err)
	}
	if len(rows) != 1 || rows[0].Title != "Newer" || rows[0].State != "merged" {
		t.Fatalf("rows after old Remove replay = %+v, want the newer merged row", rows)
	}
	if replayed.Status != pluginsdk.CommandUncertain {
		t.Fatalf("old Remove replay = %+v, want UNCERTAIN", replayed)
	}
}
