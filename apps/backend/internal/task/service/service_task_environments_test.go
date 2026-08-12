package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

type stubEnvRepo struct {
	env     *models.TaskEnvironment
	deleted bool
	getErr  error
	delErr  error
}

func (s *stubEnvRepo) CreateTaskEnvironment(context.Context, *models.TaskEnvironment) error {
	return nil
}
func (s *stubEnvRepo) CreateTaskEnvironmentRepo(context.Context, *models.TaskEnvironmentRepo) error {
	return nil
}
func (s *stubEnvRepo) ListTaskEnvironmentRepos(context.Context, string) ([]*models.TaskEnvironmentRepo, error) {
	return nil, nil
}
func (s *stubEnvRepo) UpdateTaskEnvironmentRepo(context.Context, *models.TaskEnvironmentRepo) error {
	return nil
}
func (s *stubEnvRepo) DeleteTaskEnvironmentRepo(context.Context, string) error {
	return nil
}
func (s *stubEnvRepo) DeleteTaskEnvironmentReposByEnv(context.Context, string) error {
	return nil
}
func (s *stubEnvRepo) GetTaskEnvironment(context.Context, string) (*models.TaskEnvironment, error) {
	return s.env, s.getErr
}
func (s *stubEnvRepo) GetTaskEnvironmentByTaskID(context.Context, string) (*models.TaskEnvironment, error) {
	return s.env, s.getErr
}
func (s *stubEnvRepo) UpdateTaskEnvironment(context.Context, *models.TaskEnvironment) error {
	return nil
}
func (s *stubEnvRepo) DeleteTaskEnvironment(context.Context, string) error {
	if s.delErr != nil {
		return s.delErr
	}
	s.deleted = true
	return nil
}
func (s *stubEnvRepo) DeleteTaskEnvironmentsByTask(context.Context, string) error { return nil }

type stubDestroyer struct {
	containerCalls           []string
	sandboxCalls             []string
	worktreeCalls            []string
	cancelAfterContainer     context.CancelFunc
	cancelAfterFirstWorktree context.CancelFunc
	pushCalls                int
	containerErr             error
	sandboxErr               error
	worktreeErr              error
	pushErr                  error
	containerEntered         chan struct{}
	containerRelease         chan struct{}
}

func (s *stubDestroyer) DestroyContainer(_ context.Context, id string) error {
	s.containerCalls = append(s.containerCalls, id)
	if s.containerEntered != nil {
		close(s.containerEntered)
	}
	if s.containerRelease != nil {
		<-s.containerRelease
	}
	if s.cancelAfterContainer != nil {
		s.cancelAfterContainer()
	}
	return s.containerErr
}
func (s *stubDestroyer) DestroySandbox(_ context.Context, id, _ string) error {
	s.sandboxCalls = append(s.sandboxCalls, id)
	return s.sandboxErr
}
func (s *stubDestroyer) DestroyWorktree(_ context.Context, id string) error {
	s.worktreeCalls = append(s.worktreeCalls, id)
	if s.cancelAfterFirstWorktree != nil && len(s.worktreeCalls) == 1 {
		s.cancelAfterFirstWorktree()
	}
	return s.worktreeErr
}
func (s *stubDestroyer) PushEnvironmentBranch(context.Context, *models.TaskEnvironment) error {
	s.pushCalls++
	return s.pushErr
}
func (s *stubDestroyer) GetContainerLiveStatus(context.Context, string) (*ContainerLiveStatus, error) {
	return nil, nil
}

type stubRunningChecker struct {
	running bool
	err     error
}

type stubSharedEnvironmentSessions struct {
	repository.SessionRepository
	shared bool
	err    error
}

func (s *stubSharedEnvironmentSessions) HasActiveTaskSessionsByTaskEnvironmentExcludingTask(
	context.Context,
	string,
	string,
) (bool, error) {
	return s.shared, s.err
}

func (s *stubSharedEnvironmentSessions) HasLiveTaskSessionsByTaskEnvironmentExcludingTask(
	context.Context,
	string,
	string,
) (bool, error) {
	return s.shared, s.err
}

type stubRuntimeSecretDeleter struct {
	calls []string
	err   error
}

type stubTaskEnvironmentResetGuard struct {
	err error
}

func (g stubTaskEnvironmentResetGuard) ValidateTaskEnvironmentReset(context.Context, string, string) error {
	return g.err
}

func (s *stubRuntimeSecretDeleter) DeleteTaskEnvironmentRuntimeSecrets(
	_ context.Context,
	environmentID, _, _ string,
) error {
	s.calls = append(s.calls, environmentID)
	return s.err
}

func (s *stubRunningChecker) IsAnySessionRunningForTask(context.Context, string) (bool, error) {
	return s.running, s.err
}

func newResetTestService(t *testing.T, repo *stubEnvRepo) *Service {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return &Service{
		logger:           log,
		taskEnvironments: repo,
	}
}

func TestResetTaskEnvironment_NoEnvironment(t *testing.T) {
	svc := newResetTestService(t, &stubEnvRepo{env: nil})
	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if !errors.Is(err, ErrNoEnvironment) {
		t.Fatalf("expected ErrNoEnvironment, got %v", err)
	}
}

func TestResetTaskEnvironment_SessionRunningBlocks(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{ID: "env-1", TaskID: "task-1", ContainerID: "c"}}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{running: true})
	svc.SetEnvironmentDestroyer(&stubDestroyer{})

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if !errors.Is(err, ErrSessionRunning) {
		t.Fatalf("expected ErrSessionRunning, got %v", err)
	}
	if repo.deleted {
		t.Error("expected environment row to be preserved when session is running")
	}
}

func TestResetTaskEnvironment_SharedBorrowerBlocks(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-1",
	}}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.sessions = &stubSharedEnvironmentSessions{shared: true}
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if !errors.Is(err, ErrEnvironmentShared) {
		t.Fatalf("expected ErrEnvironmentShared, got %v", err)
	}
	if len(destroyer.containerCalls) != 0 {
		t.Fatalf("shared environment was destroyed: %v", destroyer.containerCalls)
	}
	if repo.deleted {
		t.Fatal("shared environment row was deleted")
	}
}

func TestResetTaskEnvironment_WorkspaceGroupReferenceBlocks(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-1",
	}}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(destroyer)
	svc.SetTaskEnvironmentResetGuard(stubTaskEnvironmentResetGuard{err: ErrEnvironmentShared})

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if !errors.Is(err, ErrEnvironmentShared) {
		t.Fatalf("workspace-group reset error = %v, want ErrEnvironmentShared", err)
	}
	if len(destroyer.containerCalls) != 0 || repo.deleted {
		t.Fatalf("group-referenced environment mutated: destroy=%v deleted=%v", destroyer.containerCalls, repo.deleted)
	}
}

func TestSessionBlocksEnvironmentReset(t *testing.T) {
	tests := []struct {
		state models.TaskSessionState
		want  bool
	}{
		{models.TaskSessionStateCreated, false},
		{models.TaskSessionStateStarting, true},
		{models.TaskSessionStateRunning, true},
		{models.TaskSessionStateWaitingForInput, false},
		{models.TaskSessionStateCompleted, false},
		{models.TaskSessionStateFailed, false},
		{models.TaskSessionStateCancelled, false},
	}

	for _, tt := range tests {
		if got := sessionBlocksEnvironmentReset(tt.state); got != tt.want {
			t.Fatalf("sessionBlocksEnvironmentReset(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestResetTaskEnvironment_DestroysEachResourceTypeAndDeletesRow(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID:          "env-1",
		TaskID:      "task-1",
		ContainerID: "container-abc",
		SandboxID:   "sandbox-xyz",
		Repos:       []*models.TaskEnvironmentRepo{{WorktreeID: "wt-1"}},
	}}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{running: false})
	svc.SetEnvironmentDestroyer(destroyer)

	if err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.deleted {
		t.Error("expected environment row to be deleted")
	}
	if len(destroyer.containerCalls) != 1 || destroyer.containerCalls[0] != "container-abc" {
		t.Errorf("expected 1 container destroy call, got %v", destroyer.containerCalls)
	}
	if len(destroyer.sandboxCalls) != 1 || destroyer.sandboxCalls[0] != "sandbox-xyz" {
		t.Errorf("expected 1 sandbox destroy call, got %v", destroyer.sandboxCalls)
	}
	if len(destroyer.worktreeCalls) != 1 || destroyer.worktreeCalls[0] != "wt-1" {
		t.Errorf("expected 1 worktree destroy call, got %v", destroyer.worktreeCalls)
	}
}

func TestResetTaskEnvironmentDeletesRuntimeSecretsBeforeEnvironmentRow(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", AgentctlAuthSecretID: "runtime-auth",
		AgentctlBootstrapSecretID: "runtime-bootstrap",
	}}
	deleter := &stubRuntimeSecretDeleter{}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetTaskEnvironmentRuntimeSecretDeleter(deleter)

	if err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(deleter.calls) != 1 || deleter.calls[0] != "env-1" {
		t.Fatalf("runtime secret cleanup calls = %v, want env-1", deleter.calls)
	}
	if !repo.deleted {
		t.Fatal("environment row was not deleted after runtime secrets")
	}
}

func TestResetTaskEnvironmentRuntimeSecretFailurePreservesEnvironmentRow(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", AgentctlAuthSecretID: "runtime-auth",
	}}
	deleter := &stubRuntimeSecretDeleter{err: errors.New("secret store unavailable")}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetTaskEnvironmentRuntimeSecretDeleter(deleter)

	if err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}); err == nil {
		t.Fatal("ResetTaskEnvironment unexpectedly succeeded")
	}
	if repo.deleted {
		t.Fatal("environment row deleted after runtime secret cleanup failure")
	}
}

func TestResetTaskEnvironment_StopsTaskLSPBeforeTeardown(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-1",
	}}
	destroyer := &stubDestroyer{}
	lifecycle := &recordingTaskLSPLifecycle{}
	lifecycle.onCleanup = func(_ context.Context, _ string) {
		if len(destroyer.containerCalls) != 0 {
			t.Fatal("container teardown ran before task LSP cleanup")
		}
	}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(destroyer)
	svc.SetTaskLSPLifecycle(lifecycle)

	if err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := lifecycle.cleanupCalls; len(got) != 1 || got[0] != "task-1:task_environment_reset" {
		t.Fatalf("cleanup calls = %v", got)
	}
}

func TestResetTaskEnvironmentBlocksLSPAdmissionThroughRowDeletion(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-1",
	}}
	destroyer := &stubDestroyer{
		containerEntered: make(chan struct{}), containerRelease: make(chan struct{}),
	}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(destroyer)
	svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{})

	resetDone := make(chan error, 1)
	go func() {
		resetDone <- svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	}()
	<-destroyer.containerEntered

	if release, err := svc.AcquireTaskLSPAdmission(context.Background(), "task-1"); !errors.Is(err, ErrTaskLSPAdmissionBlocked) {
		if release != nil {
			release()
		}
		t.Fatalf("LSP admission during reset = %v, want blocked", err)
	}

	close(destroyer.containerRelease)
	if err := <-resetDone; err != nil {
		t.Fatal(err)
	}
	if !repo.deleted {
		t.Fatal("environment row was not deleted before reset admission released")
	}
	release, err := svc.AcquireTaskLSPAdmission(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("LSP admission did not reopen after environment reset: %v", err)
	}
	release()
}

func TestResetTaskEnvironment_LSPCleanupFailurePreservesEnvironment(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-1",
	}}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{})
	svc.SetEnvironmentDestroyer(destroyer)
	svc.SetTaskLSPLifecycle(&recordingTaskLSPLifecycle{cleanupErr: errors.New("stop failed")})

	if err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{}); err == nil {
		t.Fatal("reset succeeded despite task LSP cleanup failure")
	}
	if repo.deleted || len(destroyer.containerCalls) != 0 {
		t.Fatalf("environment mutated after LSP cleanup failure: deleted=%v calls=%v", repo.deleted, destroyer.containerCalls)
	}
}

func TestTeardownEnvironmentResources_CancellationStopsBeforeNextResource(t *testing.T) {
	svc := newResetTestService(t, &stubEnvRepo{})
	ctx, cancel := context.WithCancel(context.Background())
	destroyer := &stubDestroyer{cancelAfterContainer: cancel}
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.teardownEnvironmentResources(ctx, &models.TaskEnvironment{
		ContainerID: "container-1", SandboxID: "sandbox-1",
		Repos: []*models.TaskEnvironmentRepo{{WorktreeID: "worktree-1"}},
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("teardown error = %v, want context cancellation", err)
	}
	if len(destroyer.sandboxCalls) != 0 || len(destroyer.worktreeCalls) != 0 {
		t.Fatalf("resources destroyed after cancellation: sandboxes=%v worktrees=%v",
			destroyer.sandboxCalls, destroyer.worktreeCalls)
	}
}

func TestTeardownEnvironmentResources_MultiRepoCancellationStopsBeforeNextWorktree(t *testing.T) {
	svc := newResetTestService(t, &stubEnvRepo{})
	ctx, cancel := context.WithCancel(context.Background())
	destroyer := &stubDestroyer{cancelAfterFirstWorktree: cancel}
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.teardownEnvironmentResources(ctx, &models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", WorktreeID: "wt-first"},
			{RepositoryID: "repo-b", WorktreeID: "wt-second"},
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("teardown error = %v, want context cancellation", err)
	}
	if len(destroyer.worktreeCalls) != 1 || destroyer.worktreeCalls[0] != "wt-first" {
		t.Fatalf("expected only the first worktree destroyed before cancellation, got %v", destroyer.worktreeCalls)
	}
}

func TestTeardownEnvironmentResources_MultiRepoDestroysEveryWorktree(t *testing.T) {
	svc := newResetTestService(t, &stubEnvRepo{})
	destroyer := &stubDestroyer{}
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.teardownEnvironmentResources(context.Background(), &models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", WorktreeID: "wt-primary"},
			{RepositoryID: "repo-b", WorktreeID: "wt-secondary"},
			{RepositoryID: "repo-c", WorktreeID: "wt-tertiary"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"wt-primary", "wt-secondary", "wt-tertiary"}
	if len(destroyer.worktreeCalls) != len(want) {
		t.Fatalf("worktree destroy calls = %v, want %v", destroyer.worktreeCalls, want)
	}
	for i, id := range want {
		if destroyer.worktreeCalls[i] != id {
			t.Errorf("worktree destroy call[%d] = %q, want %q", i, destroyer.worktreeCalls[i], id)
		}
	}
}

func TestTeardownEnvironmentResources_ReposOnlyEnvironmentIsNotEmpty(t *testing.T) {
	svc := newResetTestService(t, &stubEnvRepo{})
	destroyer := &stubDestroyer{}
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.teardownEnvironmentResources(context.Background(), &models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-a", WorktreeID: "wt-a"},
			{RepositoryID: "repo-b", WorktreeID: "wt-b"},
		},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(destroyer.worktreeCalls) != 2 {
		t.Fatalf("worktree destroy calls = %v, want 2", destroyer.worktreeCalls)
	}
}

func TestTeardownEnvironmentResources_GenericWorktreeFailureRemainsError(t *testing.T) {
	worktreeErr := errors.New("worktree backend unavailable")
	svc := newResetTestService(t, &stubEnvRepo{})
	svc.SetEnvironmentDestroyer(&stubDestroyer{worktreeErr: worktreeErr})

	err := svc.teardownEnvironmentResources(context.Background(), &models.TaskEnvironment{
		Repos: []*models.TaskEnvironmentRepo{{WorktreeID: "worktree-1"}},
	})

	if !errors.Is(err, worktreeErr) {
		t.Fatalf("teardown error = %v, want %v", err, worktreeErr)
	}
}

func TestCleanupTaskEnvironment_CancellationPreservesEnvironmentRow(t *testing.T) {
	repo := &stubEnvRepo{}
	svc := newResetTestService(t, repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	errs := svc.cleanupTaskEnvironment(ctx, "task-1", taskEnvironmentCleanup{
		env: &models.TaskEnvironment{ID: "environment-1", TaskID: "task-1"}, deleteRow: true,
	})

	if !errors.Is(errors.Join(errs...), context.Canceled) {
		t.Fatalf("cleanup errors = %v, want context cancellation", errs)
	}
	if repo.deleted {
		t.Fatal("environment row deleted after cancellation")
	}
}

func TestResetTaskEnvironment_ContainerDestroyFailurePreservesRow(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID:          "env-1",
		TaskID:      "task-1",
		ContainerID: "container-abc",
	}}
	destroyer := &stubDestroyer{containerErr: errors.New("docker unreachable")}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{running: false})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if err == nil {
		t.Fatal("expected error when container destroy fails")
	}
	if repo.deleted {
		t.Error("expected environment row to be preserved when destroy fails")
	}
}

func TestResetTaskEnvironment_RunningCheckErrorFailsClosed(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID:          "env-1",
		TaskID:      "task-1",
		ContainerID: "container-abc",
	}}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{err: errors.New("db locked")})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if err == nil {
		t.Fatal("expected error when running-session check fails")
	}
	if len(destroyer.containerCalls) != 0 {
		t.Errorf("expected teardown to be skipped when guard errors, got %v", destroyer.containerCalls)
	}
	if repo.deleted {
		t.Error("expected environment row to be preserved when guard errors")
	}
}

func TestResetTaskEnvironment_TeardownIsBestEffortAcrossResources(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID:          "env-1",
		TaskID:      "task-1",
		ContainerID: "container-abc",
		Repos:       []*models.TaskEnvironmentRepo{{WorktreeID: "wt-1"}},
	}}
	destroyer := &stubDestroyer{containerErr: errors.New("docker unreachable")}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{running: false})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{})
	if err == nil {
		t.Fatal("expected joined error when container destroy fails")
	}
	if len(destroyer.containerCalls) != 1 {
		t.Errorf("expected container destroy attempted, got %v", destroyer.containerCalls)
	}
	if len(destroyer.worktreeCalls) != 1 {
		t.Errorf("expected worktree destroy attempted even when container failed, got %v", destroyer.worktreeCalls)
	}
	if repo.deleted {
		t.Error("expected environment row to be preserved when any destroy fails")
	}
}

func TestResetTaskEnvironment_PushBranchFailureAbortsResetBeforeTeardown(t *testing.T) {
	repo := &stubEnvRepo{env: &models.TaskEnvironment{
		ID:     "env-1",
		TaskID: "task-1",
		Repos:  []*models.TaskEnvironmentRepo{{WorktreeID: "wt-1", WorktreePath: "/tmp/worktree"}},
	}}
	destroyer := &stubDestroyer{pushErr: errors.New("remote rejected")}
	svc := newResetTestService(t, repo)
	svc.SetSessionRunningChecker(&stubRunningChecker{running: false})
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(context.Background(), "task-1", ResetOptions{PushBranch: true})
	if err == nil {
		t.Fatal("expected error when push fails")
	}
	if destroyer.pushCalls != 1 {
		t.Errorf("expected push to be attempted once, got %d", destroyer.pushCalls)
	}
	if len(destroyer.worktreeCalls) != 0 {
		t.Error("expected teardown to be skipped when push fails")
	}
	if repo.deleted {
		t.Error("expected environment row to be preserved when push fails")
	}
}

func TestPerformTaskCleanup_TearsDownTaskEnvironmentAndDeletesRow(t *testing.T) {
	env := &models.TaskEnvironment{
		ID: "env-1", TaskID: "task-1", ContainerID: "container-abc",
		AgentctlAuthSecretID: "runtime-auth",
	}
	repo := &stubEnvRepo{env: env}
	destroyer := &stubDestroyer{}
	secretDeleter := &stubRuntimeSecretDeleter{}
	svc := newResetTestService(t, repo)
	svc.SetEnvironmentDestroyer(destroyer)
	svc.SetTaskEnvironmentRuntimeSecretDeleter(secretDeleter)

	errs := svc.performTaskCleanup(context.Background(), "task-1", nil, nil, nil, taskEnvironmentCleanup{
		env:       env,
		deleteRow: true,
	}, nil)

	if len(errs) != 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}
	if len(destroyer.containerCalls) != 1 || destroyer.containerCalls[0] != "container-abc" {
		t.Fatalf("expected container teardown, got %v", destroyer.containerCalls)
	}
	if !repo.deleted {
		t.Fatal("expected task environment row to be deleted")
	}
	if len(secretDeleter.calls) != 1 || secretDeleter.calls[0] != "env-1" {
		t.Fatalf("runtime secret cleanup calls = %v, want env-1", secretDeleter.calls)
	}
}

func TestPerformTaskCleanup_DeletesRuntimeSecretsAfterTaskRowCascade(t *testing.T) {
	env := &models.TaskEnvironment{
		ID: "env-cascaded", TaskID: "task-deleted", ExecutorType: string(models.ExecutorTypeLocalDocker),
		AgentctlAuthSecretID: "legacy-runtime-auth",
	}
	repo := &stubEnvRepo{env: env}
	secretDeleter := &stubRuntimeSecretDeleter{}
	svc := newResetTestService(t, repo)
	svc.SetTaskEnvironmentRuntimeSecretDeleter(secretDeleter)

	errs := svc.performTaskCleanup(context.Background(), "task-deleted", nil, nil, nil, taskEnvironmentCleanup{
		env: env, deleteSecrets: true,
	}, nil)

	if len(errs) != 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}
	if len(secretDeleter.calls) != 1 || secretDeleter.calls[0] != "env-cascaded" {
		t.Fatalf("runtime secret cleanup calls = %v, want env-cascaded", secretDeleter.calls)
	}
	if repo.deleted {
		t.Fatal("cleanup attempted to delete an environment row already removed by task cascade")
	}
}

// TestBuildSSHLiveStatus_StringsAndStringEncodedInts pins the projection
// from ExecutorRunning.Metadata into the popover-shaped SSHLiveStatus. The
// SSH executor writes its numeric metadata as strings (strconv.Itoa) so
// the projection must accept both "41001" and 41001 — every JSON
// round-trip through SQLite blob storage gives us strings, but direct
// in-memory writes give us ints.
func TestBuildSSHLiveStatus_StringsAndStringEncodedInts(t *testing.T) {
	got := buildSSHLiveStatus(map[string]interface{}{
		"ssh_host":                 "koi.zeval.local",
		"ssh_port":                 "2222",
		"ssh_user":                 "zeval",
		"ssh_remote_task_dir":      "/home/zeval/.kandev/tasks/task-1",
		"ssh_remote_agentctl_pid":  "4732",
		"ssh_remote_agentctl_port": "41001",
		"ssh_local_forward_port":   "59123",
		"ssh_host_fingerprint":     "SHA256:abc",
	})
	if got.Host != "koi.zeval.local" || got.Port != 2222 || got.User != "zeval" {
		t.Errorf("connection fields = %+v, want host/port/user", got)
	}
	if got.RemoteTaskDir != "/home/zeval/.kandev/tasks/task-1" {
		t.Errorf("RemoteTaskDir = %q", got.RemoteTaskDir)
	}
	if got.RemoteAgentctlPID != 4732 || got.RemoteAgentctlPort != 41001 || got.LocalForwardPort != 59123 {
		t.Errorf("agentctl fields = %+v", got)
	}
	if got.Fingerprint != "SHA256:abc" {
		t.Errorf("Fingerprint = %q", got.Fingerprint)
	}
}

func TestBuildSSHLiveStatus_NativeIntsAlsoAccepted(t *testing.T) {
	got := buildSSHLiveStatus(map[string]interface{}{
		"ssh_host":                 "h",
		"ssh_port":                 22,
		"ssh_remote_agentctl_pid":  int64(99),
		"ssh_remote_agentctl_port": float64(41001),
	})
	if got.Port != 22 || got.RemoteAgentctlPID != 99 || got.RemoteAgentctlPort != 41001 {
		t.Errorf("native int projection failed: %+v", got)
	}
}

func TestBuildSSHLiveStatus_EmptyMetadataReturnsZeroValueStruct(t *testing.T) {
	got := buildSSHLiveStatus(map[string]interface{}{})
	if got == nil {
		t.Fatal("expected non-nil status (zero value), got nil")
	}
	if got.Host != "" || got.Port != 0 || got.RemoteAgentctlPID != 0 {
		t.Errorf("expected zero-value fields, got %+v", got)
	}
}

func TestBuildSSHLiveStatus_InvalidPortString_NoCrash(t *testing.T) {
	// SSH executor only emits Itoa'd ports, but be defensive in case
	// something else writes the metadata (e.g. a future migration or
	// import path). Don't want a stray non-numeric value to panic the
	// popover endpoint for every other field too.
	got := buildSSHLiveStatus(map[string]interface{}{
		"ssh_host": "h",
		"ssh_port": "not-a-port",
	})
	if got.Host != "h" || got.Port != 0 {
		t.Errorf("expected host preserved, port=0 on bad input, got %+v", got)
	}
}
