package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestPostgresRegistrationMessageWritersSerializeMetadataUpdates(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 3)
	repo := newPostgresMetadataCASRepo(t, db)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-registration-message-race", TaskID: casTaskID,
		State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-registration-message-race", TaskID: casTaskID,
		TaskSessionID: "session-registration-message-race",
	}); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	first := selectedPRReference("selection-race-first", apiv1.EntityReferenceModeLinkOnSend)
	second := selectedPRReference("selection-race-second", apiv1.EntityReferenceModeLinkOnSend)
	second.ID = "PR_kwDOAB"
	second.Ref = entityrefs.CanonicalRef(second.Provider, second.Kind, second.Scope, second.ID)
	second.Key = "acme/app#13"
	second.URL = "https://github.com/acme/app/pull/13"
	message := &models.Message{
		ID: "message-registration-message-race", TaskID: casTaskID,
		TaskSessionID: "session-registration-message-race", TurnID: "turn-registration-message-race",
		AuthorType: models.MessageAuthorUser, Content: "Two references",
		Metadata: map[string]any{
			"entity_references":  []apiv1.EntityReference{first, second},
			"unrelated_metadata": "preserved",
		},
	}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create accepted message: %v", err)
	}

	holder, err := db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin holder transaction: %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if err := repo.SetMessageRegistrationStatusTx(ctx, holder, message.ID, first.SelectionID, "succeeded", ""); err != nil {
		t.Fatalf("update first selection in holder transaction: %v", err)
	}

	type writeResult struct{ err error }
	writeStarted := make(chan int, 1)
	writeDone := make(chan writeResult, 1)
	writeFinished := make(chan struct{})
	go func() {
		defer close(writeFinished)
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			writeDone <- writeResult{err: err}
			return
		}
		defer func() { _ = tx.Rollback() }()
		var pid int
		if err := tx.GetContext(ctx, &pid, `SELECT pg_backend_pid()`); err != nil {
			writeStarted <- 0
			writeDone <- writeResult{err: err}
			return
		}
		writeStarted <- pid
		replacement := second
		replacement.SelectionID = "selection-race-second-new"
		replacement.RegistrationStatus = apiv1.EntityReferenceRegistrationPending
		if err := repo.replaceMessageRegistrationSelectionTx(ctx, tx, message.ID, second.SelectionID, replacement); err != nil {
			writeDone <- writeResult{err: err}
			return
		}
		if err := repo.SetMessageRegistrationStatusTx(ctx, tx, message.ID, replacement.SelectionID, "pending", ""); err != nil {
			writeDone <- writeResult{err: err}
			return
		}
		writeDone <- writeResult{err: tx.Commit()}
	}()

	pid := <-writeStarted
	if pid == 0 {
		t.Fatalf("concurrent writer could not report its PostgreSQL backend")
	}
	if err := waitForPostgresLock(ctx, db, pid, writeFinished); err != nil {
		t.Fatalf("second metadata writer did not wait on the message row lock: %v", err)
	}
	if err := holder.Commit(); err != nil {
		t.Fatalf("commit first selection update: %v", err)
	}
	select {
	case result := <-writeDone:
		if result.err != nil {
			t.Fatalf("commit concurrent selection replacement: %v", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent selection replacement did not finish after releasing the row lock")
	}

	stored, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read updated message: %v", err)
	}
	references := taskChangeRegistrationMessageReferences(t, stored.Metadata["entity_references"])
	bySelection := make(map[string]apiv1.EntityReference, len(references))
	for _, reference := range references {
		bySelection[reference.SelectionID] = reference
	}
	if len(references) != 2 || bySelection[first.SelectionID].RegistrationStatus != apiv1.EntityReferenceRegistrationLinked ||
		bySelection["selection-race-second-new"].RegistrationStatus != apiv1.EntityReferenceRegistrationPending {
		t.Fatalf("concurrent reference projection = %+v, want first linked and replacement pending", references)
	}
	if value, ok := stored.Metadata["unrelated_metadata"]; !ok || value != "preserved" {
		t.Fatalf("unrelated metadata = %#v (present=%v), want preserved value", value, ok)
	}
}
