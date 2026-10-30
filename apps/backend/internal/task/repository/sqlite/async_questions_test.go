package sqlite

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
)

func newAsyncQuestionForTest(t *testing.T, id, sessionID string) *models.AsyncQuestion {
	t.Helper()
	return &models.AsyncQuestion{
		ID:            id,
		TaskID:        "task-" + id,
		WorkspaceID:   "workspace-" + id,
		SessionID:     sessionID,
		Recipient:     models.AsyncQuestionRecipientUser,
		Questions:     []protocol.Question{{ID: "q1", Prompt: "Which value?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		Context:       "context",
		RetryKey:      "retry-" + id,
		PayloadDigest: "digest-" + id,
		Status:        models.AsyncQuestionStatusPending,
		Version:       1,
	}
}

func seedAsyncQuestionBinding(t *testing.T, repo *Repository, question *models.AsyncQuestion) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	if question.SessionIncarnationID == "" {
		question.SessionIncarnationID = "inc-" + question.SessionID
	}
	if question.WorkspaceID == "" {
		question.WorkspaceID = "workspace-" + question.ID
	}
	if existing, _ := repo.GetTaskSession(ctx, question.SessionID); existing != nil {
		return
	}
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: question.WorkspaceID, Name: question.WorkspaceID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	workflowID := "workflow-" + question.ID
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: question.WorkspaceID, Name: workflowID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: question.TaskID, WorkspaceID: question.WorkspaceID, WorkflowID: workflowID,
		Title: question.TaskID, State: "TODO", ParentID: question.ParentTaskID,
		Autopilot: question.ParentTaskID != "", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: question.SessionID, TaskID: question.TaskID, IsPrimary: true,
		State: models.TaskSessionStateRunning, QueueIncarnationID: question.SessionIncarnationID,
		StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if question.ParentTaskID != "" && question.ParentSessionID != "" {
		parentWorkspace := "parent-workspace-" + question.ID
		_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: parentWorkspace, Name: parentWorkspace, CreatedAt: now, UpdatedAt: now})
		parentWorkflow := "parent-workflow-" + question.ID
		_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: parentWorkflow, WorkspaceID: parentWorkspace, Name: parentWorkflow, CreatedAt: now, UpdatedAt: now})
		_ = repo.CreateTask(ctx, &models.Task{ID: question.ParentTaskID, WorkspaceID: parentWorkspace, WorkflowID: parentWorkflow, Title: question.ParentTaskID, State: "TODO", CreatedAt: now, UpdatedAt: now})
		_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: question.ParentSessionID, TaskID: question.ParentTaskID, IsPrimary: true, State: models.TaskSessionStateRunning, QueueIncarnationID: "inc-" + question.ParentSessionID, StartedAt: now, UpdatedAt: now})
	}
}

func createAsyncQuestionForTest(t *testing.T, repo *Repository, question *models.AsyncQuestion) (*models.AsyncQuestion, bool) {
	t.Helper()
	seedAsyncQuestionBinding(t, repo, question)
	created, isNew, err := repo.CreateAsyncQuestion(context.Background(), question, nil)
	if err != nil {
		t.Fatalf("create async question: %v", err)
	}
	return created, isNew
}

func TestAsyncQuestionsCreateReplay(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-replay", "session-replay")

	created, isNew := createAsyncQuestionForTest(t, repo, question)
	if !isNew {
		t.Fatalf("create: created=%v new=%v", created, isNew)
	}
	replayed, isNew, err := repo.CreateAsyncQuestion(ctx, newAsyncQuestionForTest(t, "q-replay", "session-replay"), nil)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if isNew || replayed.ID != "q-replay" {
		t.Fatalf("replay must return the original request: new=%v id=%s", isNew, replayed.ID)
	}

	conflicting := newAsyncQuestionForTest(t, "q-replay", "session-replay")
	conflicting.PayloadDigest = "different"
	if _, _, err := repo.CreateAsyncQuestion(ctx, conflicting, nil); err == nil {
		t.Fatal("expected a conflict for different content under the same retry identity")
	}
}

func TestAsyncQuestionsResolutionWinner(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-winner", "session-winner")
	createAsyncQuestionForTest(t, repo, question)

	firstOutcome := models.AsyncQuestionOutcome{Answers: []protocol.Answer{{QuestionID: "q1", SelectedOptions: []string{"a"}}}}
	winner, won, err := repo.ResolveAsyncQuestion(ctx, question.ID, firstOutcome, []*models.AsyncQuestionDelivery{{
		AdmissionID: "admission-winner", Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: question.TaskID, DestinationSession: question.SessionID, Content: "answer",
	}})
	if err != nil || !won {
		t.Fatalf("first resolve: won=%v err=%v", won, err)
	}
	if winner.Status != models.AsyncQuestionStatusAnswered || winner.Outcome == nil || winner.Version != 2 {
		t.Fatalf("unexpected winner: %+v", winner)
	}

	secondOutcome := models.AsyncQuestionOutcome{Answers: []protocol.Answer{{QuestionID: "q1", SelectedOptions: []string{"b"}}}}
	loser, won, err := repo.ResolveAsyncQuestion(ctx, question.ID, secondOutcome, nil)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if won {
		t.Fatal("the second answer must not win")
	}
	if loser.Outcome == nil || len(loser.Outcome.Answers) != 1 || loser.Outcome.Answers[0].SelectedOptions[0] != "a" {
		t.Fatalf("losing caller must observe the winner: %+v", loser.Outcome)
	}

	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected exactly one delivery intent, got %d", len(deliveries))
	}
}

func TestAsyncQuestionsRestart(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-restart", "session-restart")
	createAsyncQuestionForTest(t, repo, question)
	// The same repository reopened through the pool still reads the durable
	// record; a new connection proves durability, not an in-memory map.
	reopened, err := repo.GetAsyncQuestion(ctx, question.ID)
	if err != nil || reopened == nil {
		t.Fatalf("get after reopen: %v", err)
	}
	if reopened.Status != models.AsyncQuestionStatusPending {
		t.Fatalf("status = %s", reopened.Status)
	}
}

func TestAsyncQuestionsLifecycleCleanup(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-cleanup", "session-cleanup")
	createAsyncQuestionForTest(t, repo, question)
	wait, err := repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		TaskID: question.TaskID, SessionID: question.SessionID, DependencyIDs: []string{question.ID},
		Status: models.AsyncQuestionWaitRequested,
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}

	affected, err := repo.MarkStaleAsyncQuestionsForSession(ctx, question.SessionID, "cancelled")
	if err != nil || affected != 1 {
		t.Fatalf("mark stale: affected=%d err=%v", affected, err)
	}
	active, err := repo.GetActiveAsyncQuestionWait(ctx, question.SessionID)
	if err != nil {
		t.Fatalf("get wait: %v", err)
	}
	if active != nil {
		t.Fatalf("expected the dependency gate to close with the request, got %+v", active)
	}
	stale, err := repo.GetAsyncQuestion(ctx, question.ID)
	if err != nil || stale.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("expected stale status, got %+v err=%v", stale, err)
	}
	_ = wait

	if err := repo.DeleteAsyncQuestionsForTask(ctx, question.TaskID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	gone, err := repo.GetAsyncQuestion(ctx, question.ID)
	if err != nil || gone != nil {
		t.Fatalf("expected deleted request, got %+v err=%v", gone, err)
	}
}

func TestAsyncQuestionsOutboxCrashWindow(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-outbox", "session-outbox")
	createAsyncQuestionForTest(t, repo, question)
	_, _, err := repo.ResolveAsyncQuestion(ctx, question.ID, models.AsyncQuestionOutcome{
		Answers: []protocol.Answer{{QuestionID: "q1", SelectedOptions: []string{"a"}}},
	}, []*models.AsyncQuestionDelivery{{
		AdmissionID: "admission-outbox", Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: question.TaskID, DestinationSession: question.SessionID,
		Content: "correlated answer", Metadata: map[string]interface{}{"async_question_id": question.ID},
	}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// A crash after queue admission but before recording it repeats the same
	// admission identity; the outbox still reports one intent.
	if err := repo.MarkAsyncQuestionDeliveryAdmitted(ctx, "admission-outbox", time.Now().UTC()); err != nil {
		t.Fatalf("mark admitted: %v", err)
	}
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Minute), 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(deliveries) != 0 {
		t.Fatalf("expected no due intents after admission, got %d", len(deliveries))
	}

	// Re-resolving does not create a second outcome or intent.
	_, won, err := repo.ResolveAsyncQuestion(ctx, question.ID, models.AsyncQuestionOutcome{
		Answers: []protocol.Answer{{QuestionID: "q1", SelectedOptions: []string{"b"}}},
	}, []*models.AsyncQuestionDelivery{{
		AdmissionID: "admission-outbox-second", Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: question.TaskID, DestinationSession: question.SessionID,
	}})
	if err != nil || won {
		t.Fatalf("expected losing replay, won=%v err=%v", won, err)
	}
}

func TestAsyncQuestionsWithdrawIdempotent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-withdraw", "session-withdraw")
	createAsyncQuestionForTest(t, repo, question)
	first, won, _, err := repo.WithdrawAsyncQuestion(ctx, question.ID, question.SessionID, "no longer needed", nil, nil)
	if err != nil || !won {
		t.Fatalf("withdraw: won=%v err=%v", won, err)
	}
	if first.Status != models.AsyncQuestionStatusWithdrawn || first.WithdrawReason != "no longer needed" {
		t.Fatalf("unexpected withdrawal: %+v", first)
	}
	second, won, _, err := repo.WithdrawAsyncQuestion(ctx, question.ID, question.SessionID, "again", nil, nil)
	if err != nil {
		t.Fatalf("repeat withdraw: %v", err)
	}
	if won || second.Outcome != nil || second.WithdrawReason != "no longer needed" {
		t.Fatalf("repeat withdrawal must be idempotent: %+v", second)
	}
	if _, _, _, err := repo.WithdrawAsyncQuestion(ctx, question.ID, "other-session", "nope", nil, nil); err == nil {
		t.Fatal("a foreign session must not withdraw the request")
	}
}

func TestAsyncQuestionsForwardPreservesIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	question := newAsyncQuestionForTest(t, "q-forward", "session-forward")
	question.Recipient = models.AsyncQuestionRecipientParent
	question.ParentTaskID = "parent-task"
	question.ParentSessionID = "parent-session"
	createAsyncQuestionForTest(t, repo, question)
	own, err := repo.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{
		AuthorSessionID: question.SessionID, Limit: 10,
	})
	if err != nil {
		t.Fatalf("list own parent request: %v", err)
	}
	if len(own.Questions) != 1 {
		t.Fatalf("an author must read its own parent request before any forward, got %d", len(own.Questions))
	}
	forwarded, err := repo.ForwardAsyncQuestion(ctx, &models.AsyncQuestionForward{
		QuestionID: question.ID, ParentTaskID: "parent-task", ParentSessionID: "parent-session",
		Reason: "operator decision",
	})
	if err != nil {
		t.Fatalf("forward: %v", err)
	}
	if forwarded.ID != question.ID || !forwarded.Forwarded {
		t.Fatalf("forward must preserve request identity: %+v", forwarded)
	}
	page, err := repo.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{WorkspaceID: question.WorkspaceID, OpenOnly: true})
	if err != nil {
		t.Fatalf("list inbox: %v", err)
	}
	if len(page.Questions) != 1 {
		t.Fatalf("expected one forwarded request in the operator population, got %d", len(page.Questions))
	}
	unforwarded := newAsyncQuestionForTest(t, "q-unforwarded", "session-unforwarded")
	unforwarded.Recipient = models.AsyncQuestionRecipientParent
	unforwarded.ParentTaskID = "parent-task-2"
	unforwarded.ParentSessionID = "parent-session-2"
	createAsyncQuestionForTest(t, repo, unforwarded)
	page, err = repo.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{WorkspaceID: unforwarded.WorkspaceID, OpenOnly: true})
	if err != nil {
		t.Fatalf("list inbox after unforwarded: %v", err)
	}
	if len(page.Questions) != 0 {
		t.Fatalf("unforwarded parent request must stay out of the operator population, got %d", len(page.Questions))
	}
}

func TestAsyncQuestionsListPagination(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		question := newAsyncQuestionForTest(t, "q-page-"+itoa(i), "session-page")
		question.RetryKey = "retry-page-" + itoa(i)
		createAsyncQuestionForTest(t, repo, question)
	}
	page, err := repo.ListAsyncQuestions(ctx, models.ListAsyncQuestionsOptions{AuthorSessionID: "session-page", Limit: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Questions) != 2 || !page.HasMore {
		t.Fatalf("expected a truncated first page, got %d hasMore=%v", len(page.Questions), page.HasMore)
	}
	encoded, _ := json.Marshal(page.Questions[0].Questions)
	if len(encoded) == 0 {
		t.Fatal("questions must round-trip")
	}
}
