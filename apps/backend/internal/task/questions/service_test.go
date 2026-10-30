package questions_test

import (
	"context"
	"errors"
	"path/filepath"
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
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
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
	return questions.NewService(repo, nil, zap.NewNop()), repo
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
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("expected one correlated continuation intent, got %d err=%v", len(deliveries), err)
	}
	if deliveries[0].Kind != models.AsyncQuestionDeliveryOwner {
		t.Fatalf("kind = %s", deliveries[0].Kind)
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
}

func (q *recordingQueue) AdmitAsyncQuestionDelivery(_ context.Context, delivery *models.AsyncQuestionDelivery) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.deliveries = append(q.deliveries, delivery)
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
	if err != nil || admitted != 1 {
		t.Fatalf("reconcile: admitted=%d err=%v", admitted, err)
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
	if err != nil || len(createdDeliveries) != 1 {
		t.Fatalf("creation should produce one parent-question intent, got %d err=%v", len(createdDeliveries), err)
	}
	if createdDeliveries[0].Kind != models.AsyncQuestionDeliveryParentQuestion {
		t.Fatalf("creation intent kind = %s", createdDeliveries[0].Kind)
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

func TestAsyncQuestionsChildrenAndHistoryPagination(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child-page", "session-child-page", "workspace-session-child-page", "task-parent-page")
	seedExplicitQuestionBinding(t, repo, "task-parent-page", "session-parent-page", "workspace-session-parent-page", "")
	ids := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		result, err := svc.Create(ctx, questions.CreateInput{
			TaskID: "task-child-page", WorkspaceID: "workspace-session-child-page", SessionID: "session-child-page",
			SessionIncarnationID: "inc-session-child-page",
			Recipient:            models.AsyncQuestionRecipientParent, ParentTaskID: "task-parent-page", ParentSession: "session-parent-page",
			ParentSessionIncarnationID: "inc-session-parent-page",
			Questions:                  []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
			RequestID:                  "child-page-" + string(rune('a'+i)),
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		ids = append(ids, result.Question.ID)
	}
	first, err := svc.ListChildren(ctx, "task-parent-page", nil, nil, 2, "", "")
	if err != nil {
		t.Fatalf("children page 1: %v", err)
	}
	if len(first.Questions) != 2 || !first.HasMore {
		t.Fatalf("children page 1 = %d hasMore=%v", len(first.Questions), first.HasMore)
	}
	last := first.Questions[len(first.Questions)-1]
	second, err := svc.ListChildren(ctx, "task-parent-page", nil, nil, 2,
		last.CreatedAt.UTC().Format(time.RFC3339Nano), last.ID)
	if err != nil {
		t.Fatalf("children page 2: %v", err)
	}
	if len(second.Questions) != 1 || second.HasMore {
		t.Fatalf("children page 2 = %d hasMore=%v", len(second.Questions), second.HasMore)
	}
	if second.Questions[0].ID == last.ID {
		t.Fatal("child cursor repeated the first page")
	}

	for _, id := range ids {
		question, _ := svc.Get(ctx, id)
		if _, err := svc.Forward(ctx, id, "task-parent-page", "session-parent-page", "operator decision"); err != nil {
			t.Fatalf("forward %s: %v", id, err)
		}
		if _, _, err := svc.ResolveOperator(ctx, id, questions.AnswerInput{
			Answers: []questions.Answer{{QuestionID: question.Questions[0].ID, SelectedOptions: []string{"a"}}},
		}); err != nil {
			t.Fatalf("resolve %s: %v", id, err)
		}
	}
	history, err := svc.ListInboxHistory(ctx, "workspace-session-child-page", 2, "", "")
	if err != nil || len(history.Questions) != 2 || !history.HasMore {
		t.Fatalf("history page 1 = %d hasMore=%v err=%v", len(history.Questions), history.HasMore, err)
	}
	lastHistory := history.Questions[len(history.Questions)-1]
	history2, err := svc.ListInboxHistory(ctx, "workspace-session-child-page", 2,
		lastHistory.CreatedAt.UTC().Format(time.RFC3339Nano), lastHistory.ID)
	if err != nil || len(history2.Questions) != 1 {
		t.Fatalf("history page 2 = %d err=%v", len(history2.Questions), err)
	}
}

func TestAsyncQuestionsAuthorityInvalidatedByReparent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child-auth", "session-child-auth", "workspace-session-child-auth", "task-parent-auth")
	seedExplicitQuestionBinding(t, repo, "task-parent-auth", "session-parent-auth", "workspace-session-parent-auth", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-child-auth", WorkspaceID: "workspace-session-child-auth", SessionID: "session-child-auth",
		SessionIncarnationID: "inc-session-child-auth",
		Recipient:            models.AsyncQuestionRecipientParent, ParentTaskID: "task-parent-auth", ParentSession: "session-parent-auth",
		ParentSessionIncarnationID: "inc-session-parent-auth",
		Questions:                  []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID:                  "authority-reparent",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	child, err := repo.GetTask(ctx, "task-child-auth")
	if err != nil {
		t.Fatalf("get child: %v", err)
	}
	newParent := "task-parent-other"
	child.ParentID = newParent
	allowParent := func(context.Context, hierarchy.TaskHierarchyReader, *models.Task, string) error { return nil }
	if _, err := repo.UpdateTaskWithParentAdmission(ctx, child, &newParent, true, allowParent); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	if _, _, err := svc.ResolveParent(ctx, result.Question.ID, "task-parent-auth", "session-parent-auth", questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	}); !errors.Is(err, questions.ErrUnauthorized) {
		t.Fatalf("reparented answer error = %v, want unauthorized", err)
	}
	stale, err := svc.Get(ctx, result.Question.ID)
	if err != nil || stale.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("stale status = %+v err=%v", stale, err)
	}
}

func TestAsyncQuestionsAuthorityInvalidatedByTransferCancelAndIncarnation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(t *testing.T, repo *sqlite.Repository)
	}{
		{name: "workspace transfer", mutate: func(t *testing.T, repo *sqlite.Repository) {
			task, err := repo.GetTask(context.Background(), "task-session-authority")
			if err != nil {
				t.Fatalf("get task: %v", err)
			}
			task.WorkspaceID = "workspace-other"
			if err := repo.UpdateTask(context.Background(), task); err != nil {
				t.Fatalf("transfer: %v", err)
			}
		}},
		{name: "terminal session", mutate: func(t *testing.T, repo *sqlite.Repository) {
			session, err := repo.GetTaskSession(context.Background(), "session-authority")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			session.State = models.TaskSessionStateCancelled
			if err := repo.UpdateTaskSession(context.Background(), session); err != nil {
				t.Fatalf("cancel: %v", err)
			}
		}},
		{name: "incarnation replacement", mutate: func(t *testing.T, repo *sqlite.Repository) {
			session, err := repo.GetTaskSession(context.Background(), "session-authority")
			if err != nil {
				t.Fatalf("get session: %v", err)
			}
			if _, err := repo.DB().ExecContext(context.Background(),
				"UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?",
				"inc-replacement", session.ID); err != nil {
				t.Fatalf("replace incarnation: %v", err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, repo := newQuestionsService(t)
			ctx := context.Background()
			result := createOperatorQuestion(t, svc, repo, "session-authority")
			test.mutate(t, repo)
			if _, _, err := svc.ResolveOperator(ctx, result.Question.ID, questions.AnswerInput{
				Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"default"}}},
			}); !errors.Is(err, questions.ErrUnauthorized) {
				t.Fatalf("answer error = %v, want unauthorized", err)
			}
			deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
			if err != nil {
				t.Fatalf("list deliveries: %v", err)
			}
			if len(deliveries) != 0 {
				t.Fatalf("stale answer must not enqueue: %+v", deliveries)
			}
		})
	}
}
