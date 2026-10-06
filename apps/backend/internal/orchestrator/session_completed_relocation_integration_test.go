//go:build unix

package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

const completedRelocationSessionErrorStamp = "completed-relocation-session-error"

type completedRelocationServiceSlot struct {
	repositoryID string
	worktreeID   string
	branch       string
	branchSlug   string
	sourceClone  string
	destination  string
	worktreePath string
}

type completedRelocationServiceFixture struct {
	db            *sqlx.DB
	repo          *tasksqlite.Repository
	store         *worktree.SQLiteStore
	cloner        *repoclone.Cloner
	config        worktree.Config
	manager       *worktree.Manager
	svc           *Service
	agent         *completedRelocationServiceAgentManager
	launchStarts  int
	taskID        string
	sessionID     string
	environmentID string
	taskDirName   string
	slots         []completedRelocationServiceSlot
}

type completedRelocationProviderStart struct {
	executionID string
	attemptID   string
	sessionID   string
	resumeToken string
}

type completedRelocationServiceAgentManager struct {
	*mockAgentManager
	ensureWorkspaceExecution func(context.Context, string, string) error
}

func (m *completedRelocationServiceAgentManager) EnsureWorkspaceExecutionForSession(ctx context.Context, taskID, sessionID string) error {
	if m.ensureWorkspaceExecution != nil {
		return m.ensureWorkspaceExecution(ctx, taskID, sessionID)
	}
	return nil
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.1, AC-TASKS-MANAGED-CLONE-RELOCATION-001.7, AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
func TestCompletedRelocationSessionContinuity(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	path, journalPath, journalBefore, headBefore, statusBefore := continueCompletedRelocationWork(t, fixture)

	starts := make(chan completedRelocationProviderStart, 2)
	fixture.rebuildService(starts, nil)

	// The ordinary ResumeTaskSessionWithOptions path must reuse the current
	// checkout and retire only the failure observed by this resume attempt.
	resumeCompletedRelocationSession(t, fixture, starts, func(ctx context.Context) error {
		_, err := fixture.svc.ResumeTaskSessionWithOptions(ctx, fixture.taskID, fixture.sessionID, executor.ResumeOptions{})
		return err
	})
	stored, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	lastError, found := models.LoadLastAgentError(stored.Metadata)
	require.True(t, found)
	require.True(t, lastError.IsDismissed(), "the matching failed-session recovery error should be retired")
	assertCompletedRelocationSessionState(t, fixture, path, journalPath, journalBefore, headBefore, statusBefore)
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.7, AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
func TestCompletedRelocationLegacyEmptyEnvironmentBindingResumes(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	path, journalPath, journalBefore, headBefore, statusBefore := continueCompletedRelocationWork(t, fixture)

	session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	session.TaskEnvironmentID = ""
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))

	starts := make(chan completedRelocationProviderStart, 1)
	fixture.rebuildService(starts, nil)
	resumeCompletedRelocationSession(t, fixture, starts, func(ctx context.Context) error {
		_, resumeErr := fixture.svc.ResumeTaskSessionWithOptions(ctx, fixture.taskID, fixture.sessionID, executor.ResumeOptions{})
		return resumeErr
	})

	stored, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, fixture.environmentID, stored.TaskEnvironmentID,
		"normal resume must persist the selected task-owned environment binding")
	assertCompletedRelocationSessionState(t, fixture, path, journalPath, journalBefore, headBefore, statusBefore)
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.7, AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
func TestCompletedRelocationExplicitRetryPreservesNewerError(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	path, journalPath, journalBefore, headBefore, statusBefore := continueCompletedRelocationWork(t, fixture)
	starts := make(chan completedRelocationProviderStart, 1)
	fixture.rebuildService(starts, func(_ int) {
		// A newer failure can arrive while the provider is starting. The old
		// attempt must not dismiss it when its correlated ready event arrives.
		replaceSessionErrorWithoutChangingState(t, fixture, "newer-unrelated-failure", "newer failure arrived during startup")
	})
	result := make(chan error, 1)
	go func() {
		_, recoverErr := fixture.svc.RecoverSessionWithOptions(ctx, fixture.taskID, fixture.sessionID,
			models.RecoveryActionRelocateAndResume, RecoverSessionOptions{ErrorStamp: completedRelocationSessionErrorStamp})
		result <- recoverErr
	}()
	select {
	case start := <-starts:
		require.Equal(t, fixture.sessionID, start.sessionID)
		require.Equal(t, "provider-conversation-kept", start.resumeToken)
		require.NotEmpty(t, start.attemptID)
		waitForCompletedRelocationAttempt(t, fixture, start)
		fixture.svc.handleAgentBootReady(context.Background(), completedRelocationReadyEvent(start))
	case err := <-result:
		require.NoError(t, err, "explicit recovery returned before provider startup")
		t.Fatal("explicit recovery completed without starting the provider")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for provider startup")
	}
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("explicit recovery did not finish after the correlated ready event")
	}
	stored, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	lastError, found := models.LoadLastAgentError(stored.Metadata)
	require.True(t, found)
	require.Equal(t, "newer-unrelated-failure", lastError.Stamp())
	require.False(t, lastError.IsDismissed(), "successful old retry must preserve a newer failure")

	assertCompletedRelocationSessionState(t, fixture, path, journalPath, journalBefore, headBefore, statusBefore)
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.3, AC-TASKS-MANAGED-CLONE-RELOCATION-001.6, AC-TASKS-MANAGED-CLONE-RELOCATION-001.7
func TestCompletedRelocationSessionRejectsInvalidSibling(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 2)
	ctx := context.Background()
	firstPath, firstJournalPath, firstJournalBefore, firstHead, firstStatus := continueCompletedRelocationWork(t, fixture)
	secondEnvironment, err := fixture.repo.GetTaskEnvironment(ctx, fixture.environmentID)
	require.NoError(t, err)
	require.Len(t, secondEnvironment.Repos, 2)
	firstWorktreeID := secondEnvironment.Repos[0].WorktreeID
	secondPath := secondEnvironment.Repos[1].WorktreePath
	secondWorktreeID := secondEnvironment.Repos[1].WorktreeID
	secondJournalPath := secondPath + ".kandev-clone-relocation.json"
	secondJournalBefore, err := os.ReadFile(secondJournalPath)
	require.NoError(t, err)
	var secondJournal struct {
		ReplacementID string `json:"replacement_id"`
	}
	require.NoError(t, json.Unmarshal(secondJournalBefore, &secondJournal))
	require.Equal(t, secondWorktreeID, secondJournal.ReplacementID)
	secondHead := sessionRecoveryGit(t, secondPath, "rev-parse", "HEAD")
	secondStatus := sessionRecoveryGit(t, secondPath, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")

	// The second selected destination has a valid branch but the wrong provider
	// origin. A healthy completed first slot cannot mask this sibling drift.
	sessionRecoveryGit(t, secondPath, "remote", "set-url", "origin", "https://github.com/other-owner/wrong.git")
	starts := make(chan completedRelocationProviderStart, 1)
	fixture.rebuildService(starts, nil)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	if _, err := fixture.svc.executor.PreflightSessionWorktreeRecovery(ctx, fixture.taskID, session, false); err == nil {
		t.Fatal("completed first slot hid invalid provider identity in the second selected slot")
	}
	resumeResult := make(chan error, 1)
	go func() {
		_, resumeErr := fixture.svc.ResumeTaskSessionWithOptions(ctx, fixture.taskID, fixture.sessionID, executor.ResumeOptions{})
		resumeResult <- resumeErr
	}()
	select {
	case start := <-starts:
		require.Equal(t, fixture.sessionID, start.sessionID)
		require.Equal(t, "provider-conversation-kept", start.resumeToken)
		waitForCompletedRelocationAttempt(t, fixture, start)
		fixture.svc.handleAgentBootReady(context.Background(), completedRelocationReadyEvent(start))
		select {
		case resumeErr := <-resumeResult:
			require.NoError(t, resumeErr)
			t.Fatal("resume launched the provider despite invalid sibling")
		case <-time.After(5 * time.Second):
			t.Fatal("resume did not settle after provider startup")
		}
	case resumeErr := <-resumeResult:
		require.Error(t, resumeErr)
		require.Contains(t, resumeErr.Error(), "worktree directory is corrupted")
	case <-time.After(5 * time.Second):
		t.Fatal("resume neither refused the invalid sibling nor reached provider startup")
	}
	require.Zero(t, fixture.launchStarts, "provider startup must follow validation of every selected slot")
	assertCompletedRelocationSlotUnchanged(t, fixture, firstPath, firstWorktreeID, firstJournalPath, firstJournalBefore, firstHead, firstStatus)
	assertCompletedRelocationSlotUnchanged(t, fixture, secondPath, secondWorktreeID, secondJournalPath, secondJournalBefore,
		secondHead, secondStatus)
	running, err := fixture.repo.GetExecutorRunningBySessionID(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, "provider-conversation-kept", running.ResumeToken)
}

// @covers AC-TASKS-MANAGED-CLONE-RELOCATION-001.7, AC-TASKS-MANAGED-CLONE-RELOCATION-002.4
func TestCompletedRelocationRestoreWorkspaceContinuity(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	path, journalPath, journalBefore, headBefore, statusBefore := continueCompletedRelocationWork(t, fixture)
	fixture.rebuildService(make(chan completedRelocationProviderStart, 1), nil)
	ensureCalls := 0
	fixture.agent.ensureWorkspaceExecution = func(callCtx context.Context, taskID, sessionID string) error {
		ensureCalls++
		session, err := fixture.repo.GetTaskSession(callCtx, sessionID)
		if err != nil {
			return err
		}
		admission, err := fixture.svc.executor.PreflightSessionWorktreeRecovery(callCtx, taskID, session, false)
		if err != nil {
			return err
		}
		if admission != nil {
			return admission.Release(callCtx)
		}
		return nil
	}

	response, err := fixture.svc.LaunchSession(ctx, &LaunchSessionRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID, Intent: IntentRestoreWorkspace,
	})
	require.NoError(t, err)
	require.True(t, response.Success)
	require.NotNil(t, response.WorktreePath)
	require.Equal(t, path, *response.WorktreePath)
	require.Equal(t, 1, ensureCalls, "restore must pass through final lifecycle admission")
	require.Zero(t, fixture.launchStarts, "workspace restore must not start the provider")
	assertCompletedRelocationSessionState(t, fixture, path, journalPath, journalBefore, headBefore, statusBefore)
}

func newCompletedRelocationServiceFixture(t *testing.T, repositoryCount int) *completedRelocationServiceFixture {
	t.Helper()
	const (
		taskID        = "task-completed-relocation"
		sessionID     = "session-completed-relocation"
		environmentID = "environment-completed-relocation"
		taskDirName   = "task-completed-relocation-root"
	)
	ctx := context.Background()
	fixture := &completedRelocationServiceFixture{
		taskID: taskID, sessionID: sessionID, environmentID: environmentID, taskDirName: taskDirName,
	}
	dbConn, err := kandevdb.OpenSQLite(filepath.Join(t.TempDir(), "completed-relocation.db"))
	require.NoError(t, err)
	fixture.db = sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = fixture.db.Close() })
	fixture.repo, err = tasksqlite.NewWithDB(fixture.db, fixture.db, nil)
	require.NoError(t, err)
	fixture.store, err = worktree.NewSQLiteStore(fixture.db, fixture.db)
	require.NoError(t, err)
	now := time.Now().UTC()
	require.NoError(t, fixture.repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace-completed-relocation", Name: "Completed relocation"}))
	require.NoError(t, fixture.repo.CreateTask(ctx, &models.Task{
		ID: taskID, WorkspaceID: "workspace-completed-relocation", Title: "Completed relocation continuity",
	}))
	fixture.config = worktree.Config{TasksBasePath: filepath.Join(t.TempDir(), "tasks")}
	taskRoot := filepath.Join(fixture.config.TasksBasePath, taskDirName)
	require.NoError(t, os.MkdirAll(taskRoot, 0o755))
	require.NoError(t, workspaces.WriteOwnershipMarker(taskRoot, workspaces.OwnershipMarker{
		TaskID: taskID, TaskDirName: taskDirName, LayoutVersion: workspaces.LayoutVersionSemantic,
	}))
	fixture.cloner = repoclone.NewCloner(repoclone.Config{BasePath: filepath.Join(t.TempDir(), "managed-clones")}, repoclone.ProtocolHTTPS, "", nil)
	seed := filepath.Join(t.TempDir(), "seed")
	require.NoError(t, os.MkdirAll(seed, 0o755))
	sessionRecoveryGitRaw(t, seed, "init", "-b", "main")
	sessionRecoveryGit(t, seed, "config", "user.email", "relocation@example.test")
	sessionRecoveryGit(t, seed, "config", "user.name", "Relocation Test")
	require.NoError(t, os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644))
	sessionRecoveryGit(t, seed, "add", "README.md")
	sessionRecoveryGit(t, seed, "commit", "-m", "initial")

	environmentRepos := make([]*models.TaskEnvironmentRepo, 0, repositoryCount)
	for index := 0; index < repositoryCount; index++ {
		slot := completedRelocationServiceSlot{
			repositoryID: fmt.Sprintf("repository-completed-relocation-%d", index+1),
			worktreeID:   fmt.Sprintf("worktree-completed-relocation-%d", index+1),
			branch:       fmt.Sprintf("feature/completed-relocation-%d", index+1),
			branchSlug:   "main",
		}
		name := fmt.Sprintf("widget-%d", index+1)
		repository := &models.Repository{
			ID: slot.repositoryID, WorkspaceID: "workspace-completed-relocation", Name: name,
			SourceType: "provider", Provider: "github", ProviderHost: "https://github.com",
			ProviderOwner: "acme", ProviderName: name, DefaultBranch: "main", CreatedAt: now, UpdatedAt: now,
		}
		_, slot.sourceClone, _, slot.destination, _, err = fixture.cloner.ManagedCloneRelocationPaths(repository)
		require.NoError(t, err)
		repository.LocalPath = slot.destination
		require.NoError(t, fixture.repo.CreateRepository(ctx, repository))
		require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: fmt.Sprintf("task-repository-completed-relocation-%d", index+1), TaskID: taskID,
			RepositoryID: slot.repositoryID, BaseBranch: "main", Position: index, CreatedAt: now, UpdatedAt: now,
		}))
		slot.worktreePath, err = fixture.config.TaskWorktreePath(taskDirName, name, slot.branchSlug)
		require.NoError(t, err)
		for _, clone := range []string{slot.sourceClone, slot.destination} {
			require.NoError(t, os.MkdirAll(filepath.Dir(clone), 0o755))
			sessionRecoveryGitRaw(t, filepath.Dir(clone), "clone", "--no-hardlinks", seed, clone)
			sessionRecoveryGit(t, clone, "remote", "set-url", "origin", fmt.Sprintf("https://github.com/acme/%s.git", name))
			sessionRecoveryGit(t, clone, "config", "user.email", "relocation@example.test")
			sessionRecoveryGit(t, clone, "config", "user.name", "Relocation Test")
		}
		sessionRecoveryGit(t, slot.sourceClone, "checkout", "-b", slot.branch)
		tracked := fmt.Sprintf("branch-%d.txt", index+1)
		require.NoError(t, os.WriteFile(filepath.Join(slot.sourceClone, tracked), []byte("branch work\n"), 0o644))
		sessionRecoveryGit(t, slot.sourceClone, "add", tracked)
		sessionRecoveryGit(t, slot.sourceClone, "commit", "-m", "selected worktree branch")
		head := sessionRecoveryGit(t, slot.sourceClone, "rev-parse", "HEAD")
		sessionRecoveryGit(t, slot.sourceClone, "checkout", "main")
		sessionRecoveryGit(t, slot.destination, "fetch", "--no-tags", slot.sourceClone, head)
		sessionRecoveryGit(t, slot.destination, "update-ref", "refs/heads/"+slot.branch, head)
		require.NoError(t, os.MkdirAll(filepath.Dir(slot.worktreePath), 0o755))
		sessionRecoveryGit(t, slot.sourceClone, "worktree", "add", slot.worktreePath, slot.branch)
		require.NoError(t, os.WriteFile(filepath.Join(slot.worktreePath, fmt.Sprintf("user-edit-%d.txt", index+1)),
			[]byte("preserve this user edit\n"), 0o644))
		environmentRepos = append(environmentRepos, &models.TaskEnvironmentRepo{
			ID: fmt.Sprintf("environment-repo-completed-relocation-%d", index+1), TaskEnvironmentID: environmentID,
			RepositoryID: slot.repositoryID, BranchSlug: slot.branchSlug, WorktreeID: slot.worktreeID,
			WorktreePath: slot.worktreePath, WorktreeBranch: slot.branch, WorktreeSourceClonePath: slot.sourceClone,
			WorktreeSourceCommonDir: filepath.Join(slot.sourceClone, ".git"), Status: "active", Position: index,
			CreatedAt: now, UpdatedAt: now,
		})
		fixture.slots = append(fixture.slots, slot)
	}
	require.NoError(t, fixture.repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: environmentID, TaskID: taskID, OwnershipGeneration: 1,
		ExecutorType: string(models.ExecutorTypeWorktree), ExecutorID: models.ExecutorIDWorktree,
		Status: models.TaskEnvironmentStatusReady, WorkspacePath: taskRoot, TaskDirName: taskDirName, Repos: environmentRepos,
	}))
	initialError := models.LastAgentError{
		Message: "Published replacement commit could not be verified", OccurredAt: now,
		Scope: models.ErrorScopeSession, Code: models.LaunchErrorCategoryManagedCloneRelocationRequired,
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: completedRelocationSessionErrorStamp,
	}
	require.NoError(t, fixture.repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID, State: models.TaskSessionStateFailed,
		ExecutorID: models.ExecutorIDWorktree, AgentProfileID: "profile-completed-relocation",
		RepositoryID: fixture.slots[0].repositoryID, BaseBranch: "main",
		Metadata: map[string]interface{}{models.SessionMetaKeyLastAgentError: initialError},
	}))
	for _, slot := range fixture.slots {
		require.NoError(t, fixture.store.CreateWorktree(ctx, &worktree.Worktree{
			ID: slot.worktreeID, SessionID: sessionID, TaskID: taskID, TaskEnvironmentID: environmentID,
			RepositoryID: slot.repositoryID, TaskDirName: taskDirName, BranchSlug: slot.branchSlug,
			RepositoryPath: slot.destination, Path: slot.worktreePath, Branch: slot.branch, BaseBranch: "main",
			SourceClonePath: slot.sourceClone, SourceCommonDir: filepath.Join(slot.sourceClone, ".git"), Status: worktree.StatusActive,
		}))
	}
	require.NoError(t, fixture.repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID: sessionID, SessionID: sessionID, TaskID: taskID, AgentExecutionID: "execution-before-relocation",
		ResumeToken: "provider-conversation-kept", Resumable: true, Status: models.ExecutorRunningStatusStopped,
	}))
	fixture.manager, err = worktree.NewManager(fixture.config, fixture.store, testLogger())
	require.NoError(t, err)
	fixture.rebuildService(make(chan completedRelocationProviderStart, 1), nil)

	// Complete the original dirty-checkout move through the same executor and
	// manager admission used by task recovery. No provider startup is involved.
	session, err := fixture.repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	relocationCtx := worktree.WithDirtyCloneRelocation(ctx)
	relocationCtx = worktree.WithManagedCloneRelocationAuthorization(relocationCtx, func(checkCtx context.Context) error {
		current, loadErr := fixture.repo.GetTaskSession(checkCtx, sessionID)
		if loadErr != nil {
			return loadErr
		}
		if !isManagedCloneRelocationAuthorized(current, taskID, completedRelocationSessionErrorStamp) {
			return worktree.ErrManagedCloneRelocationAuthorizationStale
		}
		return nil
	})
	admission, err := fixture.svc.executor.PreflightSessionWorktreeRecovery(relocationCtx, taskID, session, false)
	require.NoError(t, err)
	require.NotNil(t, admission)
	require.NoError(t, admission.Release(ctx))
	return fixture
}

func (f *completedRelocationServiceFixture) rebuildService(
	starts chan completedRelocationProviderStart,
	afterProviderLaunch func(int),
) {
	var launchCount int
	f.launchStarts = 0
	base := &mockAgentManager{
		repoForExecutionLookup: f.repo,
		isAgentReadyFn:         func(context.Context, string) bool { return true },
		launchAgentFunc: func(callCtx context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			launchCount++
			f.launchStarts++
			if afterProviderLaunch != nil {
				afterProviderLaunch(launchCount)
			}
			starts <- completedRelocationProviderStart{
				executionID: fmt.Sprintf("execution-completed-relocation-%d", launchCount),
				attemptID:   executor.ResumeAttemptIDFromContext(callCtx),
				sessionID:   request.SessionID,
				resumeToken: request.ACPSessionID,
			}
			return &executor.LaunchAgentResponse{
				AgentExecutionID: fmt.Sprintf("execution-completed-relocation-%d", launchCount), Status: v1.AgentStatusStarting,
			}, nil
		},
	}
	f.agent = &completedRelocationServiceAgentManager{mockAgentManager: base}
	taskRepo := newMockTaskRepo()
	seedMockTaskState(taskRepo, f.taskID, v1.TaskStateInProgress)
	f.svc = NewService(DefaultServiceConfig(), nil, f.agent, taskRepo, f.repo, nil, nil, nil, testLogger())
	f.svc.executor.SetRepoCloner(f.cloner, nil)
	f.svc.executor.SetSelectedWorktreeRecoveryAdmission(f.manager.AdmitRecovery)
}

func continueCompletedRelocationWork(t *testing.T, fixture *completedRelocationServiceFixture) (string, string, []byte, string, string) {
	t.Helper()
	ctx := context.Background()
	environment, err := fixture.repo.GetTaskEnvironment(ctx, fixture.environmentID)
	require.NoError(t, err)
	require.Len(t, environment.Repos, len(fixture.slots))
	for index, slot := range fixture.slots {
		path := environment.Repos[index].WorktreePath
		require.NotEmpty(t, path)
		editPath := filepath.Join(path, fmt.Sprintf("after-relocation-%d.txt", index+1))
		require.NoError(t, os.WriteFile(editPath, []byte("current user work\n"), 0o644))
		sessionRecoveryGit(t, path, "add", filepath.Base(editPath))
		sessionRecoveryGit(t, path, "commit", "-m", "work after relocation")
		require.NoError(t, os.RemoveAll(slot.sourceClone))
	}
	currentPath := environment.Repos[0].WorktreePath
	journalPath := currentPath + ".kandev-clone-relocation.json"
	journalBefore, err := os.ReadFile(journalPath)
	require.NoError(t, err)
	headBefore := sessionRecoveryGit(t, currentPath, "rev-parse", "HEAD")
	statusBefore := sessionRecoveryGit(t, currentPath, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional")
	manager, err := worktree.NewManager(fixture.config, fixture.store, testLogger())
	require.NoError(t, err)
	fixture.manager = manager
	fixture.rebuildService(make(chan completedRelocationProviderStart, 2), nil)
	return currentPath, journalPath, journalBefore, headBefore, statusBefore
}

func resumeCompletedRelocationSession(t *testing.T, fixture *completedRelocationServiceFixture,
	starts chan completedRelocationProviderStart, launch func(context.Context) error,
) {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- launch(context.Background()) }()
	var start completedRelocationProviderStart
	select {
	case start = <-starts:
	case err := <-result:
		require.NoError(t, err, "resume returned before provider startup")
		t.Fatal("resume completed without starting the provider")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for provider startup")
	}
	require.Equal(t, fixture.sessionID, start.sessionID)
	require.Equal(t, "provider-conversation-kept", start.resumeToken)
	require.NotEmpty(t, start.attemptID)
	waitForCompletedRelocationAttempt(t, fixture, start)
	fixture.svc.handleAgentBootReady(context.Background(), completedRelocationReadyEvent(start))
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("session resume did not finish after the correlated ready event")
	}
}

func completedRelocationReadyEvent(start completedRelocationProviderStart) watcher.AgentEventData {
	return watcher.AgentEventData{
		TaskID: "task-completed-relocation", SessionID: start.sessionID,
		AgentExecutionID: start.executionID, AttemptID: start.attemptID,
	}
}

func waitForCompletedRelocationAttempt(t *testing.T, fixture *completedRelocationServiceFixture, start completedRelocationProviderStart) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		session, err := fixture.repo.GetTaskSession(context.Background(), fixture.sessionID)
		if err == nil && session != nil && session.State == models.TaskSessionStateStarting &&
			fixture.svc.resumeAttemptAllowsExecution(fixture.sessionID, start.executionID, start.attemptID) {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("resume attempt did not reach the provider startup boundary")
}

func assertCompletedRelocationSessionState(t *testing.T, fixture *completedRelocationServiceFixture,
	path, journalPath string, journalBefore []byte, headBefore, statusBefore string,
) {
	t.Helper()
	ctx := context.Background()
	environment, err := fixture.repo.GetTaskEnvironment(ctx, fixture.environmentID)
	require.NoError(t, err)
	require.Equal(t, path, environment.Repos[0].WorktreePath)
	worktreeRow, err := fixture.store.GetWorktreeByID(ctx, environment.Repos[0].WorktreeID)
	require.NoError(t, err)
	require.NotNil(t, worktreeRow)
	require.Equal(t, path, worktreeRow.Path)
	require.Equal(t, headBefore, sessionRecoveryGit(t, path, "rev-parse", "HEAD"))
	require.Equal(t, statusBefore, sessionRecoveryGit(t, path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional"))
	after, err := os.ReadFile(journalPath)
	require.NoError(t, err)
	require.Equal(t, journalBefore, after)
	contents, err := os.ReadFile(filepath.Join(path, "after-relocation-1.txt"))
	require.NoError(t, err)
	require.Equal(t, "current user work\n", string(contents))
	running, err := fixture.repo.GetExecutorRunningBySessionID(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, "provider-conversation-kept", running.ResumeToken)
}

func assertCompletedRelocationSlotUnchanged(t *testing.T, fixture *completedRelocationServiceFixture,
	path, worktreeID, journalPath string, journalBefore []byte, headBefore, statusBefore string,
) {
	t.Helper()
	ctx := context.Background()
	worktreeRow, err := fixture.store.GetWorktreeByID(ctx, worktreeID)
	require.NoError(t, err)
	require.NotNil(t, worktreeRow)
	require.Equal(t, path, worktreeRow.Path)
	require.Equal(t, headBefore, sessionRecoveryGit(t, path, "rev-parse", "HEAD"))
	require.Equal(t, statusBefore, sessionRecoveryGit(t, path, "status", "--porcelain=v1", "--untracked-files=all", "--ignored=traditional"))
	after, err := os.ReadFile(journalPath)
	require.NoError(t, err)
	require.Equal(t, journalBefore, after)
}

func replaceSessionErrorWithoutChangingState(t *testing.T, fixture *completedRelocationServiceFixture, stamp, message string) {
	t.Helper()
	err := fixture.repo.SetSessionMetadataKey(context.Background(), fixture.sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{
		Message: message, OccurredAt: time.Now().UTC(), Scope: models.ErrorScopeSession,
		Code:            models.LaunchErrorCategoryManagedCloneRelocationRequired,
		RecoveryActions: []string{models.RecoveryActionRelocateAndResume}, StampValue: stamp,
	})
	if err != nil {
		t.Errorf("write newer session recovery error: %v", err)
	}
}

func sessionRecoveryGit(t *testing.T, directory string, args ...string) string {
	t.Helper()
	return sessionRecoveryGitRaw(t, directory, append([]string{"-C", directory}, args...)...)
}

func sessionRecoveryGitRaw(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), directory, err, output)
	}
	return strings.TrimSpace(string(output))
}
