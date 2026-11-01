package state

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newChangeRequestTestStore(t *testing.T) *ChangeRequestStore {
	t.Helper()
	raw, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	store := &ChangeRequestStore{db: raw, ro: raw}
	if err := store.initSchema(); err != nil {
		t.Fatalf("init change request schema: %v", err)
	}
	return store
}

func TestChangeRequestStoreReportAndLatch(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	mergedAt := time.Now().UTC().Add(-time.Hour).Format(changeRequestTimestampLayout)
	first, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", ProviderHost: "forge.example.test", RepositoryID: "repo-1",
		Number: 7, URL: "https://forge.example.test/owner/repo/pulls/7", Title: "Fix it",
		State: ChangeRequestStateMerged, HeadBranch: "fix", BaseBranch: "main",
		MergedAt: mergedAt,
	})
	if err != nil {
		t.Fatalf("report merged: %v", err)
	}
	if existed {
		t.Fatal("first report must be new")
	}
	if first.MergedAt != mergedAt {
		t.Fatalf("merged_at = %q, want %q", first.MergedAt, mergedAt)
	}
	// A later report may update descriptive fields while keeping the row merged.
	second, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 7,
		Title: "Fix it (updated)", State: ChangeRequestStateMerged, MergedAt: mergedAt,
	})
	if err != nil {
		t.Fatalf("re-report: %v", err)
	}
	if !existed {
		t.Fatal("second report must be an update")
	}
	if second.MergedAt != mergedAt {
		t.Fatalf("merged_at after stale re-report = %q, want latched %q", second.MergedAt, mergedAt)
	}
	if second.Title != "Fix it (updated)" {
		t.Fatalf("title = %q, want latest report", second.Title)
	}
	if second.ID != first.ID {
		t.Fatal("stable id must survive re-reports")
	}
	// A stale report that says the request is open must not unmerge it.
	third, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 7,
		State: ChangeRequestStateOpen,
	})
	if err != nil {
		t.Fatalf("stale unmerge report: %v", err)
	}
	if !existed || third.State != ChangeRequestStateMerged || third.MergedAt != mergedAt {
		t.Fatalf("stale unmerge report = %+v, want merged with latched merged_at", third)
	}
}

func TestChangeRequestStoreRejectsBadState(t *testing.T) {
	store := newChangeRequestTestStore(t)
	_, _, err := store.ReportChangeRequest(context.Background(), ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: "squashed",
	})
	if err == nil {
		t.Fatal("unknown state must be rejected")
	}
}

func TestChangeRequestStoreRemove(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	mergedAt := time.Now().UTC().Add(-30 * time.Minute).Format(changeRequestTimestampLayout)
	if _, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 3,
		State: ChangeRequestStateMerged, MergedAt: mergedAt,
	}); err != nil {
		t.Fatalf("report merged: %v", err)
	}
	if err := store.RemoveChangeRequest(ctx, "inst-1", "ws-1", "forgejo", "repo-1", 3); err != nil {
		t.Fatalf("remove: %v", err)
	}
	remaining, err := store.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("rows after remove = %d, want 0", len(remaining))
	}
	// Removing a missing row is not an error.
	if err := store.RemoveChangeRequest(ctx, "inst-1", "ws-1", "forgejo", "repo-1", 3); err != nil {
		t.Fatalf("remove missing: %v", err)
	}
}

func TestChangeRequestStoreRejectsInvalidTimestamp(t *testing.T) {
	store := newChangeRequestTestStore(t)
	_, _, err := store.ReportChangeRequest(context.Background(), ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1,
		State: ChangeRequestStateOpen, CreatedAt: "not-a-timestamp",
	})
	if err == nil {
		t.Fatal("invalid timestamp must be rejected")
	}
	_, _, err = store.ReportChangeRequest(context.Background(), ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 2,
		State: ChangeRequestStateMerged,
	})
	if err == nil {
		t.Fatal("new merged row without merged_at must be rejected")
	}
	valid := time.Now().UTC().Format(changeRequestTimestampLayout)
	for name, record := range map[string]ChangeRequestRecord{
		"open with merged":   {State: ChangeRequestStateOpen, MergedAt: valid},
		"merged with closed": {State: ChangeRequestStateMerged, MergedAt: valid, ClosedAt: valid},
		"closed with merged": {State: ChangeRequestStateClosed, MergedAt: valid, ClosedAt: valid},
	} {
		record.InstallationID, record.WorkspaceID, record.TaskID = "inst-1", "ws-1", "task-1"
		record.ProviderID, record.RepositoryID, record.Number = "forgejo", "repo-1", 3
		if _, _, err := store.ReportChangeRequest(context.Background(), record); err == nil {
			t.Errorf("%s must be rejected", name)
		}
	}
}

func TestChangeRequestStoreDeleteByInstallation(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	for _, inst := range []string{"inst-1", "inst-2"} {
		if _, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
			InstallationID: inst, WorkspaceID: "ws-1", TaskID: "task-1",
			ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: ChangeRequestStateOpen,
		}); err != nil {
			t.Fatalf("report %s: %v", inst, err)
		}
	}
	if err := store.DeleteByInstallation(ctx, "inst-1"); err != nil {
		t.Fatalf("delete by installation: %v", err)
	}
	remaining, err := store.ListChangeRequestsByTask(ctx, "inst-2", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list remaining: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining rows = %d, want 1", len(remaining))
	}
	removed, err := store.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list removed: %v", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed rows = %d, want 0", len(removed))
	}
}

func TestChangeRequestStoreClosedStateIsTerminal(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	closedAt := time.Now().UTC().Add(-time.Minute).Format(changeRequestTimestampLayout)
	if _, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 8,
		State: ChangeRequestStateClosed, ClosedAt: closedAt,
	}); err != nil {
		t.Fatalf("report closed: %v", err)
	}
	reopened, existed, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 8,
		State: ChangeRequestStateOpen,
	})
	if err != nil {
		t.Fatalf("stale reopen report: %v", err)
	}
	if !existed || reopened.State != ChangeRequestStateClosed || reopened.ClosedAt != closedAt {
		t.Fatalf("stale reopen report = %+v, want closed with latched closed_at", reopened)
	}
}

func TestChangeRequestStoreRejectsTaskReassignment(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	record := ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 13, State: ChangeRequestStateOpen,
	}
	if _, _, err := store.ReportChangeRequest(ctx, record); err != nil {
		t.Fatalf("first report: %v", err)
	}
	record.TaskID = "task-2"
	if _, _, err := store.ReportChangeRequest(ctx, record); !errors.Is(err, ErrChangeRequestTaskConflict) {
		t.Fatalf("task reassignment error = %v, want ErrChangeRequestTaskConflict", err)
	}
}

func TestChangeRequestStoreDeleteByWorkspace(t *testing.T) {
	store := newChangeRequestTestStore(t)
	ctx := context.Background()
	for _, row := range []struct {
		installationID string
		workspaceID    string
		number         int64
	}{
		{"inst-1", "ws-1", 1},
		{"inst-1", "ws-2", 2},
		{"inst-2", "ws-1", 3},
	} {
		if _, _, err := store.ReportChangeRequest(ctx, ChangeRequestRecord{
			InstallationID: row.installationID, WorkspaceID: row.workspaceID, TaskID: "task-1",
			ProviderID: "forgejo", RepositoryID: "repo-1", Number: row.number, State: ChangeRequestStateOpen,
		}); err != nil {
			t.Fatalf("report %+v: %v", row, err)
		}
	}
	if err := store.DeleteByWorkspace(ctx, "ws-1"); err != nil {
		t.Fatalf("delete by workspace: %v", err)
	}
	for _, row := range []struct {
		installationID string
		workspaceID    string
		want           int
	}{
		{"inst-1", "ws-1", 0},
		{"inst-2", "ws-1", 0},
		{"inst-1", "ws-2", 1},
	} {
		rows, err := store.ListChangeRequestsByTask(ctx, row.installationID, row.workspaceID, "task-1")
		if err != nil {
			t.Fatalf("list %+v: %v", row, err)
		}
		if len(rows) != row.want {
			t.Errorf("rows for %+v = %d, want %d", row, len(rows), row.want)
		}
	}
}

func TestReportAndRemoveWithReceiptAreAtomic(t *testing.T) {
	ledger := newChangeRequestTestStore(t)
	commands := &CommandStore{db: ledger.db, ro: ledger.ro}
	if err := commands.initSchema(); err != nil {
		t.Fatalf("init command schema: %v", err)
	}
	ctx := context.Background()
	admit := func(requestID, key string) CommandRecord {
		record, _, err := commands.Admit(ctx, CommandIntent{
			InstallationID: "inst-1", WorkspaceID: "ws-1", RequestID: requestID, IdempotencyKey: key,
			Method: "ReportTaskChangeRequestExact", CapabilityID: "cap", PayloadDigest: "digest",
			TargetID: "task-1", ExpectedResourceVersion: "v", ApprovalRevision: 1, ManifestDigest: "digest",
		})
		if err != nil {
			t.Fatalf("admit %s: %v", requestID, err)
		}
		return record
	}

	reportIntent := admit("report-request", "report-key")
	change, receipt, err := ledger.ReportChangeRequestWithReceipt(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 1, State: ChangeRequestStateOpen,
	}, commands, reportIntent.Intent.OperationID, "APPLIED")
	if err != nil || change.ID == "" || receipt.Receipt.State != commandReceiptStateCompleted || receipt.Receipt.ResultStatus != "APPLIED" {
		t.Fatalf("report with receipt = change:%+v receipt:%+v err:%v", change, receipt, err)
	}

	rollbackIntent := admit("rollback-request", "rollback-key")
	if _, _, err := ledger.ReportChangeRequestWithReceipt(ctx, ChangeRequestRecord{
		InstallationID: "inst-1", WorkspaceID: "ws-1", TaskID: "task-1",
		ProviderID: "forgejo", RepositoryID: "repo-1", Number: 2, State: ChangeRequestStateOpen,
	}, commands, "missing-operation", "APPLIED"); err == nil {
		t.Fatal("missing command receipt must fail the transaction")
	}
	rows, err := ledger.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list after rollback: %v", err)
	}
	if len(rows) != 1 || rows[0].Number != 1 {
		t.Fatalf("rows after rollback = %+v, want only number 1", rows)
	}
	_ = rollbackIntent

	removeIntent := admit("remove-request", "remove-key")
	removeReceipt, err := ledger.RemoveChangeRequestWithReceipt(ctx, "inst-1", "ws-1", "forgejo", "repo-1", 1,
		commands, removeIntent.Intent.OperationID, "APPLIED")
	if err != nil || removeReceipt.Receipt.State != commandReceiptStateCompleted {
		t.Fatalf("remove with receipt = %+v err:%v", removeReceipt, err)
	}
	rows, err = ledger.ListChangeRequestsByTask(ctx, "inst-1", "ws-1", "task-1")
	if err != nil {
		t.Fatalf("list after remove: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows after remove = %d, want 0", len(rows))
	}
}
