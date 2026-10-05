package service

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type gitflowAdmissionGate struct {
	repository.RepositoryBranchPolicyRepository
	ready, release chan struct{}
}

func (g *gitflowAdmissionGate) CreateRepositoryBranchPoliciesIfEmpty(ctx context.Context, id string, policies []*models.RepositoryBranchPolicy) error {
	close(g.ready)
	select {
	case <-g.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return g.RepositoryBranchPolicyRepository.CreateRepositoryBranchPoliciesIfEmpty(ctx, id, policies)
}

func admissionGitService(t *testing.T) (*Service, *MockEventBus, *tasksqlite.Repository) {
	t.Helper()
	isolateGitEnvForTest(t)
	svc, eventBus, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	repoPath := filepath.Join(t.TempDir(), "gitflow")
	initRealGitRepo(t, repoPath)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, args := range [][]string{{"branch", "develop"}, {"branch", "release"}, {"branch", "next"},
		{"update-ref", "refs/remotes/origin/release", "HEAD"}, {"update-ref", "refs/remotes/origin/next", "HEAD"}} {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir, cmd.Env = repoPath, isolatedGitEnv()
		output, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git %v: %s", args, output)
	}
	entity, err := repo.GetRepository(ctx, "repo-policy-service")
	require.NoError(t, err)
	entity.LocalPath, entity.SourceType = repoPath, sourceTypeLocal
	require.NoError(t, repo.UpdateRepository(ctx, entity))
	return svc, eventBus, repo
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.1, AC-WORKSPACES-BRANCH-POLICIES-002.2, AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestGitflowAdmissionService(t *testing.T) {
	for _, tc := range []struct {
		name, production, development, wantProduction, wantDevelopment string
	}{
		{"defaults", "", "", "main", "develop"},
		{"normalized_remote", " origin/release ", " origin/next ", "origin/release", "origin/next"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, eventBus, repo := admissionGitService(t)
			rows, err := svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{
				RepositoryID: "repo-policy-service", ProductionBranch: tc.production, DevelopmentBranch: tc.development,
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantDevelopment, rows[0].BaseBranch)
			require.Equal(t, tc.wantProduction, rows[2].BaseBranch)
			require.Equal(t, tc.wantProduction, rows[3].PullRequestTarget)
			requireAdmissionServiceRows(t, repo, eventBus, rows)
		})
	}
	t.Run("competing_starter", checkAdmissionServiceCompetition)
}

func requireAdmissionServiceRows(t *testing.T, repo *tasksqlite.Repository, eventBus *MockEventBus, policies []*models.RepositoryBranchPolicy) {
	t.Helper()
	require.Len(t, policies, 4)
	stored, err := repo.ListRepositoryBranchPolicies(context.Background(), "repo-policy-service")
	require.NoError(t, err)
	require.ElementsMatch(t, policies, stored)
	published := eventBus.GetPublishedEvents()
	require.Len(t, published, 4)
	for i, policy := range policies {
		require.Equal(t, events.RepositoryBranchPolicyCreated, published[i].Type)
		data := published[i].Data.(map[string]interface{})
		for key, value := range map[string]string{"id": policy.ID, "repository_id": policy.RepositoryID,
			"name": policy.Name, "base_branch": policy.BaseBranch, "branch_template": policy.BranchTemplate,
			"pull_request_target": policy.PullRequestTarget, "created_at": policy.CreatedAt.Format(time.RFC3339)} {
			require.Equal(t, value, data[key], key)
		}
	}
}

func checkAdmissionServiceCompetition(t *testing.T) {
	svc, eventBus, repo := admissionGitService(t)
	delayed := branchPolicyPatchPeer(t, svc, repo)
	gate := &gitflowAdmissionGate{RepositoryBranchPolicyRepository: delayed.branchPolicies,
		ready: make(chan struct{}), release: make(chan struct{})}
	delayed.branchPolicies = gate
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	var resume sync.Once
	done := make(chan struct{})
	var loserErr error
	t.Cleanup(func() { resume.Do(func() { close(gate.release) }); cancel(); <-done })
	go func() {
		defer close(done)
		_, loserErr = delayed.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{
			RepositoryID: "repo-policy-service", ProductionBranch: "release", DevelopmentBranch: "next",
		})
	}()
	select {
	case <-gate.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	winner, err := svc.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "repo-policy-service"})
	require.NoError(t, err)
	resume.Do(func() { close(gate.release) })
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.ErrorIs(t, loserErr, ErrRepositoryBranchPolicyAlreadySeeded)
	require.Empty(t, delayed.eventBus.(*MockEventBus).GetPublishedEvents())
	requireAdmissionServiceRows(t, repo, eventBus, winner)
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-002.2, AC-WORKSPACES-BRANCH-POLICIES-002.6
func TestGitflowAdmissionControls(t *testing.T) {
	svc, eventBus, repo := admissionGitService(t)
	for _, pair := range [][2]string{{"main", "main"}, {"bad..ref", "develop"}, {"main", "missing"}} {
		_, err := svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{
			RepositoryID: "repo-policy-service", ProductionBranch: pair[0], DevelopmentBranch: pair[1],
		})
		require.ErrorIs(t, err, ErrInvalidRepositoryBranchPolicy)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := svc.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "repo-policy-service"})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRepositoryBranchPolicyAlreadySeeded)
	_, err = svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "missing"})
	require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	_, err = repo.DB().Exec(`CREATE TRIGGER admission_failure BEFORE INSERT ON repository_branch_policies
	 WHEN NEW.name = 'Hotfix' BEGIN SELECT RAISE(ABORT, 'fixture insert failure'); END`)
	require.NoError(t, err)
	_, err = svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "repo-policy-service"})
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrRepositoryBranchPolicyAlreadySeeded)
	rows, err := repo.ListRepositoryBranchPolicies(context.Background(), "repo-policy-service")
	require.NoError(t, err)
	require.Empty(t, rows)
	require.Empty(t, eventBus.GetPublishedEvents())
	_, err = repo.DB().Exec(`DROP TRIGGER admission_failure`)
	require.NoError(t, err)
	winner, err := svc.CreateGitflowRepositoryBranchPolicies(context.Background(), &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "repo-policy-service"})
	require.NoError(t, err)
	requireAdmissionServiceRows(t, repo, eventBus, winner)
	t.Run("foreign_and_deleted_parent", checkAdmissionServiceScope)
}

func checkAdmissionServiceScope(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "owned", Name: "Owned", OwnerID: "owner"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "owned-repo", WorkspaceID: "owned", Name: "Owned"}))
	_, err := svc.CreateGitflowRepositoryBranchPolicies(ctxAs("outsider"), &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "owned-repo"})
	require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	require.NoError(t, repo.DeleteRepository(ctx, "owned-repo"))
	_, err = svc.CreateGitflowRepositoryBranchPolicies(ctx, &CreateGitflowRepositoryBranchPoliciesRequest{RepositoryID: "owned-repo"})
	require.ErrorIs(t, err, repoerrors.ErrRepositoryNotFound)
	require.Empty(t, eventBus.GetPublishedEvents())
}
