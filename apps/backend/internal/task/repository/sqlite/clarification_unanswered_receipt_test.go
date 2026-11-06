package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

const lateReceiptUUID = "550e8400-e29b-41d4-a716-446655440000"

func newLateReceiptFixture(t *testing.T) (*Repository, time.Time) {
	t.Helper()
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{
		ID: "receipt-ws", Name: "receipt-ws", ClarificationUnansweredInboxEnabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	seedBundleTask(t, repo, "receipt-task", "receipt-ws")
	seedBundleSession(t, repo, "receipt-session", "receipt-task")
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	seedBundleTurnAt(t, repo, "receipt-old", "receipt-session", "receipt-task", base)
	insertClarificationMessage(t, repo, "receipt-question", "receipt-session", "receipt-task",
		"receipt-old", "pending-1", "question-1", "pending", 0, base)
	seedBundleTurnAt(t, repo, "receipt-new", "receipt-session", "receipt-task", base.Add(2*time.Minute))
	return repo, base
}

func TestLateAnswerReceiptRequiresQuestionBindingAndTimelyAdmission(t *testing.T) {
	for _, tc := range []struct {
		name string
		id   string
		at   time.Duration
		want bool
	}{
		{"older message", "late:pending-1:" + lateReceiptUUID, -time.Minute, false},
		{"valid answer", "late:pending-1:" + lateReceiptUUID, time.Minute, true},
		{"unbound legacy ID", lateReceiptUUID, time.Minute, false},
		{"different question", "late:pending-2:" + lateReceiptUUID, time.Minute, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base := newLateReceiptFixture(t)
			_, err := repo.db.Exec(`
				INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
				VALUES (?, 'receipt-session', 'receipt-task', 'receipt-new', 'user', '', 'answer', 0, 'message', '{}', ?)`,
				tc.id, base.Add(tc.at))
			if err != nil {
				t.Fatal(err)
			}
			request := models.CloseUnansweredClarification{
				Reason: models.ClarificationClosedAnsweredLate, At: base.Add(3 * time.Minute),
				SessionID: "receipt-session", SourceSessionID: "receipt-session", MessageID: tc.id,
			}
			rows, err := repo.CloseUnansweredClarificationBundle(context.Background(), "pending-1", request)
			if tc.want {
				if err != nil || len(rows) != 1 {
					t.Fatalf("valid late answer: rows=%d err=%v", len(rows), err)
				}
				return
			}
			if err != models.ErrUnansweredAdmissionUnknown || len(rows) != 0 {
				t.Fatalf("unrelated admission: rows=%d err=%v", len(rows), err)
			}
			rows, err = repo.CloseUnansweredClarificationBundle(context.Background(), "pending-1",
				models.CloseUnansweredClarification{Reason: models.ClarificationClosedByUser, At: base.Add(4 * time.Minute), SourceSessionID: "receipt-session"})
			if err != nil || len(rows) != 1 {
				t.Fatalf("manual close after rejected receipt: rows=%d err=%v", len(rows), err)
			}
		})
	}
}

func TestLateAnswerWaitsForEntireQuestionBundle(t *testing.T) {
	for _, tc := range []struct {
		name        string
		addLast     bool
		answerTime  time.Duration
		removeIndex bool
		removeTotal bool
		wantRows    int
	}{
		{name: "partial bundle", answerTime: 3 * time.Minute},
		{name: "answer before last question", addLast: true, answerTime: time.Minute},
		{name: "answer after all questions", addLast: true, answerTime: 3 * time.Minute, wantRows: 2},
		{name: "legacy missing indexes", addLast: true, answerTime: 3 * time.Minute, removeIndex: true, wantRows: 2},
		{name: "legacy missing total", answerTime: 3 * time.Minute, removeTotal: true, wantRows: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, base := newLateReceiptFixture(t)
			if tc.addLast {
				insertClarificationMessage(t, repo, "receipt-question-2", "receipt-session", "receipt-task",
					"receipt-old", "pending-1", "question-2", "pending", 1, base.Add(90*time.Second))
			}
			if _, err := repo.db.Exec(`UPDATE task_session_messages
				SET metadata=json_set(metadata,'$.question_total',2)
				WHERE id IN ('receipt-question','receipt-question-2')`); err != nil {
				t.Fatal(err)
			}
			if tc.removeIndex {
				if _, err := repo.db.Exec(`UPDATE task_session_messages SET metadata=json_remove(metadata,'$.question_index') WHERE id IN ('receipt-question','receipt-question-2')`); err != nil {
					t.Fatal(err)
				}
			}
			if tc.removeTotal {
				if _, err := repo.db.Exec(`UPDATE task_session_messages SET metadata=json_remove(metadata,'$.question_total') WHERE id='receipt-question'`); err != nil {
					t.Fatal(err)
				}
			}
			page, err := repo.ListUnansweredClarificationBundles(context.Background(), models.ListUnansweredClarificationsOptions{
				WorkspaceIDs: []string{"receipt-ws"}, Limit: 10, Now: base.Add(4 * time.Minute),
			})
			if err != nil || len(page.Bundles) != 1 || page.Bundles[0].PendingID != "pending-1" {
				t.Fatalf("unanswered list: page=%+v err=%v", page, err)
			}
			id := "late:pending-1:" + lateReceiptUUID
			if _, err := repo.db.Exec(`INSERT INTO task_session_messages
				(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
				VALUES (?, 'receipt-session', 'receipt-task', 'receipt-new', 'user', '', 'answer', 0, 'message', '{}', ?)`,
				id, base.Add(tc.answerTime)); err != nil {
				t.Fatal(err)
			}
			rows, err := repo.CloseUnansweredClarificationBundle(context.Background(), "pending-1", models.CloseUnansweredClarification{
				Reason: models.ClarificationClosedAnsweredLate, At: base.Add(4 * time.Minute),
				SessionID: "receipt-session", SourceSessionID: "receipt-session", MessageID: id,
			})
			if tc.wantRows > 0 && (err != nil || len(rows) != tc.wantRows) {
				t.Fatalf("complete answered bundle: rows=%d err=%v", len(rows), err)
			}
			if tc.wantRows == 0 && (err != models.ErrUnansweredAdmissionUnknown || len(rows) != 0) {
				t.Fatalf("incomplete or early answer: rows=%d err=%v", len(rows), err)
			}
		})
	}
}

func TestLateAnswerMergedQueueChecksEachAdmissionTime(t *testing.T) {
	ctx := context.Background()
	repo, base := newLateReceiptFixture(t)
	baseQueue, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := baseQueue.ResolveSessionIdentity(ctx, "receipt-task", "receipt-session")
	if err != nil {
		t.Fatal(err)
	}
	queue, ok := baseQueue.(interface {
		AdmitQueueMessage(context.Context, messagequeue.QueueSessionIdentity, string, *messagequeue.QueuedMessage, *messagequeue.QueueAttachmentClaim, int, *messagequeue.AutoMergePolicy, *messagequeue.WorkflowEntryIdentity) (*messagequeue.QueuedMessage, bool, error)
		AutoMergeIntoAbove(context.Context, string, string) (*messagequeue.QueuedMessage, bool, error)
	})
	if !ok {
		t.Fatal("identified queue admission is unavailable")
	}
	ids := []string{"late:pending-1:" + lateReceiptUUID, "late:pending-1:550e8400-e29b-41d4-a716-446655440001"}
	for _, id := range ids {
		message := &messagequeue.QueuedMessage{
			TaskID: "receipt-task", SessionID: "receipt-session", Content: id,
			QueuedBy: messagequeue.QueuedByUser, Metadata: map[string]interface{}{messagequeue.MetadataQueueAdmissionIDs: []string{id}},
		}
		if _, replay, err := queue.AdmitQueueMessage(ctx, identity, id, message, nil, 0, nil, nil); err != nil || replay {
			t.Fatalf("first admission: replay=%v err=%v", replay, err)
		}
		if _, replay, err := queue.AdmitQueueMessage(ctx, identity, id, message, nil, 0, nil, nil); err != nil || !replay {
			t.Fatalf("retry: replay=%v err=%v", replay, err)
		}
	}
	if _, err := repo.db.Exec("UPDATE queue_admission_receipts SET admitted_at = ? WHERE client_queue_id = ?", base.Add(-time.Minute), ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, merged, err := queue.AutoMergeIntoAbove(ctx, "receipt-session", ids[1]); err != nil || !merged {
		t.Fatalf("merge: merged=%v err=%v", merged, err)
	}
	request := models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedAnsweredLate, At: time.Now().UTC(),
		SessionID: "receipt-session", SourceSessionID: "receipt-session", MessageID: ids[0],
	}
	if rows, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-1", request); err != models.ErrUnansweredAdmissionUnknown || len(rows) != 0 {
		t.Fatalf("older merged ID: rows=%d err=%v", len(rows), err)
	}
	request.MessageID = ids[1]
	if rows, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-1", request); err != nil || len(rows) != 1 {
		t.Fatalf("newer merged ID: rows=%d err=%v", len(rows), err)
	}
}

func TestUnansweredCloseUsesAuthorizedSourceSession(t *testing.T) {
	ctx := context.Background()
	repo, base := newLateReceiptFixture(t)
	seedBundleSession(t, repo, "other-session", "receipt-task")
	seedBundleTurnAt(t, repo, "other-old", "other-session", "receipt-task", base)
	insertClarificationMessage(t, repo, "other-question", "other-session", "receipt-task",
		"other-old", "pending-1", "question-1", "pending", 0, base)
	seedBundleTurnAt(t, repo, "other-new", "other-session", "receipt-task", base.Add(time.Minute))
	rows, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-1", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedByUser, At: base.Add(3 * time.Minute), SourceSessionID: "other-session",
	})
	if err != nil || len(rows) != 1 || rows[0].TaskSessionID != "other-session" {
		t.Fatalf("manual close scope: rows=%+v err=%v", rows, err)
	}
	rows, err = repo.CloseUnansweredClarificationBundle(ctx, "pending-1", models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedByUser, At: base.Add(4 * time.Minute), SourceSessionID: "receipt-session",
	})
	if err != nil || len(rows) != 1 || rows[0].TaskSessionID != "receipt-session" {
		t.Fatalf("other question must remain open: rows=%+v err=%v", rows, err)
	}
}

func TestLateAnswerUsesAuthorizedSourceSession(t *testing.T) {
	ctx := context.Background()
	repo, base := newLateReceiptFixture(t)
	seedBundleSession(t, repo, "other-session", "receipt-task")
	seedBundleTurnAt(t, repo, "other-old", "other-session", "receipt-task", base)
	insertClarificationMessage(t, repo, "other-question", "other-session", "receipt-task",
		"other-old", "pending-1", "question-1", "pending", 0, base.Add(4*time.Minute))
	seedBundleTurnAt(t, repo, "other-new", "other-session", "receipt-task", base.Add(5*time.Minute))
	id := "late:pending-1:" + lateReceiptUUID
	if _, err := repo.db.Exec(`INSERT INTO task_session_messages
		(id, task_session_id, task_id, turn_id, author_type, author_id, content, requests_input, type, metadata, created_at)
		VALUES (?, 'receipt-session', 'receipt-task', 'receipt-new', 'user', '', 'answer', 0, 'message', '{}', ?)`,
		id, base.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	request := models.CloseUnansweredClarification{
		Reason: models.ClarificationClosedAnsweredLate, At: base.Add(6 * time.Minute),
		SessionID: "receipt-session", SourceSessionID: "other-session", MessageID: id,
	}
	if rows, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-1", request); err != models.ErrUnansweredAdmissionUnknown || len(rows) != 0 {
		t.Fatalf("later question must reject earlier answer: rows=%d err=%v", len(rows), err)
	}
	request.SourceSessionID = "receipt-session"
	rows, err := repo.CloseUnansweredClarificationBundle(ctx, "pending-1", request)
	if err != nil || len(rows) != 1 || rows[0].TaskSessionID != "receipt-session" {
		t.Fatalf("authorized earlier question: rows=%+v err=%v", rows, err)
	}
}
