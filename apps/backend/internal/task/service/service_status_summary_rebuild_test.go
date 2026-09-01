package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
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

type cancelingRejectStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	summary      *statussummary.TaskStatusSummary
	cancel       context.CancelFunc
	compareCalls int
}

type exhaustingStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	summary      statussummary.TaskStatusSummary
	compareCalls int
}

type vanishingStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	compareCalls int
}

type failingStatusSummaryRepository struct {
	repository.TaskStatusSummaryRepository
	err        error
	failTaskID string
}

func (r *failingStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	if r.failTaskID == "" || stored.TaskID == r.failTaskID {
		return false, r.err
	}
	return r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, stored)
}

func (r *vanishingStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	ctx context.Context,
	stored *statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.compareCalls++
	if r.compareCalls == 1 {
		return false, nil
	}
	return r.TaskStatusSummaryRepository.CompareAndUpdateTaskStatusSummary(ctx, stored)
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

func (r *cancelingRejectStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	context.Context,
	*statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.compareCalls++
	if r.compareCalls == 1 {
		r.cancel()
	}
	return false, nil
}

func (r *cancelingRejectStatusSummaryRepository) LoadTaskStatusSummaries(
	context.Context,
	[]string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	return map[string]*statussummary.TaskStatusSummary{"task-1": r.summary}, nil
}

func (r *exhaustingStatusSummaryRepository) CompareAndUpdateTaskStatusSummary(
	context.Context,
	*statussummary.StoredTaskStatusSummary,
) (bool, error) {
	r.compareCalls++
	return false, nil
}

func (r *exhaustingStatusSummaryRepository) LoadTaskStatusSummaries(
	context.Context,
	[]string,
) (map[string]*statussummary.TaskStatusSummary, error) {
	summary := r.summary
	return map[string]*statussummary.TaskStatusSummary{"task-1": &summary}, nil
}

func (c statusSummaryQueuedPromptCounter) CountPendingByTaskIDs(_ context.Context, taskIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(taskIDs))
	for _, id := range taskIDs {
		out[id] = c.counts[id]
	}
	return out, nil
}

func TestReconcileTaskStatusSummariesRepairsMissingTaskOnce(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	now := time.Date(2026, 8, 1, 18, 0, 0, 0, time.UTC)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "environment-1", TaskID: "task-1", WorkspacePath: "/workspace/task-1",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	session := &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-1",
		TaskEnvironmentID: "environment-1",
		State:             models.TaskSessionStateRunning,
		IsPrimary:         true,
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
	task := &models.Task{
		ID: "task-1", WorkspaceID: "ws-1",
		Metadata: map[string]interface{}{
			models.MetaKeyLastLaunchError: models.TaskLaunchError{
				Message:          "The linked pull request is already closed or merged.",
				OccurredAt:       now.Add(time.Minute),
				Code:             models.LaunchErrorCategoryPRAlreadyClosed,
				RecoveryActions:  []string{models.RecoveryActionMarkReviewDone},
				TaskRepositoryID: "task-repository-1",
				StampValue:       "task-error-1",
			},
		},
	}
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
	if summary.ActiveError == nil || summary.ActiveError.Preview != "The linked pull request is already closed or merged." {
		t.Fatalf("active error = %+v", summary.ActiveError)
	}
	if summary.ActiveError.TaskRepositoryID != "task-repository-1" ||
		summary.ActiveError.Category != models.LaunchErrorCategoryPRAlreadyClosed {
		t.Fatalf("task-owned active error = %+v", summary.ActiveError)
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
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskStatusSummaryUpdated {
		t.Fatalf("missing-summary repair events = %+v, want one summary update", published)
	}

	if _, err := svc.ReconcileTaskStatusSummaries(ctx, []*models.Task{task}, sessions, pending, got); err != nil {
		t.Fatalf("second ReconcileTaskStatusSummaries: %v", err)
	}
	if prReader.calls != 1 {
		t.Fatalf("PR reader calls after existing summary = %d, want one", prReader.calls)
	}
	if published = eventBus.GetPublishedEvents(); len(published) != 1 {
		t.Fatalf("events after no-op reconcile = %+v, want no duplicate", published)
	}
}

func TestReconcileTaskStatusSummariesUsesNewestSnapshotOncePerSharedEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.statusSummaries = repo
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	const environmentID = "environment-summary-shared"
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: "task-1", WorkspacePath: "/workspace/summary-shared",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	oldAt := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	newAt := oldAt.Add(time.Minute)
	sessions := make([]*models.TaskSession, 0, 2)
	for _, session := range []*models.TaskSession{
		{ID: "summary-shared-old", TaskID: "task-1", TaskEnvironmentID: environmentID, State: models.TaskSessionStateCompleted, StartedAt: oldAt, UpdatedAt: oldAt},
		{ID: "summary-shared-new", TaskID: "task-1", TaskEnvironmentID: environmentID, State: models.TaskSessionStateCompleted, StartedAt: newAt, UpdatedAt: newAt},
	} {
		if err := repo.CreateTaskSession(ctx, session); err != nil {
			t.Fatalf("CreateTaskSession(%s): %v", session.ID, err)
		}
		sessions = append(sessions, session)
	}
	for _, snapshot := range []*models.GitSnapshot{
		{ID: "summary-snapshot-old", SessionID: sessions[0].ID, TaskEnvironmentID: environmentID, Files: map[string]interface{}{"old.go": true}, CreatedAt: oldAt, Metadata: map[string]interface{}{"repository_name": "shared", "branch_additions": 1}},
		{ID: "summary-snapshot-new", SessionID: sessions[1].ID, TaskEnvironmentID: environmentID, Files: map[string]interface{}{"new.go": true}, CreatedAt: newAt, Metadata: map[string]interface{}{"repository_name": "shared", "branch_additions": 9}},
	} {
		if err := repo.CreateGitSnapshot(ctx, snapshot); err != nil {
			t.Fatalf("CreateGitSnapshot(%s): %v", snapshot.ID, err)
		}
	}

	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	got, err := svc.ReconcileTaskStatusSummaries(
		ctx, []*models.Task{task}, map[string][]*models.TaskSession{task.ID: sessions},
		map[string]models.TaskPendingAction{}, map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got[task.ID] == nil || got[task.ID].Git == nil {
		t.Fatalf("summary = %+v, want Git summary", got[task.ID])
	}
	if got[task.ID].Git.Additions != 9 {
		t.Fatalf("Git additions = %d, want newest shared-environment value 9", got[task.ID].Git.Additions)
	}
}

func TestReconcileTaskStatusSummariesUsesTaskEnvironmentWithoutSessions(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.statusSummaries = repo
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	const environmentID = "environment-summary-without-sessions"
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: "task-1", WorkspacePath: "/workspace/summary-without-sessions",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}
	if err := repo.CreateGitSnapshot(ctx, &models.GitSnapshot{
		ID:                "snapshot-without-session",
		TaskEnvironmentID: environmentID,
		Files:             map[string]interface{}{"main.go": true},
		Metadata:          map[string]interface{}{"repository_name": "root", "branch_additions": 4},
		CreatedAt:         time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("CreateGitSnapshot: %v", err)
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}},
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	if got["task-1"] == nil || got["task-1"].Git == nil {
		t.Fatalf("summary = %+v, want Git summary from task-owned environment", got["task-1"])
	}
	if got["task-1"].Git.Additions != 4 || got["task-1"].Git.ChangedFiles != 1 {
		t.Fatalf("Git summary = %+v, want additions=4 and changed_files=1", got["task-1"].Git)
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

func TestStatusSummaryActivityRebuildBackfillsAndPreservesNewerStoredValue(t *testing.T) {
	svc, _, repo := createTestService(t)
	svc.statusSummaries = repo
	svc.taskActivity = repo
	queueDB := sqlx.NewDb(repo.DB(), "sqlite3")
	if _, err := messagequeue.NewSQLiteRepository(queueDB, queueDB); err != nil {
		t.Fatalf("initialize queue repository: %v", err)
	}
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	base := time.Date(2026, time.August, 17, 10, 0, 0, 0, time.UTC)
	expected := base.Add(3 * time.Hour)
	if _, err := repo.DB().Exec(
		`UPDATE tasks SET created_at = ?, updated_at = ? WHERE id = ?`,
		base, expected, "task-1",
	); err != nil {
		t.Fatalf("set task timestamps: %v", err)
	}

	tasks := []*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}}
	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		tasks,
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{},
	)
	if err != nil {
		t.Fatalf("rebuild missing status summary: %v", err)
	}
	backfilled := got["task-1"]
	if backfilled == nil || backfilled.LastActivityAt == nil || !backfilled.LastActivityAt.Equal(expected) {
		t.Fatalf("backfilled activity = %+v, want %v", backfilled, expected)
	}

	newer := expected.Add(time.Hour)
	stored := statussummary.TaskStatusSummary{
		Revision:       9,
		LastActivityAt: &newer,
	}
	accepted, err := repo.CompareAndUpdateTaskStatusSummary(ctx, &statussummary.StoredTaskStatusSummary{
		TaskID: "task-1", WorkspaceID: "ws-1", Summary: stored,
	})
	if err != nil || !accepted {
		t.Fatalf("seed newer summary: accepted=%v err=%v", accepted, err)
	}
	got, err = svc.ReconcileTaskStatusSummaries(
		ctx,
		tasks,
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"task-1": &stored},
	)
	if err != nil {
		t.Fatalf("reconcile newer status summary: %v", err)
	}
	preserved := got["task-1"]
	if preserved == nil || preserved.Revision != stored.Revision || preserved.LastActivityAt == nil || !preserved.LastActivityAt.Equal(newer) {
		t.Fatalf("newer stored activity = %+v, want revision %d at %v", preserved, stored.Revision, newer)
	}
}

func TestReconcileTaskStatusSummariesInvalidatesStaleSummaryOnRepairFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	repairErr := errors.New("status summary write failed")
	svc.statusSummaries = &failingStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		err:                         repairErr,
	}
	stored := &statussummary.TaskStatusSummary{Revision: 4}
	task := &models.Task{ID: "task-1", WorkspaceID: "ws-1"}
	session := &models.TaskSession{
		ID:     "session-1",
		TaskID: task.ID,
		State:  models.TaskSessionStateWaitingForInput,
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		context.Background(),
		[]*models.Task{task},
		map[string][]*models.TaskSession{task.ID: {session}},
		map[string]models.TaskPendingAction{
			session.ID: models.TaskPendingActionClarification,
		},
		map[string]*statussummary.TaskStatusSummary{task.ID: stored},
	)

	if !errors.Is(err, repairErr) {
		t.Fatalf("ReconcileTaskStatusSummaries error = %v, want %v", err, repairErr)
	}
	if summary, exists := got[task.ID]; !exists || summary != nil {
		t.Fatalf("stale summary = %+v, present=%v; want explicit invalidation", summary, exists)
	}
}

func TestReconcileTaskStatusSummariesRebuildsUnrelatedMissingSummaryAfterRepairFailure(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-2", WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
	}); err != nil {
		t.Fatalf("CreateTask(task-2): %v", err)
	}
	repairErr := errors.New("task-1 summary write failed")
	svc.statusSummaries = &failingStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		err:                         repairErr,
		failTaskID:                  "task-1",
	}
	tasks := []*models.Task{
		{ID: "task-1", WorkspaceID: "ws-1"},
		{ID: "task-2", WorkspaceID: "ws-1"},
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		tasks,
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"task-1": {Revision: 3, PendingAction: "clarification"}},
	)

	if !errors.Is(err, repairErr) {
		t.Fatalf("ReconcileTaskStatusSummaries error = %v, want %v", err, repairErr)
	}
	if summary, exists := got["task-1"]; !exists || summary != nil {
		t.Fatalf("failed task summary = %+v, present=%v; want explicit invalidation", summary, exists)
	}
	if got["task-2"] == nil {
		t.Fatal("unrelated missing task summary was not rebuilt")
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

func TestReconcileExistingSummaryStopsBeforeRetryWhenContextIsCanceled(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	stored := &statussummary.TaskStatusSummary{
		Revision:      4,
		PendingAction: string(models.TaskPendingActionClarification),
	}
	rejecting := &cancelingRejectStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		summary: &statussummary.TaskStatusSummary{
			Revision:      5,
			PendingAction: string(models.TaskPendingActionClarification),
		},
		cancel: cancel,
	}
	svc.statusSummaries = rejecting
	svc.sessions = &statusSummarySessionRepository{
		SessionRepository: repo,
		sessions: []*models.TaskSession{{
			ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateCompleted,
		}},
	}
	svc.messages = &authoritativePendingMessageRepository{MessageRepository: repo}

	_, err := svc.reconcileExistingSummary(
		ctx,
		&models.Task{ID: "task-1", WorkspaceID: "ws-1"},
		stored,
		"",
		time.Time{},
		false,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("reconcileExistingSummary error = %v, want context canceled", err)
	}
	if rejecting.compareCalls != 1 {
		t.Fatalf("compare calls after cancellation = %d, want 1", rejecting.compareCalls)
	}
}

func TestReconcileTaskStatusSummariesMarksStaleRowInvalidAfterCASExhaustion(t *testing.T) {
	svc, _, repo := createTestService(t)
	core, logs := observer.New(zapcore.WarnLevel)
	log, logErr := commonlogger.NewFromZap(zap.New(core))
	if logErr != nil {
		t.Fatalf("create logger: %v", logErr)
	}
	svc.logger = log
	stored := statussummary.TaskStatusSummary{Revision: 4, QueuedPromptCount: 9}
	exhausting := &exhaustingStatusSummaryRepository{
		TaskStatusSummaryRepository: repo,
		summary:                     stored,
	}
	svc.statusSummaries = exhausting
	session := &models.TaskSession{
		ID: "session-1", TaskID: "task-1", State: models.TaskSessionStateWaitingForInput,
	}
	svc.sessions = &statusSummarySessionRepository{
		SessionRepository: repo,
		sessions:          []*models.TaskSession{session},
	}
	svc.messages = &authoritativePendingMessageRepository{
		MessageRepository: repo,
		actions: map[string]models.TaskPendingAction{
			"session-1": models.TaskPendingActionClarification,
		},
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		context.Background(),
		[]*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}},
		map[string][]*models.TaskSession{"task-1": {session}},
		map[string]models.TaskPendingAction{"session-1": models.TaskPendingActionClarification},
		map[string]*statussummary.TaskStatusSummary{"task-1": &stored},
	)
	if err == nil {
		t.Fatal("CAS exhaustion error = nil")
	}
	if summary, exists := got["task-1"]; !exists || summary != nil {
		t.Fatalf("summary after CAS exhaustion = %+v, present=%v; want explicit invalidation", summary, exists)
	}
	if exhausting.compareCalls != maxSummaryReconcileAttempts {
		t.Fatalf("compare calls = %d, want %d", exhausting.compareCalls, maxSummaryReconcileAttempts)
	}
	entries := logs.FilterMessage("task status summary compare-and-set retries exhausted").All()
	if len(entries) != 1 {
		t.Fatalf("CAS exhaustion warning count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["task_id"] != "task-1" || fields["attempts"] != int64(maxSummaryReconcileAttempts) {
		t.Fatalf("CAS exhaustion warning fields = %#v", fields)
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

func TestReconcileTaskStatusSummariesRebuildsSummaryThatVanishesDuringRetry(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	vanishing := &vanishingStatusSummaryRepository{TaskStatusSummaryRepository: repo}
	svc.statusSummaries = vanishing
	stored := &statussummary.TaskStatusSummary{
		Revision:      4,
		PendingAction: string(models.TaskPendingActionClarification),
	}

	got, err := svc.ReconcileTaskStatusSummaries(
		ctx,
		[]*models.Task{{ID: "task-1", WorkspaceID: "ws-1"}},
		map[string][]*models.TaskSession{},
		map[string]models.TaskPendingAction{},
		map[string]*statussummary.TaskStatusSummary{"task-1": stored},
	)
	if err != nil {
		t.Fatalf("ReconcileTaskStatusSummaries: %v", err)
	}
	rebuilt := got["task-1"]
	if rebuilt == nil || rebuilt == stored || rebuilt.Revision != 1 || rebuilt.PendingAction != "" {
		t.Fatalf("summary after vanished retry = %+v, want rebuilt revision 1", rebuilt)
	}
	if vanishing.compareCalls != 2 {
		t.Fatalf("compare calls = %d, want reconcile plus rebuild", vanishing.compareCalls)
	}
}
