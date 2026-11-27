package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	releaseTaskID    = "task-release"
	releaseSessionID = "session-release"
	releaseEnvID     = "environment-release"
)

// seedOwnedCreatingClaim stores a session that owns the task's creating
// environment, as the workspace election leaves it.
func seedOwnedCreatingClaim(repo *mockRepository, state models.TaskSessionState) *models.TaskEnvironment {
	repo.tasks[releaseTaskID] = &models.Task{ID: releaseTaskID}
	repo.sessions[releaseSessionID] = &models.TaskSession{
		ID: releaseSessionID, TaskID: releaseTaskID, AgentProfileID: "profile-123",
		TaskEnvironmentID: releaseEnvID, State: state,
		StartedAt: time.Now(), UpdatedAt: time.Now(),
	}
	env := &models.TaskEnvironment{
		ID: releaseEnvID, TaskID: releaseTaskID, ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusCreating, MaterializationSessionID: releaseSessionID,
	}
	repo.taskEnvironments[env.ID] = env
	return env
}

func releaseTask() *v1.Task {
	return &v1.Task{ID: releaseTaskID, WorkspaceID: "workspace-123"}
}

func failRepositoryInfo(repo *mockRepository) {
	repo.listTaskRepositoriesFunc = func(context.Context, string) ([]*models.TaskRepository, error) {
		return nil, errors.New("repository info unavailable")
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestLaunchPreparedSessionReleasesOwnedClaimOnEarlyError(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	failRepositoryInfo(repo)
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if err == nil {
		t.Fatal("LaunchPreparedSession succeeded, want the repository info error")
	}
	if env.Status != models.TaskEnvironmentStatusFailed || env.MaterializationSessionID != "" {
		t.Fatalf("environment = status %q owner %q, want failed without owner", env.Status, env.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestLaunchPreparedSessionKeepsClaimWhenExecutionAlreadyRunning(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateRunning)
	exec := newTestExecutor(t, &mockAgentManager{
		isAgentRunningForSessionFunc: func(context.Context, string) bool { return true },
	}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if !errors.Is(err, ErrExecutionAlreadyRunning) {
		t.Fatalf("LaunchPreparedSession error = %v, want ErrExecutionAlreadyRunning", err)
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != releaseSessionID {
		t.Fatalf("environment = status %q owner %q, want the claim kept", env.Status, env.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestLaunchPreparedSessionKeepsClaimWhenSupersedingResumeCancelled(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	failRepositoryInfo(repo)
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	ctx, cancel := context.WithCancel(WithCancellableResumeContext(context.Background()))
	cancel()

	_, err := exec.LaunchPreparedSession(ctx, releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if err == nil {
		t.Fatal("LaunchPreparedSession succeeded, want an error")
	}
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != releaseSessionID {
		t.Fatalf("environment = status %q owner %q, want the claim kept", env.Status, env.MaterializationSessionID)
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
func TestLaunchFailureLeavesClaimTakenOverDuringLaunch(t *testing.T) {
	repo := newMockRepository()
	seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	// The launch works on its own snapshot of the row, as with a database.
	snapshot := func() (*models.TaskEnvironment, error) {
		copied := *repo.taskEnvironments[releaseEnvID]
		return &copied, nil
	}
	repo.getTaskEnvironmentFunc = func(context.Context, string) (*models.TaskEnvironment, error) { return snapshot() }
	repo.getTaskEnvironmentByTaskIDFunc = func(context.Context, string) (*models.TaskEnvironment, error) { return snapshot() }
	exec := newTestExecutor(t, &mockAgentManager{
		launchAgentFunc: func(context.Context, *LaunchAgentRequest) (*LaunchAgentResponse, error) {
			// Another start takes the claim over while this launch is slow.
			repo.taskEnvironments[releaseEnvID].MaterializationSessionID = "session-new-owner"
			return nil, errors.New("setup failed")
		},
	}, repo)

	_, err := exec.LaunchPreparedSession(context.Background(), releaseTask(), releaseSessionID,
		LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})

	if err == nil {
		t.Fatal("LaunchPreparedSession succeeded, want the launch error")
	}
	env := repo.taskEnvironments[releaseEnvID]
	if env.Status != models.TaskEnvironmentStatusCreating || env.MaterializationSessionID != "session-new-owner" {
		t.Fatalf("environment = status %q owner %q, want the new owner's claim kept", env.Status, env.MaterializationSessionID)
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("former owner wrote the environment %d times, want 0", len(repo.updateTaskEnvironmentCalls))
	}
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.7
func TestPersistTaskEnvironmentRefusesPublicationAfterClaimTakenOver(t *testing.T) {
	repo := newMockRepository()
	launchSnapshot := &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree),
		Status: models.TaskEnvironmentStatusCreating, MaterializationSessionID: "session-1",
	}
	current := *launchSnapshot
	current.MaterializationSessionID = "session-2"
	repo.taskEnvironments[current.ID] = &current
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	session := &models.TaskSession{ID: "session-1", TaskID: "task-1", TaskEnvironmentID: launchSnapshot.ID}

	err := e.persistTaskEnvironment(context.Background(), "task-1", session, launchSnapshot,
		&LaunchAgentRequest{TaskID: "task-1", ExecutorType: string(models.ExecutorTypeWorktree)},
		&LaunchAgentResponse{WorktreePath: "/tasks/task-1/repo", Worktrees: []RepoWorktreeResult{{
			RepositoryID: "repo-1", WorktreeID: "wt-1", WorktreePath: "/tasks/task-1/repo", WorktreeBranch: "feature/task-1",
		}}},
		executorConfig{ExecutorID: models.ExecutorIDWorktree})

	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("persistTaskEnvironment error = %v, want workspace reuse unsafe", err)
	}
	if len(repo.finalizeTaskEnvironmentCalls) != 0 || len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("former owner published the environment: finalize=%d update=%d",
			len(repo.finalizeTaskEnvironmentCalls), len(repo.updateTaskEnvironmentCalls))
	}
	if current.Status != models.TaskEnvironmentStatusCreating || current.MaterializationSessionID != "session-2" {
		t.Fatalf("environment = status %q owner %q, want the new owner's claim kept", current.Status, current.MaterializationSessionID)
	}
}

// releaseObservingRepository wraps the mock repository to observe when a
// launch performs its claim release.
type releaseObservingRepository struct {
	*mockRepository
	onRelease func()
}

func (r *releaseObservingRepository) FailTaskEnvironmentMaterialization(ctx context.Context, sessionID string) (bool, error) {
	if r.onRelease != nil {
		r.onRelease()
	}
	return r.mockRepository.FailTaskEnvironmentMaterialization(ctx, sessionID)
}

type launchMarkerContextKey struct{}

func withLaunchMarker(ctx context.Context, marker string) context.Context {
	return context.WithValue(ctx, launchMarkerContextKey{}, marker)
}

// @covers AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-004.1
func TestLaunchPreparedSessionReleasesClaimBeforeLaterStartEntersSessionLock(t *testing.T) {
	repo := newMockRepository()
	env := seedOwnedCreatingClaim(repo, models.TaskSessionStateCreated)
	failRepositoryInfo(repo)

	var (
		mu          sync.Mutex
		events      []string
		releasing   = make(chan struct{})
		resume      = make(chan struct{})
		entered     = make(chan struct{})
		onceRelease sync.Once
	)
	mark := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	observed := &releaseObservingRepository{mockRepository: repo}
	observed.onRelease = func() {
		onceRelease.Do(func() {
			close(releasing)
			<-resume
			mark("release-performed")
		})
	}
	repo.getTaskSessionFunc = func(ctx context.Context, id string) (*models.TaskSession, error) {
		if marker, _ := ctx.Value(launchMarkerContextKey{}).(string); marker == "later-start" {
			mark("later-start-entered-lock")
			select {
			case <-entered:
			default:
				close(entered)
			}
		}
		repo.mu.Lock()
		session, ok := repo.sessions[id]
		repo.mu.Unlock()
		if !ok {
			return nil, fmt.Errorf("task session not found: %s", id)
		}
		return cloneMockTaskSession(session), nil
	}
	exec := newTestExecutor(t, &mockAgentManager{}, observed)

	firstErr := make(chan error, 1)
	secondErr := make(chan error, 1)
	go func() {
		_, err := exec.LaunchPreparedSession(withLaunchMarker(context.Background(), "first"), releaseTask(), releaseSessionID,
			LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})
		firstErr <- err
	}()
	<-releasing
	go func() {
		_, err := exec.LaunchPreparedSession(withLaunchMarker(context.Background(), "later-start"), releaseTask(), releaseSessionID,
			LaunchOptions{AgentProfileID: "profile-123", StartAgent: true})
		secondErr <- err
	}()
	select {
	case <-entered:
		t.Fatal("a later same-session start entered the session lock while the failed launch was still releasing its claim")
	case <-time.After(100 * time.Millisecond):
	}
	close(resume)
	firstLaunchErr := <-firstErr
	secondLaunchErr := <-secondErr

	if firstLaunchErr == nil || secondLaunchErr == nil {
		t.Fatalf("launch errors = %v, %v, want both launches to fail on the repository info error", firstLaunchErr, secondLaunchErr)
	}
	mu.Lock()
	defer mu.Unlock()
	releaseAt, enteredAt := -1, -1
	for i, event := range events {
		switch event {
		case "release-performed":
			releaseAt = i
		case "later-start-entered-lock":
			enteredAt = i
		}
	}
	if enteredAt == -1 {
		t.Fatal("the later start never entered the session lock")
	}
	if releaseAt == -1 || releaseAt > enteredAt {
		t.Fatalf("release events %v, want the claim release performed before the later start entered the session lock", events)
	}
	if env.Status != models.TaskEnvironmentStatusFailed || env.MaterializationSessionID != "" {
		t.Fatalf("environment = status %q owner %q, want the first launch's own claim failed without an owner",
			env.Status, env.MaterializationSessionID)
	}
}
