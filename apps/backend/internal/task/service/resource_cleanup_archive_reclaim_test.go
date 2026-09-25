package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
)

func TestArchiveReclaimWaitsForCleanCheckoutThenRemovesIt(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, manager, sourcePath, wt := createArchiveReclaimFixture(t)
	svc.StopTaskResourceCleanupWorker()
	serviceCleanupDone := make(chan struct{}, 1)
	svc.setCleanupDoneForTestHook(serviceCleanupDone)
	sentinel := filepath.Join(wt.Path, "untracked-recovery.txt")
	if err := os.WriteFile(sentinel, []byte("keep this work\n"), 0o600); err != nil {
		t.Fatalf("write dirty worktree marker: %v", err)
	}

	if err := svc.ArchiveTask(ctx, wt.TaskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	archiveJob := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchive)
	if err := svc.processTaskResourceCleanupJob(ctx, archiveJob.ID); err != nil {
		t.Fatalf("process archive cleanup: %v", err)
	}
	waitForCleanupDone(t, svc)

	archiveJob, err := repo.GetTaskResourceCleanupJob(ctx, archiveJob.ID)
	if err != nil {
		t.Fatalf("reload archive cleanup job: %v", err)
	}
	if archiveJob.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("archive cleanup state = %q, want succeeded before follow-up", archiveJob.State)
	}
	reclaimJob := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if reclaimJob.State != models.TaskResourceCleanupStatePending {
		t.Fatalf("archive reclaim state = %q, want pending", reclaimJob.State)
	}

	if err := svc.processTaskResourceCleanupJob(ctx, reclaimJob.ID); err != nil {
		t.Fatalf("defer dirty reclaim: %v", err)
	}
	reclaimJob, err = repo.GetTaskResourceCleanupJob(ctx, reclaimJob.ID)
	if err != nil {
		t.Fatalf("reload deferred reclaim: %v", err)
	}
	if reclaimJob.State != models.TaskResourceCleanupStateWaitingForClean || reclaimJob.NextAttemptAt == nil {
		t.Fatalf("dirty reclaim state = %q next=%v, want waiting_for_clean with a due time", reclaimJob.State, reclaimJob.NextAttemptAt)
	}
	if time.Until(*reclaimJob.NextAttemptAt) < 23*time.Hour {
		t.Fatalf("dirty reclaim next attempt = %v, want approximately 24 hours from now", reclaimJob.NextAttemptAt)
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep this work\n" {
		t.Fatalf("dirty worktree marker = %q, %v; want it preserved", got, err)
	}
	if persisted, err := manager.GetByID(ctx, wt.ID); err != nil || persisted.Status != worktree.StatusActive {
		t.Fatalf("dirty worktree row = %+v, %v; want active", persisted, err)
	}

	if err := os.Remove(sentinel); err != nil {
		t.Fatalf("clean worktree marker: %v", err)
	}
	if err := repo.CompleteTaskResourceCleanupJob(
		ctx, reclaimJob.ID, models.TaskResourceCleanupStatePending, "", nil,
	); err != nil {
		t.Fatalf("make reclaim due: %v", err)
	}
	if err := svc.processTaskResourceCleanupJob(ctx, reclaimJob.ID); err != nil {
		t.Fatalf("reclaim clean checkout: %v", err)
	}
	reclaimJob, err = repo.GetTaskResourceCleanupJob(ctx, reclaimJob.ID)
	if err != nil {
		t.Fatalf("reload successful reclaim: %v", err)
	}
	if reclaimJob.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("clean reclaim state = %q, want succeeded", reclaimJob.State)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clean archived worktree still exists: %v", err)
	}
	if branches := strings.TrimSpace(string(runGitTestCmd(t, sourcePath, "branch", "--list", wt.Branch))); branches == "" {
		t.Fatal("archive reclaim removed the local branch")
	}
}

func TestResumeBackfillsAndReclaimsArchivedWorktreeWithoutArchiveJob(t *testing.T) {
	ctx := context.Background()
	_, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	if err := repo.ArchiveTask(ctx, wt.TaskID); err != nil {
		t.Fatalf("archive task without a cleanup job: %v", err)
	}

	// A fresh service has no in-memory archive event to recover. Startup
	// reconciliation discovers the active row and the task cleanup worker
	// performs the clean recheck without storage maintenance scheduling.
	restarted := newServiceOverRepository(t, repo)
	manager := newCleanupTestWorktreeManager(t, repo)
	manager.SetRepositoryProvider(worktree.NewRepositoryAdapter(repo))
	restarted.SetWorktreeCleanup(manager)
	if err := restarted.ResumeTaskResourceCleanupJobs(ctx); err != nil {
		t.Fatalf("resume task cleanup jobs: %v", err)
	}
	job := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if job.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("backfilled reclaim state = %q, want succeeded", job.State)
	}
	if _, err := os.Stat(wt.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backfilled clean worktree still exists: %v", err)
	}
}

func TestArchiveReclaimBackfillUsesBoundedIdempotentCursorAcrossRestart(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	const taskID = "task-archive-reclaim-backfill-batch"
	seedCleanupTaskAndSession(t, repo, taskID, "session-archive-reclaim-backfill-batch")
	const count = archiveReclaimBackfillBatchSize + 1
	worktreeRepos := make([]*models.TaskEnvironmentRepo, 0, count)
	for i := 0; i < count; i++ {
		repositoryID := fmt.Sprintf("repo-reclaim-batch-%03d", i)
		repositoryPath := filepath.Join(t.TempDir(), repositoryID)
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: repositoryID, WorkspaceID: "ws-" + taskID, Name: repositoryID, LocalPath: repositoryPath,
		}); err != nil {
			t.Fatalf("create repository %s: %v", repositoryID, err)
		}
		worktreeRepos = append(worktreeRepos, &models.TaskEnvironmentRepo{
			ID: "env-repo-" + repositoryID, RepositoryID: repositoryID,
			WorktreeID:   fmt.Sprintf("wt-reclaim-batch-%03d", i),
			WorktreePath: filepath.Join(repositoryPath, "worktree"), Status: worktree.StatusActive,
		})
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady, Repos: worktreeRepos,
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}

	if err := svc.reconcileArchivedWorktreeReclaimCandidates(ctx); err != nil {
		t.Fatalf("first bounded backfill page: %v", err)
	}
	restarted := newServiceOverRepository(t, repo)
	if err := restarted.reconcileArchivedWorktreeReclaimCandidates(ctx); err != nil {
		t.Fatalf("repeat first page after restart: %v", err)
	}
	if err := restarted.reconcileArchivedWorktreeReclaimCandidates(ctx); err != nil {
		t.Fatalf("second bounded backfill page: %v", err)
	}
	if err := restarted.reconcileArchivedWorktreeReclaimCandidates(ctx); err != nil {
		t.Fatalf("finish bounded backfill cursor: %v", err)
	}
	var jobs int
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM task_resource_cleanup_jobs WHERE task_id = ? AND trigger = ?
	`, taskID, models.TaskResourceCleanupTriggerArchiveReclaim).Scan(&jobs); err != nil {
		t.Fatalf("count reclaim jobs: %v", err)
	}
	if jobs != count {
		t.Fatalf("backfilled reclaim jobs = %d, want %d", jobs, count)
	}
}

type archiveReclaimWorktreeFake struct {
	mu        sync.Mutex
	worktrees map[string][]*worktree.Worktree
	dirty     bool
	refs      int
	entered   chan struct{}
	release   chan struct{}
	enterOnce sync.Once
	cleanups  int
}

func (f *archiveReclaimWorktreeFake) OnTaskDeleted(context.Context, string) error { return nil }

func (f *archiveReclaimWorktreeFake) GetAllByTaskID(_ context.Context, taskID string) ([]*worktree.Worktree, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*worktree.Worktree(nil), f.worktrees[taskID]...), nil
}

func (f *archiveReclaimWorktreeFake) CleanupWorktrees(context.Context, []*worktree.Worktree) error {
	return nil
}

func (f *archiveReclaimWorktreeFake) CleanupWorktreesPreservingBranches(context.Context, []*worktree.Worktree) error {
	return nil
}

func (f *archiveReclaimWorktreeFake) CountActiveWorktreeReferences(ctx context.Context, _ string, _ []string) (int, error) {
	if f.entered != nil {
		f.enterOnce.Do(func() { close(f.entered) })
		select {
		case <-f.release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.refs, nil
}

func (*archiveReclaimWorktreeFake) ReleaseWorktreeReference(context.Context, *worktree.Worktree) error {
	return nil
}

func (f *archiveReclaimWorktreeFake) InspectDirtyWorktrees(_ context.Context, worktrees []*worktree.Worktree) ([]worktree.DirtyWorktree, error) {
	f.mu.Lock()
	dirty := f.dirty
	f.mu.Unlock()
	if !dirty {
		return nil, nil
	}
	items := make([]worktree.DirtyWorktree, 0, len(worktrees))
	for _, wt := range worktrees {
		items = append(items, worktree.DirtyWorktree{WorktreeID: wt.ID, DirtyFiles: []string{"private.txt"}})
	}
	return items, nil
}

func (f *archiveReclaimWorktreeFake) CleanupArchivedWorktree(
	_ context.Context, wt *worktree.Worktree, taskID, _, _ string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cleanups++
	worktrees := f.worktrees[taskID]
	for i, current := range worktrees {
		if current != nil && current.ID == wt.ID {
			f.worktrees[taskID] = append(worktrees[:i], worktrees[i+1:]...)
			break
		}
	}
	return nil
}

func TestUnarchiveRefusesRunningArchiveReclaim(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	const taskID = "task-running-archive-reclaim"
	seedCleanupTaskAndSession(t, repo, taskID, "session-running-archive-reclaim")
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.ArchivedAt == nil {
		t.Fatalf("load archived task: task=%+v err=%v", task, err)
	}
	wt := &worktree.Worktree{
		ID: "wt-running-archive-reclaim", TaskID: taskID, RepositoryID: "repo-running-archive-reclaim",
		RepositoryPath: "/tmp/reclaim-repo", Path: "/tmp/reclaim-repo/worktree", Status: worktree.StatusActive,
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: wt.RepositoryID, WorkspaceID: "ws-" + taskID, Name: wt.RepositoryID, LocalPath: wt.RepositoryPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree", WorkspacePath: "/tmp/reclaim-repo",
		Status: models.TaskEnvironmentStatusReady, Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-" + wt.ID, RepositoryID: wt.RepositoryID, WorktreeID: wt.ID,
			WorktreePath: wt.Path, Status: worktree.StatusActive,
		}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	snapshot := fmt.Sprintf(`{"worktree_id":%q,"worktree_path":%q,"repository_path":%q,"archived_at":%q}`,
		wt.ID, wt.Path, wt.RepositoryPath, task.ArchivedAt.UTC().Format(time.RFC3339Nano))
	job := &models.TaskResourceCleanupJob{
		OperationID: "archive_reclaim:" + taskID + ":" + wt.ID + ":running",
		TaskID:      taskID, Trigger: models.TaskResourceCleanupTriggerArchiveReclaim,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: snapshot,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("create reclaim job: %v", err)
	}
	fake := &archiveReclaimWorktreeFake{
		worktrees: map[string][]*worktree.Worktree{taskID: {wt}},
		entered:   make(chan struct{}), release: make(chan struct{}),
	}
	svc.SetWorktreeCleanup(fake)
	processDone := make(chan error, 1)
	go func() { processDone <- svc.processTaskResourceCleanupJob(ctx, job.ID) }()
	select {
	case <-fake.entered:
	case <-time.After(time.Second):
		t.Fatal("archive reclaim did not reach its reference check")
	}
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	if _, err := handoff.UnarchiveTaskTree(ctx, taskID); !errors.Is(err, ErrCleanupCancellationRace) {
		t.Fatalf("UnarchiveTaskTree error = %v, want cleanup cancellation race", err)
	}
	close(fake.release)
	if err := <-processDone; err != nil {
		t.Fatalf("archive reclaim processor: %v", err)
	}
	stillArchived, err := repo.GetTask(ctx, taskID)
	if err != nil || stillArchived == nil || stillArchived.ArchivedAt == nil {
		t.Fatalf("task after rejected unarchive = %+v, %v; want archived", stillArchived, err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || got.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("archive reclaim after unarchive race = %+v, %v; want safe completion", got, err)
	}
}

func TestCascadeUnarchiveRefusesRunningArchiveReclaim(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	const (
		workspaceID   = "ws-cascade-running-reclaim"
		workflowID    = "wf-cascade-running-reclaim"
		rootID        = "task-cascade-running-reclaim-root"
		childID       = "task-cascade-running-reclaim-child"
		sessionID     = "session-cascade-running-reclaim-child"
		repositoryID  = "repo-cascade-running-reclaim"
		environmentID = "env-cascade-running-reclaim-child"
	)
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: workspaceID}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: workflowID}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	for _, task := range []*models.Task{
		{ID: rootID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step", Title: rootID, Priority: "medium"},
		{ID: childID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step", Title: childID, Priority: "medium", ParentID: rootID},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: childID, State: models.TaskSessionStateCompleted}); err != nil {
		t.Fatalf("create child session: %v", err)
	}
	repositoryPath := t.TempDir()
	worktreePath := filepath.Join(repositoryPath, "child-worktree")
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: workspaceID, Name: repositoryID, LocalPath: repositoryPath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	wt := &worktree.Worktree{
		ID: "wt-cascade-running-reclaim", TaskID: childID, RepositoryID: repositoryID,
		RepositoryPath: repositoryPath, Path: worktreePath, Status: worktree.StatusActive,
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: childID, ExecutorType: "worktree", WorkspacePath: repositoryPath,
		Status: models.TaskEnvironmentStatusReady, Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-cascade-running-reclaim", RepositoryID: repositoryID,
			WorktreeID: wt.ID, WorktreePath: wt.Path, Status: worktree.StatusActive,
		}},
	}); err != nil {
		t.Fatalf("create child environment: %v", err)
	}
	childSession, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load child session: %v", err)
	}
	childSession.TaskEnvironmentID = environmentID
	if err := repo.UpdateTaskSession(ctx, childSession); err != nil {
		t.Fatalf("link child session to environment: %v", err)
	}
	fake := &archiveReclaimWorktreeFake{
		worktrees: map[string][]*worktree.Worktree{childID: {wt}},
		entered:   make(chan struct{}), release: make(chan struct{}),
	}
	svc.SetWorktreeCleanup(fake)
	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	archived, err := handoff.ArchiveTaskTree(ctx, rootID, true)
	if err != nil {
		t.Fatalf("archive task tree: %v", err)
	}
	child, err := repo.GetTask(ctx, childID)
	if err != nil || child == nil || child.ArchivedAt == nil {
		t.Fatalf("load archived child: task=%+v err=%v", child, err)
	}
	snapshot, err := json.Marshal(archiveReclaimSnapshot{
		WorktreeID: wt.ID, WorktreePath: wt.Path, RepositoryPath: wt.RepositoryPath,
		ArchivedAt: child.ArchivedAt.UTC(),
	})
	if err != nil {
		t.Fatalf("encode reclaim snapshot: %v", err)
	}
	reclaimJob := &models.TaskResourceCleanupJob{
		ID: "job-cascade-running-reclaim", OperationID: "archive_reclaim:" + childID + ":" + wt.ID,
		TaskID: childID, Trigger: models.TaskResourceCleanupTriggerArchiveReclaim,
		State: models.TaskResourceCleanupStatePending, ResourceSnapshot: string(snapshot),
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, reclaimJob); err != nil {
		t.Fatalf("create reclaim job: %v", err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- svc.processTaskResourceCleanupJob(ctx, reclaimJob.ID) }()
	select {
	case <-fake.entered:
	case <-time.After(time.Second):
		close(fake.release)
		t.Fatal("archive reclaim did not reach its reference check")
	}
	unarchiveErr := error(nil)
	_, unarchiveErr = handoff.UnarchiveTaskTree(ctx, rootID)
	close(fake.release)
	if err := <-processDone; err != nil {
		t.Fatalf("archive reclaim processor: %v", err)
	}
	if !errors.Is(unarchiveErr, ErrCleanupCancellationRace) {
		t.Fatalf("UnarchiveTaskTree error = %v, want cleanup cancellation race", unarchiveErr)
	}
	for _, taskID := range []string{rootID, childID} {
		task, err := repo.GetTask(ctx, taskID)
		if err != nil || task == nil || task.ArchivedAt == nil || task.ArchivedByCascadeID != archived.CascadeID {
			t.Fatalf("task %s after rejected unarchive = %+v, %v; want archived by %s", taskID, task, err, archived.CascadeID)
		}
	}
	rootArchive, err := repo.GetTaskResourceCleanupJobByOperationID(ctx,
		string(models.TaskResourceCleanupTriggerCascadeArchive)+":"+archived.CascadeID+":"+rootID)
	if err != nil || rootArchive == nil || rootArchive.State != models.TaskResourceCleanupStatePrepared {
		t.Fatalf("root archive cleanup after rollback = %+v, %v; want restored prepared job", rootArchive, err)
	}
}

func TestCancelArchiveTaskResourceCleanupCancelsWaitingReclaim(t *testing.T) {
	ctx := context.Background()
	svc, _, repo := createTestService(t)
	svc.StopTaskResourceCleanupWorker()
	const taskID = "task-cancel-waiting-reclaim"
	seedCleanupTaskAndSession(t, repo, taskID, "session-cancel-waiting-reclaim")
	if err := repo.ArchiveTask(ctx, taskID); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	job := &models.TaskResourceCleanupJob{
		ID: "job-cancel-waiting-reclaim", OperationID: "archive_reclaim:" + taskID + ":worktree",
		TaskID: taskID, Trigger: models.TaskResourceCleanupTriggerArchiveReclaim,
		State: models.TaskResourceCleanupStateWaitingForClean, ResourceSnapshot: `{}`,
	}
	if err := repo.CreateTaskResourceCleanupJob(ctx, job); err != nil {
		t.Fatalf("create waiting reclaim job: %v", err)
	}
	if err := svc.CancelArchiveTaskResourceCleanup(ctx, taskID); err != nil {
		t.Fatalf("cancel archive cleanup: %v", err)
	}
	got, err := repo.GetTaskResourceCleanupJob(ctx, job.ID)
	if err != nil || got.State != models.TaskResourceCleanupStateCancelled {
		t.Fatalf("waiting reclaim state = %+v, %v; want cancelled", got, err)
	}
}

type pausedArchiveReclaimInsertRepository struct {
	taskrepository.TaskResourceCleanupRepository
	delegate *sqliterepo.Repository

	insertReached chan struct{}
	allowInsert   chan struct{}
	insertDone    chan struct{}

	snapshotRead  chan struct{}
	allowSnapshot chan struct{}
}

type archiveUnarchiveTestResult struct {
	outcome *CascadeOutcome
	err     error
}

func (r *pausedArchiveReclaimInsertRepository) CreateArchiveReclaimTaskResourceCleanupJob(
	ctx context.Context,
	job *models.TaskResourceCleanupJob,
	archivedAt time.Time,
) (bool, error) {
	close(r.insertReached)
	<-r.allowInsert
	created, err := r.delegate.CreateArchiveReclaimTaskResourceCleanupJob(ctx, job, archivedAt)
	close(r.insertDone)
	return created, err
}

func (r *pausedArchiveReclaimInsertRepository) ListArchiveTaskResourceCleanupJobs(
	ctx context.Context,
	taskID string,
) ([]*models.TaskResourceCleanupJob, error) {
	jobs, err := r.delegate.ListArchiveTaskResourceCleanupJobs(ctx, taskID)
	close(r.snapshotRead)
	<-r.allowSnapshot
	return jobs, err
}

func TestArchiveReclaimBackfillAndCascadeUnarchiveShareArchiveFence(t *testing.T) {
	for _, insertBeforeUnarchiveMutation := range []bool{true, false} {
		name := "insert after mutation is skipped"
		if insertBeforeUnarchiveMutation {
			name = "insert after cancellation scan blocks unarchive"
		}
		t.Run(name, func(t *testing.T) { runArchiveReclaimUnarchiveRace(t, insertBeforeUnarchiveMutation) })
	}
}

func runArchiveReclaimUnarchiveRace(t *testing.T, insertBeforeUnarchiveMutation bool) {
	t.Helper()
	ctx := context.Background()
	svc, _, repo, _, _, wt := createArchiveReclaimFixture(t)
	svc.StopTaskResourceCleanupWorker()
	const cascadeID = "cascade-backfill-unarchive-race"
	if changed, err := repo.ArchiveTaskIfActive(ctx, wt.TaskID, cascadeID); err != nil || !changed {
		t.Fatalf("archive task with cascade stamp: changed=%v err=%v", changed, err)
	}

	cleanupRepo := &pausedArchiveReclaimInsertRepository{
		TaskResourceCleanupRepository: repo,
		delegate:                      repo,
		insertReached:                 make(chan struct{}),
		allowInsert:                   make(chan struct{}),
		insertDone:                    make(chan struct{}),
		snapshotRead:                  make(chan struct{}),
		allowSnapshot:                 make(chan struct{}),
	}
	svc.resourceCleanups = cleanupRepo
	insertReleased, snapshotReleased := false, false
	defer func() {
		if !insertReleased {
			close(cleanupRepo.allowInsert)
		}
		if !snapshotReleased {
			close(cleanupRepo.allowSnapshot)
		}
	}()

	backfillDone := make(chan error, 1)
	go func() { backfillDone <- svc.reconcileArchivedWorktreeReclaimCandidates(ctx) }()
	waitForSignal(t, cleanupRepo.insertReached, "backfill candidate insertion")

	handoff := NewHandoffService(repo, repo, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)
	unarchiveDone := make(chan archiveUnarchiveTestResult, 1)
	go func() {
		outcome, err := handoff.UnarchiveTaskTree(ctx, wt.TaskID)
		unarchiveDone <- archiveUnarchiveTestResult{outcome: outcome, err: err}
	}()
	waitForSignal(t, cleanupRepo.snapshotRead, "archive cleanup cancellation snapshot")

	if insertBeforeUnarchiveMutation {
		finishBackfillBeforeUnarchiveMutation(t, cleanupRepo, backfillDone, unarchiveDone)
		insertReleased, snapshotReleased = true, true
	} else {
		finishBackfillAfterUnarchiveMutation(t, cleanupRepo, backfillDone, unarchiveDone)
		insertReleased, snapshotReleased = true, true
	}
	assertArchiveReclaimUnarchiveRaceOutcome(t, repo, wt.TaskID, cascadeID, insertBeforeUnarchiveMutation)
}

func finishBackfillBeforeUnarchiveMutation(
	t *testing.T,
	cleanupRepo *pausedArchiveReclaimInsertRepository,
	backfillDone <-chan error,
	unarchiveDone <-chan archiveUnarchiveTestResult,
) {
	t.Helper()
	close(cleanupRepo.allowInsert)
	waitForSignal(t, cleanupRepo.insertDone, "backfill insert commit")
	if err := <-backfillDone; err != nil {
		t.Fatalf("backfill candidate: %v", err)
	}
	close(cleanupRepo.allowSnapshot)
	result := waitForUnarchiveResult(t, unarchiveDone)
	if !errors.Is(result.err, ErrCleanupCancellationRace) {
		t.Fatalf("cascade unarchive error = %v, want cleanup cancellation race", result.err)
	}
}

func finishBackfillAfterUnarchiveMutation(
	t *testing.T,
	cleanupRepo *pausedArchiveReclaimInsertRepository,
	backfillDone <-chan error,
	unarchiveDone <-chan archiveUnarchiveTestResult,
) {
	t.Helper()
	close(cleanupRepo.allowSnapshot)
	result := waitForUnarchiveResult(t, unarchiveDone)
	if result.err != nil || result.outcome == nil {
		t.Fatalf("cascade unarchive = %+v, %v; want success", result.outcome, result.err)
	}
	close(cleanupRepo.allowInsert)
	waitForSignal(t, cleanupRepo.insertDone, "backfill insert after unarchive")
	if err := <-backfillDone; err != nil {
		t.Fatalf("backfill stale candidate: %v", err)
	}
}

func assertArchiveReclaimUnarchiveRaceOutcome(
	t *testing.T,
	repo *sqliterepo.Repository,
	taskID, cascadeID string,
	insertBeforeUnarchiveMutation bool,
) {
	t.Helper()
	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil || task == nil {
		t.Fatalf("reload task after unarchive race: task=%+v err=%v", task, err)
	}
	jobs, err := repo.ListArchiveTaskResourceCleanupJobs(context.Background(), taskID)
	if err != nil {
		t.Fatalf("list archive cleanup jobs: %v", err)
	}
	reclaimJobs := 0
	for _, job := range jobs {
		if job.Trigger != models.TaskResourceCleanupTriggerArchiveReclaim {
			continue
		}
		reclaimJobs++
		if insertBeforeUnarchiveMutation && job.State != models.TaskResourceCleanupStatePending {
			t.Errorf("race reclaim job state = %q, want pending while task remains archived", job.State)
		}
	}
	if insertBeforeUnarchiveMutation && (task.ArchivedAt == nil || task.ArchivedByCascadeID != cascadeID) {
		t.Errorf("task after fenced unarchive = %+v, want archived by %s", task, cascadeID)
	}
	if insertBeforeUnarchiveMutation && reclaimJobs != 1 {
		t.Errorf("reclaim jobs after cancelled snapshot = %d, want 1", reclaimJobs)
	}
	if !insertBeforeUnarchiveMutation && task.ArchivedAt != nil {
		t.Errorf("task after successful unarchive remains archived: %+v", task)
	}
	if !insertBeforeUnarchiveMutation && reclaimJobs != 0 {
		t.Errorf("reclaim jobs inserted after unarchive = %d, want none", reclaimJobs)
	}
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForUnarchiveResult(t *testing.T, result <-chan archiveUnarchiveTestResult) archiveUnarchiveTestResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for cascade unarchive")
		return archiveUnarchiveTestResult{}
	}
}

type lateDirtyCleanupScriptHandler struct{}

func (*lateDirtyCleanupScriptHandler) ExecuteSetupScript(context.Context, worktree.ScriptExecutionRequest) error {
	return nil
}

func (*lateDirtyCleanupScriptHandler) ExecuteCleanupScript(_ context.Context, req worktree.ScriptExecutionRequest) error {
	return os.WriteFile(filepath.Join(req.WorkingDir, "late-dirty-work.txt"), []byte("preserve after script\n"), 0o600)
}

func TestArchiveCleanupRetainsCheckoutDirtiedByCleanupScript(t *testing.T) {
	ctx := context.Background()
	svc, _, repo, manager, _, wt := createArchiveReclaimFixture(t)
	svc.StopTaskResourceCleanupWorker()
	manager.SetScriptMessageHandler(&lateDirtyCleanupScriptHandler{})
	repository, err := repo.GetRepository(ctx, wt.RepositoryID)
	if err != nil {
		t.Fatalf("load repository: %v", err)
	}
	repository.CleanupScript = "inject late dirt"
	if err := repo.UpdateRepository(ctx, repository); err != nil {
		t.Fatalf("set repository cleanup script: %v", err)
	}

	if err := svc.ArchiveTask(ctx, wt.TaskID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}
	archiveJob := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchive)
	if err := svc.processTaskResourceCleanupJob(ctx, archiveJob.ID); err != nil {
		t.Fatalf("process archive cleanup with late dirtiness: %v", err)
	}

	archiveJob, err = repo.GetTaskResourceCleanupJob(ctx, archiveJob.ID)
	if err != nil || archiveJob.State != models.TaskResourceCleanupStateSucceeded {
		t.Fatalf("archive cleanup job = %+v, %v; want succeeded after retaining late-dirty checkout", archiveJob, err)
	}
	reclaimJob := latestCleanupJob(t, repo, wt.TaskID, models.TaskResourceCleanupTriggerArchiveReclaim)
	if reclaimJob.State != models.TaskResourceCleanupStatePending {
		t.Fatalf("archive reclaim state = %q, want pending", reclaimJob.State)
	}
	lateDirtyFile := filepath.Join(wt.Path, "late-dirty-work.txt")
	if got, err := os.ReadFile(lateDirtyFile); err != nil || string(got) != "preserve after script\n" {
		t.Fatalf("late dirty file = %q, %v; want preserved", got, err)
	}
	if persisted, err := manager.GetByID(ctx, wt.ID); err != nil || persisted.Status != worktree.StatusActive {
		t.Fatalf("late-dirty worktree row = %+v, %v; want active", persisted, err)
	}
}

func createArchiveReclaimFixture(t *testing.T) (
	*Service, *MockEventBus, *sqliterepo.Repository, *worktree.Manager, string, *worktree.Worktree,
) {
	t.Helper()
	ctx := context.Background()
	svc, eventBus, repo := createTestService(t)
	const (
		taskID       = "task-archive-reclaim-flow"
		sessionID    = "session-archive-reclaim-flow"
		repositoryID = "repo-archive-reclaim-flow"
	)
	seedCleanupTaskAndSession(t, repo, taskID, sessionID)
	sourcePath := initSimpleGitRepo(t)
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: repositoryID, WorkspaceID: "ws-" + taskID, Name: repositoryID,
		SourceType: "local", LocalPath: sourcePath,
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	manager := newCleanupTestWorktreeManager(t, repo)
	wt, err := manager.Create(ctx, worktree.CreateRequest{
		TaskID: taskID, SessionID: sessionID, TaskTitle: "Archive reclaim flow",
		RepositoryID: repositoryID, RepositoryPath: sourcePath,
		BaseBranch: "main", TaskDirName: taskID, RepoName: repositoryID,
	})
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-" + taskID, TaskID: taskID, ExecutorType: "worktree",
		WorkspacePath: filepath.Dir(wt.Path), Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-repo-" + wt.ID, RepositoryID: repositoryID, BranchSlug: wt.BranchSlug,
			WorktreeID: wt.ID, WorktreePath: wt.Path, WorktreeBranch: wt.Branch, Status: worktree.StatusActive,
		}},
	}); err != nil {
		t.Fatalf("create task environment: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("load task session: %v", err)
	}
	session.TaskEnvironmentID = "env-" + taskID
	if err := repo.UpdateTaskSession(ctx, session); err != nil {
		t.Fatalf("link task session to environment: %v", err)
	}
	manager.SetRepositoryProvider(worktree.NewRepositoryAdapter(repo))
	svc.SetWorktreeCleanup(manager)
	return svc, eventBus, repo, manager, sourcePath, wt
}
