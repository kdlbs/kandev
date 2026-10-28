package backendapp

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/entityrefs"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestSQLiteQueueRegistrationAdmissionAndEditPersistIntentsWithQueueChanges(t *testing.T) {
	_, taskRepo, db := newTaskChangeCoordinatorHarnessWithDB(t)
	runQueueRegistrationAdmissionAndEditPersistIntentsWithQueueChanges(t, taskRepo, db)
}

func TestPostgresQueueRegistrationAdmissionAndEditPersistIntentsWithQueueChanges(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize PostgreSQL task repository: %v", err)
	}
	if _, err := github.NewStore(db, db); err != nil {
		t.Fatalf("initialize PostgreSQL GitHub repository: %v", err)
	}
	runQueueRegistrationAdmissionAndEditPersistIntentsWithQueueChanges(t, taskRepo, db)
}

func runQueueRegistrationAdmissionAndEditPersistIntentsWithQueueChanges(
	t *testing.T, taskRepo *tasksqlite.Repository, db *sqlx.DB,
) {
	t.Helper()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{
		UserID: "user-registration", Role: authn.RoleMember,
	})
	seedTaskChangeCoordinatorTask(t, taskRepo, "ws-queue-registration", "task-queue-registration", "repo-queue-registration",
		"https://github.com", "acme", "api")
	if _, err := db.ExecContext(ctx, db.Rebind(`UPDATE repositories SET provider_repo_id = 'provider-repo-17' WHERE id = ?`), "repo-queue-registration"); err != nil {
		t.Fatalf("set provider repository identity: %v", err)
	}
	identity := messagequeue.QueueSessionIdentity{
		TaskID: "task-queue-registration", SessionID: "session-queue-registration", SessionIncarnationID: "incarnation-1",
	}
	if err := taskRepo.CreateTaskSession(ctx, &models.TaskSession{
		ID: identity.SessionID, TaskID: identity.TaskID, QueueIncarnationID: identity.SessionIncarnationID,
		State: models.TaskSessionStateWaitingForInput, StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("create queue session: %v", err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(db, db)
	if err != nil {
		t.Fatalf("initialize queue repository: %v", err)
	}
	queue := messagequeue.NewService(queueRepo, 10, logger.Default())
	processor := newTaskChangeRegistrationProcessor(taskRepo, nil, taskChangeLinkCoordinator{}, db, nil, testRegistrationActorResolver)
	queue.SetQueueRegistrationWriters(processor.QueueAdmissionRegistrationTx, processor.QueueEditRegistrationTx)

	reference := queueRegistrationReference("queue-selection-1", v1.EntityReferenceModeLinkOnSend)
	metadata := map[string]interface{}{
		messagequeue.MetadataEntityReferences: []v1.EntityReference{reference},
		"unrelated":                           "preserve",
	}
	accepted, replayed, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "queue-admission-1", "Review this change", "", messagequeue.QueuedByUser,
		false, nil, metadata, nil,
	)
	if err != nil || replayed || accepted == nil {
		t.Fatalf("accept identified queued reference: entry=%+v replay=%v err=%v", accepted, replayed, err)
	}
	intent, err := taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, reference.SelectionID)
	if err != nil || intent == nil || intent.SourceKind != "queue_admission" || intent.ActorID != "user-registration" ||
		intent.QueueSessionID != identity.SessionID || intent.MessageID != "" || intent.State != "pending" {
		t.Fatalf("accepted queue registration intent = (%+v, err=%v)", intent, err)
	}
	var queueEntryID string
	if err := db.GetContext(ctx, &queueEntryID, db.Rebind(`SELECT queue_entry_id FROM task_change_registration_intents WHERE id = ?`), intent.ID); err != nil || queueEntryID != accepted.ID {
		t.Fatalf("queue intent entry identity = (%q, err=%v), want %q", queueEntryID, err, accepted.ID)
	}
	if got := entityrefs.NormalizePersisted(accepted.Metadata[messagequeue.MetadataEntityReferences]); len(got) != 1 ||
		got[0].RegistrationStatus != v1.EntityReferenceRegistrationPending {
		t.Fatalf("accepted queue reference status = %+v, want pending", got)
	}
	if accepted.Metadata["unrelated"] != "preserve" {
		t.Fatalf("queue admission dropped unrelated metadata: %#v", accepted.Metadata)
	}

	acceptedAgain, replayed, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "queue-admission-1", "Review this change", "", messagequeue.QueuedByUser,
		false, nil, metadata, nil,
	)
	if err != nil || !replayed || acceptedAgain.ID != accepted.ID {
		t.Fatalf("replay queue registration admission: entry=%+v replay=%v err=%v", acceptedAgain, replayed, err)
	}
	var intentCount int
	if err := db.GetContext(ctx, &intentCount, db.Rebind(`SELECT COUNT(*) FROM task_change_registration_intents WHERE task_id = ? AND selection_id = ?`), identity.TaskID, reference.SelectionID); err != nil || intentCount != 1 {
		t.Fatalf("queue admission replay intent count = (%d, err=%v), want one", intentCount, err)
	}

	failIntent := errors.New("reject after writing intent")
	queue.SetQueueRegistrationWriters(func(
		writerCtx context.Context,
		tx *sqlx.Tx,
		queueIdentity messagequeue.QueueSessionIdentity,
		admissionID string,
		candidate, admitted *messagequeue.QueuedMessage,
	) error {
		if err := processor.QueueAdmissionRegistrationTx(writerCtx, tx, queueIdentity, admissionID, candidate, admitted); err != nil {
			return err
		}
		return failIntent
	}, processor.QueueEditRegistrationTx)
	failedReference := queueRegistrationReference("queue-selection-rollback", v1.EntityReferenceModeLinkOnSend)
	_, _, err = queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "queue-admission-rollback", "Rollback me", "", messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{failedReference}}, nil,
	)
	if !errors.Is(err, failIntent) {
		t.Fatalf("queue registration writer error = %v, want injected failure", err)
	}
	rolledBack, err := taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, failedReference.SelectionID)
	if err != nil || rolledBack != nil {
		t.Fatalf("registration intent survived rejected queue admission: intent=%+v err=%v", rolledBack, err)
	}
	queue.SetQueueRegistrationWriters(processor.QueueAdmissionRegistrationTx, processor.QueueEditRegistrationTx)
	if _, replayed, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "queue-admission-rollback", "Rollback me", "", messagequeue.QueuedByUser, false, nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{failedReference}}, nil,
	); err != nil || replayed {
		t.Fatalf("rejected queue admission left a replay receipt: replay=%v err=%v", replayed, err)
	}
	if _, err := queueRepo.DeleteByIDForSession(ctx, identity, "queue-admission-rollback"); err != nil {
		t.Fatalf("remove accepted queue entry: %v", err)
	}
	retained, err := taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, failedReference.SelectionID)
	if err != nil || retained == nil {
		t.Fatalf("queue removal discarded accepted registration: intent=%+v err=%v", retained, err)
	}

	editWriterFailure := errors.New("reject queue edit after writing intent")
	queue.SetQueueRegistrationWriters(processor.QueueAdmissionRegistrationTx, func(
		writerCtx context.Context,
		tx *sqlx.Tx,
		operationID string,
		previous, updated *messagequeue.QueuedMessage,
	) error {
		if err := processor.QueueEditRegistrationTx(writerCtx, tx, operationID, previous, updated); err != nil {
			return err
		}
		return editWriterFailure
	})
	failedEditReference := queueRegistrationReference("queue-selection-edit-rollback", v1.EntityReferenceModeLinkOnSend)
	lease, err := queue.BeginEdit(ctx, identity.SessionID, accepted.ID, "editor-rollback")
	if err != nil {
		t.Fatalf("begin queue edit rollback test: %v", err)
	}
	_, err = queue.UpdateMessageWithLease(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "edit-rollback", "editor-rollback",
		lease.TargetRevision, "Review this change", nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference, failedEditReference}})
	if !errors.Is(err, editWriterFailure) {
		t.Fatalf("queue edit registration error = %v, want injected failure", err)
	}
	rolledBack, err = taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, failedEditReference.SelectionID)
	if err != nil || rolledBack != nil {
		t.Fatalf("registration intent survived rejected queue edit: intent=%+v err=%v", rolledBack, err)
	}
	entries, err := queueRepo.ListBySession(ctx, identity.SessionID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read queue after rejected edit: entries=%+v err=%v", entries, err)
	}
	if got := entityrefs.NormalizePersisted(entries[0].Metadata[messagequeue.MetadataEntityReferences]); len(got) != 1 || got[0].SelectionID != reference.SelectionID {
		t.Fatalf("rejected queue edit changed references: %+v", got)
	}
	if err := queue.EndEdit(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "editor-rollback"); err != nil {
		t.Fatalf("end rejected queue edit lease: %v", err)
	}
	queue.SetQueueRegistrationWriters(processor.QueueAdmissionRegistrationTx, processor.QueueEditRegistrationTx)

	lease, err = queue.BeginEdit(ctx, identity.SessionID, accepted.ID, "editor-1")
	if err != nil {
		t.Fatalf("begin queue edit: %v", err)
	}
	unchangedMetadata := map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}}
	_, err = queue.UpdateMessageWithLease(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "edit-unchanged", "editor-1",
		lease.TargetRevision, "Review this change", nil, unchangedMetadata)
	if err != nil {
		t.Fatalf("save unchanged queue reference: %v", err)
	}
	if err := queue.EndEdit(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "editor-1"); err != nil {
		t.Fatalf("end unchanged edit lease: %v", err)
	}
	if err := db.GetContext(ctx, &intentCount, db.Rebind(`SELECT COUNT(*) FROM task_change_registration_intents WHERE task_id = ? AND selection_id = ?`), identity.TaskID, reference.SelectionID); err != nil || intentCount != 1 {
		t.Fatalf("unchanged Save created another intent: count=%d err=%v", intentCount, err)
	}

	lease, err = queue.BeginEdit(ctx, identity.SessionID, accepted.ID, "editor-2")
	if err != nil {
		t.Fatalf("begin relink edit: %v", err)
	}
	reselected := queueRegistrationReference("queue-selection-2", v1.EntityReferenceModeLinkOnSend)
	_, err = queue.UpdateMessageWithLease(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "edit-reselect", "editor-2",
		lease.TargetRevision, "Review this change", nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reselected}})
	if err != nil {
		t.Fatalf("save reselected queue reference: %v", err)
	}
	newIntent, err := taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, reselected.SelectionID)
	if err != nil || newIntent == nil || newIntent.SourceKind != "queue_edit" || newIntent.State != "pending" {
		t.Fatalf("queue edit registration intent = (%+v, err=%v)", newIntent, err)
	}
	if err := db.GetContext(ctx, &queueEntryID, db.Rebind(`SELECT queue_entry_id FROM task_change_registration_intents WHERE id = ?`), newIntent.ID); err != nil || queueEntryID != accepted.ID {
		t.Fatalf("queue edit intent entry identity = (%q, err=%v), want %q", queueEntryID, err, accepted.ID)
	}
	if err := queue.EndEdit(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "editor-2"); err != nil {
		t.Fatal(err)
	}
	target, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, "queue-merge-target", "Earlier context", "", messagequeue.QueuedByUser, false, nil, nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.ReorderEntries(ctx, identity.SessionID, []string{target.ID, accepted.ID}); err != nil {
		t.Fatal(err)
	}
	accepted, err = queue.MergeIntoAbove(ctx, identity.SessionID, accepted.ID, messagequeue.QueuedByUser)
	if err != nil {
		t.Fatalf("fold registered queue entry: %v", err)
	}
	var notifiedTaskID, notifiedSessionID string
	processor.SetQueueStatusNotifier(func(_ context.Context, taskID, sessionID string) {
		notifiedTaskID, notifiedSessionID = taskID, sessionID
	})
	if err := processor.finishIntent(ctx, newIntent, "failed", "provider_unavailable", nil, ""); err != nil {
		t.Fatalf("project queue registration failure: %v", err)
	}
	if err := processor.finishIntent(ctx, newIntent, "failed", "provider_unavailable", nil, ""); err != nil {
		t.Fatalf("reconcile repeated queue registration result: %v", err)
	}
	if notifiedTaskID != identity.TaskID || notifiedSessionID != identity.SessionID {
		t.Fatalf("queue registration status notification = (%q, %q), want current task/session", notifiedTaskID, notifiedSessionID)
	}
	entries, err = queueRepo.ListBySession(ctx, identity.SessionID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read queue status after registration: entries=%+v err=%v", entries, err)
	}
	queuedReferences := entityrefs.NormalizePersisted(entries[0].Metadata[messagequeue.MetadataEntityReferences])
	if len(queuedReferences) != 1 || queuedReferences[0].SelectionID != reselected.SelectionID ||
		queuedReferences[0].RegistrationStatus != v1.EntityReferenceRegistrationFailed {
		t.Fatalf("queue status projection = %+v, want failed reselected reference", queuedReferences)
	}
	lease, err = queue.BeginEdit(ctx, identity.SessionID, accepted.ID, "editor-status")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.UpdateMessageWithLease(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "edit-status", "editor-status",
		lease.TargetRevision, entries[0].Content, nil,
		map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reselected}}); err != nil {
		t.Fatalf("save unchanged reference after background failure: %v", err)
	}
	entries, err = queueRepo.ListBySession(ctx, identity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	queuedReferences = entityrefs.NormalizePersisted(entries[0].Metadata[messagequeue.MetadataEntityReferences])
	if len(queuedReferences) != 1 || queuedReferences[0].RegistrationStatus != v1.EntityReferenceRegistrationFailed {
		t.Fatalf("unchanged Save lost authoritative queue status: %+v", queuedReferences)
	}
	if err := queue.EndEdit(ctx, identity.SessionID, accepted.ID, lease.LeaseID, "editor-status"); err != nil {
		t.Fatalf("end status projection edit lease: %v", err)
	}

	if err := taskRepo.CreateTurn(ctx, &models.Turn{ID: "turn-queue-registration", TaskID: identity.TaskID, TaskSessionID: identity.SessionID}); err != nil {
		t.Fatalf("create dispatch turn: %v", err)
	}
	dispatched := &models.Message{
		ID: "message-queue-registration", TaskID: identity.TaskID, TaskSessionID: identity.SessionID,
		TurnID: "turn-queue-registration", AuthorType: models.MessageAuthorUser, AuthorID: "user-registration",
		Type: models.MessageTypeMessage, Content: entries[0].Content, Metadata: entries[0].Metadata,
		CreatedAt: time.Now().UTC(),
	}
	if err := taskRepo.CreateMessage(ctx, dispatched); err != nil {
		t.Fatalf("record dispatched queue message: %v", err)
	}
	transferred, err := taskRepo.GetTaskChangeRegistrationIntent(ctx, identity.TaskID, newIntent.ID)
	if err != nil || transferred == nil || transferred.MessageID != dispatched.ID || transferred.SourceKind != tasksqlite.TaskChangeRegistrationSourceQueueEdit {
		t.Fatalf("dispatched queue registration source = (%+v, err=%v), want same edit intent attached to transcript", transferred, err)
	}
	storedMessage, err := taskRepo.GetMessage(ctx, dispatched.ID)
	if err != nil || storedMessage == nil {
		t.Fatalf("read dispatched transcript message: message=%+v err=%v", storedMessage, err)
	}
	messageReferences := entityrefs.NormalizePersisted(storedMessage.Metadata[messagequeue.MetadataEntityReferences])
	if len(messageReferences) != 1 || messageReferences[0].SelectionID != reselected.SelectionID ||
		messageReferences[0].RegistrationStatus != v1.EntityReferenceRegistrationFailed {
		t.Fatalf("dispatched transcript registration status = %+v, want truthful failed state", messageReferences)
	}
	verifyQueueRegistrationClear(t, ctx, queue, taskRepo, identity, []string{reselected.SelectionID}, 1)
	verifyQueueRegistrationSendNow(t, ctx, queue, queueRepo, taskRepo, identity)
}

func verifyQueueRegistrationClear(
	t *testing.T,
	ctx context.Context,
	queue *messagequeue.Service,
	taskRepo *tasksqlite.Repository,
	identity messagequeue.QueueSessionIdentity,
	selectionIDs []string,
	expectedRemoved int,
) {
	t.Helper()
	result, err := queue.CancelAllForSession(ctx, identity)
	if err != nil || result == nil || len(result.Removed) != expectedRemoved {
		t.Fatalf("clear registered queue entries: result=%+v err=%v, want %d removed", result, err, expectedRemoved)
	}
	for _, selectionID := range selectionIDs {
		intent, err := taskRepo.GetTaskChangeRegistrationIntentBySelection(ctx, identity.TaskID, selectionID)
		if err != nil || intent == nil {
			t.Fatalf("clear discarded accepted intent %q: intent=%+v err=%v", selectionID, intent, err)
		}
	}
}

func verifyQueueRegistrationSendNow(
	t *testing.T,
	ctx context.Context,
	queue *messagequeue.Service,
	queueRepo messagequeue.Repository,
	taskRepo *tasksqlite.Repository,
	identity messagequeue.QueueSessionIdentity,
) {
	t.Helper()
	registrations := []struct {
		selectionID string
		admissionID string
		changeID    string
		content     string
	}{
		{selectionID: "send-now-selection-one", admissionID: "send-now-admission-one", changeID: "PR_kwDOA1", content: "Send Now reference one"},
		{selectionID: "send-now-selection-two", admissionID: "send-now-admission-two", changeID: "PR_kwDOA2", content: "Send Now reference two"},
	}
	for _, registration := range registrations {
		reference := queueRegistrationReferenceForChange(registration.selectionID, registration.changeID, v1.EntityReferenceModeLinkOnSend)
		entry, replayed, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
			ctx, identity, registration.admissionID, registration.content, "",
			messagequeue.QueuedByUser, false, nil,
			map[string]interface{}{messagequeue.MetadataEntityReferences: []v1.EntityReference{reference}}, nil,
		)
		if err != nil || replayed || entry == nil {
			t.Fatalf("admit Send Now registration %q: entry=%+v replay=%v err=%v", registration.selectionID, entry, replayed, err)
		}
	}
	entries, err := queueRepo.ListBySession(ctx, identity.SessionID)
	if err != nil || len(entries) != 1 {
		t.Fatalf("read folded Send Now registration source: entries=%+v err=%v", entries, err)
	}
	claim, err := queue.ClaimSendNowForSession(ctx, identity, entries)
	if err != nil || claim == nil {
		t.Fatalf("claim Send Now registration sources: claim=%+v err=%v", claim, err)
	}
	originIDs, ok := claim.Dispatch.Metadata[messagequeue.MetadataRegistrationOriginIDs].([]string)
	if !ok || len(originIDs) != len(registrations) || originIDs[0] != registrations[0].admissionID || originIDs[1] != registrations[1].admissionID {
		t.Fatalf("Send Now registration origins = (%#v, valid=%v), want admission union", claim.Dispatch.Metadata[messagequeue.MetadataRegistrationOriginIDs], ok)
	}
	dispatchReferences := entityrefs.NormalizePersisted(claim.Dispatch.Metadata[messagequeue.MetadataEntityReferences])
	if len(dispatchReferences) != len(registrations) || dispatchReferences[0].SelectionID != registrations[0].selectionID ||
		dispatchReferences[1].SelectionID != registrations[1].selectionID {
		t.Fatalf("Send Now registration references = %+v, want both accepted selections", dispatchReferences)
	}
	if err := queue.RestoreSendNowClaim(ctx, claim); err != nil {
		t.Fatalf("restore Send Now registration sources: %v", err)
	}
	verifyQueueRegistrationClear(t, ctx, queue, taskRepo, identity,
		[]string{registrations[0].selectionID, registrations[1].selectionID}, 1)
}

func queueRegistrationReference(selectionID string, mode v1.EntityReferenceAssociationMode) v1.EntityReference {
	return queueRegistrationReferenceForChange(selectionID, "PR_kwDOAA", mode)
}

func queueRegistrationReferenceForChange(
	selectionID string,
	changeID string,
	mode v1.EntityReferenceAssociationMode,
) v1.EntityReference {
	return v1.EntityReference{
		Version:  v1.EntityReferenceVersion,
		Ref:      entityrefs.CanonicalRef("github", "pull_request", "github.com/acme/api", changeID),
		Provider: "github", Kind: "pull_request", ID: changeID, Key: "acme/api#17",
		Title: "Accepted PR", URL: "https://github.com/acme/api/pull/17", Scope: "github.com/acme/api",
		SelectionID: selectionID, AssociationMode: mode,
	}
}
