package state

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/db"
)

func TestCommandStoreAdmitReplaysAndRejectsPayloadMismatch(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := NewCommandStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	intent := CommandIntent{
		InstallationID: "installation-1", WorkspaceID: "workspace-1", RequestID: "request-1",
		IdempotencyKey: "key-1", Method: "UpdateTaskExact", CapabilityID: "host.v2.write:tasks",
		PayloadDigest: "digest-1", TargetID: "task-1", ExpectedResourceVersion: "version-1",
		ApprovalRevision: 4, ManifestDigest: "manifest-1",
	}

	first, replayed, err := store.Admit(context.Background(), intent)
	if err != nil || replayed {
		t.Fatalf("first Admit: replayed=%v err=%v", replayed, err)
	}
	second, replayed, err := store.Admit(context.Background(), intent)
	if err != nil || !replayed {
		t.Fatalf("retry Admit: replayed=%v err=%v", replayed, err)
	}
	if second.Intent.OperationID != first.Intent.OperationID || second.Receipt.ID != first.Receipt.ID {
		t.Fatalf("retry changed durable identity: first=%+v second=%+v", first, second)
	}

	intent.PayloadDigest = "digest-2"
	if _, _, err := store.Admit(context.Background(), intent); !errors.Is(err, ErrCommandPayloadConflict) {
		t.Fatalf("changed payload error = %v, want ErrCommandPayloadConflict", err)
	}
}

func TestCommandStoreCompletesReceiptAcrossReload(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	pool := db.NewPool(conn, conn)
	store, err := NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	record, _, err := store.Admit(context.Background(), CommandIntent{
		InstallationID: "installation-1", WorkspaceID: "workspace-1", RequestID: "request-1",
		IdempotencyKey: "key-1", Method: "UpdateTaskExact", CapabilityID: "host.v2.write:tasks",
		PayloadDigest: "digest-1", TargetID: "task-1", ExpectedResourceVersion: "version-1",
		ApprovalRevision: 4, ManifestDigest: "manifest-1",
	})
	if err != nil {
		t.Fatalf("Admit: %v", err)
	}
	if _, err := store.Complete(context.Background(), record.Intent.OperationID, "APPLIED", "", "task-1", "version-2"); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	reopened, err := NewCommandStore(pool)
	if err != nil {
		t.Fatalf("reopen CommandStore: %v", err)
	}
	got, err := reopened.Get(context.Background(), record.Intent.OperationID)
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.Receipt.State != "completed" || got.Receipt.ResultStatus != "APPLIED" || got.Receipt.ResourceVersion != "version-2" {
		t.Fatalf("receipt after reload = %+v, want completed APPLIED at version-2", got.Receipt)
	}
}

func TestCommandStoreSourceWritebackReceiptPersistsAndNeverRestartsSending(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	pool := db.NewPool(conn, conn)
	store, err := NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	input := SourceWritebackIntent{
		OperationID: "operation-1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		TaskID: "task-1", Provider: "jira", SourceID: "PROJ-1", Operation: "comment",
		ExpectedSourceVersion: "source-version-1", PayloadDigest: "payload-digest-1",
	}
	prepared, replayed, err := store.PrepareSourceWriteback(context.Background(), input)
	if err != nil || replayed || prepared.State != "prepared" {
		t.Fatalf("prepare = %+v, replayed=%v, err=%v", prepared, replayed, err)
	}
	if prepared.InstallationID != input.InstallationID || prepared.TaskID != input.TaskID || prepared.SourceID != input.SourceID {
		t.Fatalf("receipt omitted actor or source subject: %+v", prepared)
	}
	if _, started, err := store.StartSourceWriteback(context.Background(), input.OperationID); err != nil || !started {
		t.Fatalf("first Start = started %v, err %v", started, err)
	}
	if _, started, err := store.StartSourceWriteback(context.Background(), input.OperationID); err != nil || started {
		t.Fatalf("second Start = started %v, err %v, want no second send", started, err)
	}

	reopened, err := NewCommandStore(pool)
	if err != nil {
		t.Fatalf("reopen CommandStore: %v", err)
	}
	recovered, err := reopened.GetSourceWriteback(context.Background(), input.OperationID)
	if err != nil || recovered.State != "sending" {
		t.Fatalf("recovered in-flight receipt = %+v, err=%v", recovered, err)
	}
	completed, err := reopened.FinishSourceWriteback(context.Background(), input.OperationID, "APPLIED", "", "comment-1")
	if err != nil || completed.State != "completed" || completed.ProviderReceiptID != "comment-1" {
		t.Fatalf("finish = %+v, err=%v", completed, err)
	}
	replay, err := reopened.FinishSourceWriteback(context.Background(), input.OperationID, "APPLIED", "", "comment-1")
	if err != nil || replay.ProviderReceiptID != "comment-1" {
		t.Fatalf("finish replay = %+v, err=%v", replay, err)
	}
	if _, err := reopened.FinishSourceWriteback(context.Background(), input.OperationID, "UNCERTAIN", "timeout", ""); !errors.Is(err, ErrSourceWritebackConflict) {
		t.Fatalf("changed outcome error = %v, want ErrSourceWritebackConflict", err)
	}
}

func TestCommandStoreSourceWritebackRejectsChangedPreparedIdentity(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	store, err := NewCommandStore(db.NewPool(conn, conn))
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	input := SourceWritebackIntent{
		OperationID: "operation-1", InstallationID: "installation-1", WorkspaceID: "workspace-1",
		TaskID: "task-1", Provider: "linear", SourceID: "issue-1", Operation: "transition",
		ExpectedSourceVersion: "source-version-1", PayloadDigest: "payload-digest-1",
	}
	if _, _, err := store.PrepareSourceWriteback(context.Background(), input); err != nil {
		t.Fatalf("PrepareSourceWriteback: %v", err)
	}
	input.SourceID = "issue-2"
	if _, _, err := store.PrepareSourceWriteback(context.Background(), input); !errors.Is(err, ErrSourceWritebackConflict) {
		t.Fatalf("changed source identity error = %v, want ErrSourceWritebackConflict", err)
	}
}

func TestCommandStoreTaskDirectivesPersistAndResolveOnce(t *testing.T) {
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })
	pool := db.NewPool(conn, conn)
	store, err := NewCommandStore(pool)
	if err != nil {
		t.Fatalf("NewCommandStore: %v", err)
	}
	input := TaskDirectiveRecord{
		ID: "directive-operation-1", OperationID: "directive-operation-1", InstallationID: "installation-1",
		WorkspaceID: "workspace-1", TaskID: "task-1", SessionID: "session-1",
		IdempotencyKey: "directive-1", PayloadDigest: "digest-1", CapabilityClass: "host.v2.write:tasks",
		InstructionDigest: "sha256:instruction", ExpectedTaskResourceVersion: "task-version",
		ExpectedSessionResourceVersion: "session-version", ApprovalRevision: 3,
		ExpiresAt: "2099-01-01T00:00:00Z", AuditID: "audit-1",
	}
	first, replayed, err := store.IssueTaskDirective(context.Background(), input)
	if err != nil || replayed {
		t.Fatalf("first issue = %+v, replayed=%v, err=%v", first, replayed, err)
	}
	second, replayed, err := store.IssueTaskDirective(context.Background(), input)
	if err != nil || !replayed || second.ID != first.ID || second.ResourceVersion() != first.ResourceVersion() {
		t.Fatalf("replayed issue = %+v, replayed=%v, err=%v", second, replayed, err)
	}
	resolved, replayed, err := store.ResolveTaskDirective(context.Background(), "installation-1", "workspace-1", first.ID,
		first.ResourceVersion(), "resolve-operation-1", "completed", "sha256:resolution")
	if err != nil || replayed || resolved.State != "resolved" {
		t.Fatalf("resolve = %+v, replayed=%v, err=%v", resolved, replayed, err)
	}
	replayedDirective, replayed, err := store.ResolveTaskDirective(context.Background(), "installation-1", "workspace-1", first.ID,
		first.ResourceVersion(), "resolve-operation-1", "completed", "sha256:resolution")
	if err != nil || !replayed || replayedDirective.ResourceVersion() != resolved.ResourceVersion() {
		t.Fatalf("resolve retry = %+v, replayed=%v, err=%v", replayedDirective, replayed, err)
	}
	if _, _, err := store.ResolveTaskDirective(context.Background(), "installation-1", "workspace-1", first.ID,
		first.ResourceVersion(), "other-resolve-operation", "completed", "sha256:resolution"); !errors.Is(err, ErrTaskDirectiveConflict) {
		t.Fatalf("second resolution error = %v, want conflict", err)
	}
}
