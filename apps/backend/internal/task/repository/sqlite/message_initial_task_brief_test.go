package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/plancomments"
	"github.com/kandev/kandev/internal/task/repository/admission"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type initialTaskBriefMessageWriter interface {
	CreateMessageWithInitialTaskBrief(context.Context, *models.Message, *admission.InitialTaskBriefCandidate) error
}

type initialTaskBriefPlanCommentWriter interface {
	CreateMessageWithPlanCommentsWithInitialTaskBrief(
		context.Context,
		*models.Message,
		*admission.InitialTaskBriefCandidate,
		[]models.TaskPlanCommentRef,
		bool,
		models.TaskSessionState,
		*messagequeue.QueueAttachmentClaim,
	) (*models.TaskPlanCommentSnapshot, error)
}

type initialTaskBriefQueuedPlanCommentWriter interface {
	CreateMessageWithPlanCommentsAndQueueWithInitialTaskBrief(
		context.Context,
		*models.Message,
		*messagequeue.QueuedMessage,
		*admission.InitialTaskBriefCandidate,
		[]models.TaskPlanCommentRef,
		bool,
		models.TaskSessionState,
		*messagequeue.QueueAttachmentClaim,
		int,
	) (*models.TaskPlanCommentSnapshot, error)
}

func requireInitialTaskBriefMessageWriter(t *testing.T, repo *Repository) initialTaskBriefMessageWriter {
	t.Helper()
	writer, ok := any(repo).(initialTaskBriefMessageWriter)
	if !ok {
		t.Fatal("Repository does not implement initial task brief message admission")
	}
	return writer
}

func setInitialTaskBriefDescription(t *testing.T, repo *Repository, taskID, description string) {
	t.Helper()
	if _, err := repo.db.ExecContext(context.Background(), repo.db.Rebind(
		`UPDATE tasks SET description = ? WHERE id = ?`,
	), description, taskID); err != nil {
		t.Fatalf("update task description: %v", err)
	}
}

func initialTaskBriefMessage(taskID, sessionID, turnID, id, content string) *models.Message {
	return &models.Message{
		ID: id, TaskID: taskID, TaskSessionID: sessionID, TurnID: turnID,
		AuthorType: models.MessageAuthorUser, Content: content,
	}
}

func initialTaskBriefCandidate(description, content string) *admission.InitialTaskBriefCandidate {
	return &admission.InitialTaskBriefCandidate{
		DescriptionSnapshot: description,
		Content:             content,
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5, AC-TASKS-INITIAL-TASK-BRIEF-001.6
func TestInitialTaskBriefAdmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-admission"
		sessionID = "session-initial-brief-admission"
		turnID    = "turn-initial-brief-admission"
		brief     = "Original task brief"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	writer := requireInitialTaskBriefMessageWriter(t, repo)

	first := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-first", "user instruction")
	firstCandidate := initialTaskBriefCandidate(brief, "Original task brief\n\nuser instruction")
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, first, firstCandidate); err != nil {
		t.Fatalf("first admission: %v", err)
	}
	if !firstCandidate.Selected || first.Content != firstCandidate.Content || first.PromptIndex != 1 {
		t.Fatalf("first admission message=%+v candidate=%+v", first, firstCandidate)
	}

	second := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-second", "later instruction")
	secondCandidate := initialTaskBriefCandidate(brief, "Original task brief\n\nlater instruction")
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, second, secondCandidate); err != nil {
		t.Fatalf("second admission: %v", err)
	}
	if secondCandidate.Selected || second.Content != "later instruction" || second.PromptIndex != 2 {
		t.Fatalf("second admission message=%+v candidate=%+v", second, secondCandidate)
	}

	stored, err := repo.GetMessageWithPromptIndex(ctx, first.ID)
	if err != nil {
		t.Fatalf("read first message: %v", err)
	}
	if stored.Content != first.Content || stored.PromptIndex != 1 {
		t.Fatalf("stored first message=%+v", stored)
	}

	if err := repo.DeleteMessage(ctx, first.ID); err != nil {
		t.Fatalf("delete first message: %v", err)
	}
	third := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-third", "after deletion")
	thirdCandidate := initialTaskBriefCandidate(brief, "Original task brief\n\nafter deletion")
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, third, thirdCandidate); err != nil {
		t.Fatalf("post-delete admission: %v", err)
	}
	if thirdCandidate.Selected || third.PromptIndex != 3 || third.Content != "after deletion" {
		t.Fatalf("post-delete admission message=%+v candidate=%+v", third, thirdCandidate)
	}
	hasHistory, err := repo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || !hasHistory {
		t.Fatalf("prompt history after deletion = %v, err=%v", hasHistory, err)
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.5, AC-TASKS-INITIAL-TASK-BRIEF-001.6
func TestInitialTaskBriefAdmissionRollbackAndStaleSnapshot(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-rollback"
		sessionID = "session-initial-brief-rollback"
		turnID    = "turn-initial-brief-rollback"
		brief     = "Stable task brief"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	writer := requireInitialTaskBriefMessageWriter(t, repo)

	message := initialTaskBriefMessage(taskID, sessionID, "missing-turn", "initial-brief-retry", "instruction")
	originalContent := message.Content
	candidate := initialTaskBriefCandidate(brief, "Stable task brief\n\ninstruction")
	message.PromptIndex = 99
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, message, candidate); err == nil {
		t.Fatal("invalid turn admission succeeded")
	}
	if message.Content != originalContent || message.PromptIndex != 99 || candidate.Selected {
		t.Fatalf("rollback changed caller state: message=%+v candidate=%+v", message, candidate)
	}
	hasHistory, err := repo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || hasHistory {
		t.Fatalf("history after rollback = %v, err=%v", hasHistory, err)
	}

	message.TurnID = turnID
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, message, candidate); err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if !candidate.Selected || message.Content != candidate.Content || message.PromptIndex != 1 {
		t.Fatalf("retry admission message=%+v candidate=%+v", message, candidate)
	}

	staleTaskID := "task-initial-brief-stale"
	staleSessionID := "session-initial-brief-stale"
	staleTurnID := "turn-initial-brief-stale"
	seedForMsgTest(t, repo, staleTaskID, staleSessionID, staleTurnID)
	setInitialTaskBriefDescription(t, repo, staleTaskID, brief)
	staleMessage := initialTaskBriefMessage(staleTaskID, staleSessionID, staleTurnID, "initial-brief-stale", "instruction")
	staleCandidate := initialTaskBriefCandidate(brief, "Stable task brief\n\ninstruction")
	setInitialTaskBriefDescription(t, repo, staleTaskID, "Changed task brief")
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, staleMessage, staleCandidate); !errors.Is(err, repoerrors.ErrInitialTaskBriefStale) {
		t.Fatalf("stale admission error = %v, want ErrInitialTaskBriefStale", err)
	}
	if staleCandidate.Selected || staleMessage.Content != "instruction" {
		t.Fatalf("stale admission changed caller state: message=%+v candidate=%+v", staleMessage, staleCandidate)
	}
	if hasHistory, err := repo.HasUserPromptHistory(ctx, staleSessionID); err != nil || hasHistory {
		t.Fatalf("stale history = %v, err=%v", hasHistory, err)
	}
	staleCandidate.DescriptionSnapshot = "Changed task brief"
	if err := writer.CreateMessageWithInitialTaskBrief(ctx, staleMessage, staleCandidate); err != nil {
		t.Fatalf("fresh stale retry: %v", err)
	}
	if !staleCandidate.Selected || staleMessage.PromptIndex != 1 {
		t.Fatalf("fresh stale retry message=%+v candidate=%+v", staleMessage, staleCandidate)
	}
}

func TestInitialTaskBriefAdmissionRejectsOversizedRenderedPrompt(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-size"
		sessionID = "session-initial-brief-size"
		turnID    = "turn-initial-brief-size"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, "Size constrained task brief")
	message := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-size-message", "instruction")
	originalContent := message.Content
	candidate := initialTaskBriefCandidate("Size constrained task brief", strings.Repeat("x", plancomments.MaxRenderedPromptBytes+1))
	err := requireInitialTaskBriefMessageWriter(t, repo).CreateMessageWithInitialTaskBrief(ctx, message, candidate)
	if !errors.Is(err, plancomments.ErrRenderedTooLarge) {
		t.Fatalf("oversized admission error = %v, want ErrRenderedTooLarge", err)
	}
	if candidate.Selected || message.Content != originalContent || message.PromptIndex != 0 {
		t.Fatalf("oversized admission changed caller state: message=%+v candidate=%+v", message, candidate)
	}
	if hasHistory, historyErr := repo.HasUserPromptHistory(ctx, sessionID); historyErr != nil || hasHistory {
		t.Fatalf("history after oversized admission = %v, err=%v", hasHistory, historyErr)
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.5
func TestInitialTaskBriefAdmissionSerializesConcurrentFirstSends(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-race"
		sessionID = "session-initial-brief-race"
		turnID    = "turn-initial-brief-race"
		brief     = "Concurrent task brief"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	writer := requireInitialTaskBriefMessageWriter(t, repo)
	messages := []*models.Message{
		initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-race-a", "instruction a"),
		initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-race-b", "instruction b"),
	}
	candidates := []*admission.InitialTaskBriefCandidate{
		initialTaskBriefCandidate(brief, "Concurrent task brief\n\ninstruction a"),
		initialTaskBriefCandidate(brief, "Concurrent task brief\n\ninstruction b"),
	}
	errs := make([]error, len(messages))
	var wg sync.WaitGroup
	start := make(chan struct{})
	for index := range messages {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			errs[index] = writer.CreateMessageWithInitialTaskBrief(ctx, messages[index], candidates[index])
		}(index)
	}
	close(start)
	wg.Wait()
	for index, err := range errs {
		if err != nil {
			t.Fatalf("concurrent admission %d: %v", index, err)
		}
	}
	winners := 0
	for index := range candidates {
		if candidates[index].Selected {
			winners++
			if messages[index].Content != candidates[index].Content || messages[index].PromptIndex != 1 {
				t.Fatalf("winner %d message=%+v candidate=%+v", index, messages[index], candidates[index])
			}
		} else if messages[index].PromptIndex != 2 {
			t.Fatalf("loser %d message=%+v", index, messages[index])
		}
	}
	if winners != 1 {
		t.Fatalf("concurrent winners = %d, want one", winners)
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5
func TestInitialTaskBriefAdmissionCompetesWithWorkflowFallback(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-fallback-race"
		sessionID = "session-initial-brief-fallback-race"
		turnID    = "turn-initial-brief-fallback-race"
		brief     = "Fallback race task brief"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	writer := requireInitialTaskBriefMessageWriter(t, repo)
	claimer, ok := any(repo).(promptHistoryClaimer)
	if !ok {
		t.Fatal("repository does not expose ClaimInitialPromptFallback")
	}

	message := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-fallback-race", "direct instruction")
	candidate := initialTaskBriefCandidate(brief, "Fallback race task brief\n\ndirect instruction")
	start := make(chan struct{})
	var wg sync.WaitGroup
	var claimed bool
	var claimErr error
	var createErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		claimed, claimErr = claimer.ClaimInitialPromptFallback(ctx, sessionID)
	}()
	go func() {
		defer wg.Done()
		<-start
		createErr = writer.CreateMessageWithInitialTaskBrief(ctx, message, candidate)
	}()
	close(start)
	wg.Wait()
	if claimErr != nil {
		t.Fatalf("fallback claim: %v", claimErr)
	}
	if createErr != nil {
		t.Fatalf("direct admission: %v", createErr)
	}
	if claimed == candidate.Selected {
		t.Fatalf("fallback/direct winners are not exclusive: fallback=%t candidate=%+v", claimed, candidate)
	}
	if candidate.Selected && message.Content != candidate.Content {
		t.Fatalf("direct winner content = %q, want %q", message.Content, candidate.Content)
	}
	if !candidate.Selected && message.Content != "direct instruction" {
		t.Fatalf("fallback winner retained content = %q, want direct instruction", message.Content)
	}
	if message.PromptIndex != 1 {
		t.Fatalf("fallback race prompt index = %d, want 1", message.PromptIndex)
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.3, AC-TASKS-INITIAL-TASK-BRIEF-001.5
func TestInitialTaskBriefAdmissionRespectsZeroValuedFallbackReservation(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const (
		taskID    = "task-initial-brief-zero-reservation"
		sessionID = "session-initial-brief-zero-reservation"
		turnID    = "turn-initial-brief-zero-reservation"
		brief     = "Reserved fallback task brief"
	)
	seedForMsgTest(t, repo, taskID, sessionID, turnID)
	setInitialTaskBriefDescription(t, repo, taskID, brief)
	claimer := any(repo).(promptHistoryClaimer)
	claimed, err := claimer.ClaimInitialPromptFallback(ctx, sessionID)
	if err != nil || !claimed {
		t.Fatalf("fallback reservation = %t, %v; want claimed", claimed, err)
	}

	message := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-zero-reservation-message", "direct instruction")
	candidate := initialTaskBriefCandidate(brief, "Reserved fallback task brief\n\ndirect instruction")
	if err := requireInitialTaskBriefMessageWriter(t, repo).CreateMessageWithInitialTaskBrief(ctx, message, candidate); err != nil {
		t.Fatalf("admission after fallback reservation: %v", err)
	}
	if candidate.Selected || message.Content != "direct instruction" || message.PromptIndex != 1 {
		t.Fatalf("zero reservation admission message=%+v candidate=%+v", message, candidate)
	}
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.5, AC-TASKS-INITIAL-TASK-BRIEF-001.8
func TestInitialTaskBriefAdmissionSharesPlanCommentAndQueueBoundaries(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	queueDB := sqlx.NewDb(repo.DB(), "sqlite3")
	if _, err := messagequeue.NewSQLiteRepository(queueDB, queueDB); err != nil {
		t.Fatalf("initialize queue schema: %v", err)
	}
	seedMessagePlanComment(t, ctx, repo, "initial-brief-plan")
	setInitialTaskBriefDescription(t, repo, "task-message-comments-initial-brief-plan", "Plan task brief")
	candidate := initialTaskBriefCandidate(
		"Plan task brief",
		plancomments.WithPlaceholder("Plan task brief\n\ntyped content"),
	)
	message := planCommentMessage("initial-brief-plan", "initial-brief-plan-message")
	writer, ok := any(repo).(initialTaskBriefPlanCommentWriter)
	if !ok {
		t.Fatal("Repository does not implement plan-comment initial task brief admission")
	}
	if _, err := writer.CreateMessageWithPlanCommentsWithInitialTaskBrief(
		ctx, message, candidate, []models.TaskPlanCommentRef{{ID: "comment-initial-brief-plan", Version: 1}},
		true, "", nil,
	); err != nil {
		t.Fatalf("plan-comment initial admission: %v", err)
	}
	if !candidate.Selected || message.Content == "" {
		t.Fatalf("plan-comment candidate=%+v message=%+v", candidate, message)
	}
	if !containsExactlyOnce(message.Content, "Plan task brief") || message.PromptIndex != 1 {
		t.Fatalf("plan-comment message content=%q index=%d", message.Content, message.PromptIndex)
	}

	queueSuffix := "initial-brief-queue"
	seedMessagePlanComment(t, ctx, repo, queueSuffix)
	setInitialTaskBriefDescription(t, repo, "task-message-comments-"+queueSuffix, "Queued task brief")
	queuedMessage := planCommentMessage(queueSuffix, "initial-brief-queue-message")
	queued := &messagequeue.QueuedMessage{
		ID: queuedMessage.ID, TaskID: queuedMessage.TaskID, SessionID: queuedMessage.TaskSessionID,
		Content: queuedMessage.Content, QueuedBy: messagequeue.QueuedByUser,
	}
	queuedCandidate := initialTaskBriefCandidate(
		"Queued task brief",
		plancomments.WithPlaceholder("Queued task brief\n\ntyped content"),
	)
	queueWriter, ok := any(repo).(initialTaskBriefQueuedPlanCommentWriter)
	if !ok {
		t.Fatal("Repository does not implement queued initial task brief admission")
	}
	if _, err := queueWriter.CreateMessageWithPlanCommentsAndQueueWithInitialTaskBrief(
		ctx, queuedMessage, queued, queuedCandidate,
		[]models.TaskPlanCommentRef{{ID: "comment-" + queueSuffix, Version: 1}},
		true, "", nil, 10,
	); err != nil {
		t.Fatalf("queued initial admission: %v", err)
	}
	if !queuedCandidate.Selected || queued.Content != queuedMessage.Content ||
		!containsExactlyOnce(queued.Content, "Queued task brief") {
		t.Fatalf("queued content=%q message=%q candidate=%+v", queued.Content, queuedMessage.Content, queuedCandidate)
	}
}

func containsExactlyOnce(content, needle string) bool {
	return len(content) > 0 && strings.Count(content, needle) == 1
}

func newInitialTaskBriefRepoAtPath(t *testing.T) (*Repository, *sqlx.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "initial-task-brief-restart.db")
	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(dbConn, "sqlite3")
	repo, err := NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		_ = sqlxDB.Close()
		t.Fatalf("new repo: %v", err)
	}
	return repo, sqlxDB, dbPath
}

// @covers AC-TASKS-INITIAL-TASK-BRIEF-001.6
func TestInitialTaskBriefAdmissionSurvivesRepositoryRestart(t *testing.T) {
	repo, sqlxDB, dbPath := newInitialTaskBriefRepoAtPath(t)
	ctx := context.Background()
	const (
		taskID      = "task-initial-brief-restart"
		sessionID   = "session-initial-brief-restart"
		turnID      = "turn-initial-brief-restart"
		workspaceID = "workspace-initial-brief-restart"
	)
	seedWorkspace(t, repo, workspaceID)
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, Title: "Restart task"}); err != nil {
		t.Fatalf("seed restart task: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("seed restart session: %v", err)
	}
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: taskID}); err != nil {
		t.Fatalf("seed restart turn: %v", err)
	}
	setInitialTaskBriefDescription(t, repo, taskID, "Restart task brief")
	candidate := initialTaskBriefCandidate("Restart task brief", "Restart task brief\n\ninstruction")
	message := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-restart-message", "instruction")
	if err := requireInitialTaskBriefMessageWriter(t, repo).CreateMessageWithInitialTaskBrief(ctx, message, candidate); err != nil {
		t.Fatalf("initial restart fixture: %v", err)
	}
	if err := sqlxDB.Close(); err != nil {
		t.Fatalf("close first repository: %v", err)
	}

	dbConn, err := db.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("reopen sqlite: %v", err)
	}
	reopenedDB := sqlx.NewDb(dbConn, "sqlite3")
	reopenedRepo, err := NewWithDB(reopenedDB, reopenedDB, nil)
	if err != nil {
		_ = reopenedDB.Close()
		t.Fatalf("reopen repository: %v", err)
	}
	t.Cleanup(func() { _ = reopenedDB.Close() })
	hasHistory, err := reopenedRepo.HasUserPromptHistory(ctx, sessionID)
	if err != nil || !hasHistory {
		t.Fatalf("history after restart = %v, err=%v", hasHistory, err)
	}
	second := initialTaskBriefMessage(taskID, sessionID, turnID, "initial-brief-restart-second", "later")
	secondCandidate := initialTaskBriefCandidate("Restart task brief", "Restart task brief\n\nlater")
	if err := requireInitialTaskBriefMessageWriter(t, reopenedRepo).CreateMessageWithInitialTaskBrief(ctx, second, secondCandidate); err != nil {
		t.Fatalf("post-restart admission: %v", err)
	}
	if secondCandidate.Selected || second.Content != "later" || second.PromptIndex != 2 {
		t.Fatalf("post-restart message=%+v candidate=%+v", second, secondCandidate)
	}
}
