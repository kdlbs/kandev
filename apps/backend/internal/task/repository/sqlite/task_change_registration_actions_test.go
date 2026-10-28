package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	apiv1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestTaskChangeRegistrationActionsSQLite(t *testing.T) {
	runTaskChangeRegistrationActions(t, newRepoForMetadataCASTests(t))
}

func TestTaskChangeRegistrationActionsPostgres(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runTaskChangeRegistrationActions(t, newPostgresMetadataCASRepo(t, db))
}

func runTaskChangeRegistrationActions(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-registration-actions", TaskID: casTaskID,
		State: models.TaskSessionStateWaitingForInput}); err != nil {
		t.Fatalf("create task session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: "turn-registration-actions", TaskID: casTaskID,
		TaskSessionID: "session-registration-actions"}); err != nil {
		t.Fatalf("create task turn: %v", err)
	}

	reference := selectedPRReference("selection-original", apiv1.EntityReferenceModeLinkOnSend)
	reference.RegistrationStatus = apiv1.EntityReferenceRegistrationUnlinked
	message := &models.Message{ID: "message-registration-actions", TaskID: casTaskID,
		TaskSessionID: "session-registration-actions", TurnID: "turn-registration-actions",
		AuthorType: models.MessageAuthorUser, AuthorID: "user-cas", Content: "Check this pull request",
		Metadata: map[string]any{"entity_references": []apiv1.EntityReference{reference}}}
	if err := repo.CreateMessage(ctx, message); err != nil {
		t.Fatalf("create origin message: %v", err)
	}
	intent, err := repo.GetTaskChangeRegistrationIntentBySelection(ctx, casTaskID, reference.SelectionID)
	if err != nil || intent == nil {
		t.Fatalf("read accepted registration intent: intent=%+v err=%v", intent, err)
	}
	oldRevision := int64(1)
	detachedRevision := int64(2)
	tx, err := repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_change_registration_intents
		SET provider_host = ?, repository_id = ?, connection_scope = ?, provider_repository_id = ?, change_request_id = ?
		WHERE id = ?`), "https://github.com", "repository-cas", "builtin:ws-cas", "42", "12", intent.ID); err != nil {
		_ = tx.Rollback()
		t.Fatalf("resolve accepted intent identity: %v", err)
	}
	if updated, err := repo.SetTaskChangeRegistrationIntentResultTx(ctx, tx, intent.ID, "pending", "unlinked", "unlinked",
		json.RawMessage(`{"provider":"github","number":"12"}`), &oldRevision, &detachedRevision,
		"association-registration-1"); err != nil || !updated {
		_ = tx.Rollback()
		t.Fatalf("mark accepted intent unlinked: updated=%v err=%v", updated, err)
	}
	if err := repo.SetMessageRegistrationStatusTx(ctx, tx, message.ID, reference.SelectionID, "unlinked", "unlinked"); err != nil {
		_ = tx.Rollback()
		t.Fatalf("mark message reference unlinked: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit unlinked accepted intent: %v", err)
	}

	relinked, replayed, err := repo.ActTaskChangeRegistration(ctx, casTaskID, reference.SelectionID,
		"relink-operation-1", "user-cas", "relink")
	if err != nil || replayed || relinked == nil || relinked.State != "pending" ||
		relinked.SelectionID != "relink-operation-1" || relinked.ExpectedRevision == nil || *relinked.ExpectedRevision != 2 ||
		relinked.OperationID != "relink-operation-1" {
		t.Fatalf("create explicit relink: intent=%+v replayed=%v err=%v", relinked, replayed, err)
	}
	replayedIntent, replayed, err := repo.ActTaskChangeRegistration(ctx, casTaskID, reference.SelectionID,
		"relink-operation-1", "user-cas", "relink")
	if err != nil || !replayed || replayedIntent == nil || replayedIntent.ID != relinked.ID {
		t.Fatalf("replay explicit relink: intent=%+v replayed=%v err=%v", replayedIntent, replayed, err)
	}
	if _, _, err := repo.ActTaskChangeRegistration(ctx, casTaskID, reference.SelectionID,
		"relink-operation-1", "another-user", "relink"); !errors.Is(err, ErrTaskChangeRegistrationOperationConflict) {
		t.Fatalf("relink operation identity reuse error=%v, want conflict", err)
	}
	messageAfterRelink, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read relinked message: %v", err)
	}
	references := taskChangeRegistrationMessageReferences(t, messageAfterRelink.Metadata["entity_references"])
	if len(references) != 1 || references[0].SelectionID != relinked.SelectionID ||
		references[0].AssociationMode != apiv1.EntityReferenceModeLinkOnSend ||
		references[0].RegistrationStatus != apiv1.EntityReferenceRegistrationPending {
		t.Fatalf("relinked message reference=%+v", references)
	}

	associationRevision := int64(3)
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated, err := repo.SetTaskChangeRegistrationIntentResultTx(ctx, tx, relinked.ID, "pending", "succeeded", "",
		json.RawMessage(`{"provider":"github","number":"12"}`), relinked.ExpectedRevision, &associationRevision,
		"association-registration-1"); err != nil || !updated {
		_ = tx.Rollback()
		t.Fatalf("commit relink success: updated=%v err=%v", updated, err)
	}
	if err := repo.SetMessageRegistrationStatusTx(ctx, tx, message.ID, relinked.SelectionID, "succeeded", ""); err != nil {
		_ = tx.Rollback()
		t.Fatalf("project relink success: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit relink success transaction: %v", err)
	}

	notifications, err := repo.MarkTaskChangeRegistrationUnlinked(ctx, casTaskID, "ws-cas", "github",
		"association-registration-1", "https://github.com", "builtin:ws-cas", "42", "12", 2)
	if err != nil || len(notifications.MessageIDs) != 0 || len(notifications.QueueSessionIDs) != 0 {
		t.Fatalf("stale unlink event changed relink: notifications=%+v err=%v", notifications, err)
	}
	current, err := repo.GetTaskChangeRegistrationIntent(ctx, casTaskID, relinked.ID)
	if err != nil || current == nil || current.State != "succeeded" || current.AssociationRevision == nil || *current.AssociationRevision != 3 {
		t.Fatalf("stale unlink event changed new intent: intent=%+v err=%v", current, err)
	}

	notifications, err = repo.MarkTaskChangeRegistrationUnlinked(ctx, casTaskID, "ws-cas", "github",
		"association-registration-1", "https://github.com", "builtin:ws-cas", "42", "12", 4)
	if err != nil || len(notifications.MessageIDs) != 1 || notifications.MessageIDs[0] != message.ID {
		t.Fatalf("current unlink event projection: notifications=%+v err=%v", notifications, err)
	}
	current, err = repo.GetTaskChangeRegistrationIntent(ctx, casTaskID, relinked.ID)
	if err != nil || current == nil || current.State != "unlinked" || current.AssociationRevision == nil || *current.AssociationRevision != 4 {
		t.Fatalf("current unlink intent projection: intent=%+v err=%v", current, err)
	}
	messageAfterUnlink, err := repo.GetMessage(ctx, message.ID)
	if err != nil {
		t.Fatalf("read unlinked message: %v", err)
	}
	references = taskChangeRegistrationMessageReferences(t, messageAfterUnlink.Metadata["entity_references"])
	if len(references) != 1 || references[0].RegistrationStatus != apiv1.EntityReferenceRegistrationUnlinked {
		t.Fatalf("unlinked message reference=%+v", references)
	}

	queueReference := selectedPRReference("queue-selection-transferred", apiv1.EntityReferenceModeLinkOnSend)
	queueReference.RegistrationStatus = apiv1.EntityReferenceRegistrationUnlinked
	expectedRevision := int64(3)
	associationRevision = int64(4)
	queueIntent := &TaskChangeRegistrationIntent{
		TaskID: casTaskID, WorkspaceID: "ws-cas", ActorID: "user-cas",
		SourceKind: TaskChangeRegistrationSourceQueueAdmission, OriginID: "queue-admission-registration-actions",
		QueueSessionID: "session-registration-actions", QueueEntryID: "queue-entry-registration-actions",
		SourceRevision: "1", SelectionID: queueReference.SelectionID, ReferenceRef: queueReference.Ref,
		ReferenceSnapshot: mustJSON(t, queueReference), Provider: "github", ProviderHost: "https://github.com",
		RepositoryID: "repository-cas", ConnectionScope: "builtin:ws-cas", ProviderRepositoryID: "42",
		ChangeRequestID: "12", ExpectedRevision: &expectedRevision, AssociationRevision: &associationRevision,
		OperationID: "queue-admission-registration-actions", OperationFingerprint: "queue-registration-fingerprint",
		OperationReceipt: "association-registration-1", State: "unlinked",
	}
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.CreateTaskChangeRegistrationIntentTx(ctx, tx, queueIntent); err != nil {
		_ = tx.Rollback()
		t.Fatalf("create transferred queue intent: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit transferred queue intent: %v", err)
	}
	transferredMessage := &models.Message{
		ID: "message-registration-actions-queue", TaskID: casTaskID,
		TaskSessionID: "session-registration-actions", TurnID: "turn-registration-actions",
		AuthorType: models.MessageAuthorUser, AuthorID: "user-cas", Content: "Queue reference",
		Metadata: map[string]any{
			"entity_references":       []apiv1.EntityReference{queueReference},
			"registration_origin_ids": []string{queueIntent.OriginID},
			"unrelated":               "keep",
		},
	}
	if err := repo.CreateMessage(ctx, transferredMessage); err != nil {
		t.Fatalf("create transcript from queued registration: %v", err)
	}
	transferred, err := repo.GetTaskChangeRegistrationIntentBySelection(ctx, casTaskID, queueReference.SelectionID)
	if err != nil || transferred == nil || transferred.MessageID != transferredMessage.ID ||
		transferred.SourceKind != TaskChangeRegistrationSourceQueueAdmission {
		t.Fatalf("queue intent transfer = (%+v, err=%v)", transferred, err)
	}
	queueRelink, replayed, err := repo.ActTaskChangeRegistration(ctx, casTaskID, queueReference.SelectionID,
		"queue-relink-operation", "user-cas", "relink")
	if err != nil || replayed || queueRelink == nil || queueRelink.State != "pending" ||
		queueRelink.MessageID != transferredMessage.ID || queueRelink.SourceKind != TaskChangeRegistrationSourceQueueAdmission {
		t.Fatalf("relink transferred queue intent: intent=%+v replayed=%v err=%v", queueRelink, replayed, err)
	}
	tx, err = repo.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	notifications, err = repo.ProjectTaskChangeRegistrationStatusTx(ctx, tx, queueRelink, "failed", "provider_unavailable")
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("project transferred queue status: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit transferred queue projection: %v", err)
	}
	if len(notifications.MessageIDs) != 1 || notifications.MessageIDs[0] != transferredMessage.ID || len(notifications.QueueSessionIDs) != 0 {
		t.Fatalf("transferred queue notifications=%+v", notifications)
	}
	messageAfterQueueRelink, err := repo.GetMessage(ctx, transferredMessage.ID)
	if err != nil {
		t.Fatalf("read queue-origin relinked message: %v", err)
	}
	references = taskChangeRegistrationMessageReferences(t, messageAfterQueueRelink.Metadata["entity_references"])
	if len(references) != 1 || references[0].SelectionID != queueRelink.SelectionID ||
		references[0].RegistrationStatus != apiv1.EntityReferenceRegistrationFailed || messageAfterQueueRelink.Metadata["unrelated"] != "keep" {
		t.Fatalf("queue-origin relink message projection=%+v metadata=%#v", references, messageAfterQueueRelink.Metadata)
	}
}

func taskChangeRegistrationMessageReferences(t *testing.T, value any) []apiv1.EntityReference {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal message reference projection: %v", err)
	}
	var references []apiv1.EntityReference
	if err := json.Unmarshal(encoded, &references); err != nil {
		t.Fatalf("decode message reference projection: %v", err)
	}
	return references
}
