package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/statussummary"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type statusSummaryRebuildPRReader struct {
	calls int
}

func (r *statusSummaryRebuildPRReader) ListTaskStatusSummaryPullRequests(
	context.Context,
	[]string,
) (map[string][]statussummary.PullRequestInput, error) {
	r.calls++
	return map[string][]statussummary.PullRequestInput{
		"task-1": {{
			Key:         "repo-pr-1",
			State:       "open",
			Number:      42,
			URL:         "https://github.test/pr/42",
			ReviewState: "approved",
			ChecksState: "success",
		}},
	}, nil
}

type statusSummaryRebuildActivityProvider struct{}

func (statusSummaryRebuildActivityProvider) ForegroundActivity(string) v1.ForegroundActivity {
	return v1.ForegroundActivityGenerating
}

func (statusSummaryRebuildActivityProvider) ActiveSubagentCount(string) int { return 3 }

type statusSummaryQueuedPromptCounter struct {
	counts map[string]int
}

type rejectingStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	competing statussummary.StoredTaskStatusSummary
	rejected  bool
}

type vanishingStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	compareCalls int
}

func (r *vanishingStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	context.Context,
	*statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.compareCalls++
	return false, nil
}

func (r *vanishingStatusSummaryRepository) LoadTaskStatusSummaries(
	context.Context,
	[]string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	return map[string]*statussummary.TaskStatusSummary{}, nil
}

type authoritativePendingMessageRepository struct {
	repository.MessageRepository
	actions map[string]models.TaskPendingAction
	calls   int
}

type statusSummarySessionRepository struct {
	repository.SessionRepository
	sessions []*models.TaskSession
	calls    int
}

func (r *statusSummarySessionRepository) ListTaskSessions(
	context.Context,
	string,
) ([]*models.TaskSession, error) {
	r.calls++
	return r.sessions, nil
}

func (r *authoritativePendingMessageRepository) GetPendingActionsBySessionIDs(
	context.Context,
	[]string,
) (map[string]models.TaskPendingAction, error) {
	r.calls++
	return r.actions, nil
}

func (r *rejectingStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	if !r.rejected {
		r.rejected = true
		if _, err := r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, &r.competing); err != nil {
			return false, err
		}
		return false, nil
	}
	return r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

func (c statusSummaryQueuedPromptCounter) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = c.counts[id]
	}
	return out, nil
}

func TestReconcileTaskStatusSummariesRepairsMissingTaskOnce(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateRunning,
		IsPrimary: true,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyLastAgentError: map[string]interface{}{
				"message":     "agent failed to complete the turn",
				"occurred_at": now.Add(-time.Minute).Format(time.RFC3339Nano),
			},
		},
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:           "snapshot-1",
		SessionID:    session.ID,
		SnapshotType: models.SnapshotTypeStatusUpdate,
		Ahead:        2,
		Behind:       1,
		Files:        map[string]interface{}{"a.go": map[string]interface{}{}, "b.go": map[string]interface{}{}},
		TriggeredBy:  "agent_completed",
		Metadata: map[string]interface{}{
			"repository_name":  "kandev",
			"branch_additions": 5,
			"branch_deletions": 2,
		},
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	// createTestService predates the status-summary repository field, so wire
	// it explicitly here while keeping the shared fixture unchanged.
	svc.statusSummaries = repo
	svc.SetForegroundActivityProvider(statusSummaryRebuildActivityProvider{})
	prReader := &statusSummaryRebuildPRReader{}
	svc.SetTaskStatusSummaryPRReader(prReader)
	svc.SetQueuedPromptCounter(statusSummaryQueuedPromptCounter{counts: map[string]int{"task-1": 2}})
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	sessions := map[string][]*models.TaskSession{task.ID: {session}}
	pending := map[string]models.TaskPendingAction{session.ID: models.TaskPendingActionPermission}

	got, err := svc.ReconcileTaskStatusSummaries(
		ctx, []*models.Task{task}, sessions, pending, map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	summary := got[task.ID]
	if summary == nil {
		t.Fatal("repaired summary missing from returned map")
	}
	if summary.Revision != 1 || summary.PrimarySession == nil || summary.PrimarySession.ID != session.ID {
		t.Fatalf("repaired summary identity = %+v", summary)
	}
	if summary.PendingAction != string(models.TaskPendingActionPermission) {
		t.Fatalf("pending action = %q", summary.PendingAction)
	}
	if summary.ForegroundActivity != "generating" || summary.ActiveSubagentCount != 3 {
		t.Fatalf("activity summary = %+v", summary)
	}
	if summary.ActiveError == nil || summary.ActiveError.Preview != "agent failed to complete the turn" {
		t.Fatalf("active error = %+v", summary.ActiveError)
	}
	if summary.Git == nil || summary.Git.Additions != 5 || summary.Git.Deletions != 2 || summary.Git.ChangedFiles != 2 {
		t.Fatalf("Git summary = %+v", summary.Git)
	}
	if summary.PullRequest == nil || summary.PullRequest.Number != 42 || summary.PullRequest.AggregateState != "ready" {
		t.Fatalf("pull request summary = %+v", summary.PullRequest)
	}
	if summary.QueuedPromptCount != 2 {
		t.Fatalf("queued prompt count = %d, want 2 from the queued counter", summary.QueuedPromptCount)
	}
	if prReader.calls != 1 {
		t.Fatalf("PR reader calls = %d, want one batch read", prReader.calls)
	}

	persisted, err := repo.LoadTaskStatusSummaries(ctx, []string{task.ID})
	if err != nil {
		t.Fatalf("LoadTaskStatusSummaries: %v", err)
	}
	if persisted[task.ID] == nil || persisted[task.ID].Revision != 1 {
		t.Fatalf("persisted summary = %+v", persisted[task.ID])
	}

	if _, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, sessions, pending, got); err != nil {
		t.Fatalf("second ReconcileTaskStatusSummaries: %v", err)
	}
	if prReader.calls != 1 {
		t.Fatalf("PR reader calls after existing summary = %d, want one", prReader.calls)
	}
}

// The rebuild path is what runs after a restart, so it is where a stale record
// used to come back. A session whose error was cleared must rebuild clean.
func TestReconcileTaskStatusSummariesSkipsClearedAgentErrors(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-1",
		State:     models.TaskSessionStateWaitingForInput,
		IsPrimary: true,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	// The orchestrator writes JSON null on turn completion rather than deleting
	// the key, so the rebuild must treat that as "no failure".
	if err := repo.SetSessionMetadataKey(ctx, session.ID, models.SessionMetaKeyLastAgentError, nil); err != nil {
		t.Fatalf("clear last agent error: %v", err)
	}
	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}

	svc.statusSummaries = repo
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{task},
		map[string][]*models.TaskSession{task.ID: {stored}},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	summary := got[task.ID]
	if summary == nil {
		t.Fatal("repaired summary missing from returned map")
	}
	if summary.ActiveError != nil {
		t.Fatalf("active error = %+v, want a cleared record to stay invisible", summary.ActiveError)
	}
}

func TestReconcileTaskStatusSummariesRepairsExistingPendingOnly(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	svc.statusSummaries = repo

	storedAt := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	stored := statussummary.TaskStatusSummary{
		Revision:            7,
		UpdatedAt:           storedAt,
		PrimarySession:      &statussummary.PrimarySessionSummary{ID: "primary-1", State: "RUNNING"},
		ForegroundActivity:  "generating",
		ActiveSubagentCount: 2,
		PendingAction:       string(models.TaskPendingActionClarification),
		Git:                 &statussummary.GitSummary{ChangedFiles: 4, Ahead: 1},
		QueuedPromptCount:   3,
	}
	accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID:      "task-1",
		WorkspaceID: "ws-1",
		Summary:     stored,
	})
	if err != nil || !accepted {
		t.Fatalf("seed task status summary: accepted=%v err=%v", accepted, err)
	}
	eventBus.ClearEvents()

	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{task},
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"task-1": &stored},
	)
	if err != nil {
		t.Fatalf("repair existing pending state: %v", err)
	}
	repaired := got[task.ID]
	if repaired == nil || repaired.PendingAction != "" || repaired.Revision != 8 {
		t.Fatalf("repaired existing summary = %+v", repaired)
	}
	if repaired.PrimarySession == nil || repaired.PrimarySession.ID != "primary-1" ||
		repaired.ForegroundActivity != "generating" || repaired.ActiveSubagentCount != 2 ||
		repaired.Git == nil || repaired.Git.ChangedFiles != 4 || repaired.QueuedPromptCount != 3 {
		t.Fatalf("unrelated summary fields changed: %+v", repaired)
	}
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskStatusSummaryUpdated {
		t.Fatalf("repair events = %+v, want one complete summary update", published)
	}

	eventBus.ClearEvents()
	if _, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{task},
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		got,
	); err != nil {
		t.Fatalf("repeat existing pending repair: %v", err)
	}
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("no-op repair published events = %+v", eventBus.GetPublishedEvents())
	}
}

func TestReconcileTaskStatusSummariesReReadsPendingAfterCASRejection(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	stored := statussummary.TaskStatusSummary{
		Revision:      4,
		PendingAction: string(models.TaskPendingActionClarification),
	}
	accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "task-1", WorkspaceID: "ws-1", Summary: stored,
	})
	if err != nil || !accepted {
		t.Fatalf("seed task status summary: accepted=%v err=%v", accepted, err)
	}
	svc.statusSummaries = &rejectingStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		competing: statussummary.StoredTaskStatusSummary{
			TaskID:      "task-1",
			WorkspaceID: "ws-1",
			Summary: statussummary.TaskStatusSummary{
				Revision:          5,
				PendingAction:     string(models.TaskPendingActionClarification),
				QueuedPromptCount: 9,
			},
		},
	}
	pendingMessages := &authoritativePendingMessageRepository{
		MessageRepository: repo,
		actions: map[string]models.TaskPendingAction{
			"session-1": models.TaskPendingActionClarification,
		},
	}
	svc.messages = pendingMessages

	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: task.ID,
		State:  models.TaskSessionStateWaitingForInput,
	}
	sessionRepo := &statusSummarySessionRepository{
		SessionRepository: repo,
		sessions:          []*models.TaskSession{session},
	}
	svc.sessions = sessionRepo
	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{task},
		map[string][]*models.TaskSession{task.ID: {session}},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"task-1": &stored},
	)
	if err != nil {
		t.Fatalf("reconcile after CAS rejection: %v", err)
	}
	repaired := got[task.ID]
	if repaired == nil || repaired.Revision != 5 ||
		repaired.PendingAction != string(models.TaskPendingActionClarification) || repaired.QueuedPromptCount != 9 {
		t.Fatalf("summary after CAS rejection = %+v", repaired)
	}
	if pendingMessages.calls != 1 {
		t.Fatalf("authoritative pending reads = %d, want 1", pendingMessages.calls)
	}
	if sessionRepo.calls != 1 {
		t.Fatalf("authoritative session reads = %d, want 1", sessionRepo.calls)
	}
}

func TestReconcileTaskStatusSummariesReReadsSessionStateAfterCASRejection(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	stored := statussummary.TaskStatusSummary{Revision: 4}
	accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "task-1", WorkspaceID: "ws-1", Summary: stored,
	})
	if err != nil || !accepted {
		t.Fatalf("seed task status summary: accepted=%v err=%v", accepted, err)
	}
	svc.statusSummaries = &rejectingStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		competing: statussummary.StoredTaskStatusSummary{
			TaskID:      "task-1",
			WorkspaceID: "ws-1",
			Summary: statussummary.TaskStatusSummary{
				Revision:      5,
				PendingAction: string(models.TaskPendingActionClarification),
			},
		},
	}
	pendingMessages := &authoritativePendingMessageRepository{
		MessageRepository: repo,
		actions: map[string]models.TaskPendingAction{
			"session-1": models.TaskPendingActionClarification,
		},
	}
	svc.messages = pendingMessages
	sessionRepo := &statusSummarySessionRepository{
		SessionRepository: repo,
		sessions: []*models.TaskSession{{
			ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCompleted,
		}},
	}
	svc.sessions = sessionRepo
	staleSession := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}},
		map[string][]*models.TaskSession{"task-1": {staleSession}},
		map[string]models.TaskPendingAction{
			"session-1": models.TaskPendingActionClarification,
		},
		map[string]*statussummary.TaskStatusSummary{"task-1": &stored},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	repaired := got["task-1"]
	if repaired == nil || repaired.Revision != 6 || repaired.PendingAction != "" {
		t.Fatalf("summary after session-state refresh = %+v, want revision 6 without pending", repaired)
	}
	if pendingMessages.calls != 1 || sessionRepo.calls != 1 {
		t.Fatalf("retry refresh calls: pending=%d sessions=%d, want 1 each", pendingMessages.calls, sessionRepo.calls)
	}
}

func TestReconcileTaskStatusSummariesKeepsSnapshotWhenTaskVanishesDuringRetry(t *testing.T) {
	svc, _, repo := createTestService(t)
	vanishing := &vanishingStatusSummaryRepository{TaskStatusSummaryRepository: repo}
	svc.statusSummaries = vanishing
	stored := &statussummary.TaskStatusSummary{
		Revision:      4,
		PendingAction: string(models.TaskPendingActionClarification),
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		context.Background(),
		[]*models.Task{{ID: "deleted-task", WorkspaceID: "ws-1"}},
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"deleted-task": stored},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got["deleted-task"] != stored {
		t.Fatalf("summary after vanished retry = %+v, want original snapshot", got["deleted-task"])
	}
	if vanishing.compareCalls != 1 {
		t.Fatalf("compare calls = %d, want 1 without spurious rebuild", vanishing.compareCalls)
	}
}
