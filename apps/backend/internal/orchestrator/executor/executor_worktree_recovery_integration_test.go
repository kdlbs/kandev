package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/repoclone"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func seedSelectedWorktreeRecoveryEnvironment(
	repo *mockRepository,
	taskID, sessionID string,
	sessionState models.TaskSessionState,
) {
	repo.repositories["repo-recovery"] = &models.Repository{
		ID:                   "repo-recovery",
		Name:                 "recovery",
		LocalPath:            "/repos/recovery",
		WorktreeBranchPrefix: "feature/",
	}
	repo.taskRepositories["task-repo-recovery"] = &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: taskID, RepositoryID: "repo-recovery", Position: 0, BaseBranch: "main",
	}
	repo.executors[models.ExecutorIDWorktree] = &models.Executor{
		ID: models.ExecutorIDWorktree, Type: models.ExecutorTypeWorktree, Status: models.ExecutorStatusActive,
	}
	environmentRepos := []*models.TaskEnvironmentRepo{{
		ID:                "environment-repo-recovery",
		TaskEnvironmentID: "environment-recovery",
		RepositoryID:      "repo-recovery",
		BranchSlug:        "main",
		WorktreeID:        "worktree-recovery",
		WorktreePath:      "/tasks/recovery/recovery",
		WorktreeBranch:    "feature/recovery",
		Status:            "active",
		Position:          0,
	}}
	repo.taskEnvironments["environment-recovery"] = &models.TaskEnvironment{
		ID:                  "environment-recovery",
		TaskID:              taskID,
		OwnershipGeneration: 1,
		ExecutorType:        string(models.ExecutorTypeWorktree),
		Status:              models.TaskEnvironmentStatusReady,
		WorkspacePath:       "/tasks/recovery/recovery",
		TaskDirName:         "recovery_abc",
		Repos:               environmentRepos,
	}
	repo.taskEnvironmentRepos["environment-recovery"] = environmentRepos
	repo.sessions[sessionID] = &models.TaskSession{
		ID:                sessionID,
		TaskID:            taskID,
		TaskEnvironmentID: "environment-recovery",
		AgentProfileID:    "profile-recovery",
		ExecutorID:        models.ExecutorIDWorktree,
		RepositoryID:      "repo-recovery",
		BaseBranch:        "main",
		State:             sessionState,
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
}

func TestWorktreeRecoveryLaunchIntegration(t *testing.T) {
	const taskID = "task-recovery-launch"
	const sessionID = "session-recovery-launch"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCreated)

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.LaunchPreparedSession(context.Background(), &v1.Task{
		ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery launch",
	}, sessionID, LaunchOptions{
		AgentProfileID: "profile-recovery",
		ExecutorID:     models.ExecutorIDWorktree,
		StartAgent:     false,
	})
	if err != nil {
		t.Fatalf("LaunchPreparedSession: %v", err)
	}
	if admissionCalls != 1 {
		t.Fatalf("selected recovery admission calls = %d, want 1", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func TestPreparedSessionRecoveryBindsSelectedEnvironmentBeforeAdmission(t *testing.T) {
	const taskID = "task-prepared-recovery-binding"
	const existingSessionID = "session-prepared-recovery-existing"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, existingSessionID, models.TaskSessionStateCancelled)

	var admissionRequest worktree.RecoveryAdmissionRequest
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.prepareSessionAttempt(context.Background(), &v1.Task{
		ID: taskID, WorkspaceID: "workspace-recovery", Title: "Prepared recovery binding",
	}, "profile-recovery", models.ExecutorIDWorktree, "", "", true, "", nil)
	if err != nil {
		t.Fatalf("prepareSessionAttempt: %v", err)
	}
	if got := admissionRequest.SelectionSnapshot.SessionTaskEnvironmentID; got != "environment-recovery" {
		t.Fatalf("snapshot session environment ID = %q, want selected environment ID", got)
	}
	if !admissionRequest.SelectionSnapshot.SessionEnvironmentMatchesSelected() {
		t.Fatal("prepared-session recovery snapshot does not match its selected environment")
	}
}

func TestWorktreeRecoveryResumeIntegration(t *testing.T) {
	const taskID = "task-recovery-resume"
	const sessionID = "session-recovery-resume"
	repo := newMockRepository()
	seedSelectedWorktreeRecoveryEnvironment(repo, taskID, sessionID, models.TaskSessionStateCancelled)
	repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery resume"}

	var admissionRequest worktree.RecoveryAdmissionRequest
	admissionCalls := 0
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	exec.SetSelectedWorktreeRecoveryAdmission(func(_ context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
		admissionCalls++
		admissionRequest = req
		return nil, nil
	})

	_, err := exec.ResumeSession(context.Background(), repo.sessions[sessionID], false)
	if err != nil {
		t.Fatalf("ResumeSession: %v", err)
	}
	if admissionCalls != 2 {
		t.Fatalf("selected recovery admission calls = %d, want preflight and pre-launch inspection", admissionCalls)
	}
	assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
}

func TestMainCheckoutLaunchIntegration(t *testing.T) {
	cases := []struct {
		name       string
		state      models.TaskSessionState
		additional bool
		resume     bool
	}{
		{name: "prepared launch", state: models.TaskSessionStateCreated},
		{name: "additional session launch", state: models.TaskSessionStateCreated, additional: true},
		{name: "resume", state: models.TaskSessionStateCancelled, resume: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const taskID = "task-main-checkout-launch"
			const primarySessionID = "session-main-checkout-primary"
			const additionalSessionID = "session-main-checkout-additional"
			repo := newMockRepository()
			seedSelectedWorktreeRecoveryEnvironment(repo, taskID, primarySessionID, tc.state)
			mainPath, before := createExecutorMainCheckout(t)
			worktreeRecord := repo.taskEnvironmentRepos["environment-recovery"][0]
			repository := repo.repositories["repo-recovery"]
			environment := repo.taskEnvironments["environment-recovery"]
			repository.LocalPath = mainPath
			worktreeRecord.WorktreePath = mainPath
			worktreeRecord.WorktreeBranch = before.branch
			environment.WorkspacePath = mainPath
			wt := &worktree.Worktree{
				ID: "worktree-recovery", TaskID: taskID, SessionID: primarySessionID,
				TaskEnvironmentID: "environment-recovery", RepositoryID: "repo-recovery",
				TaskDirName: "recovery_abc", BranchSlug: "main", RepositoryPath: mainPath,
				Path: mainPath, Branch: before.branch, Status: worktree.StatusActive,
			}
			store := &mainCheckoutRecoveryStore{worktree: wt, repository: repo}
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			if err != nil {
				t.Fatalf("create logger: %v", err)
			}
			t.Cleanup(func() { _ = log.Close() })
			manager, err := worktree.NewManager(worktree.Config{
				Enabled: true, TasksBasePath: filepath.Join(t.TempDir(), "managed-tasks"),
				BranchPrefix: "feature/",
			}, store, log)
			if err != nil {
				t.Fatalf("create worktree manager: %v", err)
			}
			sessionID := primarySessionID
			if tc.additional {
				repo.sessions[primarySessionID].State = models.TaskSessionStateRunning
				additional := *repo.sessions[primarySessionID]
				additional.ID = additionalSessionID
				additional.State = models.TaskSessionStateCreated
				repo.sessions[additionalSessionID] = &additional
				sessionID = additionalSessionID
			}
			if tc.resume {
				repo.tasks[taskID] = &models.Task{ID: taskID, WorkspaceID: "workspace-recovery", Title: "Recovery resume"}
			}

			var admissionRequest worktree.RecoveryAdmissionRequest
			admissionCalls := 0
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			exec.SetSelectedWorktreeRecoveryAdmission(func(ctx context.Context, req worktree.RecoveryAdmissionRequest) (*worktree.RecoveryAdmission, error) {
				admissionCalls++
				admissionRequest = req
				return manager.AdmitRecovery(ctx, req)
			})
			if tc.resume {
				_, err = exec.ResumeSession(context.Background(), repo.sessions[sessionID], false)
			} else {
				_, err = exec.LaunchPreparedSession(context.Background(), &v1.Task{
					ID: taskID, WorkspaceID: "workspace-recovery", Title: "Main checkout launch",
				}, sessionID, LaunchOptions{
					AgentProfileID: "profile-recovery", ExecutorID: models.ExecutorIDWorktree, StartAgent: false,
				})
			}
			if err != nil {
				t.Fatalf("main-checkout launch path: %v", err)
			}
			expectedAdmissionCalls := 1
			if tc.resume {
				expectedAdmissionCalls = 2
			}
			if admissionCalls != expectedAdmissionCalls {
				t.Fatalf("selected recovery admission calls = %d, want %d", admissionCalls, expectedAdmissionCalls)
			}
			assertSelectedWorktreeRecoveryRequest(t, admissionRequest, taskID, sessionID)
			if got := admissionRequest.Slots[0].RepositoryPath; got != mainPath {
				t.Fatalf("selected repository path = %q, want canonical main checkout %q", got, mainPath)
			}
			assertExecutorMainCheckoutState(t, mainPath, before)
			if _, err := os.Lstat(mainPath + ".kandev-recovery.json"); !os.IsNotExist(err) {
				t.Fatalf("main checkout received recovery record: %v", err)
			}
			if store.worktree != wt || store.worktree.Path != mainPath {
				t.Fatal("selected admission replaced the canonical worktree record")
			}
			if tc.additional {
				if primary := repo.sessions[primarySessionID]; primary == nil || primary.ID != primarySessionID || primary.State != models.TaskSessionStateRunning {
					t.Fatalf("additional-session launch changed the live primary session: %+v", primary)
				}
			}
		})
	}
}

type mainCheckoutRecoveryStore struct {
	worktree.Store
	worktree   *worktree.Worktree
	repository *mockRepository
}

func (s *mainCheckoutRecoveryStore) GetWorktreeByID(_ context.Context, id string) (*worktree.Worktree, error) {
	if s.worktree == nil || s.worktree.ID != id {
		return nil, nil
	}
	return s.worktree, nil
}

func (s *mainCheckoutRecoveryStore) ReadRecoverySelectionSnapshot(
	_ context.Context,
	expected models.WorkspaceRecoverySelectionSnapshot,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	if s.repository == nil {
		return models.WorkspaceRecoverySelectionSnapshot{}, fmt.Errorf("recovery repository unavailable")
	}
	session := s.repository.sessions[expected.SessionID]
	environment := s.repository.taskEnvironments[expected.TaskEnvironmentID]
	if session == nil || environment == nil {
		return models.WorkspaceRecoverySelectionSnapshot{}, fmt.Errorf("selected recovery records missing")
	}
	return models.CaptureWorkspaceRecoverySelectionSnapshot(session, environment, func(id string) (*models.Repository, error) {
		repository := s.repository.repositories[id]
		if repository == nil {
			return nil, fmt.Errorf("selected recovery repository %q missing", id)
		}
		return repository, nil
	})
}

type executorMainCheckoutSnapshot struct {
	branch string
	head   string
	index  []byte
	files  map[string][]byte
}

func createExecutorMainCheckout(t *testing.T) (string, executorMainCheckoutSnapshot) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create main checkout: %v", err)
	}
	gitCommandInTest(t, path, "init", "-b", "main")
	gitCommandInTest(t, path, "config", "user.email", "test@example.com")
	gitCommandInTest(t, path, "config", "user.name", "Test User")
	writeExecutorMainCheckoutFile(t, path, ".gitignore", "ignored.txt\n")
	writeExecutorMainCheckoutFile(t, path, "tracked.txt", "committed\n")
	gitCommandInTest(t, path, "add", ".gitignore", "tracked.txt")
	gitCommandInTest(t, path, "commit", "-m", "initial commit")
	gitCommandInTest(t, path, "checkout", "-b", "feature/main-checkout")
	writeExecutorMainCheckoutFile(t, path, "tracked.txt", "unstaged edit\n")
	writeExecutorMainCheckoutFile(t, path, "staged.txt", "staged edit\n")
	gitCommandInTest(t, path, "add", "staged.txt")
	writeExecutorMainCheckoutFile(t, path, "untracked.txt", "untracked data\n")
	writeExecutorMainCheckoutFile(t, path, "ignored.txt", "ignored data\n")
	return path, captureExecutorMainCheckoutState(t, path)
}

func writeExecutorMainCheckoutFile(t *testing.T, root, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
		t.Fatalf("write main checkout file %q: %v", name, err)
	}
}

func captureExecutorMainCheckoutState(t *testing.T, path string) executorMainCheckoutSnapshot {
	t.Helper()
	snapshot := executorMainCheckoutSnapshot{
		branch: gitOutputInTest(t, path, "symbolic-ref", "--short", "HEAD"),
		head:   gitOutputInTest(t, path, "rev-parse", "--verify", "HEAD"),
		files:  make(map[string][]byte),
	}
	var err error
	snapshot.index, err = os.ReadFile(filepath.Join(path, ".git", "index"))
	if err != nil {
		t.Fatalf("read main checkout index: %v", err)
	}
	for _, name := range []string{"tracked.txt", "staged.txt", "untracked.txt", "ignored.txt"} {
		snapshot.files[name], err = os.ReadFile(filepath.Join(path, name))
		if err != nil {
			t.Fatalf("read main checkout file %q: %v", name, err)
		}
	}
	return snapshot
}

func assertExecutorMainCheckoutState(t *testing.T, path string, want executorMainCheckoutSnapshot) {
	t.Helper()
	got := captureExecutorMainCheckoutState(t, path)
	if got.branch != want.branch || got.head != want.head || string(got.index) != string(want.index) {
		t.Fatalf("main checkout identity/index changed: got branch=%q head=%q index=%x, want branch=%q head=%q index=%x",
			got.branch, got.head, got.index, want.branch, want.head, want.index)
	}
	for name, contents := range want.files {
		if string(got.files[name]) != string(contents) {
			t.Fatalf("main checkout file %q changed: got %q, want %q", name, got.files[name], contents)
		}
	}
}

func TestManagedCloneRelocationProofIncludesLegacyOwnerNamePath(t *testing.T) {
	root := t.TempDir()
	cloner := repoclone.NewCloner(repoclone.Config{BasePath: root}, repoclone.ProtocolHTTPS, "", nil)
	repository := &models.Repository{
		ID: "repo-legacy", WorkspaceID: "workspace-legacy", SourceType: "provider", Provider: "github",
		ProviderHost: "https://github.com", ProviderOwner: "acme", ProviderName: "widget",
	}
	proof := managedCloneRelocationProof(cloner, repository, "", "")
	if proof == nil {
		t.Fatal("managedCloneRelocationProof() returned nil for a managed provider repository")
	}
	if want := filepath.Join(root, "acme", "widget"); proof.LegacyOwnerNameSourcePath != want {
		t.Fatalf("legacy owner/name source = %q, want %q", proof.LegacyOwnerNameSourcePath, want)
	}
}

func assertSelectedWorktreeRecoveryRequest(
	t *testing.T,
	req worktree.RecoveryAdmissionRequest,
	taskID, sessionID string,
) {
	t.Helper()
	if req.TaskID != taskID || req.SessionID != sessionID {
		t.Fatalf("admission identity = task %q/session %q, want %q/%q", req.TaskID, req.SessionID, taskID, sessionID)
	}
	if req.TaskEnvironmentID != "environment-recovery" || req.OwnerTaskID != taskID {
		t.Fatalf("admission environment = %q, owner %q, want environment-recovery/%q", req.TaskEnvironmentID, req.OwnerTaskID, taskID)
	}
	if req.OwnershipGeneration != 1 || req.ExecutorType != string(models.ExecutorTypeWorktree) {
		t.Fatalf("admission authority = generation %d/executor %q, want 1/worktree", req.OwnershipGeneration, req.ExecutorType)
	}
	if len(req.Slots) != 1 {
		t.Fatalf("admission slots = %d, want 1", len(req.Slots))
	}
	slot := req.Slots[0]
	if slot.WorktreeID != "worktree-recovery" || slot.RepositoryID != "repo-recovery" || slot.BranchSlug != "main" {
		t.Fatalf("admission slot = %+v, want selected canonical worktree", slot)
	}
}
