package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

type policyPatchReadGate struct {
	repository.RepositoryBranchPolicyRepository
	ready, release chan struct{}
	once           sync.Once
}

func (g *policyPatchReadGate) GetRepositoryBranchPolicy(ctx context.Context, id string) (*models.RepositoryBranchPolicy, error) {
	policy, err := g.RepositoryBranchPolicyRepository.GetRepositoryBranchPolicy(ctx, id)
	if err != nil {
		return nil, err
	}
	g.once.Do(func() {
		close(g.ready)
		select {
		case <-g.release:
		case <-ctx.Done():
		}
	})
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return policy, nil
}

func branchPolicyPatchPeer(t *testing.T, svc *Service, repo *tasksqlite.Repository) *Service {
	t.Helper()
	var seq int
	var name, filename string
	require.NoError(t, repo.DB().QueryRow("PRAGMA database_list").Scan(&seq, &name, &filename))
	connection, err := db.OpenSQLite(filename)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	database := sqlx.NewDb(connection, "sqlite3")
	peer, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, peer.Close()) })
	return NewService(Repos{Workspaces: peer, RepoEntities: peer, BranchPolicies: peer}, NewMockEventBus(), svc.logger, RepositoryDiscoveryConfig{})
}

func createPatchPolicy(t *testing.T, svc *Service) *models.RepositoryBranchPolicy {
	t.Helper()
	policy, err := svc.CreateRepositoryBranchPolicy(context.Background(), &CreateRepositoryBranchPolicyRequest{
		RepositoryID: "repo-policy-service", Name: "Feature", Description: "original",
		BaseBranch: "develop", BranchTemplate: "feature/{title}-{suffix}", PullRequestTarget: "develop",
	})
	require.NoError(t, err)
	return policy
}

func overlapPolicyPatches(t *testing.T, delayed, first *Service, id string, late, early *UpdateRepositoryBranchPolicyRequest) *models.RepositoryBranchPolicy {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	gate := &policyPatchReadGate{RepositoryBranchPolicyRepository: delayed.branchPolicies, ready: make(chan struct{}), release: make(chan struct{})}
	delayed.branchPolicies = gate
	finished := make(chan struct{})
	var result *models.RepositoryBranchPolicy
	var resultErr error
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(gate.release) }); cancel(); <-finished })
	go func() { defer close(finished); result, resultErr = delayed.UpdateRepositoryBranchPolicy(ctx, id, late) }()
	select {
	case <-gate.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, earlyErr := first.UpdateRepositoryBranchPolicy(ctx, id, early)
	release.Do(func() { close(gate.release) })
	select {
	case <-finished:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, earlyErr)
	require.NoError(t, resultErr)
	return result
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.8
func TestBranchPolicyPatchConcurrentEdits(t *testing.T) {
	metadata := &UpdateRepositoryBranchPolicyRequest{Description: stringPointer("new description")}
	workflow := &UpdateRepositoryBranchPolicyRequest{BaseBranch: stringPointer("main"), BranchTemplate: stringPointer("hotfix/{title}-{suffix}"), PullRequestTarget: stringPointer("release")}
	for _, tc := range []struct {
		name        string
		late, early *UpdateRepositoryBranchPolicyRequest
	}{
		{"description after workflow", metadata, workflow},
		{"workflow after description", workflow, metadata},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedBranchPolicyRepository(t, repo)
			policy := createPatchPolicy(t, svc)
			peer := branchPolicyPatchPeer(t, svc, repo)
			result := overlapPolicyPatches(t, svc, peer, policy.ID, tc.late, tc.early)
			stored, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
			require.NoError(t, err)
			require.Equal(t, "new description", stored.Description)
			require.Equal(t, "main", stored.BaseBranch, "successful overlapping patch reverted the saved workflow")
			require.Equal(t, "hotfix/{title}-{suffix}", stored.BranchTemplate)
			require.Equal(t, "release", stored.PullRequestTarget)
			require.Equal(t, *stored, *result)
		})
	}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.10
func TestBranchPolicyPatchTargetDefaults(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target *string
		base   *string
		want   string
	}{
		{"omitted target after base", nil, nil, "develop"},
		{"blank target after base", stringPointer(""), nil, "main"},
		{"whitespace target after base", stringPointer("  "), nil, "main"},
		{"combined base and blank", stringPointer(""), stringPointer(" release "), "release"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			seedBranchPolicyRepository(t, repo)
			policy := createPatchPolicy(t, svc)
			peer := branchPolicyPatchPeer(t, svc, repo)
			result := overlapPolicyPatches(t, svc, peer, policy.ID,
				&UpdateRepositoryBranchPolicyRequest{Description: stringPointer("new description"), BaseBranch: tc.base, PullRequestTarget: tc.target},
				&UpdateRepositoryBranchPolicyRequest{BaseBranch: stringPointer("main")})
			require.Equal(t, tc.want, result.PullRequestTarget)
			require.Equal(t, "new description", result.Description)
			saved, err := repo.GetRepositoryBranchPolicy(context.Background(), policy.ID)
			require.NoError(t, err)
			require.Equal(t, *saved, *result)
		})
	}
}

// @covers AC-WORKSPACES-BRANCH-POLICIES-001.4, AC-WORKSPACES-BRANCH-POLICIES-001.9
func TestBranchPolicyPatchValidationRollback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		request *UpdateRepositoryBranchPolicyRequest
	}{
		{"base", &UpdateRepositoryBranchPolicyRequest{BaseBranch: stringPointer("bad..ref")}},
		{"target", &UpdateRepositoryBranchPolicyRequest{PullRequestTarget: stringPointer("bad ref")}},
		{"template", &UpdateRepositoryBranchPolicyRequest{BranchTemplate: stringPointer("feature/{unknown}")}},
		{"empty name", &UpdateRepositoryBranchPolicyRequest{Name: stringPointer(" ")}},
		{"long name", &UpdateRepositoryBranchPolicyRequest{Name: stringPointer(strings.Repeat("a", 101))}},
		{"long description", &UpdateRepositoryBranchPolicyRequest{Description: stringPointer(strings.Repeat("a", 501))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, eventBus, repo := createTestService(t)
			seedBranchPolicyRepository(t, repo)
			original := createPatchPolicy(t, svc)
			eventBus.ClearEvents()
			tc.request.BaseBranch = coalescePatchBase(tc.request.BaseBranch)
			_, err := svc.UpdateRepositoryBranchPolicy(context.Background(), original.ID, tc.request)
			require.ErrorIs(t, err, ErrInvalidRepositoryBranchPolicy)
			stored, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
			require.NoError(t, err)
			require.Equal(t, *original, *stored)
			require.Empty(t, eventBus.GetPublishedEvents())
			// A successful mutation proves that this fixture can publish updates.
			_, err = svc.UpdateRepositoryBranchPolicy(context.Background(), original.ID, &UpdateRepositoryBranchPolicyRequest{Description: stringPointer("valid")})
			require.NoError(t, err)
			require.Len(t, eventBus.GetPublishedEvents(), 1)
		})
	}
}

func coalescePatchBase(base *string) *string {
	if base == nil {
		return stringPointer("main")
	}
	return base
}

func TestBranchPolicyPatchControls(t *testing.T) {
	t.Run("full write and empty description", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		seedBranchPolicyRepository(t, repo)
		original := createPatchPolicy(t, svc)
		result, err := svc.UpdateRepositoryBranchPolicy(context.Background(), original.ID, &UpdateRepositoryBranchPolicyRequest{
			Name: stringPointer(" Hotfix "), Description: stringPointer(""), BaseBranch: stringPointer("main"),
			BranchTemplate: stringPointer("hotfix/{title}-{suffix}"), PullRequestTarget: stringPointer("release"),
		})
		require.NoError(t, err)
		require.Equal(t, "Hotfix", result.Name)
		require.Empty(t, result.Description)
		require.Equal(t, "main", result.BaseBranch)
		require.Equal(t, "hotfix/{title}-{suffix}", result.BranchTemplate)
		require.Equal(t, "release", result.PullRequestTarget)
		require.Equal(t, original.CreatedAt, result.CreatedAt)
		requirePolicyPatchEvent(t, eventBus, result)
	})
	t.Run("same field last commit", func(t *testing.T) {
		svc, _, repo := createTestService(t)
		seedBranchPolicyRepository(t, repo)
		original := createPatchPolicy(t, svc)
		result := overlapPolicyPatches(t, svc, branchPolicyPatchPeer(t, svc, repo), original.ID,
			&UpdateRepositoryBranchPolicyRequest{Name: stringPointer("Last")}, &UpdateRepositoryBranchPolicyRequest{Name: stringPointer("First")})
		require.Equal(t, "Last", result.Name)
	})
	t.Run("omitted invalid legacy target", func(t *testing.T) {
		svc, eventBus, repo := createTestService(t)
		seedBranchPolicyRepository(t, repo)
		original := createPatchPolicy(t, svc)
		original.PullRequestTarget = ""
		require.NoError(t, repo.UpdateRepositoryBranchPolicy(context.Background(), original))
		eventBus.ClearEvents()
		_, err := svc.UpdateRepositoryBranchPolicy(context.Background(), original.ID, &UpdateRepositoryBranchPolicyRequest{Description: stringPointer("valid")})
		require.ErrorIs(t, err, ErrInvalidRepositoryBranchPolicy)
		stored, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
		require.NoError(t, err)
		require.Equal(t, *original, *stored)
		require.Empty(t, eventBus.GetPublishedEvents())
	})
	t.Run("name conflict", checkPolicyPatchNameConflict)
	t.Run("foreign workspace", checkPolicyPatchForeignWorkspace)
	t.Run("name template target mix", checkPolicyPatchMixedFields)
	t.Run("deleted policy", func(t *testing.T) { checkPolicyPatchDeleted(t, false) })
	t.Run("deleted parent", func(t *testing.T) { checkPolicyPatchDeleted(t, true) })
}

func requirePolicyPatchEvent(t *testing.T, eventBus *MockEventBus, policy *models.RepositoryBranchPolicy) {
	t.Helper()
	published := eventBus.GetPublishedEvents()
	event := published[len(published)-1]
	require.Equal(t, events.RepositoryBranchPolicyUpdated, event.Type)
	data := event.Data.(map[string]interface{})
	for key, value := range map[string]string{"id": policy.ID, "repository_id": policy.RepositoryID, "name": policy.Name,
		"description": policy.Description, "base_branch": policy.BaseBranch, "branch_template": policy.BranchTemplate,
		"pull_request_target": policy.PullRequestTarget, "updated_at": policy.UpdatedAt.Format(time.RFC3339)} {
		require.Equal(t, value, data[key], key)
	}
}

func checkPolicyPatchNameConflict(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	original := createPatchPolicy(t, svc)
	_, err := svc.CreateRepositoryBranchPolicy(context.Background(), &CreateRepositoryBranchPolicyRequest{
		RepositoryID: original.RepositoryID, Name: "Hotfix", BaseBranch: "main", BranchTemplate: "hotfix/{title}-{suffix}",
	})
	require.NoError(t, err)
	eventBus.ClearEvents()
	_, err = svc.UpdateRepositoryBranchPolicy(context.Background(), original.ID, &UpdateRepositoryBranchPolicyRequest{Name: stringPointer(" HOTFIX "), BaseBranch: stringPointer("main")})
	require.ErrorIs(t, err, ErrRepositoryBranchPolicyNameConflict)
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
	require.NoError(t, err)
	require.Equal(t, *original, *saved)
	require.Empty(t, eventBus.GetPublishedEvents())
}

func checkPolicyPatchDeleted(t *testing.T, parent bool) {
	svc, eventBus, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	original := createPatchPolicy(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	gate := &policyPatchReadGate{RepositoryBranchPolicyRepository: repo, ready: make(chan struct{}), release: make(chan struct{})}
	svc.branchPolicies = gate
	var release sync.Once
	done := make(chan struct{})
	var updateErr error
	t.Cleanup(func() { release.Do(func() { close(gate.release) }); cancel(); <-done })
	eventBus.ClearEvents()
	go func() {
		defer close(done)
		_, updateErr = svc.UpdateRepositoryBranchPolicy(ctx, original.ID, &UpdateRepositoryBranchPolicyRequest{Description: stringPointer("late")})
	}()
	select {
	case <-gate.ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if parent {
		require.NoError(t, repo.DeleteRepository(ctx, original.RepositoryID))
	} else {
		deleted, err := repo.DeleteRepositoryBranchPolicy(ctx, original.ID)
		require.NoError(t, err)
		require.True(t, deleted)
	}
	release.Do(func() { close(gate.release) })
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Error(t, updateErr)
	require.Empty(t, eventBus.GetPublishedEvents())
	_, err := repo.GetRepositoryBranchPolicy(ctx, original.ID)
	require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNotFound)
}

func checkPolicyPatchForeignWorkspace(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	require.NoError(t, repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-policy-service", Name: "Owned", OwnerID: "owner"}))
	require.NoError(t, repo.CreateRepository(context.Background(), &models.Repository{ID: "repo-policy-service", WorkspaceID: "ws-policy-service", Name: "Policy repo"}))
	original := createPatchPolicy(t, svc)
	eventBus.ClearEvents()
	_, err := svc.UpdateRepositoryBranchPolicy(ctxAs("outsider"), original.ID, &UpdateRepositoryBranchPolicyRequest{Name: stringPointer("Changed")})
	require.ErrorIs(t, err, repoerrors.ErrRepositoryBranchPolicyNotFound)
	require.Empty(t, eventBus.GetPublishedEvents())
	saved, err := repo.GetRepositoryBranchPolicy(context.Background(), original.ID)
	require.NoError(t, err)
	require.Equal(t, *original, *saved)
}

func checkPolicyPatchMixedFields(t *testing.T) {
	svc, _, repo := createTestService(t)
	seedBranchPolicyRepository(t, repo)
	original := createPatchPolicy(t, svc)
	result := overlapPolicyPatches(t, svc, branchPolicyPatchPeer(t, svc, repo), original.ID,
		&UpdateRepositoryBranchPolicyRequest{Name: stringPointer("Release"), PullRequestTarget: stringPointer("release")},
		&UpdateRepositoryBranchPolicyRequest{BranchTemplate: stringPointer("release/{title}-{suffix}")})
	require.Equal(t, "Release", result.Name)
	require.Equal(t, "release", result.PullRequestTarget)
	require.Equal(t, "release/{title}-{suffix}", result.BranchTemplate)
}
