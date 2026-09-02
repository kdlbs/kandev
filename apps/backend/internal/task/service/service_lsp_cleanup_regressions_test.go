package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestTaskLSPTreatsDeletedInheritedEnvironmentAsUnavailable(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-deleted", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-deleted",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteTaskEnvironment(ctx, "env-deleted"); err != nil {
		t.Fatal(err)
	}

	environment, err := svc.GetTaskEnvironmentForTaskLSP(ctx, "task-child")
	if err != nil {
		t.Fatalf("stale inherited environment returned an error: %v", err)
	}
	if environment != nil {
		t.Fatalf("stale inherited environment = %#v, want nil", environment)
	}
}

func TestTaskLSPIgnoresDeletedEnvironmentWhenCurrentEnvironmentExists(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	for _, taskID := range []string{"task-child", "task-live-owner"} {
		if err := repo.CreateTask(ctx, &models.Task{
			ID: taskID, WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
			Title: taskID, Priority: "medium",
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, environment := range []*models.TaskEnvironment{
		{ID: "env-deleted", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocalDocker)},
		{ID: "env-live", TaskID: "task-live-owner", ExecutorType: string(models.ExecutorTypeLocalDocker)},
	} {
		if err := repo.CreateTaskEnvironment(ctx, environment); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for index, environmentID := range []string{"env-deleted", "env-live"} {
		if err := repo.CreateTaskSession(ctx, &models.TaskSession{
			ID: "session-child-" + environmentID, TaskID: "task-child", TaskEnvironmentID: environmentID,
			State: models.TaskSessionStateCompleted, StartedAt: now.Add(time.Duration(index) * time.Minute),
			UpdatedAt: now.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.DeleteTaskEnvironment(ctx, "env-deleted"); err != nil {
		t.Fatal(err)
	}

	environment, err := svc.GetTaskEnvironmentForTaskLSP(ctx, "task-child")
	if err != nil {
		t.Fatalf("resolve current inherited environment: %v", err)
	}
	if environment == nil || environment.ID != "env-live" {
		t.Fatalf("current inherited environment = %#v, want env-live", environment)
	}
}

func TestTaskLSPBorrowerUsesPhysicalDockerRepositoryPositions(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	for _, repository := range []*models.Repository{
		{ID: "repo-alpha", WorkspaceID: "ws-1", Name: "alpha", LocalPath: "/source/alpha", DefaultBranch: "main"},
		{ID: "repo-beta", WorkspaceID: "ws-1", Name: "beta", LocalPath: "/source/beta", DefaultBranch: "main"},
	} {
		if err := repo.CreateRepository(ctx, repository); err != nil {
			t.Fatal(err)
		}
	}
	for position, repositoryID := range []string{"repo-beta", "repo-alpha"} {
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: "child-" + repositoryID, TaskID: "task-child", RepositoryID: repositoryID,
			BaseBranch: "main", Position: position,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-docker", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status: models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{
			{RepositoryID: "repo-alpha", BranchSlug: "main", Position: 0},
			{RepositoryID: "repo-beta", BranchSlug: "main", Position: 1},
		},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-docker",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	info, err := svc.GetWorkspaceInfoForTaskLSP(ctx, "task-child", "env-docker")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.WorkspaceRepositories) != 2 {
		t.Fatalf("borrower repository projection = %#v", info.WorkspaceRepositories)
	}
	wantPhysicalPositions := map[string]int{"repo-alpha": 0, "repo-beta": 1}
	for _, repository := range info.WorkspaceRepositories {
		if repository.TaskHostPosition == nil || *repository.TaskHostPosition != wantPhysicalPositions[repository.RepositoryID] {
			t.Fatalf("repository %q physical position = %v, want %d",
				repository.RepositoryID, repository.TaskHostPosition, wantPhysicalPositions[repository.RepositoryID])
		}
	}
}

func TestTaskLSPBorrowerRejectsMissingDockerRepositoryMapping(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-beta", WorkspaceID: "ws-1", Name: "beta", LocalPath: "/source/beta", DefaultBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "child-repo-beta", TaskID: "task-child", RepositoryID: "repo-beta", BaseBranch: "main",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-legacy-docker", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocalDocker),
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-legacy-docker",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.GetWorkspaceInfoForTaskLSP(ctx, "task-child", "env-legacy-docker"); err == nil ||
		!strings.Contains(err.Error(), "physical repository mapping") {
		t.Fatalf("missing Docker physical mapping error = %v", err)
	}
}

func TestResetTaskEnvironmentBlocksWarmBorrower(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-shared", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocal),
		ContainerID: "container-shared", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-shared",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	destroyer := &stubDestroyer{}
	svc.SetEnvironmentDestroyer(destroyer)

	err := svc.ResetTaskEnvironment(ctx, "task-123", ResetOptions{})
	if !errors.Is(err, ErrEnvironmentShared) {
		t.Fatalf("warm borrower reset error = %v, want ErrEnvironmentShared", err)
	}
	if len(destroyer.containerCalls) != 0 {
		t.Fatalf("shared environment was destroyed: %v", destroyer.containerCalls)
	}
	if environment, lookupErr := repo.GetTaskEnvironment(ctx, "env-shared"); lookupErr != nil || environment == nil {
		t.Fatalf("shared environment row removed: environment=%#v err=%v", environment, lookupErr)
	}
}

func TestResetWaitsForBorrowerTaskLSPAdmissionOnPhysicalEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-shared", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocal),
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-shared",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	releaseAdmission, err := svc.AcquireTaskLSPAdmission(ctx, "task-child")
	if err != nil {
		t.Fatal(err)
	}
	resetDone := make(chan error, 1)
	go func() { resetDone <- svc.ResetTaskEnvironment(ctx, "task-123", ResetOptions{}) }()
	select {
	case err := <-resetDone:
		releaseAdmission()
		t.Fatalf("reset crossed active borrower admission: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseAdmission()
	if err := <-resetDone; !errors.Is(err, ErrEnvironmentShared) {
		t.Fatalf("reset after borrower admission = %v, want ErrEnvironmentShared", err)
	}
}

func TestBorrowerTaskLSPAdmissionFailsWhilePhysicalEnvironmentResets(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "task-child", WorkspaceID: "ws-1", WorkflowID: "wf-123", WorkflowStepID: "step-123",
		Title: "task-child", Priority: "medium",
	}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-resetting", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocal),
		ContainerID: "container-resetting", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	destroyer := &stubDestroyer{
		containerEntered: make(chan struct{}), containerRelease: make(chan struct{}),
	}
	svc.SetEnvironmentDestroyer(destroyer)
	resetDone := make(chan error, 1)
	go func() { resetDone <- svc.ResetTaskEnvironment(ctx, "task-123", ResetOptions{}) }()
	<-destroyer.containerEntered

	now := time.Now().UTC()
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-child", TaskID: "task-child", TaskEnvironmentID: "env-resetting",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	release, err := svc.AcquireTaskLSPAdmission(ctx, "task-child")
	if release != nil {
		release()
	}
	if !errors.Is(err, ErrTaskLSPAdmissionBlocked) {
		close(destroyer.containerRelease)
		<-resetDone
		t.Fatalf("borrower admission during physical reset = %v, want blocked", err)
	}

	close(destroyer.containerRelease)
	if err := <-resetDone; err != nil {
		t.Fatal(err)
	}
}

func TestTaskLSPAdmissionFailsWhileAsyncEnvironmentCleanupRuns(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-async-cleanup", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocal),
		ContainerID: "container-async-cleanup", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	destroyer := &stubDestroyer{
		containerEntered: make(chan struct{}), containerRelease: make(chan struct{}),
	}
	svc.SetEnvironmentDestroyer(destroyer)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))

	if err := svc.ArchiveTask(ctx, "task-123"); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	select {
	case <-destroyer.containerEntered:
	case <-time.After(time.Second):
		t.Fatal("async environment teardown did not start")
	}

	release, err := svc.AcquireTaskLSPAdmission(ctx, "task-123")
	if release != nil {
		release()
	}
	if !errors.Is(err, ErrTaskLSPAdmissionBlocked) {
		close(destroyer.containerRelease)
		waitForCleanupDone(t, svc)
		t.Fatalf("LSP admission during async environment teardown = %v, want blocked", err)
	}

	close(destroyer.containerRelease)
	waitForCleanupDone(t, svc)
}

func TestTaskLSPAdmissionFailsWhileCascadeEnvironmentCleanupRuns(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-cascade-cleanup", TaskID: "task-123", ExecutorType: string(models.ExecutorTypeLocal),
		ContainerID: "container-cascade-cleanup", Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatal(err)
	}
	destroyer := &stubDestroyer{
		containerEntered: make(chan struct{}), containerRelease: make(chan struct{}),
	}
	svc.SetEnvironmentDestroyer(destroyer)
	svc.setCleanupDoneForTestHook(make(chan struct{}, 1))
	handoff := NewHandoffService(repo, nil, nil, nil, nil, nil)
	handoff.SetTaskResourceCleaner(svc)

	if _, err := handoff.ArchiveTaskTree(ctx, "task-123", false); err != nil {
		t.Fatalf("archive task tree: %v", err)
	}
	select {
	case <-destroyer.containerEntered:
	case <-time.After(time.Second):
		t.Fatal("cascade environment teardown did not start")
	}

	release, err := svc.AcquireTaskLSPAdmission(ctx, "task-123")
	if release != nil {
		release()
	}
	if !errors.Is(err, ErrTaskLSPAdmissionBlocked) {
		close(destroyer.containerRelease)
		waitForCleanupDone(t, svc)
		t.Fatalf("LSP admission during cascade environment teardown = %v, want blocked", err)
	}

	close(destroyer.containerRelease)
	waitForCleanupDone(t, svc)
}

func TestTaskResourceCleanupSnapshotPersistsLegacyRuntimeSecretReferences(t *testing.T) {
	taskSvc, repo := setupOfficeTest(t)
	env := &models.TaskEnvironment{
		ID: "env-legacy-secrets", TaskID: "task-legacy-secrets",
		AgentctlAuthSecretID: "legacy-auth-secret-id", AgentctlBootstrapSecretID: "legacy-bootstrap-secret-id",
	}

	job, err := taskSvc.persistTaskResourceCleanup(
		context.Background(), "task-legacy-secrets", models.TaskResourceCleanupTriggerDelete, "",
		nil, nil, nil, taskEnvironmentCleanup{env: env}, true,
	)
	if err != nil {
		t.Fatalf("persistTaskResourceCleanup: %v", err)
	}
	stored, err := repo.GetTaskResourceCleanupJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetTaskResourceCleanupJob: %v", err)
	}
	for _, secretID := range []string{"legacy-auth-secret-id", "legacy-bootstrap-secret-id"} {
		if !strings.Contains(stored.ResourceSnapshot, secretID) {
			t.Fatalf("resource snapshot omitted legacy secret reference %q: %s", secretID, stored.ResourceSnapshot)
		}
	}
}
