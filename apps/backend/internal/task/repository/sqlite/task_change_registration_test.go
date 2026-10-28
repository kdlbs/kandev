package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/testutil"
)

func TestTaskChangeRegistrationIntentSQLite(t *testing.T) {
	runTaskChangeRegistrationIntentContract(t, newRepoForMetadataCASTests(t))
}

func TestTaskChangeRegistrationIntentPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo := newPostgresMetadataCASRepo(t, db)
	runTaskChangeRegistrationIntentContract(t, repo)
}

func runTaskChangeRegistrationIntentContract(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)

	first := taskChangeRegistrationIntent("intent-first", `{"url":"https://github.com/acme/app/pull/12"}`)
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	created, inserted, err := repo.CreateTaskChangeRegistrationIntentTx(ctx, tx, first)
	if err != nil || !inserted || created.ID != first.ID {
		t.Fatalf("create intent = (%+v, inserted=%v, err=%v)", created, inserted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	replay := taskChangeRegistrationIntent("intent-replay", `{"url":"https://github.com/acme/app/pull/12"}`)
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	replayed, inserted, err := repo.CreateTaskChangeRegistrationIntentTx(ctx, tx, replay)
	if err != nil || inserted || replayed.ID != first.ID {
		t.Fatalf("exact replay = (%+v, inserted=%v, err=%v)", replayed, inserted, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	changed := taskChangeRegistrationIntent("intent-changed", `{"url":"https://github.com/acme/other/pull/12"}`)
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateTaskChangeRegistrationIntentTx(ctx, tx, changed); !errors.Is(err, ErrTaskChangeRegistrationConflict) {
		_ = tx.Rollback()
		t.Fatalf("changed origin reuse error = %v, want conflict", err)
	}
	_ = tx.Rollback()

	associationRevision := int64(4)
	updated, err := repo.SetTaskChangeRegistrationIntentResult(ctx, first.ID, "pending", "succeeded", "",
		json.RawMessage(`{"provider":"github","number":12}`), nil, &associationRevision, "receipt-1")
	if err != nil || !updated {
		t.Fatalf("set intent result = (%v, err=%v)", updated, err)
	}
	if updated, err := repo.SetTaskChangeRegistrationIntentResult(ctx, first.ID, "pending", "failed", "stale", nil, nil, nil, ""); err != nil || updated {
		t.Fatalf("stale result update = (%v, err=%v), want CAS miss", updated, err)
	}
	stored, err := repo.GetTaskChangeRegistrationIntent(ctx, first.TaskID, first.ID)
	if err != nil || stored == nil || stored.State != "succeeded" || stored.AssociationRevision == nil ||
		*stored.AssociationRevision != associationRevision || stored.OperationReceipt != "receipt-1" {
		t.Fatalf("stored result = (%+v, err=%v)", stored, err)
	}

	rolledBack := taskChangeRegistrationIntent("intent-rollback", `{"url":"https://github.com/acme/app/pull/13"}`)
	rolledBack.OriginID = "operation-rollback"
	rolledBack.OperationID = "operation-rollback"
	rolledBack.ReferenceRef = "github:app#13"
	rolledBack.ChangeRequestID = "13"
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateTaskChangeRegistrationIntentTx(ctx, tx, rolledBack); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SetTaskChangeRegistrationIntentResultTx(ctx, tx, first.ID, "succeeded", "failed", "test", nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetTaskChangeRegistrationIntent(ctx, rolledBack.TaskID, rolledBack.ID); err != nil || got != nil {
		t.Fatalf("rolled back intent = (%+v, err=%v), want absent", got, err)
	}
	stored, err = repo.GetTaskChangeRegistrationIntent(ctx, first.TaskID, first.ID)
	if err != nil || stored == nil || stored.State != "succeeded" {
		t.Fatalf("rolled back result = (%+v, err=%v), want succeeded", stored, err)
	}
}

func taskChangeRegistrationIntent(id, snapshot string) *TaskChangeRegistrationIntent {
	return &TaskChangeRegistrationIntent{
		ID: id, TaskID: casTaskID, WorkspaceID: "ws-cas", SourceKind: "agent_registration",
		OriginID: "operation-1", SourceRevision: "0", ReferenceRef: "github:app#12",
		ReferenceSnapshot: snapshot, Provider: "github", ProviderHost: "https://github.com",
		ConnectionScope: "builtin:ws-cas", ProviderRepositoryID: "42", ChangeRequestID: "12",
		OperationID: "operation-1", OperationFingerprint: "fingerprint-1",
	}
}
