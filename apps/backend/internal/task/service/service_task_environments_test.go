package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
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
	containerCalls []string
	sandboxCalls   []string
	worktreeCalls  []string
	pushCalls      int
	containerErr   error
	sandboxErr     error
	worktreeErr    error
	pushErr        error
}

func (s *stubDestroyer) DestroyContainer(_ context.Context, id string) error {
	s.containerCalls = append(s.containerCalls, id)
	return s.containerErr
}
func (s *stubDestroyer) DestroySandbox(_ context.Context, id, _ string) error {
	s.sandboxCalls = append(s.sandboxCalls, id)
	return s.sandboxErr
}
func (s *stubDestroyer) DestroyWorktree(_ context.Context, id string) error {
	s.worktreeCalls = append(s.worktreeCalls, id)
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
		WorktreeID:  "wt-1",
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
		WorktreeID:  "wt-1",
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
		ID:           "env-1",
		TaskID:       "task-1",
		WorktreeID:   "wt-1",
		WorktreePath: "/tmp/worktree",
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
		ID:          "env-1",
		TaskID:      "task-1",
		ContainerID: "container-abc",
	}
	repo := &stubEnvRepo{env: env}
	destroyer := &stubDestroyer{}
	svc := newResetTestService(t, repo)
	svc.SetEnvironmentDestroyer(destroyer)

	errs := svc.performTaskCleanup(context.Background(), "task-1", nil, nil, taskEnvironmentCleanup{
		env:       env,
		deleteRow: true,
	}, false)

	if len(errs) != 0 {
		t.Fatalf("unexpected cleanup errors: %v", errs)
	}
	if len(destroyer.containerCalls) != 1 || destroyer.containerCalls[0] != "container-abc" {
		t.Fatalf("expected container teardown, got %v", destroyer.containerCalls)
	}
	if !repo.deleted {
		t.Fatal("expected task environment row to be deleted")
	}
}
