package orchestrator

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/questions"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	"github.com/kandev/kandev/internal/testutil"
	"go.uber.org/zap"
)

func newAsyncQuestionBootSession(
	t *testing.T,
	repo *tasksqlite.Repository,
	taskID, sessionID string,
	waitStatus models.AsyncQuestionWaitStatus,
) *models.AsyncQuestionWait {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	workspaceID := "workspace-" + taskID
	workflowID := "workflow-" + taskID
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: taskID, WorkspaceID: workspaceID, WorkflowID: workflowID, Title: taskID, State: "TODO", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	incarnationID := "inc-" + sessionID
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID, IsPrimary: true, State: models.TaskSessionStateRunning, QueueIncarnationID: incarnationID, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	turnID := "turn-" + sessionID
	if err := repo.CreateTurn(ctx, &models.Turn{ID: turnID, TaskSessionID: sessionID, TaskID: taskID, StartedAt: now, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	question := &models.AsyncQuestion{
		ID: "question-" + sessionID, TaskID: taskID, WorkspaceID: workspaceID,
		SessionID: sessionID, SessionIncarnationID: incarnationID, TurnID: turnID,
		Recipient: models.AsyncQuestionRecipientUser,
		Questions: []protocol.Question{{ID: "choice", Prompt: "Choose", Options: []protocol.Option{{ID: "one", Label: "One"}}}},
		RetryKey:  "retry-" + sessionID, PayloadDigest: "digest-" + sessionID,
		Status: models.AsyncQuestionStatusPending, Version: 1,
	}
	if _, _, err := repo.CreateAsyncQuestion(ctx, question, nil); err != nil {
		t.Fatal(err)
	}
	wait, err := repo.CreateAsyncQuestionWait(ctx, &models.AsyncQuestionWait{
		WorkspaceID: workspaceID, TaskID: taskID, SessionID: sessionID, TurnID: turnID,
		DependencyIDs: []string{question.ID}, Status: waitStatus,
	})
	if err != nil {
		t.Fatal(err)
	}
	return wait
}

func newAsyncQuestionBootRepositories(t *testing.T) (*tasksqlite.Repository, *messagequeue.Service, messagequeue.Repository) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "async-question-boot.db"))
	if err != nil {
		t.Fatal(err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	if err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(sqlxDB, sqlxDB)
	if err != nil {
		t.Fatal(err)
	}
	return taskRepo, messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger()), queueRepo
}

func TestAsyncQuestionsDisabledBootClearsWaitProjection(t *testing.T) {
	ctx := context.Background()
	repo, queue, _ := newAsyncQuestionBootRepositories(t)
	requested := newAsyncQuestionBootSession(t, repo, "task-requested", "session-requested", models.AsyncQuestionWaitRequested)
	parked := newAsyncQuestionBootSession(t, repo, "task-parked", "session-parked", models.AsyncQuestionWaitParked)
	releasing := newAsyncQuestionBootSession(t, repo, "task-releasing", "session-releasing", models.AsyncQuestionWaitReleaseRequested)
	blocking := newAsyncQuestionBootSession(t, repo, "task-blocking", "session-blocking", models.AsyncQuestionWaitRequested)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "blocking-clarification", TaskID: "task-blocking", TaskSessionID: "session-blocking",
		TurnID: "turn-session-blocking", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, Content: "blocking question",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "task-parked", WorkspaceID: "workspace-task-parked",
		Summary: statussummary.TaskStatusSummary{Revision: 1, PendingAction: string(models.TaskPendingActionClarification)},
	}); err != nil {
		t.Fatal(err)
	}

	service := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("PrepareStartup: %v", err)
	}

	for _, wait := range []*models.AsyncQuestionWait{requested, parked, releasing, blocking} {
		stored, err := repo.GetAsyncQuestionWait(ctx, wait.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != models.AsyncQuestionWaitCancelled || stored.ReleasedBy != "feature_disabled" {
			t.Errorf("wait %s after disabled boot = (%s, %s), want (cancelled, feature_disabled)", stored.ID, stored.Status, stored.ReleasedBy)
		}
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{
		"session-requested", "session-parked", "session-releasing", "session-blocking",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"session-requested", "session-parked", "session-releasing"} {
		if action := actions[sessionID]; action != "" {
			t.Errorf("disabled async wait left pending action %q for %s", action, sessionID)
		}
	}
	if actions["session-blocking"] != models.TaskPendingActionClarification {
		t.Errorf("real blocking clarification action = %q, want clarification", actions["session-blocking"])
	}

	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, Executors: repo, TaskEnvironments: repo, StatusSummaries: repo,
	}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	tasks := make([]*models.Task, 0, 4)
	sessionsByTask := make(map[string][]*models.TaskSession, 4)
	for _, taskID := range []string{"task-requested", "task-parked", "task-releasing", "task-blocking"} {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		sessions, err := repo.ListTaskSessions(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		sessionsByTask[taskID] = sessions
	}
	storedSummaries, err := taskSvc.GetTaskStatusSummaries(ctx, []string{"task-requested", "task-parked", "task-releasing", "task-blocking"})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := taskSvc.ReconcileTaskStatusSummaries(ctx, tasks, sessionsByTask, actions, storedSummaries)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got := reconciled["task-parked"].PendingAction; got != "" {
		t.Errorf("reconciled parked task summary pending action = %q, want empty", got)
	}
	if got := reconciled["task-blocking"].PendingAction; got != string(models.TaskPendingActionClarification) {
		t.Errorf("reconciled blocking task summary pending action = %q, want clarification", got)
	}
}

func TestPostgresAsyncQuestionsDisabledRestartClearsPendingAction(t *testing.T) {
	ctx := context.Background()
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatal(err)
	}
	queueRepo, err := messagequeue.NewSQLiteRepository(database, database)
	if err != nil {
		t.Fatal(err)
	}
	queue := messagequeue.NewService(queueRepo, messagequeue.DefaultMaxPerSession, testLogger())
	requested := newAsyncQuestionBootSession(t, repo, "pg-task-requested", "pg-session-requested", models.AsyncQuestionWaitRequested)
	parked := newAsyncQuestionBootSession(t, repo, "pg-task-parked", "pg-session-parked", models.AsyncQuestionWaitParked)
	releasing := newAsyncQuestionBootSession(t, repo, "pg-task-releasing", "pg-session-releasing", models.AsyncQuestionWaitReleaseRequested)
	blocking := newAsyncQuestionBootSession(t, repo, "pg-task-blocking", "pg-session-blocking", models.AsyncQuestionWaitRequested)
	if err := repo.CreateMessage(ctx, &models.Message{
		ID: "pg-blocking-clarification", TaskID: "pg-task-blocking", TaskSessionID: "pg-session-blocking",
		TurnID: "turn-pg-session-blocking", AuthorType: models.MessageAuthorAgent,
		Type: models.MessageTypeClarificationRequest, Content: "blocking question",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "pg-task-parked", WorkspaceID: "workspace-pg-task-parked",
		Summary: statussummary.TaskStatusSummary{Revision: 1, PendingAction: string(models.TaskPendingActionClarification)},
	}); err != nil {
		t.Fatal(err)
	}

	service := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("PrepareStartup: %v", err)
	}
	for _, wait := range []*models.AsyncQuestionWait{requested, parked, releasing, blocking} {
		stored, err := repo.GetAsyncQuestionWait(ctx, wait.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.Status != models.AsyncQuestionWaitCancelled || stored.ReleasedBy != "feature_disabled" {
			t.Errorf("wait %s after disabled boot = (%s, %s), want (cancelled, feature_disabled)", stored.ID, stored.Status, stored.ReleasedBy)
		}
	}
	actions, err := repo.GetPendingActionsBySessionIDs(ctx, []string{
		"pg-session-requested", "pg-session-parked", "pg-session-releasing", "pg-session-blocking",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, sessionID := range []string{"pg-session-requested", "pg-session-parked", "pg-session-releasing"} {
		if got := actions[sessionID]; got != "" {
			t.Errorf("disabled async wait left pending action %q for %s", got, sessionID)
		}
	}
	if actions["pg-session-blocking"] != models.TaskPendingActionClarification {
		t.Errorf("real blocking clarification action = %q, want clarification", actions["pg-session-blocking"])
	}

	taskSvc := taskservice.NewService(taskservice.Repos{
		Workspaces: repo, Tasks: repo, TaskRepos: repo, Workflows: repo, Messages: repo, Turns: repo,
		Sessions: repo, Executors: repo, TaskEnvironments: repo, StatusSummaries: repo,
	}, nil, testLogger(), taskservice.RepositoryDiscoveryConfig{})
	tasks := make([]*models.Task, 0, 4)
	sessionsByTask := make(map[string][]*models.TaskSession, 4)
	for _, taskID := range []string{"pg-task-requested", "pg-task-parked", "pg-task-releasing", "pg-task-blocking"} {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, task)
		sessions, err := repo.ListTaskSessions(ctx, taskID)
		if err != nil {
			t.Fatal(err)
		}
		sessionsByTask[taskID] = sessions
	}
	ids := []string{"pg-task-requested", "pg-task-parked", "pg-task-releasing", "pg-task-blocking"}
	storedSummaries, err := taskSvc.GetTaskStatusSummaries(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := taskSvc.ReconcileTaskStatusSummaries(ctx, tasks, sessionsByTask, actions, storedSummaries)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got := reconciled["pg-task-parked"].PendingAction; got != "" {
		t.Errorf("reconciled parked task summary pending action = %q, want empty", got)
	}
	if got := reconciled["pg-task-blocking"].PendingAction; got != string(models.TaskPendingActionClarification) {
		t.Errorf("reconciled blocking task summary pending action = %q, want clarification", got)
	}
}

func TestAsyncQuestionsDisabledRetiresOnlyUnsentQueueEntries(t *testing.T) {
	ctx := context.Background()
	repo, queue, queueRepo := newAsyncQuestionBootRepositories(t)
	ownerWait := newAsyncQuestionBootSession(t, repo, "task-retirement-owner", "session-retirement-owner", models.AsyncQuestionWaitReleased)
	parentWait := newAsyncQuestionBootSession(t, repo, "task-retirement-parent", "session-retirement-parent", models.AsyncQuestionWaitReleased)
	ownerQuestionID := ownerWait.DependencyIDs[0]
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-retirement-parent-active", TaskID: "task-retirement-parent", State: models.TaskSessionStateRunning,
		QueueIncarnationID: "inc-session-retirement-parent-active", StartedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	parentIdentity := messagequeue.QueueSessionIdentity{
		TaskID: "task-retirement-parent", SessionID: "session-retirement-parent-active",
		SessionIncarnationID: "inc-session-retirement-parent-active",
	}
	if err := queueRepoInsert(queueRepo, parentIdentity, &messagequeue.QueuedMessage{
		ID: "ordinary-before-retirement", SessionID: parentIdentity.SessionID, TaskID: parentIdentity.TaskID,
		Content: "first ordinary prompt", QueuedBy: messagequeue.QueuedByUser,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	retiredAdmissionID := "admission-parent-rebound"
	metadata := map[string]interface{}{
		messagequeue.MetadataAsyncQuestion: ownerQuestionID,
		"async_question_kind":              string(models.AsyncQuestionDeliveryParentQuestion),
		"async_question_notification":      true,
		"sender_task_id":                   "task-retirement-owner",
		"sender_session_id":                "session-retirement-owner",
	}
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: retiredAdmissionID, QuestionID: ownerQuestionID, Kind: models.AsyncQuestionDeliveryParentQuestion,
		DestinationTaskID: "task-retirement-parent", DestinationSession: "session-retirement-parent",
		DestinationIncarnationID: "inc-session-retirement-parent", Content: "parent prompt after rebind",
		Metadata: metadata, State: models.AsyncQuestionDeliveryPending, CreatedAt: now, NextAttemptAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, parentIdentity, retiredAdmissionID, "parent prompt after rebind", "", "async-question", false, nil, metadata, nil,
	); err != nil {
		t.Fatal(err)
	}
	if err := queueRepoInsert(queueRepo, parentIdentity, &messagequeue.QueuedMessage{
		ID: "ordinary-after-retirement", SessionID: parentIdentity.SessionID, TaskID: parentIdentity.TaskID,
		Content: "second ordinary prompt", QueuedBy: messagequeue.QueuedByUser,
	}); err != nil {
		t.Fatal(err)
	}
	before, admittedAt, err := queue.AsyncQuestionAdmissionDisposition(ctx, retiredAdmissionID, ownerQuestionID)
	if err != nil || before != messagequeue.AsyncQuestionAdmissionQueued || admittedAt.IsZero() {
		t.Fatalf("rebound admission before startup = (%s, %s, %v), want queued receipt", before, admittedAt, err)
	}

	backoffID := "admission-backoff-no-receipt"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: backoffID, QuestionID: ownerQuestionID, OutcomeVersion: 2, Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: "task-retirement-owner", DestinationSession: "session-retirement-owner",
		Content: "prompt retry in backoff", State: models.AsyncQuestionDeliveryPending,
		CreatedAt: now, NextAttemptAt: now.Add(time.Hour), Attempts: 4, LastError: "temporary delivery error",
	}); err != nil {
		t.Fatal(err)
	}
	transcriptID := "admission-transcript-preserved"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: transcriptID, QuestionID: ownerQuestionID, Kind: models.AsyncQuestionDeliveryTranscript,
		DestinationTaskID: "task-retirement-owner", DestinationSession: "session-retirement-owner",
		Content: "transcript intent", State: models.AsyncQuestionDeliveryPending, CreatedAt: now, NextAttemptAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	historyID := "admission-delivered-history"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: historyID, QuestionID: ownerQuestionID, OutcomeVersion: 3, Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: "task-retirement-owner", DestinationSession: "session-retirement-owner",
		Content: "already delivered", State: models.AsyncQuestionDeliveryPending, CreatedAt: now, NextAttemptAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAsyncQuestionDeliveryAdmitted(ctx, historyID, now); err != nil {
		t.Fatal(err)
	}

	acceptedWait := newAsyncQuestionBootSession(t, repo, "task-retirement-accepted", "session-retirement-accepted", models.AsyncQuestionWaitReleased)
	acceptedQuestionID := acceptedWait.DependencyIDs[0]
	acceptedIdentity := messagequeue.QueueSessionIdentity{
		TaskID: "task-retirement-accepted", SessionID: "session-retirement-accepted",
		SessionIncarnationID: "inc-session-retirement-accepted",
	}
	acceptedID := "admission-already-accepted"
	acceptedMetadata := map[string]interface{}{
		messagequeue.MetadataAsyncQuestion: acceptedQuestionID,
		"async_question_kind":              string(models.AsyncQuestionDeliveryOwner),
		"async_question_notification":      true,
	}
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: acceptedID, QuestionID: acceptedQuestionID, Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: acceptedIdentity.TaskID, DestinationSession: acceptedIdentity.SessionID,
		DestinationIncarnationID: acceptedIdentity.SessionIncarnationID, Content: "accepted prompt",
		Metadata: acceptedMetadata, State: models.AsyncQuestionDeliveryPending, CreatedAt: now, NextAttemptAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, acceptedIdentity, acceptedID, "accepted prompt", "", "async-question", false, nil, acceptedMetadata, nil,
	); err != nil {
		t.Fatal(err)
	}
	accepted, exists, _, err := queue.ReserveQueuedWithAutoRunForSession(ctx, acceptedIdentity)
	if err != nil || !exists {
		t.Fatalf("reserve accepted async prompt = (%#v, %t, %v)", accepted, exists, err)
	}
	if err := queue.MarkPendingQueueDispatchAccepted(ctx, accepted); err != nil {
		t.Fatal(err)
	}

	service := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("PrepareStartup: %v", err)
	}

	entries, err := queueRepo.ListBySession(ctx, parentIdentity.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "ordinary-before-retirement" || entries[1].ID != "ordinary-after-retirement" {
		t.Fatalf("ordinary queue after retirement = %#v, want original FIFO", entries)
	}
	state, receiptTime, err := queue.AsyncQuestionAdmissionDisposition(ctx, retiredAdmissionID, ownerQuestionID)
	if err != nil || state != messagequeue.AsyncQuestionAdmissionSettled || !receiptTime.Equal(admittedAt) {
		t.Fatalf("retired rebound receipt = (%s, %s, %v), want settled with original timestamp", state, receiptTime, err)
	}
	assertAsyncQuestionDelivery(t, repo, ownerQuestionID, retiredAdmissionID, models.AsyncQuestionDeliveryParentQuestion, models.AsyncQuestionDeliveryAbandoned, "feature_disabled", &admittedAt)
	assertAsyncQuestionDelivery(t, repo, ownerQuestionID, backoffID, models.AsyncQuestionDeliveryOwner, models.AsyncQuestionDeliveryAbandoned, "feature_disabled", nil)
	assertAsyncQuestionDelivery(t, repo, ownerQuestionID, transcriptID, models.AsyncQuestionDeliveryTranscript, models.AsyncQuestionDeliveryPending, "", nil)
	assertAsyncQuestionDelivery(t, repo, ownerQuestionID, historyID, models.AsyncQuestionDeliveryOwner, models.AsyncQuestionDeliveryAdmitted, "", nil)
	assertAsyncQuestionDelivery(t, repo, acceptedQuestionID, acceptedID, models.AsyncQuestionDeliveryOwner, models.AsyncQuestionDeliveryAdmitted, "", nil)
	if _, err := repo.GetAsyncQuestion(ctx, parentWait.DependencyIDs[0]); err != nil {
		t.Fatalf("parent request history after disabled startup: %v", err)
	}
	_ = acceptedWait
}

func TestAsyncQuestionsDisabledRecoveryDoesNotReplay(t *testing.T) {
	ctx := context.Background()
	repo, queue, queueRepo := newAsyncQuestionBootRepositories(t)
	wait := newAsyncQuestionBootSession(t, repo, "task-uncertain-question", "session-uncertain-question", models.AsyncQuestionWaitRequested)
	questionID := wait.DependencyIDs[0]
	identity := messagequeue.QueueSessionIdentity{
		TaskID: "task-uncertain-question", SessionID: "session-uncertain-question",
		SessionIncarnationID: "inc-session-uncertain-question",
	}
	metadata := map[string]interface{}{
		messagequeue.MetadataAsyncQuestion: questionID,
		"async_question_notification":      true,
	}
	const admissionID = "admission-uncertain-question"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: admissionID, QuestionID: questionID, Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: identity.TaskID, DestinationSession: identity.SessionID,
		DestinationIncarnationID: identity.SessionIncarnationID, Content: "uncertain prompt",
		Metadata: metadata, State: models.AsyncQuestionDeliveryPending,
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := queue.QueueMessageWithMetadataForSessionWithClientQueueID(
		ctx, identity, admissionID, "uncertain prompt", "", "async-question", false, nil, metadata, nil,
	); err != nil {
		t.Fatal(err)
	}
	ordinary := &messagequeue.QueuedMessage{
		ID: "ordinary-after-uncertain-question", SessionID: identity.SessionID, TaskID: identity.TaskID,
		Content: "ordinary prompt", QueuedBy: messagequeue.QueuedByUser,
	}
	if err := queueRepo.InsertForSession(ctx, identity, ordinary, 20); err != nil {
		t.Fatal(err)
	}
	claimed, exists, _, err := queue.ReserveQueuedWithAutoRunForSession(ctx, identity)
	if err != nil || !exists || claimed == nil || claimed.ID != admissionID {
		t.Fatalf("initial async reservation = (%#v, %t, %v)", claimed, exists, err)
	}
	state, _, err := queue.AsyncQuestionAdmissionDisposition(ctx, admissionID, questionID)
	if err != nil || state != messagequeue.AsyncQuestionAdmissionUncertain {
		t.Fatalf("pre-startup queue claim state = (%s, %v), want uncertain", state, err)
	}

	service := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("off PrepareStartup: %v", err)
	}
	assertAsyncQuestionDelivery(t, repo, questionID, admissionID, models.AsyncQuestionDeliveryOwner, models.AsyncQuestionDeliveryAbandoned, "feature_disabled", nil)
	claims, err := queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(claims) != 1 || claims[0].Accepted || claims[0].Message.ID != admissionID {
		t.Fatalf("uncertain claim after disabled boot = (%#v, %v), want parked original claim", claims, err)
	}
	entries, err := queueRepo.ListBySession(ctx, identity.SessionID)
	if err != nil || len(entries) != 1 || entries[0].ID != ordinary.ID {
		t.Fatalf("ordinary queue after disabled boot = (%#v, %v), want preserved ordinary prompt", entries, err)
	}

	service.SetAsyncQuestionsEnabled(true)
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("enabled PrepareStartup: %v", err)
	}
	claims, err = queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(claims) != 1 || claims[0].Message.ID != admissionID {
		t.Fatalf("uncertain claim after re-enable = (%#v, %v), want unchanged parked claim", claims, err)
	}
	if err := service.restorePendingQueueDispatchesForRecovery(ctx, identity.SessionID); err != nil {
		t.Fatalf("explicit recovery: %v", err)
	}
	claims, err = queue.ListPendingQueueDispatches(ctx)
	if err != nil || len(claims) != 0 {
		t.Fatalf("claims after explicit recovery = (%#v, %v), want retired claim cleared", claims, err)
	}
	if _, err := queue.FindEntryByID(ctx, admissionID); !errors.Is(err, messagequeue.ErrEntryNotFound) {
		t.Fatalf("retired prompt after recovery = %v, want not found", err)
	}
	ordinaryDrained, drained := queue.TakeQueuedIfAutoRun(ctx, identity.SessionID)
	if !drained || ordinaryDrained.ID != ordinary.ID {
		t.Fatalf("ordinary prompt after recovery = (%#v, %t), want drainable ordinary work", ordinaryDrained, drained)
	}
}

func TestAsyncQuestionsDisabledRestartPreservesRecords(t *testing.T) {
	ctx := context.Background()
	repo, queue, _ := newAsyncQuestionBootRepositories(t)
	wait := newAsyncQuestionBootSession(t, repo, "task-history-preserved", "session-history-preserved", models.AsyncQuestionWaitRequested)
	questionID := wait.DependencyIDs[0]
	question, err := repo.GetAsyncQuestion(ctx, questionID)
	if err != nil {
		t.Fatal(err)
	}
	outcome := models.AsyncQuestionOutcome{
		Rejected: true, RejectReason: "not needed", ResponderKind: models.AsyncQuestionResponderUser,
		ResponderID: "operator-1", RespondedAt: time.Now().UTC(), Version: question.Version + 1,
	}
	resolved, won, err := repo.ResolveAsyncQuestion(ctx, questionID, outcome, nil)
	if err != nil || !won || resolved.Outcome == nil {
		t.Fatalf("seed answer history = (%#v, %t, %v)", resolved, won, err)
	}
	transcriptID := "transcript-history-preserved"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: transcriptID, QuestionID: questionID, Kind: models.AsyncQuestionDeliveryTranscript,
		DestinationTaskID: question.TaskID, DestinationSession: question.SessionID,
		Content: "transcript body", State: models.AsyncQuestionDeliveryPending,
	}); err != nil {
		t.Fatal(err)
	}
	historyID := "answer-history-preserved"
	if err := repo.CreateAsyncQuestionDelivery(ctx, &models.AsyncQuestionDelivery{
		AdmissionID: historyID, QuestionID: questionID, Kind: models.AsyncQuestionDeliveryOwner,
		DestinationTaskID: question.TaskID, DestinationSession: question.SessionID,
		Content: "answer body", State: models.AsyncQuestionDeliveryPending,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAsyncQuestionDeliveryAdmitted(ctx, historyID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	service := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := service.PrepareStartup(ctx); err != nil {
		t.Fatalf("PrepareStartup: %v", err)
	}
	stored, err := repo.GetAsyncQuestion(ctx, questionID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != models.AsyncQuestionStatusRejected || stored.Outcome == nil || stored.Outcome.RejectReason != outcome.RejectReason {
		t.Fatalf("question outcome after disabled restart = %+v, want preserved rejection", stored)
	}
	storedWait, err := repo.GetAsyncQuestionWait(ctx, wait.ID)
	if err != nil || storedWait.Status != models.AsyncQuestionWaitCancelled || len(storedWait.DependencyIDs) != 1 || storedWait.DependencyIDs[0] != questionID {
		t.Fatalf("question wait after disabled restart = (%#v, %v), want cancelled with dependency history", storedWait, err)
	}
	assertAsyncQuestionDelivery(t, repo, questionID, transcriptID, models.AsyncQuestionDeliveryTranscript, models.AsyncQuestionDeliveryPending, "", nil)
	assertAsyncQuestionDelivery(t, repo, questionID, historyID, models.AsyncQuestionDeliveryOwner, models.AsyncQuestionDeliveryAdmitted, "", nil)
}

func TestAsyncQuestionsReenableKeepsValidAuthority(t *testing.T) {
	ctx := context.Background()
	repo, queue, _ := newAsyncQuestionBootRepositories(t)
	wait := newAsyncQuestionBootSession(t, repo, "task-reenabled-owner", "session-reenabled-owner", models.AsyncQuestionWaitRequested)
	questionID := wait.DependencyIDs[0]
	offService := &Service{repo: repo, logger: testLogger(), messageQueue: queue, turnService: &housekeepingTurns{}}
	if err := offService.PrepareStartup(ctx); err != nil {
		t.Fatalf("off PrepareStartup: %v", err)
	}

	questionsService := questions.NewService(repo, nil, zap.NewNop())
	answered, won, err := questionsService.ResolveOperator(ctx, questionID, questions.AnswerInput{
		Answers:      []questions.Answer{{QuestionID: "choice", SelectedOptions: []string{"one"}}},
		SubmissionID: "answer-after-reenable",
	})
	if err != nil || !won || answered.Status != models.AsyncQuestionStatusAnswered || answered.Outcome == nil {
		t.Fatalf("eligible retained request after re-enable = (%#v, %t, %v)", answered, won, err)
	}

	parentTaskID := "task-reenabled-parent-old"
	newAsyncQuestionBootSession(t, repo, parentTaskID, "session-reenabled-parent-old", models.AsyncQuestionWaitReleased)
	newAsyncQuestionBootSession(t, repo, "task-reenabled-parent-new", "session-reenabled-parent-new", models.AsyncQuestionWaitReleased)
	if _, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", parentTaskID, "task-reenabled-owner"); err != nil {
		t.Fatal(err)
	}
	parentQuestion := &models.AsyncQuestion{
		ID: "parent-question-after-reenable", TaskID: "task-reenabled-owner", WorkspaceID: "workspace-task-reenabled-owner",
		SessionID: "session-reenabled-owner", SessionIncarnationID: "inc-session-reenabled-owner",
		TurnID: "turn-session-reenabled-owner", Recipient: models.AsyncQuestionRecipientParent,
		ParentTaskID: parentTaskID, ParentSessionID: "session-reenabled-parent-old",
		ParentSessionIncarnationID: "inc-session-reenabled-parent-old",
		Questions:                  []protocol.Question{{ID: "parent-choice", Prompt: "Choose", Options: []protocol.Option{{ID: "one", Label: "One"}}}},
		RetryKey:                   "parent-after-reenable", PayloadDigest: "parent-after-reenable", Status: models.AsyncQuestionStatusPending,
	}
	if _, _, err := repo.CreateAsyncQuestion(ctx, parentQuestion, nil); err != nil {
		t.Fatalf("create parent request: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, "UPDATE tasks SET parent_id = ? WHERE id = ?", "task-reenabled-parent-new", "task-reenabled-owner"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := questionsService.ResolveParent(ctx, parentQuestion.ID, "task-reenabled-parent-new", "session-reenabled-parent-new", questions.AnswerInput{Text: "new parent answer"}); !errors.Is(err, questions.ErrUnauthorized) {
		t.Fatalf("changed direct parent answer error = %v, want unauthorized", err)
	}
	if _, _, err := questionsService.ResolveParent(ctx, parentQuestion.ID, parentTaskID, "session-reenabled-parent-old", questions.AnswerInput{Text: "old parent answer"}); !errors.Is(err, questions.ErrUnauthorized) {
		t.Fatalf("former parent answer error = %v, want unauthorized after authority recheck", err)
	}
	stale, err := repo.GetAsyncQuestion(ctx, parentQuestion.ID)
	if err != nil || stale.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("reparented request after attempted answers = (%#v, %v), want stale", stale, err)
	}

	replacedWait := newAsyncQuestionBootSession(t, repo, "task-replaced-author", "session-replaced-author", models.AsyncQuestionWaitReleased)
	replacedQuestionID := replacedWait.DependencyIDs[0]
	if _, err := repo.DB().ExecContext(ctx, "UPDATE task_sessions SET queue_incarnation_id = ? WHERE id = ?", "replacement-incarnation", "session-replaced-author"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := questionsService.ResolveOperator(ctx, replacedQuestionID, questions.AnswerInput{
		Answers: []questions.Answer{{QuestionID: "choice", SelectedOptions: []string{"one"}}}, SubmissionID: "replaced-author-answer",
	}); !errors.Is(err, questions.ErrUnauthorized) {
		t.Fatalf("replaced author answer error = %v, want unauthorized", err)
	}
	replaced, err := repo.GetAsyncQuestion(ctx, replacedQuestionID)
	if err != nil || replaced.Status != models.AsyncQuestionStatusStale {
		t.Fatalf("replaced author's request after answer attempt = (%#v, %v), want stale", replaced, err)
	}
}

func queueRepoInsert(
	repo messagequeue.Repository,
	identity messagequeue.QueueSessionIdentity,
	message *messagequeue.QueuedMessage,
) error {
	return repo.InsertForSession(context.Background(), identity, message, 20)
}

func assertAsyncQuestionDelivery(
	t *testing.T,
	repo *tasksqlite.Repository,
	questionID string,
	admissionID string,
	kind models.AsyncQuestionDeliveryKind,
	wantState models.AsyncQuestionDeliveryState,
	wantLastError string,
	wantAdmittedAt *time.Time,
) {
	t.Helper()
	var state, lastError string
	var admittedAt *time.Time
	rows, err := repo.ListAsyncQuestionDeliveriesForQuestion(context.Background(), questionID, kind)
	if err != nil {
		t.Fatal(err)
	}
	for _, delivery := range rows {
		if delivery.AdmissionID != admissionID {
			continue
		}
		state, lastError = string(delivery.State), delivery.LastError
		admittedAt = delivery.AdmittedAt
		if state != string(wantState) || lastError != wantLastError {
			t.Fatalf("delivery %s state/error = (%s, %s), want (%s, %s)", admissionID, state, lastError, wantState, wantLastError)
		}
		if wantAdmittedAt != nil && (admittedAt == nil || !admittedAt.Equal(*wantAdmittedAt)) {
			t.Fatalf("delivery %s admitted_at = %v, want %s", admissionID, admittedAt, wantAdmittedAt)
		}
		return
	}
	t.Fatalf("delivery %s not found", admissionID)
}
