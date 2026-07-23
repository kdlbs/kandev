package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

type nilTaskSessionRepo struct {
	repository.SessionRepository
}

func TestStartTurnPersistsImmutableEffectiveRuntimeConfigSnapshot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()

	options := []streams.ConfigOption{
		{
			Type: "select", ID: "collaboration_mode", Name: "Collaboration mode", CurrentValue: "default",
			Options: []streams.ConfigOptionValue{{Value: "default", Name: "Default"}},
		},
		{
			Type: "select", ID: "reasoning_effort", Name: "Reasoning effort", CurrentValue: "medium",
			Options: []streams.ConfigOptionValue{
				{Value: "medium", Name: "Medium"},
				{Value: "high", Name: "High"},
				{Value: "low", Name: "Low"},
			},
		},
	}
	session := &models.TaskSession{
		ID:        "session-turn-config",
		TaskID:    "task-123",
		State:     models.TaskSessionStateRunning,
		StartedAt: now,
		UpdatedAt: now,
		AgentProfileSnapshot: map[string]interface{}{
			"model": "profile-model",
			"mode":  "default",
		},
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{
				Model: "gpt-5.6-sol", Mode: "agent",
				ConfigOptions: map[string]string{
					"collaboration_mode": "default",
					"reasoning_effort":   "medium",
				},
			},
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{
				ConfigOptions: map[string]string{"reasoning_effort": "high"},
			},
			models.SessionMetaKeyACPConfigBaseline: map[string]string{
				"collaboration_mode": "default",
				"reasoning_effort":   "medium",
			},
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID: "gpt-5.6-sol",
				ConfigOptions:  options,
			},
		},
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	first, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn first: %v", err)
	}
	if err := svc.CompleteTurn(ctx, first.ID); err != nil {
		t.Fatalf("CompleteTurn first: %v", err)
	}
	if err := svc.PersistSessionRuntimeConfigOption(ctx, session.ID, "reasoning_effort", "low"); err != nil {
		t.Fatalf("PersistSessionRuntimeConfigOption: %v", err)
	}
	second, err := svc.StartTurn(ctx, session.ID)
	if err != nil {
		t.Fatalf("StartTurn second: %v", err)
	}

	storedFirst, err := repo.GetTurn(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetTurn first: %v", err)
	}
	storedSecond, err := repo.GetTurn(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetTurn second: %v", err)
	}
	firstSnapshot, ok := models.LoadTurnRuntimeConfigSnapshot(storedFirst.Metadata)
	if !ok {
		t.Fatal("first turn metadata does not contain a runtime config snapshot")
	}
	secondSnapshot, ok := models.LoadTurnRuntimeConfigSnapshot(storedSecond.Metadata)
	if !ok {
		t.Fatal("second turn metadata does not contain a runtime config snapshot")
	}

	if firstSnapshot.Model != "gpt-5.6-sol" || firstSnapshot.Mode != "agent" {
		t.Fatalf("first model/mode = %q/%q", firstSnapshot.Model, firstSnapshot.Mode)
	}
	wantFirstOptions := []models.TurnRuntimeConfigOption{
		{ID: "collaboration_mode", Name: "Collaboration mode", Value: "default", ValueName: "Default"},
		{ID: "reasoning_effort", Name: "Reasoning effort", Value: "high", ValueName: "High"},
	}
	require.Equal(t, wantFirstOptions, firstSnapshot.ConfigOptions, "first turn config options")
	if firstSnapshot.ConfigBaseline["reasoning_effort"] != "medium" {
		t.Fatalf("first baseline = %#v", firstSnapshot.ConfigBaseline)
	}
	if secondSnapshot.ConfigOptions[1].Value != "low" || secondSnapshot.ConfigOptions[1].ValueName != "Low" {
		t.Fatalf("second reasoning option = %#v", secondSnapshot.ConfigOptions[1])
	}
	if firstSnapshot.ConfigOptions[1].Value != "high" {
		t.Fatalf("first turn was relabeled after later override: %#v", firstSnapshot.ConfigOptions[1])
	}
}

func TestBuildTurnRuntimeConfigSnapshotFallsBackToSelectorModel(t *testing.T) {
	snapshot := buildTurnRuntimeConfigSnapshot(&models.TaskSession{
		Metadata: map[string]interface{}{
			models.SessionMetaKeyACPModelState: lifecycle.SessionModelsSnapshot{
				CurrentModelID: "selector-model",
			},
		},
	})

	if snapshot.Model != "selector-model" {
		t.Fatalf("snapshot model = %q, want selector-model", snapshot.Model)
	}
}

func (nilTaskSessionRepo) GetTaskSession(context.Context, string) (*models.TaskSession, error) {
	return nil, nil
}

func (nilTaskSessionRepo) SetSessionMetadataKey(context.Context, string, string, interface{}) error {
	panic("SetSessionMetadataKey should not be called for a nil session")
}

func TestGetWorkspaceInfoForSession_BasicFields(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:                 "session-1",
		TaskID:             "task-123",
		TaskEnvironmentID:  "env-123",
		AgentProfileID:     "profile-1",
		ExecutionProfileID: "claude-opus",
		State:              models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		Metadata: map[string]interface{}{
			"acp_session_id": "acp-123",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Add a worktree to the session
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID:             "wt1",
		SessionID:      "session-1",
		WorktreeID:     "wid1",
		RepositoryID:   "repo1",
		WorktreePath:   "/tmp/worktrees/session-1",
		WorktreeBranch: "feature/test",
		CreatedAt:      now,
	}); err != nil {
		t.Fatalf("failed to create worktree: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123', got %q", info.TaskID)
	}
	if info.SessionID != "session-1" {
		t.Errorf("expected SessionID 'session-1', got %q", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("expected TaskEnvironmentID 'env-123', got %q", info.TaskEnvironmentID)
	}
	if info.WorkspacePath != "/tmp/worktrees/session-1" {
		t.Errorf("expected WorkspacePath '/tmp/worktrees/session-1', got %q", info.WorkspacePath)
	}
	if info.AgentProfileID != "profile-1" {
		t.Errorf("expected AgentProfileID 'profile-1', got %q", info.AgentProfileID)
	}
	if info.ExecutionProfileID != "claude-opus" {
		t.Errorf("expected ExecutionProfileID 'claude-opus', got %q", info.ExecutionProfileID)
	}
	if info.AgentID != "auggie" {
		t.Errorf("expected AgentID 'auggie', got %q", info.AgentID)
	}
	if info.ACPSessionID != "acp-123" {
		t.Errorf("expected ACPSessionID 'acp-123', got %q", info.ACPSessionID)
	}
}

func TestGetWorkspaceInfoForSession_ProjectsPersistedWorktreeIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-recovery", WorkspaceID: "ws-1", Name: "api", LocalPath: "/source/api", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "task-repo-recovery", TaskID: "task-123", RepositoryID: "repo-recovery", BaseBranch: "main", CheckoutBranch: "feature/recovery",
	}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-recovery", TaskID: "task-123", State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
		ID: "session-worktree-recovery", SessionID: "session-recovery", WorktreeID: "worktree-recovery", RepositoryID: "repo-recovery",
		BranchSlug: "feature-recovery", WorktreePath: "/tasks/task-recovery/api-feature-recovery", CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSessionWorktree: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-recovery")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if len(info.WorkspaceRepositories) != 1 {
		t.Fatalf("WorkspaceRepositories = %#v, want one repository", info.WorkspaceRepositories)
	}
	got := info.WorkspaceRepositories[0]
	if got.WorktreeID != "worktree-recovery" || got.BranchSlug != "feature-recovery" || got.BranchIdentitySlug != "feature-recovery" {
		t.Fatalf("recovery repository identity = %+v, want persisted worktree ID and branch slug", got)
	}
}

func TestGetWorkspaceInfoForSession_ProjectsDistinctWorktreesForSameRepositoryBranches(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-multi-recovery", WorkspaceID: "ws-1", Name: "api", LocalPath: "/source/api", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	for position, branch := range []string{"feature/one", "feature/two"} {
		if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{
			ID: fmt.Sprintf("task-repo-multi-%d", position), TaskID: "task-123", RepositoryID: "repo-multi-recovery",
			BaseBranch: "main", CheckoutBranch: branch, Position: position,
		}); err != nil {
			t.Fatalf("CreateTaskRepository %q: %v", branch, err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-multi-recovery", TaskID: "task-123", State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	for _, worktree := range []*models.TaskSessionWorktree{
		{ID: "session-worktree-one", SessionID: "session-multi-recovery", WorktreeID: "worktree-one", RepositoryID: "repo-multi-recovery", BranchSlug: "feature-one", CreatedAt: now},
		{ID: "session-worktree-two", SessionID: "session-multi-recovery", WorktreeID: "worktree-two", RepositoryID: "repo-multi-recovery", BranchSlug: "feature-two", CreatedAt: now},
	} {
		if err := repo.CreateTaskSessionWorktree(ctx, worktree); err != nil {
			t.Fatalf("CreateTaskSessionWorktree %q: %v", worktree.BranchSlug, err)
		}
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-multi-recovery")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if len(info.WorkspaceRepositories) != 2 {
		t.Fatalf("WorkspaceRepositories = %#v, want two branch-specific repositories", info.WorkspaceRepositories)
	}
	for _, got := range info.WorkspaceRepositories {
		wantWorktreeID := map[string]string{"feature/one": "worktree-one", "feature/two": "worktree-two"}[got.CheckoutBranch]
		if got.WorktreeID != wantWorktreeID || got.BranchSlug != got.BranchIdentitySlug {
			t.Fatalf("recovery repository for %q = %+v, want worktree %q and matching branch identity", got.CheckoutBranch, got, wantWorktreeID)
		}
	}
}

func TestGetWorkspaceInfoForSession_LeavesLegacyRepositoryIdentityEmpty(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)

	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-legacy", WorkspaceID: "ws-1", Name: "api", LocalPath: "/source/api"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-legacy", TaskID: "task-123", RepositoryID: "repo-legacy", BaseBranch: "main"}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-legacy", TaskID: "task-123", State: models.TaskSessionStateCompleted}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-legacy")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	got := info.WorkspaceRepositories[0]
	if got.WorktreeID != "" || got.BranchSlug != "" || got.BranchIdentitySlug != "" {
		t.Fatalf("legacy recovery identity = %+v, want empty persisted worktree fields", got)
	}
}

func TestGetWorkspaceInfoForSession_UsesRepositoryDefaultBranchIdentity(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-default-branch", WorkspaceID: "ws-1", Name: "api", LocalPath: "/source/api", DefaultBranch: "main"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if err := repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "task-repo-default-branch", TaskID: "task-123", RepositoryID: "repo-default-branch"}); err != nil {
		t.Fatalf("CreateTaskRepository: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-default-branch", TaskID: "task-123", State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{ID: "session-worktree-default-branch", SessionID: "session-default-branch", WorktreeID: "worktree-default-branch", RepositoryID: "repo-default-branch", BranchSlug: "main", CreatedAt: now}); err != nil {
		t.Fatalf("CreateTaskSessionWorktree: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-default-branch")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if got := info.WorkspaceRepositories[0]; got.WorktreeID != "worktree-default-branch" || got.BranchIdentitySlug != "main" {
		t.Fatalf("default branch recovery identity = %+v, want persisted main worktree", got)
	}
}

func TestGetWorkspaceInfoForSession_UsesHashDisambiguatedBranchIdentities(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-hash-branches", WorkspaceID: "ws-1", Name: "api", LocalPath: "/source/api", DefaultBranch: "main"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	taskRepos := []*models.TaskRepository{
		{ID: "task-repo-hash-one", TaskID: "task-123", RepositoryID: "repo-hash-branches", BaseBranch: "feature/a", Position: 0, Metadata: map[string]interface{}{"pr_number": 101}},
		{ID: "task-repo-hash-two", TaskID: "task-123", RepositoryID: "repo-hash-branches", BaseBranch: "feature-a", Position: 1, Metadata: map[string]interface{}{"pr_number": 202}},
	}
	for _, taskRepo := range taskRepos {
		if err := repo.CreateTaskRepository(ctx, taskRepo); err != nil {
			t.Fatalf("CreateTaskRepository %q: %v", taskRepo.ID, err)
		}
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session-hash-branches", TaskID: "task-123", State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	plans := worktree.BuildBranchIdentityPlans([]worktree.BranchIdentityInput{
		{RepositoryID: "repo-hash-branches", BaseBranch: "feature/a", DefaultBranch: "main", PRNumber: 101, Position: 0},
		{RepositoryID: "repo-hash-branches", BaseBranch: "feature-a", DefaultBranch: "main", PRNumber: 202, Position: 1},
	})
	for index, plan := range plans {
		if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
			ID: fmt.Sprintf("session-worktree-hash-%d", index), SessionID: "session-hash-branches", WorktreeID: fmt.Sprintf("worktree-hash-%d", index),
			RepositoryID: "repo-hash-branches", BranchSlug: plan.IdentitySlug, CreatedAt: now,
		}); err != nil {
			t.Fatalf("CreateTaskSessionWorktree %q: %v", plan.IdentitySlug, err)
		}
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-hash-branches")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if len(info.WorkspaceRepositories) != len(plans) {
		t.Fatalf("WorkspaceRepositories = %#v, want %d branch-specific repositories", info.WorkspaceRepositories, len(plans))
	}
	for index, got := range info.WorkspaceRepositories {
		if got.WorktreeID != fmt.Sprintf("worktree-hash-%d", index) || got.BranchSlug != plans[index].IdentitySlug || got.BranchIdentitySlug != plans[index].IdentitySlug {
			t.Fatalf("hash-disambiguated recovery repository %d = %+v, want worktree %q and identity %q", index, got, fmt.Sprintf("worktree-hash-%d", index), plans[index].IdentitySlug)
		}
	}
}

func TestGetWorkspaceInfoForSession_RuntimeConfigOptionsSetOnlyWhenOptionsPresent(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	modelOnly := &models.TaskSession{
		ID:        "session-model-only",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{Model: "gpt-5.3-codex-spark"},
		},
	}
	if err := repo.CreateTaskSession(ctx, modelOnly); err != nil {
		t.Fatalf("failed to create model-only session: %v", err)
	}
	modelOnlyInfo, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-model-only")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession model-only: %v", err)
	}
	if modelOnlyInfo.RuntimeConfigOptionsSet {
		t.Fatal("model-only runtime config should not mark config options as set")
	}

	withOptions := &models.TaskSession{
		ID:        "session-with-options",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyRuntimeConfig: models.SessionRuntimeConfig{
				Model:         "gpt-5.3-codex-spark",
				ConfigOptions: map[string]string{"reasoning_effort": "medium"},
			},
			models.SessionMetaKeyRuntimeConfigOverrides: models.SessionRuntimeConfig{
				Model:         "gpt-5.4",
				Mode:          "acceptEdits",
				ConfigOptions: map[string]string{"reasoning_effort": "low"},
			},
		},
	}
	if err := repo.CreateTaskSession(ctx, withOptions); err != nil {
		t.Fatalf("failed to create options session: %v", err)
	}
	optionsInfo, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-with-options")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession with options: %v", err)
	}
	if !optionsInfo.RuntimeConfigOptionsSet {
		t.Fatal("runtime config with options should mark config options as set")
	}
	if optionsInfo.RuntimeModel != "gpt-5.4" || optionsInfo.RuntimeConfigOptions["reasoning_effort"] != "low" {
		t.Fatalf("explicit overrides not applied last: %#v", optionsInfo)
	}
	if optionsInfo.SessionMode != "acceptEdits" {
		t.Fatalf("explicit mode override not applied: %#v", optionsInfo)
	}
}

func TestPersistSessionRuntimeConfigOptionWritesExplicitOverride(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID: "session-override", TaskID: "task-123",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if err := svc.PersistSessionRuntimeConfigOption(ctx, session.ID, "reasoning_effort", "low"); err != nil {
		t.Fatalf("PersistSessionRuntimeConfigOption: %v", err)
	}

	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(stored.Metadata)
	if !ok || overrides.ConfigOptions["reasoning_effort"] != "low" {
		t.Fatalf("runtime overrides = %#v, %v", overrides, ok)
	}
	if _, ok := models.LoadSessionRuntimeConfig(stored.Metadata); ok {
		t.Fatal("explicit selection should not synthesize a provider snapshot")
	}
}

func TestPersistSessionRuntimeOverridesMergeConcurrentSelections(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	setupTestTask(t, repo)
	now := time.Now().UTC()
	session := &models.TaskSession{
		ID: "session-concurrent-overrides", TaskID: "task-123",
		State: models.TaskSessionStateCompleted, StartedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		errs <- svc.PersistSessionRuntimeModel(ctx, session.ID, "mock-smart")
	}()
	go func() {
		defer wg.Done()
		<-start
		errs <- svc.PersistSessionRuntimeConfigOption(ctx, session.ID, "effort", "low")
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("persist override: %v", err)
		}
	}

	stored, err := repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetTaskSession: %v", err)
	}
	overrides, ok := models.LoadSessionRuntimeConfigOverrides(stored.Metadata)
	if !ok || overrides.Model != "mock-smart" || overrides.ConfigOptions["effort"] != "low" {
		t.Fatalf("merged overrides = %#v, %v", overrides, ok)
	}
}

func TestPersistSessionRuntimeModelMissingSessionDoesNotPanic(t *testing.T) {
	svc := &Service{sessions: nilTaskSessionRepo{}}

	err := svc.PersistSessionRuntimeModel(context.Background(), "missing-session", "gpt-5.3-codex-spark")

	if err == nil {
		t.Fatal("expected missing session error")
	}
}

// TestGetWorkspaceInfoForSession_IncludesSessionMode verifies the persisted
// session permission mode is surfaced on WorkspaceInfo so the lifecycle can apply
// it as a mode override on a fresh launch. See issue #1183.
func TestGetWorkspaceInfoForSession_IncludesSessionMode(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateRunning,
		Metadata:  map[string]interface{}{models.SessionMetaKeySessionMode: "acceptEdits"},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.SessionMode != "acceptEdits" {
		t.Errorf("expected SessionMode 'acceptEdits', got %q", info.SessionMode)
	}
}

func TestGetWorkspaceInfoForSession_InfersTaskID(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Pass empty taskID - should be inferred from the session
	info, err := svc.GetWorkspaceInfoForSession(ctx, "", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.TaskID != "task-123" {
		t.Errorf("expected TaskID 'task-123' inferred from session, got %q", info.TaskID)
	}
}

func TestGetWorkspaceInfoForSession_ExecutorInfo(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	// Create executor
	exec := &models.Executor{
		ID:        "exec-1",
		Name:      "My Sprites Executor",
		Type:      models.ExecutorTypeSprites,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateExecutor(ctx, exec); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	// Create session with executor reference
	session := &models.TaskSession{
		ID:         "session-1",
		TaskID:     "task-123",
		ExecutorID: "exec-1",
		State:      models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create executor running record
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er-1",
		SessionID:        "session-1",
		TaskID:           "task-123",
		ExecutorID:       "exec-1",
		Runtime:          "sprites",
		AgentExecutionID: "agent-exec-abc123",
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.ExecutorType != "sprites" {
		t.Errorf("expected ExecutorType 'sprites', got %q", info.ExecutorType)
	}
	if info.RuntimeName != "sprites" {
		t.Errorf("expected RuntimeName 'sprites', got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "agent-exec-abc123" {
		t.Errorf("expected AgentExecutionID 'agent-exec-abc123', got %q", info.AgentExecutionID)
	}
}

func TestGetWorkspaceInfoForSession_IncludesEnvironmentReconnectMetadata(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID:        "exec-1",
		Name:      "Docker",
		Type:      models.ExecutorTypeLocalDocker,
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}

	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-123",
		TaskID:        "task-123",
		ExecutorType:  string(models.ExecutorTypeLocalDocker),
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/host/repo",
		ContainerID:   "container-from-env",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}

	session := &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		ExecutorID:        "exec-1",
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "codex",
		},
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:               "er-1",
		SessionID:        "session-1",
		TaskID:           "task-123",
		ExecutorID:       "exec-1",
		Runtime:          "docker",
		AgentExecutionID: "running-exec",
		ContainerID:      "container-from-running",
		Metadata: map[string]interface{}{
			lifecycle.MetadataKeyAuthTokenSecret:      "secret-token",
			lifecycle.MetadataKeyBootstrapNonceSecret: "secret-nonce",
			lifecycle.MetadataKeyImageTagOverride:     "kandev:test",
			"task_description":                        "drop me",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to upsert executor running: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.WorkspacePath != "/host/repo" {
		t.Fatalf("WorkspacePath = %q, want /host/repo", info.WorkspacePath)
	}
	if info.AgentExecutionID != "running-exec" {
		t.Fatalf("AgentExecutionID = %q, want running-exec", info.AgentExecutionID)
	}
	if info.Metadata[lifecycle.MetadataKeyContainerID] != "container-from-running" {
		t.Fatalf("container metadata = %v, want container-from-running", info.Metadata[lifecycle.MetadataKeyContainerID])
	}
	if info.Metadata[lifecycle.MetadataKeyAuthTokenSecret] != "secret-token" {
		t.Fatalf("auth secret metadata missing: %v", info.Metadata)
	}
	if info.Metadata[lifecycle.MetadataKeyBootstrapNonceSecret] != "secret-nonce" {
		t.Fatalf("nonce secret metadata missing: %v", info.Metadata)
	}
	if info.Metadata[lifecycle.MetadataKeyImageTagOverride] != "kandev:test" {
		t.Fatalf("image override metadata missing: %v", info.Metadata)
	}
	if _, ok := info.Metadata["task_description"]; ok {
		t.Fatalf("launch-only metadata should not be retained: %v", info.Metadata)
	}
}

func TestGetWorkspaceInfoForSession_NoExecutorRunning(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-1",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// No executor running record - should still succeed with empty executor fields
	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.RuntimeName != "" {
		t.Errorf("expected empty RuntimeName, got %q", info.RuntimeName)
	}
	if info.AgentExecutionID != "" {
		t.Errorf("expected empty AgentExecutionID, got %q", info.AgentExecutionID)
	}
	if info.ExecutorType != "" {
		t.Errorf("expected empty ExecutorType, got %q", info.ExecutorType)
	}
}

// TestGetWorkspaceInfoForSession_AlignsTaskEnvironmentIDOnFallback locks in
// the fix in applyTaskEnvironmentToWorkspaceInfo: when the session's stored
// TaskEnvironmentID points to a missing/stale row, GetWorkspaceInfoForSession
// falls back to GetTaskEnvironmentByTaskID. The previous implementation kept
// info.TaskEnvironmentID pointing at the stale ID while picking up
// metadata/workspace path from the fallback env — a downstream ID/metadata
// mismatch that broke reconciler keying. The fix always aligns the ID with
// the env we resolved against.
func TestGetWorkspaceInfoForSession_AlignsTaskEnvironmentIDOnFallback(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	// Real env exists under a different ID than what the session points at.
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "real-env",
		TaskID:        "task-123",
		Status:        models.TaskEnvironmentStatusReady,
		WorkspacePath: "/host/real-env",
		ContainerID:   "container-real",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}

	// Session stores a stale TaskEnvironmentID — the env it points at has
	// been deleted or rolled over, but the session row still references it.
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "stale-env",
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}

	if info.TaskEnvironmentID != "real-env" {
		t.Fatalf("TaskEnvironmentID = %q, want real-env (alignment fix should overwrite the session's stale ID)", info.TaskEnvironmentID)
	}
	if info.WorkspacePath != "/host/real-env" {
		t.Fatalf("WorkspacePath = %q, want /host/real-env (metadata must come from the resolved env, same as the ID)", info.WorkspacePath)
	}
	if got := info.Metadata["container_id"]; got != "container-real" {
		t.Fatalf("metadata container_id = %v, want container-real", got)
	}
}

func TestGetWorkspaceInfoForSession_SessionNotFound(t *testing.T) {
	svc, _, _ := createTestService(t)
	ctx := context.Background()

	_, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestGetWorkspaceInfoForEnvironment(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:        "env-123",
		TaskID:    "task-123",
		Status:    models.TaskEnvironmentStatusReady,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create task environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-2",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		State:             models.TaskSessionStateCompleted,
		AgentProfileSnapshot: map[string]interface{}{
			"agent_name": "auggie",
		},
		StartedAt: now.Add(time.Minute),
		UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("failed to create newer session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForEnvironment(ctx, "env-123")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForEnvironment returned error: %v", err)
	}
	if info.SessionID != "session-2" {
		t.Errorf("SessionID = %q, want session-2", info.SessionID)
	}
	if info.TaskEnvironmentID != "env-123" {
		t.Errorf("TaskEnvironmentID = %q, want env-123", info.TaskEnvironmentID)
	}
}

// Multi-repo: the workspace path agentctl boots with must be the task root
// (parent of every per-repo subdir) so its scanRepositorySubdirs detects all
// repos and starts a per-repo tracker for each. Returning a single repo's
// path would collapse fan-out into the legacy single-tracker mode and
// suppress the per-repo events the Changes panel needs to render headers.
func TestGetWorkspaceInfoForSession_MultiRepoReturnsTaskRoot(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	session := &models.TaskSession{
		ID:        "session-multi",
		TaskID:    "task-123",
		State:     models.TaskSessionStateCompleted,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	for i, path := range []string{
		"/tmp/tasks/do-nothing_mvo/kandev",
		"/tmp/tasks/do-nothing_mvo/thm",
	} {
		if err := repo.CreateTaskSessionWorktree(ctx, &models.TaskSessionWorktree{
			ID:           fmt.Sprintf("wt%d", i),
			SessionID:    session.ID,
			WorktreeID:   fmt.Sprintf("wid%d", i),
			RepositoryID: fmt.Sprintf("repo%d", i),
			Position:     i,
			WorktreePath: path,
			CreatedAt:    now,
		}); err != nil {
			t.Fatalf("create worktree %d: %v", i, err)
		}
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", session.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession: %v", err)
	}
	if info.WorkspacePath != "/tmp/tasks/do-nothing_mvo" {
		t.Errorf("expected WorkspacePath '/tmp/tasks/do-nothing_mvo' (task root), got %q",
			info.WorkspacePath)
	}
}

// AbandonOpenTurns: turns left open by a previous crash must close with
// completed_at = started_at so analytics' active_duration_ms doesn't get
// poisoned with the dead window and the UI's running timer doesn't count
// from a stale start.
func TestAbandonOpenTurns_ZeroesDuration(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	stale := &models.Turn{
		ID:            "turn-stale",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().Add(-90 * time.Hour).UTC(),
	}
	if err := repo.CreateTurn(ctx, stale); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	got, err := repo.GetTurn(ctx, stale.ID)
	if err != nil {
		t.Fatalf("GetTurn: %v", err)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected completed_at to be set")
	}
	if !got.CompletedAt.Equal(got.StartedAt) {
		t.Fatalf("expected completed_at == started_at (zero duration), got started=%v completed=%v",
			got.StartedAt, *got.CompletedAt)
	}
}

// AbandonOpenTurns sweeps the same pending tool calls that CompleteTurn does:
// otherwise the UI would show "running" tool calls forever on an abandoned turn.
func TestAbandonOpenTurns_CompletesPendingToolCalls(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	turn := &models.Turn{
		ID:            "turn-with-tool",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	toolMsg := &models.Message{
		ID:            "msg-tool-1",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		TurnID:        turn.ID,
		AuthorType:    models.MessageAuthorAgent,
		Type:          models.MessageTypeToolCall,
		Content:       "running tool",
		Metadata: map[string]interface{}{
			"tool_call_id": "tc-1",
			"status":       "running",
		},
	}
	if err := repo.CreateMessage(ctx, toolMsg); err != nil {
		t.Fatalf("CreateMessage: %v", err)
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	got, err := repo.GetMessageByToolCallID(ctx, sessionID, "tc-1")
	if err != nil {
		t.Fatalf("GetMessageByToolCallID: %v", err)
	}
	if got == nil {
		t.Fatal("expected tool call message to exist")
	}
	if status, _ := got.Metadata["status"].(string); status != "complete" {
		t.Fatalf("expected tool call status='complete', got %q", status)
	}
}

// AbandonOpenTurns publishes turn.completed for each closed turn so the WS
// gateway can broadcast the state change and the frontend clears its
// activeBySession entry.
func TestAbandonOpenTurns_PublishesTurnCompletedEvent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	turn := &models.Turn{
		ID:            "turn-event",
		TaskSessionID: sessionID,
		TaskID:        "task-123",
		StartedAt:     time.Now().UTC(),
	}
	if err := repo.CreateTurn(ctx, turn); err != nil {
		t.Fatalf("CreateTurn: %v", err)
	}

	eventBus.ClearEvents()
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	var found bool
	for _, ev := range eventBus.GetPublishedEvents() {
		if ev.Type == events.TurnCompleted {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected turn.completed event to be published")
	}
}

// AbandonOpenTurns is a no-op when no open turn exists (e.g. resume called on
// a session whose previous turn already completed cleanly).
func TestAbandonOpenTurns_NoOpenTurns(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	eventBus.ClearEvents()
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	if got := len(eventBus.GetPublishedEvents()); got != 0 {
		t.Fatalf("expected no events when nothing to abandon, got %d", got)
	}
}

// AbandonOpenTurns iteration cap: with more than maxIterations open turns,
// the function still returns nil (caller swallows the error and the next
// completeTurnForSession sweep mops up the remainder).
func TestAbandonOpenTurns_IterationCap(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	sessionID := setupTestSession(t, repo)

	const seeded = 20 // > maxIterations (16)
	for i := 0; i < seeded; i++ {
		turn := &models.Turn{
			ID:            fmt.Sprintf("turn-cap-%d", i),
			TaskSessionID: sessionID,
			TaskID:        "task-123",
			StartedAt:     time.Now().UTC(),
		}
		if err := repo.CreateTurn(ctx, turn); err != nil {
			t.Fatalf("seed turn %d: %v", i, err)
		}
	}

	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("AbandonOpenTurns: %v", err)
	}

	// First sweep closes exactly maxIterations; remainder still open.
	turns, err := repo.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListTurnsBySession: %v", err)
	}
	open := 0
	for _, turn := range turns {
		if turn.CompletedAt == nil {
			open++
		}
	}
	if open != seeded-16 {
		t.Fatalf("expected %d open turns after first sweep (cap=16), got %d", seeded-16, open)
	}

	// Second sweep finishes the job.
	if err := svc.AbandonOpenTurns(ctx, sessionID); err != nil {
		t.Fatalf("second AbandonOpenTurns: %v", err)
	}
	turns, _ = repo.ListTurnsBySession(ctx, sessionID)
	for _, turn := range turns {
		if turn.CompletedAt == nil {
			t.Fatalf("expected all turns closed after second sweep, %s still open", turn.ID)
		}
	}
}

func TestMergeExecutorConfigMetadata(t *testing.T) {
	// Fixture values — arbitrary test inputs, not real host config.
	const (
		execHost   = "ssh.example.com"
		execUser   = "agent"
		execFP     = "SHA256:test-fingerprint"
		liveHost   = "live-host-from-running-record"
		staleHost  = "stale-host-from-executor"
		droppedKey = "not_a_persistent_key"
	)

	// SSH connection keys from the executor record must be projected when absent.
	info := &lifecycle.WorkspaceInfo{}
	mergeExecutorConfigMetadata(info, map[string]string{
		lifecycle.MetadataKeySSHHost:            execHost,
		lifecycle.MetadataKeySSHUser:            execUser,
		lifecycle.MetadataKeySSHHostFingerprint: execFP,
		droppedKey:                              "dropme",
	})
	if got := info.Metadata[lifecycle.MetadataKeySSHHost]; got != execHost {
		t.Fatalf("ssh_host = %v, want %q", got, execHost)
	}
	if got := info.Metadata[lifecycle.MetadataKeySSHHostFingerprint]; got != execFP {
		t.Fatalf("ssh_host_fingerprint = %v, want %q", got, execFP)
	}
	if _, ok := info.Metadata[droppedKey]; ok {
		t.Fatal("non-persistent key should be filtered out")
	}

	// Existing (live running-record) values must win over executor config.
	info2 := &lifecycle.WorkspaceInfo{Metadata: map[string]interface{}{
		lifecycle.MetadataKeySSHHost: liveHost,
	}}
	mergeExecutorConfigMetadata(info2, map[string]string{
		lifecycle.MetadataKeySSHHost: staleHost,
		lifecycle.MetadataKeySSHUser: execUser,
	})
	if got := info2.Metadata[lifecycle.MetadataKeySSHHost]; got != liveHost {
		t.Fatalf("ssh_host = %v, want %q (existing must win)", got, liveHost)
	}
	if got := info2.Metadata[lifecycle.MetadataKeySSHUser]; got != execUser {
		t.Fatalf("ssh_user = %v, want %q (absent key should be filled)", got, execUser)
	}

	// Empty config is a no-op.
	info3 := &lifecycle.WorkspaceInfo{}
	mergeExecutorConfigMetadata(info3, nil)
	if len(info3.Metadata) != 0 {
		t.Fatalf("empty config should not create metadata, got %v", info3.Metadata)
	}

	// Alias-only executors (host read from ~/.ssh/config) must have their alias
	// projected — otherwise targetFromMetadata still throws "host (or host_alias)
	// is required". Regression guard for the FilterPersistentMetadata gap.
	const execAlias = "my-remote-box"
	info4 := &lifecycle.WorkspaceInfo{}
	mergeExecutorConfigMetadata(info4, map[string]string{
		lifecycle.MetadataKeySSHHostAlias: execAlias,
		lifecycle.MetadataKeySSHShell:     "zsh",
	})
	if got := info4.Metadata[lifecycle.MetadataKeySSHHostAlias]; got != execAlias {
		t.Fatalf("ssh_host_alias = %v, want %q (alias-only executor must survive)", got, execAlias)
	}
	if got := info4.Metadata[lifecycle.MetadataKeySSHShell]; got != "zsh" {
		t.Fatalf("ssh_shell = %v, want zsh (per-profile key should project)", got)
	}

	// Session-scoped runtime keys must NEVER be projected from executor config —
	// projecting a stale one would make restore reattach to a dead remote
	// agentctl instance instead of creating a fresh one.
	info5 := &lifecycle.WorkspaceInfo{}
	mergeExecutorConfigMetadata(info5, map[string]string{
		lifecycle.MetadataKeySSHHost:               execHost,
		lifecycle.MetadataKeySSHRemoteAgentctlPort: "40123",
		lifecycle.MetadataKeySSHRemoteAgentctlPID:  "9999",
		lifecycle.MetadataKeySSHRemoteSessionDir:   "/home/agent/.kandev/sessions/old",
	})
	for _, k := range []string{
		lifecycle.MetadataKeySSHRemoteAgentctlPort,
		lifecycle.MetadataKeySSHRemoteAgentctlPID,
		lifecycle.MetadataKeySSHRemoteSessionDir,
	} {
		if _, ok := info5.Metadata[k]; ok {
			t.Errorf("session-scoped key %q must NOT be projected from executor config", k)
		}
	}
	if got := info5.Metadata[lifecycle.MetadataKeySSHHost]; got != execHost {
		t.Fatalf("ssh_host = %v, want %q (connection key should still project)", got, execHost)
	}
}

// TestGetWorkspaceInfoForSession_SSHConfigProjectedWhenNoRunningRecord exercises
// the end-to-end fallback: a completed SSH session with no ExecutorRunning
// record must still surface the executor's ssh_host so a terminal / restore
// does not fail with "host (or host_alias) is required in executor config".
func TestGetWorkspaceInfoForSession_SSHConfigProjectedWhenNoRunningRecord(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	setupTestTask(t, repo)
	now := time.Now().UTC()

	const execHost = "ssh.example.com"
	if err := repo.CreateExecutor(ctx, &models.Executor{
		ID:   "ssh-exec-1",
		Name: "my-ssh-host",
		Type: models.ExecutorTypeSSH,
		Config: map[string]string{
			lifecycle.MetadataKeySSHHost:            execHost,
			lifecycle.MetadataKeySSHUser:            "agent",
			lifecycle.MetadataKeySSHHostFingerprint: "SHA256:test-fingerprint",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("failed to create ssh executor: %v", err)
	}

	// Completed session pointing at the SSH executor, with NO ExecutorRunning record.
	session := &models.TaskSession{
		ID:                "session-ssh-1",
		TaskID:            "task-123",
		TaskEnvironmentID: "env-123",
		AgentProfileID:    "profile-1",
		ExecutorID:        "ssh-exec-1",
		State:             models.TaskSessionStateCompleted,
		StartedAt:         now,
		UpdatedAt:         now,
	}
	if err := repo.CreateTaskSession(ctx, session); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	info, err := svc.GetWorkspaceInfoForSession(ctx, "task-123", "session-ssh-1")
	if err != nil {
		t.Fatalf("GetWorkspaceInfoForSession returned error: %v", err)
	}
	if info.ExecutorType != string(models.ExecutorTypeSSH) {
		t.Fatalf("ExecutorType = %q, want ssh", info.ExecutorType)
	}
	if got, _ := info.Metadata[lifecycle.MetadataKeySSHHost].(string); got != execHost {
		t.Fatalf("ssh_host = %v, want %q (executor config must be projected as fallback)", info.Metadata[lifecycle.MetadataKeySSHHost], execHost)
	}
}
