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
			for _, delivery := range deliveries {
				if delivery.Kind != models.AsyncQuestionDeliveryTranscript {
					t.Fatalf("stale answer must not enqueue a continuation: %+v", delivery)
				}
			}
		})
	}
}

func TestAsyncQuestionsWorkerLifecycleIsIdempotent(t *testing.T) {
	svc, _ := newQuestionsService(t)
	svc.SetDeliveryQueue(&recordingQueue{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.Start(ctx)
	svc.Start(ctx)
	svc.Stop()
	svc.Stop()
	svc.Start(ctx)
	svc.Stop()
}

func TestAsyncQuestionsWaitAcknowledgementDoesNotOverwriteFencedRelease(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	result := createOperatorQuestion(t, svc, repo, "session-wait-fenced")
	waitResult, err := svc.Wait(ctx, questions.WaitInput{
		TaskID: "task-session-wait-fenced", SessionID: "session-wait-fenced",
		Dependencies: []string{result.Question.ID}, Reason: "dependency",
	})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	// The answer commits release_requested before the pause acknowledgement.
	if err := repo.SetAsyncQuestionWaitStatus(ctx, waitResult.Wait.ID, models.AsyncQuestionWaitReleaseRequested, "test"); err != nil {
		t.Fatalf("fence release: %v", err)
	}
	status, err := svc.AcknowledgeWaitPause(ctx, waitResult.Wait.ID)
	if err != nil {
		t.Fatalf("acknowledge: %v", err)
	}
	if status != models.AsyncQuestionWaitReleased {
		t.Fatalf("status = %s, want released", status)
	}
	if svc.HasParkedWait(ctx, "session-wait-fenced") {
		t.Fatal("a fenced release must not be overwritten as parked")
	}
}

func TestAsyncQuestionsChildrenRequireCurrentParent(t *testing.T) {
	svc, repo := newQuestionsService(t)
	ctx := context.Background()
	seedExplicitQuestionBinding(t, repo, "task-child-current", "session-child-current", "workspace-session-child-current", "task-parent-current")
	seedExplicitQuestionBinding(t, repo, "task-parent-current", "session-parent-current", "workspace-session-parent-current", "")
	result, err := svc.Create(ctx, questions.CreateInput{
		TaskID: "task-child-current", WorkspaceID: "workspace-session-child-current", SessionID: "session-child-current",
		SessionIncarnationID: "inc-session-child-current",
		Recipient:            models.AsyncQuestionRecipientParent, ParentTaskID: "task-parent-current", ParentSession: "session-parent-current",
		ParentSessionIncarnationID: "inc-session-parent-current",
		Questions:                  []protocol.Question{{Prompt: "Which?", Options: []protocol.Option{{ID: "a", Label: "A"}, {ID: "b", Label: "B"}}}},
		RequestID:                  "child-current",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	page, err := svc.ListChildren(ctx, "task-parent-current", nil, nil, 10, "", "")
	if err != nil || len(page.Questions) != 1 {
		t.Fatalf("children before reparent = %d err=%v", len(page.Questions), err)
	}
	if _, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", "task-parent-other", "task-child-current"); err != nil {
		t.Fatalf("reparent: %v", err)
	}
	page, err = svc.ListChildren(ctx, "task-parent-current", nil, nil, 10, "", "")
	if err != nil || len(page.Questions) != 0 {
		t.Fatalf("a former parent must not read child questions: %d err=%v", len(page.Questions), err)
	}
	_ = result
}
