package questions_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newQuestionsRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "async-questions.db")
	conn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	repo, err := sqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	t.Cleanup(func() { _ = sqlxDB.Close() })
	return repo
}

func newQuestionsService(t *testing.T) (*questions.Service, *sqlite.Repository) {
	t.Helper()
	repo := newQuestionsRepo(t)
	service := questions.NewService(repo, nil, zap.NewNop())
	service.SetTranscriptWriter(testTranscriptWriter{})
	return service, repo
}

type testTranscriptWriter struct{}

func (testTranscriptWriter) WriteAsyncQuestionTranscriptEntry(
	context.Context,
	*models.AsyncQuestionDelivery,
	*models.AsyncQuestion,
) error {
	return nil
}

// seedQuestionBinding creates the durable task/session rows the authority
// check reads. It returns the workspace ID used by the question.
func seedQuestionBinding(t *testing.T, repo *sqlite.Repository, sessionID, parentTaskID string) string {
	t.Helper()
	workspaceID := "workspace-" + sessionID
	seedExplicitQuestionBinding(t, repo, "task-"+sessionID, sessionID, workspaceID, parentTaskID)
	return workspaceID
}

func seedExplicitQuestionBinding(t *testing.T, repo *sqlite.Repository, taskID, sessionID, workspaceID, parentTaskID string) {
	t.Helper()
	ctx := context.Background()
	workflowID := "workflow-" + sessionID
	now := time.Now().UTC()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	task := &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: taskID,
		State: "TODO", ParentID: parentTaskID, Autopilot: parentTaskID != "",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	session := &models.TaskSession{
		ID: sessionID, TaskID: taskID, IsPrimary: true, State: models.TaskSessionStateRunning,
		QueueIncarnationID: "inc-" + sessionID, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("seed session: %v", err)
	}
}

func createOperatorQuestion(t *testing.T, svc *questions.Service, repo *sqlite.Repository, sessionID string) *questions.CreateResult {
	t.Helper()
	workspaceID := seedQuestionBinding(t, repo, sessionID, "")
	result, err := svc.Create(context.Background(), questions.CreateInput{
		TaskID:               "task-" + sessionID,
		WorkspaceID:          workspaceID,
		SessionID:            sessionID,
		SessionIncarnationID: "inc-" + sessionID,
		Recipient:            models.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			Prompt:  "Which value belongs in the field?",
			Options: []protocol.Option{{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"}},
		}},
		Context:   "API integration",
		RequestID: "request-" + sessionID,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	return result
}

type questionChangeRecorder struct {
	changes []questions.QuestionChange
}

func (r *questionChangeRecorder) AsyncQuestionChanged(_ context.Context, change questions.QuestionChange) {
	r.changes = append(r.changes, change)
}

func TestAsyncQuestionNotificationOncePerRequest(t *testing.T) {
	svc, repo := newQuestionsService(t)
	recorder := &questionChangeRecorder{}
	svc.SetNotifier(recorder)
	created := createOperatorQuestion(t, svc, repo, "session-notify-once")

	replayed, err := svc.Create(context.Background(), questions.CreateInput{
		TaskID: "task-session-notify-once", WorkspaceID: "workspace-session-notify-once",
		SessionID: "session-notify-once", SessionIncarnationID: "inc-session-notify-once",
		Recipient: models.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			Prompt: "Which value belongs in the field?",
			Options: []protocol.Option{
				{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"},
			},
		}},
		Context: "API integration", RequestID: "request-session-notify-once",
	})
	require.NoError(t, err)
	require.False(t, replayed.Created)
	require.Equal(t, created.Question.ID, replayed.Question.ID)
	require.Len(t, recorder.changes, 1)
	require.Equal(t, "created", recorder.changes[0].Kind)
	require.Equal(t, created.Question.ID, recorder.changes[0].QuestionID)
	require.Equal(t, models.AsyncQuestionRecipientUser, recorder.changes[0].Recipient)
}

func TestAsyncQuestionsTranscriptEntryIsIdempotent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	workspaceID := seedQuestionBinding(t, repo, "session-transcript", "")
	input := questions.CreateInput{
		TaskID:               "task-session-transcript",
		WorkspaceID:          workspaceID,
		SessionID:            "session-transcript",
		SessionIncarnationID: "inc-session-transcript",
		Recipient:            models.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			ID: "question-choice", Prompt: "Which value belongs in the field?",
			Options: []protocol.Option{{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"}},
		}},
		RequestID: "transcript-request",
	}
	created, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	replayed, err := svc.Create(ctx, input)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Question.ID != created.Question.ID || replayed.Created {
		t.Fatalf("replay = %+v, want original question %s", replayed, created.Question.ID)
	}
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Second), 20)
	if err != nil {
		t.Fatalf("list transcript intents: %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].Kind != models.AsyncQuestionDeliveryKind("transcript") {
		t.Fatalf("transcript intents = %+v, want one idempotent transcript intent", deliveries)
	}
}

func TestAsyncQuestionsOperatorAskKeepsTurnRunning(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-operator")
	if !result.Created || result.Question.ID == "" {
		t.Fatalf("expected a durable receipt: %+v", result)
	}
	if result.Question.Status != models.AsyncQuestionStatusPending {
		t.Fatalf("status = %s", result.Question.Status)
	}
	// The receipt is immediate: no answer exists yet and no waiter blocks.
	if result.Question.Outcome != nil {
		t.Fatal("creation must not record an answer")
	}
	replayed, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-session-operator", WorkspaceID: "workspace-1", SessionID: "session-operator",
		Recipient: models.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{
			Prompt:  "Which value belongs in the field?",
			Options: []protocol.Option{{ID: "default", Label: "Default"}, {ID: "empty", Label: "Empty"}},
		}},
		Context: "API integration", RequestID: "request-session-operator",
	})
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replayed.Created || replayed.Question.ID != result.Question.ID {
		t.Fatalf("exact retry must return the original request")
	}
}

func TestAsyncQuestionsOperatorCheckpointRead(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-checkpoint")

	updated, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"default"}, CustomText: " use the documented default "}},
	})
	if err != nil || !won {
		t.Fatalf("resolve: won=%v err=%v", won, err)
	}
	if updated.Outcome == nil || updated.Outcome.Answers[0].SelectedOptions[0] != "default" {
		t.Fatalf("outcome must preserve item and option identity: %+v", updated.Outcome)
	}
	if updated.Outcome.Answers[0].CustomText != "use the documented default" {
		t.Fatalf("custom text must be preserved and trimmed: %q", updated.Outcome.Answers[0].CustomText)
	}

	page, err := svc.ListOwn(ctx, "session-checkpoint", nil, nil, 50, "", "")
	if err != nil {
		t.Fatalf("list own: %v", err)
	}
	if len(page.Questions) != 1 || page.Questions[0].Status != models.AsyncQuestionStatusAnswered {
		t.Fatalf("checkpoint read must expose the answer: %+v", page.Questions)
	}
	// Reads never consume the answer.
	again, err := svc.ListOwn(ctx, "session-checkpoint", nil, nil, 50, "", "")
	if err != nil || len(again.Questions) != 1 || again.Questions[0].Outcome == nil {
		t.Fatalf("repeated read must expose the same answer: %v", err)
	}

	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	var ownerDelivery *models.AsyncQuestionDelivery
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryOwner {
			ownerDelivery = delivery
		}
	}
	if ownerDelivery == nil {
		t.Fatalf("expected one correlated continuation intent, got %+v", deliveries)
	}
	if !strings.Contains(ownerDelivery.Content, "Which value belongs in the field?") ||
		!strings.Contains(ownerDelivery.Content, "Default") {
		t.Fatalf("correlated answer must carry the original questions and labels: %s", ownerDelivery.Content)
	}
}

func TestAsyncQuestionsRejectionDistinctFromAnswer(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-reject")
	updated, _, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Rejected: true, RejectReason: "not needed",
	})
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if updated.Status != models.AsyncQuestionStatusRejected || updated.Outcome == nil || !updated.Outcome.Rejected {
		t.Fatalf("rejection must be a distinct terminal outcome: %+v", updated)
	}
	if updated.Status == models.AsyncQuestionStatusAnswered {
		t.Fatal("rejection must not be recorded as an affirmative answer")
	}
}

func TestAsyncQuestionsWithdrawWinner(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-withdraw-svc")
	withdrawn, err := svc.Withdraw(ctx, result.Question.ID, "session-withdraw-svc", "documentation supplied the value")
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if withdrawn.Status != models.AsyncQuestionStatusWithdrawn || withdrawn.Outcome != nil {
		t.Fatalf("unexpected withdrawal: %+v", withdrawn)
	}
	answered, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	})
	if err != nil {
		t.Fatalf("late answer: %v", err)
	}
	if won || answered.Status != models.AsyncQuestionStatusWithdrawn {
		t.Fatal("a later answer must not reopen a withdrawn request")
	}
	if _, err := svc.Withdraw(ctx, result.Question.ID, "other-session", "again"); err == nil {
		t.Fatal("a foreign session must not withdraw")
	}
}

func TestAsyncQuestionsWaitReadyAndGate(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	first := createOperatorQuestion(t, svc, repo, "session-wait")

	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-wait", SessionID: "session-wait",
		Dependencies: []string{first.Question.ID}, Reason: "remaining work needs the decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if waitResult.Status != questions.WaitStatusWaiting || waitResult.Wait == nil {
		t.Fatalf("expected a requested gate: %+v", waitResult)
	}
	require.True(t, svc.HasParkedWait(ctx, "session-wait"),
		"a requested gate fails closed until the pause settles or the answer releases it")
	if err := svc.MarkWaitParked(ctx, waitResult.Wait.ID); err != nil {
		t.Fatalf("mark parked: %v", err)
	}
	if !svc.HasParkedWait(ctx, "session-wait") {
		t.Fatal("expected a parked gate")
	}
	if _, _, err := svc.ResolveOperator(ctx, first.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: first.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if svc.HasParkedWait(ctx, "session-wait") {
		t.Fatal("a terminal dependency must release the parked gate")
	}
}

func TestAsyncQuestionsWaitReadyWithoutPausing(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-ready")
	if _, _, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-ready", SessionID: "session-ready",
		Dependencies: []string{result.Question.ID}, Reason: "remaining work needs the decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if waitResult.Status != questions.WaitStatusReady || waitResult.Wait != nil {
		t.Fatalf("a resolved dependency must return ready without a gate: %+v", waitResult)
	}
	if len(waitResult.Questions) != 1 || waitResult.Questions[0].Outcome == nil {
		t.Fatal("ready must include the terminal snapshot")
	}
}

func TestAsyncQuestionsWaitRequiresOwnDependencies(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	foreign := createOperatorQuestion(t, svc, repo, "session-other")
	if _, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-mine", SessionID: "session-mine",
		Dependencies: []string{foreign.Question.ID}, Reason: "not mine",
	}); err == nil {
		t.Fatal("wait must reject a dependency owned by another session")
	}
}

type recordingQueue struct {
	mu         sync.Mutex
	deliveries []*models.AsyncQuestionDelivery
	removed    []*models.AsyncQuestionDelivery
	admitErr   error
}

type recordingTranscriptWriter struct {
	calls      int
	questionID string
}

func (w *recordingTranscriptWriter) WriteAsyncQuestionTranscriptEntry(
	_ context.Context,
	_ *models.AsyncQuestionDelivery,
	question *models.AsyncQuestion,
) error {
	w.calls++
	w.questionID = question.ID
	return nil
}

func TestAsyncQuestionsTranscriptDeliveryIsReconciledOnce(t *testing.T) {
	svc, repo := newQuestionsService(t)
	queue := &recordingQueue{}
	writer := &recordingTranscriptWriter{}
	svc.SetDeliveryQueue(queue)
	svc.SetTranscriptWriter(writer)
	result := createOperatorQuestion(t, svc, repo, "session-transcript-reconcile")
	for range 2 {
		if _, err := svc.ReconcileOnce(context.Background()); err != nil {
			t.Fatalf("reconcile transcript: %v", err)
		}
	}
	if writer.calls != 1 || writer.questionID != result.Question.ID {
		t.Fatalf("transcript writes = %d for %q, want one for %q", writer.calls, writer.questionID, result.Question.ID)
	}
	if len(queue.deliveries) != 0 {
		t.Fatalf("transcript delivery reached the agent queue: %+v", queue.deliveries)
	}
}

func (q *recordingQueue) AdmitAsyncQuestionDelivery(_ context.Context, delivery *models.AsyncQuestionDelivery) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.admitErr != nil {
		return q.admitErr
	}
	q.deliveries = append(q.deliveries, delivery)
	return nil
}

func (q *recordingQueue) RemoveAsyncQuestionDelivery(_ context.Context, delivery *models.AsyncQuestionDelivery) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.removed = append(q.removed, delivery)
	remaining := q.deliveries[:0]
	for _, queued := range q.deliveries {
		if queued.AdmissionID != delivery.AdmissionID {
			remaining = append(remaining, queued)
		}
	}
	q.deliveries = remaining
	return nil
}

func TestAsyncQuestionsDeliveryAdmissionReplay(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	queue := &recordingQueue{}
	svc.SetDeliveryQueue(queue)

	result := createOperatorQuestion(t, svc, repo, "session-delivery")
	if _, _, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"empty"}}},
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	admitted, err := svc.ReconcileOnce(ctx)
	if err != nil || admitted != 2 {
		t.Fatalf("reconcile should admit the transcript and answer: admitted=%d err=%v", admitted, err)
	}
	// A second pass must not admit the same outcome again.
	admitted, err = svc.ReconcileOnce(ctx)
	if err != nil || admitted != 0 {
		t.Fatalf("second reconcile: admitted=%d err=%v", admitted, err)
	}
	if len(queue.deliveries) != 1 || queue.deliveries[0].QuestionID != result.Question.ID {
		t.Fatalf("unexpected admissions: %+v", queue.deliveries)
	}
}

func TestAsyncQuestionsForwardNeedsRecordedParent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child", "session-child", "workspace-session-child", "task-parent")
	seedExplicitQuestionBinding(t, repo, "task-parent", "session-parent", "workspace-session-parent", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-child", WorkspaceID: "workspace-session-child", SessionID: "session-child",
		SessionIncarnationID: "inc-session-child",
		Recipient:            models.AsyncQuestionRecipientParent, ParentTaskID: "task-parent", ParentSession: "session-parent",
		ParentSessionIncarnationID: "inc-session-parent",
		Questions: []protocol.Question{{
			Prompt: "Which API shape?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		}},
		RequestID: "parent-request",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.Forward(ctx, result.Question.ID, "task-other-parent", "session-other", "operator decision"); err == nil {
		t.Fatal("forward must reject an unrelated parent")
	}
	forwarded, err := svc.Forward(ctx, result.Question.ID, "task-parent", "session-parent", "operator decision")
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if !forwarded.Forwarded || forwarded.ID != result.Question.ID {
		t.Fatalf("forward must preserve identity: %+v", forwarded)
	}
	createdDeliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	parentQuestionCount := 0
	for _, delivery := range createdDeliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryParentQuestion {
			parentQuestionCount++
		}
	}
	if parentQuestionCount != 1 {
		t.Fatalf("creation should produce one parent-question intent, got %+v", createdDeliveries)
	}
	if _, won, err := svc.ResolveParent(ctx, result.Question.ID, "task-parent", "session-parent", questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	}); err != nil || !won {
		t.Fatalf("parent answer: won=%v err=%v", won, err)
	}
	if _, won, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"b"}}},
	}); err != nil || won {
		t.Fatalf("operator answer must lose to the parent winner: won=%v err=%v", won, err)
	}
	allDeliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	kinds := map[models.AsyncQuestionDeliveryKind]int{}
	for _, delivery := range allDeliveries {
		kinds[delivery.Kind]++
	}
	if kinds[models.AsyncQuestionDeliveryParentQuestion] != 1 ||
		kinds[models.AsyncQuestionDeliveryParentOutcome] != 1 ||
		kinds[models.AsyncQuestionDeliveryOwner] != 1 {
		t.Fatalf("expected distinct parent question/outcome and owner intents, got %+v", kinds)
	}
}

func TestAsyncQuestionParentDeliveryWaitsForSession(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-waiting-child", "session-waiting-child", "workspace-waiting-child", "task-waiting-parent")
	seedExplicitQuestionBinding(t, repo, "task-waiting-parent", "session-original-parent", "workspace-waiting-parent", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-waiting-child", WorkspaceID: "workspace-waiting-child",
		SessionID: "session-waiting-child", SessionIncarnationID: "inc-session-waiting-child",
		Recipient: models.AsyncQuestionRecipientParent, ParentTaskID: "task-waiting-parent",
		ParentSession: "session-original-parent", ParentSessionIncarnationID: "inc-session-original-parent",
		Questions: []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID: "parent-delivery-waits",
	})
	require.NoError(t, err)
	queue := &recordingQueue{admitErr: questions.ErrDestinationUnavailable}
	svc.SetDeliveryQueue(queue)

	admitted, err := svc.ReconcileOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, admitted, "the transcript intent is independent of parent session readiness")
	deliveries, err := repo.ListAsyncQuestionDeliveriesForQuestion(ctx, result.Question.ID, models.AsyncQuestionDeliveryParentQuestion)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	require.Equal(t, models.AsyncQuestionDeliveryPending, deliveries[0].State)

	require.NoError(t, repo.MarkAsyncQuestionDeliveryFailed(ctx, deliveries[0].AdmissionID, 1, time.Now().UTC().Add(-time.Second), "parent has no active session"))
	queue.admitErr = nil
	admitted, err = svc.ReconcileOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, admitted)
	admittedDeliveries, err := repo.ListAsyncQuestionDeliveriesForQuestion(ctx, result.Question.ID, models.AsyncQuestionDeliveryParentQuestion)
	require.NoError(t, err)
	require.Equal(t, models.AsyncQuestionDeliveryAdmitted, admittedDeliveries[0].State)
}

func TestAsyncQuestionsReplacementParentSessionAnswers(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-replacement-child", "session-replacement-child", "workspace-replacement-child", "task-replacement-parent")
	seedExplicitQuestionBinding(t, repo, "task-replacement-parent", "session-original-parent", "workspace-replacement-parent", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-replacement-child", WorkspaceID: "workspace-replacement-child",
		SessionID: "session-replacement-child", SessionIncarnationID: "inc-session-replacement-child",
		Recipient: models.AsyncQuestionRecipientParent, ParentTaskID: "task-replacement-parent",
		ParentSession: "session-original-parent", ParentSessionIncarnationID: "inc-session-original-parent",
		Questions: []protocol.Question{{
			Prompt: "Which value?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}},
		}},
		RequestID: "replacement-parent-session",
	})
	require.NoError(t, err)

	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-replacement-parent", TaskID: "task-replacement-parent",
		State: models.TaskSessionStateRunning, QueueIncarnationID: "inc-session-replacement-parent",
		StartedAt: now, UpdatedAt: now,
	}))
	updated, won, err := svc.ResolveParent(ctx, result.Question.ID, "task-replacement-parent", "session-replacement-parent", questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	})
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, models.AsyncQuestionStatusAnswered, updated.Status)
}

func TestAsyncQuestionsWaitAnswerRaceDoesNotStrand(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-wait-race")

	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-wait-race", SessionID: "session-wait-race",
		Dependencies: []string{result.Question.ID}, Reason: "remaining work needs the decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if waitResult.Status != questions.WaitStatusWaiting {
		t.Fatalf("expected a requested gate, got %s", waitResult.Status)
	}
	// The answer lands before the pause settles. Marking the gate parked must
	// observe the terminal dependency and release it instead of stranding it.
	if _, _, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	}); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if err := svc.MarkWaitParked(ctx, waitResult.Wait.ID); err != nil {
		t.Fatalf("mark parked: %v", err)
	}
	if svc.HasParkedWait(ctx, "session-wait-race") {
		t.Fatal("a gate released by a racing answer must not remain active")
	}
}

type fixedSessionState struct {
	state    string
	archived bool
	found    bool
}

func (f fixedSessionState) AsyncQuestionSessionState(context.Context, string) (string, bool, bool, error) {
	return f.state, f.archived, f.found, nil
}

func TestAsyncQuestionsInboxExcludesTerminalDestinations(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	createOperatorQuestion(t, svc, repo, "session-terminal")
	svc.SetSessionStateReader(fixedSessionState{state: "CANCELLED", found: true})
	if _, err := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET state = 'CANCELLED' WHERE id = ?", "session-terminal"); err != nil {
		t.Fatalf("cancel session: %v", err)
	}

	page, err := svc.ListInbox(ctx, "workspace-session-terminal", 50, "", "")
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(page.Questions) != 0 {
		t.Fatalf("a cancelled session's request must not remain actionable: %+v", page.Questions)
	}
	count, err := svc.CountInbox(ctx, "workspace-session-terminal")
	if err != nil || count != 0 {
		t.Fatalf("count = %d err = %v, want 0", count, err)
	}

	svc.SetSessionStateReader(fixedSessionState{state: "RUNNING", found: true})
	if _, err := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET state = 'RUNNING' WHERE id = ?", "session-terminal"); err != nil {
		t.Fatalf("restore session: %v", err)
	}
	count, err = svc.CountInbox(ctx, "workspace-session-terminal")
	if err != nil || count != 1 {
		t.Fatalf("count = %d err = %v, want 1", count, err)
	}
}

func TestAsyncQuestionsWaitRestartReconciliation(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-restart-wait")
	wakeCalls := 0
	svc.SetWakeDestination(func(context.Context, string, string) { wakeCalls++ })
	svc.SetSessionStateReader(fixedSessionState{state: "WAITING_FOR_INPUT", found: true})

	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-restart-wait", SessionID: "session-restart-wait",
		Dependencies: []string{result.Question.ID}, Reason: "dependency",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := svc.ReconcileWaitGates(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if svc.HasParkedWait(ctx, "session-restart-wait") {
		t.Fatal("a requested gate without a surviving pause must not strand completion")
	}
	_ = waitResult

	// A release requested before restart settles to released and wakes once.
	second := createOperatorQuestion(t, svc, repo, "session-restart-release")
	secondWait, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-restart-release", SessionID: "session-restart-release",
		Dependencies: []string{second.Question.ID}, Reason: "dependency",
	})
	if err != nil {
		t.Fatalf("wait second: %v", err)
	}
	if _, _, err := svc.ResolveOperator(ctx, second.Question.ID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: second.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
	}); err != nil {
		t.Fatalf("resolve second: %v", err)
	}
	if err := svc.ReconcileWaitGates(ctx); err != nil {
		t.Fatalf("reconcile second: %v", err)
	}
	if wakeCalls != 1 {
		t.Fatalf("wake calls = %d, want 1", wakeCalls)
	}
	_ = secondWait
}

func TestAsyncQuestionsWithdrawWakeIsAtomic(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	question := createOperatorQuestion(t, svc, repo, "session-withdraw-wake")
	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-withdraw-wake", SessionID: "session-withdraw-wake",
		Dependencies: []string{question.Question.ID}, Reason: "dependency",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := svc.MarkWaitParked(ctx, waitResult.Wait.ID); err != nil {
		t.Fatalf("park: %v", err)
	}
	if _, err := svc.Withdraw(ctx, question.Question.ID, "session-withdraw-wake", "no longer needed"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	wakes := 0
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryWake {
			wakes++
			if delivery.OutcomeVersion != 2 {
				t.Fatalf("wake version = %d, want the winning version 2", delivery.OutcomeVersion)
			}
		}
	}
	if wakes != 1 {
		t.Fatalf("wake intents = %d, want exactly 1 committed atomically", wakes)
	}
	if svc.HasParkedWait(ctx, "session-withdraw-wake") {
		t.Fatal("withdrawal must release the parked gate")
	}
}
