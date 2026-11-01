package questions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	"github.com/kandev/kandev/internal/task/repository/hierarchy"
	"github.com/kandev/kandev/internal/task/repository/sqlite"
)

type staleQuestionEventObserver struct {
	repo     *sqlite.Repository
	statuses []models.AsyncQuestionStatus
}

func (o *staleQuestionEventObserver) AsyncQuestionChanged(ctx context.Context, change questions.QuestionChange) {
	question, err := o.repo.GetAsyncQuestion(ctx, change.QuestionID)
	if err == nil && question != nil {
		o.statuses = append(o.statuses, question.Status)
	}
}

func createParentQuestion(t *testing.T, svc *questions.Service, repo *sqlite.Repository, suffix string) *questions.CreateResult {
	t.Helper()
	childTaskID := "task-child-" + suffix
	childSessionID := "session-child-" + suffix
	parentTaskID := "task-parent-" + suffix
	parentSessionID := "session-parent-" + suffix
	workspaceID := "workspace-" + suffix
	seedExplicitQuestionBinding(t, repo, childTaskID, childSessionID, workspaceID, parentTaskID)
	seedRelatedParentBinding(t, repo, parentTaskID, parentSessionID, workspaceID)
	result, err := svc.Create(context.Background(), questions.CreateInput{
		TaskID: childTaskID, WorkspaceID: workspaceID, SessionID: childSessionID,
		SessionIncarnationID: "inc-" + childSessionID,
		Recipient:            models.AsyncQuestionRecipientParent,
		ParentTaskID:         parentTaskID, ParentSession: parentSessionID,
		ParentSessionIncarnationID: "inc-" + parentSessionID,
		Questions:                  []protocol.Question{{Prompt: "Choose?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID:                  "request-" + suffix,
	})
	if err != nil {
		t.Fatalf("create parent question: %v", err)
	}
	return result
}

func seedRelatedParentBinding(t *testing.T, repo *sqlite.Repository, taskID, sessionID, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	workflowID := "workflow-" + sessionID
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("seed parent workflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: taskID,
		State: "TODO", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed parent task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, IsPrimary: true, State: models.TaskSessionStateRunning,
		QueueIncarnationID: "inc-" + sessionID, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed parent session: %v", err)
	}
}

func TestAsyncQuestionsSweepStalesReparentedRequest(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	queue := &recordingQueue{}
	svc.SetDeliveryQueue(queue)
	result := createParentQuestion(t, svc, repo, "sweep-reparent")
	wait, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: result.Question.TaskID, SessionID: result.Question.SessionID,
		Dependencies: []string{result.Question.ID}, Reason: "parent decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := svc.MarkWaitParked(ctx, wait.Wait.ID); err != nil {
		t.Fatalf("park wait: %v", err)
	}
	child, err := repo.GetTask(ctx, result.Question.TaskID)
	if err != nil {
		t.Fatalf("get child task: %v", err)
	}
	newParentID := "task-new-parent-sweep-reparent"
	allowParent := func(context.Context, hierarchy.TaskHierarchyReader, *models.Task, string) error { return nil }
	if _, err := repo.UpdateTaskWithParentAdmission(ctx, child, &newParentID, true, allowParent); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	if _, err := svc.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	stale, err := svc.Get(ctx, result.Question.ID)
	if err != nil || stale.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("request = %+v err=%v, want stale after one sweep", stale, err)
	}
	if svc.HasParkedWait(ctx, result.Question.SessionID) {
		t.Fatal("stale dependency left its author parked")
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	wakeCount := 0
	for _, delivery := range queue.deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryWake {
			wakeCount++
		}
		if delivery.Kind == models.AsyncQuestionDeliveryParentQuestion {
			t.Fatalf("stale parent request was delivered: %+v", delivery)
		}
	}
	if wakeCount != 1 {
		t.Fatalf("wake deliveries = %d, want exactly one", wakeCount)
	}
}

func TestAsyncQuestionsSweepStalesArchivedParent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createParentQuestion(t, svc, repo, "sweep-archived-parent")
	parent, err := repo.GetTask(ctx, result.Question.ParentTaskID)
	if err != nil {
		t.Fatalf("get parent: %v", err)
	}
	if err := repo.ArchiveTask(ctx, parent.ID); err != nil {
		t.Fatalf("archive parent: %v", err)
	}
	queue := &recordingQueue{}
	svc.SetDeliveryQueue(queue)
	if _, err := svc.ReconcileOnce(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	stale, err := svc.Get(ctx, result.Question.ID)
	if err != nil || stale.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("request = %+v err=%v, want stale after parent archive", stale, err)
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	for _, delivery := range queue.deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryParentQuestion {
			t.Fatalf("archived parent request was delivered: %+v", delivery)
		}
	}
}

func TestAsyncQuestionsWithdrawAbandonsUnsentParentPrompt(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createParentQuestion(t, svc, repo, "withdraw-unsent-parent")
	initial, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	if err != nil {
		t.Fatalf("list initial deliveries: %v", err)
	}
	parentPrompts := 0
	for _, delivery := range initial {
		if delivery.Kind == models.AsyncQuestionDeliveryParentQuestion {
			parentPrompts++
		}
	}
	if parentPrompts != 1 {
		t.Fatalf("initial deliveries = %+v, want one parent prompt", initial)
	}
	if _, err := svc.Withdraw(ctx, result.Question.ID, result.Question.SessionID, "resolved locally"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	remaining, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	if err != nil {
		t.Fatalf("list remaining deliveries: %v", err)
	}
	for _, delivery := range remaining {
		if delivery.Kind == models.AsyncQuestionDeliveryParentQuestion {
			t.Fatalf("unsent parent prompt remained due after withdrawal: %+v", remaining)
		}
	}
}

func TestAsyncQuestionsStaleDependencyWakesAuthor(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createParentQuestion(t, svc, repo, "stale-answer-wakes")
	wait, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: result.Question.TaskID, SessionID: result.Question.SessionID,
		Dependencies: []string{result.Question.ID}, Reason: "parent decision",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if err := svc.MarkWaitParked(ctx, wait.Wait.ID); err != nil {
		t.Fatalf("park wait: %v", err)
	}
	child, err := repo.GetTask(ctx, result.Question.TaskID)
	if err != nil {
		t.Fatalf("get child task: %v", err)
	}
	newParentID := "task-new-parent-stale-answer-wakes"
	allowParent := func(context.Context, hierarchy.TaskHierarchyReader, *models.Task, string) error { return nil }
	if _, err := repo.UpdateTaskWithParentAdmission(ctx, child, &newParentID, true, allowParent); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	observer := &staleQuestionEventObserver{repo: repo}
	svc.SetNotifier(observer)
	_, _, err = svc.ResolveParent(ctx, result.Question.ID, result.Question.ParentTaskID, result.Question.ParentSessionID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: result.Question.Questions[0].ID, SelectedOptions: []string{"a"}}},
	})
	if !errors.Is(err, questions.ErrUnauthorized) {
		t.Fatalf("stale answer error = %v, want unauthorized", err)
	}
	if svc.HasParkedWait(ctx, result.Question.SessionID) {
		t.Fatal("stale dependency left its author parked")
	}
	deliveries, err := repo.ListAsyncQuestionDeliveries(ctx, time.Now().UTC().Add(time.Hour), 20)
	if err != nil {
		t.Fatalf("list wake intents: %v", err)
	}
	wakeCount := 0
	for _, delivery := range deliveries {
		if delivery.Kind == models.AsyncQuestionDeliveryWake {
			wakeCount++
		}
	}
	if wakeCount != 1 {
		t.Fatalf("stale deliveries = %+v, want one author wake", deliveries)
	}
	if len(observer.statuses) != 1 || observer.statuses[0] != models.AsyncQuestionStatusStale {
		t.Fatalf("published stale states = %v, want one committed stale event", observer.statuses)
	}
}

func TestAsyncQuestionsWithdrawRemovesQueuedParentPrompt(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	queue := &recordingQueue{}
	svc.SetDeliveryQueue(queue)
	result := createParentQuestion(t, svc, repo, "withdraw-queued-parent")
	if _, err := svc.ReconcileOnce(ctx); err != nil {
		t.Fatalf("admit parent prompt: %v", err)
	}
	queue.mu.Lock()
	if len(queue.deliveries) != 1 || queue.deliveries[0].Kind != models.AsyncQuestionDeliveryParentQuestion {
		queue.mu.Unlock()
		t.Fatalf("queued parent prompts = %+v, want one before withdrawal", queue.deliveries)
	}
	queue.mu.Unlock()
	if _, err := svc.Withdraw(ctx, result.Question.ID, result.Question.SessionID, "resolved locally"); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	queue.mu.Lock()
	defer queue.mu.Unlock()
	if len(queue.deliveries) != 0 {
		t.Fatalf("withdrawn prompt remains queued: %+v", queue.deliveries)
	}
	if len(queue.removed) != 1 || queue.removed[0].Kind != models.AsyncQuestionDeliveryParentQuestion {
		t.Fatalf("removed parent prompts = %+v, want one", queue.removed)
	}
}
