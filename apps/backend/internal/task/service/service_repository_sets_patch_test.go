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
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func newSetPatchServices(t *testing.T) (*Service, *Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	firstDB, path := serviceTestSQLiteTemplate.Open(t)
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	secondDB := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, secondDB.Close()) })
	first := sqliterepo.NewWithInitializedDB(firstDB, firstDB, nil)
	second := sqliterepo.NewWithInitializedDB(secondDB, secondDB, nil)
	eventBus := NewMockEventBus()
	build := func(repo *sqliterepo.Repository) *Service {
		return NewService(Repos{Workspaces: repo, RepoEntities: repo, RepositorySets: repo},
			eventBus, accessTestLogger(t), RepositoryDiscoveryConfig{})
	}
	return build(first), build(second), eventBus, first
}

func seedSetPatch(t *testing.T, svc *Service, repo *sqliterepo.Repository) *models.RepositorySet {
	t.Helper()
	seedSetWorkspace(t, svc, repo)
	set, err := svc.CreateRepositorySet(context.Background(), &CreateRepositorySetRequest{
		WorkspaceID: "ws-1", Name: "Original name", Description: "Original description",
		Repositories: []RepositorySetMemberInput{
			{RepositoryID: "repo-gateway", BaseBranch: "original-gateway"},
			{RepositoryID: "repo-web", BaseBranch: "original-web"},
		},
	})
	require.NoError(t, err)
	return set
}

type setPatchWriteGate struct {
	repository.RepositorySetRepository
	ready   chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *setPatchWriteGate) unblock() { g.once.Do(func() { close(g.release) }) }

func (g *setPatchWriteGate) PatchRepositorySet(
	ctx context.Context, id string, patch *models.RepositorySetPatch,
) error {
	close(g.ready)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.release:
		return g.RepositorySetRepository.PatchRepositorySet(ctx, id, patch)
	}
}

type setPatchResult struct {
	set *models.RepositorySet
	err error
}

func startGatedSetPatch(
	t *testing.T, ctx context.Context, svc *Service, id string, req *UpdateRepositorySetRequest,
) (*setPatchWriteGate, <-chan setPatchResult) {
	t.Helper()
	gate := &setPatchWriteGate{RepositorySetRepository: svc.repositorySets,
		ready: make(chan struct{}), release: make(chan struct{})}
	svc.repositorySets = gate
	result := make(chan setPatchResult, 1)
	finished := make(chan struct{})
	workerCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(func() {
		cancel()
		gate.unblock()
		<-finished
	})
	go func() {
		defer close(finished)
		set, err := svc.UpdateRepositorySet(workerCtx, id, req)
		result <- setPatchResult{set: set, err: err}
	}()
	select {
	case <-gate.ready:
	case <-finished:
		t.Fatal("update finished before reaching its store write")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return gate, result
}

func awaitSetPatch(t *testing.T, ctx context.Context, results <-chan setPatchResult) setPatchResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-ctx.Done():
		t.Fatal(ctx.Err())
		return setPatchResult{}
	}
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.9, AC-WORKSPACES-REPOSITORY-SETS-001.10
func TestRepositorySetConcurrentDisjointPatches(t *testing.T) {
	name, description := "New name", "New description"
	members := []RepositorySetMemberInput{
		{RepositoryID: "repo-orders", BaseBranch: "release-orders"},
		{RepositoryID: "repo-web", BaseBranch: "release-web"},
	}
	cases := []struct {
		name            string
		first, second   UpdateRepositorySetRequest
		wantName        string
		wantDescription string
		memberChange    bool
	}{
		{"description then name", UpdateRepositorySetRequest{Description: &description},
			UpdateRepositorySetRequest{Name: &name}, name, description, false},
		{"name then description", UpdateRepositorySetRequest{Name: &name},
			UpdateRepositorySetRequest{Description: &description}, name, description, false},
		{"members then name", UpdateRepositorySetRequest{Repositories: &members},
			UpdateRepositorySetRequest{Name: &name}, name, "Original description", true},
		{"name then members", UpdateRepositorySetRequest{Name: &name},
			UpdateRepositorySetRequest{Repositories: &members}, name, "Original description", true},
	}
	for _, tc := range cases {
		for _, overlap := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/sequential", true: "/overlap"}[overlap], func(t *testing.T) {
				runDisjointSetPatches(t, &tc.first, &tc.second, tc.wantName, tc.wantDescription, tc.memberChange, overlap)
			})
		}
	}
}

func runDisjointSetPatches(
	t *testing.T, first, second *UpdateRepositorySetRequest,
	wantName, wantDescription string, memberChange, overlap bool,
) {
	t.Helper()
	a, b, eventBus, repo := newSetPatchServices(t)
	initial := seedSetPatch(t, a, repo)
	eventBus.ClearEvents()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var lastResponse *models.RepositorySet
	if overlap {
		gate, results := startGatedSetPatch(t, ctx, a, initial.ID, first)
		_, err := b.UpdateRepositorySet(ctx, initial.ID, second)
		require.NoError(t, err)
		gate.unblock()
		result := awaitSetPatch(t, ctx, results)
		require.NoError(t, result.err)
		lastResponse = result.set
	} else {
		_, err := a.UpdateRepositorySet(ctx, initial.ID, first)
		require.NoError(t, err)
		lastResponse, err = b.UpdateRepositorySet(ctx, initial.ID, second)
		require.NoError(t, err)
	}
	stored, err := repo.GetRepositorySet(ctx, initial.ID)
	require.NoError(t, err)
	require.Equal(t, stored, lastResponse)
	require.Equal(t, wantName, stored.Name, "successful disjoint patches lost name")
	require.Equal(t, wantDescription, stored.Description, "successful disjoint patches lost description")
	if memberChange {
		require.Equal(t, []string{"repo-orders", "repo-web"}, stored.RepositoryIDs())
		require.Equal(t, "release-orders", stored.Items[0].BaseBranch)
		require.Equal(t, "release-web", stored.Items[1].BaseBranch)
		require.Equal(t, 0, stored.Items[0].Position)
		require.Equal(t, 1, stored.Items[1].Position)
	} else {
		require.Equal(t, initial.Items, stored.Items)
	}
	updates := eventBus.GetPublishedEvents()
	require.Len(t, updates, 2)
	for _, event := range updates {
		require.Equal(t, "repository_set.updated", event.Type)
	}
	last := updates[1].Data.(map[string]interface{})
	require.Equal(t, stored.Name, last["name"])
	require.Equal(t, stored.Description, last["description"])
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.8, AC-WORKSPACES-REPOSITORY-SETS-001.9
func TestRepositorySetPatchPresenceAndValidation(t *testing.T) {
	t.Run("accepted presence", testSetPatchPresence)
	t.Run("rejected combined patches", testSetPatchValidation)
	t.Run("foreign set authorization", func(t *testing.T) {
		svc, _, eventBus, repo := newSetPatchServices(t)
		ctx := context.Background()
		require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "owned", Name: "Owned", OwnerID: "owner"}))
		set := &models.RepositorySet{WorkspaceID: "owned", Name: "Private", Description: "untouched"}
		require.NoError(t, repo.CreateRepositorySet(ctx, set))
		_, err := svc.UpdateRepositorySet(ctxAs("stranger"), set.ID,
			&UpdateRepositorySetRequest{Description: stringPointer("rejected")})
		require.ErrorIs(t, err, repoerrors.ErrRepositorySetNotFound)
		loaded, err := repo.GetRepositorySet(ctx, set.ID)
		require.NoError(t, err)
		require.Equal(t, set, loaded)
		require.Empty(t, eventBus.GetPublishedEvents())
	})
}

func testSetPatchPresence(t *testing.T) {
	members := []RepositorySetMemberInput{{RepositoryID: "repo-orders", BaseBranch: "orders-base"}}
	cases := []struct {
		name                      string
		req                       UpdateRepositorySetRequest
		wantName, wantDescription string
	}{
		{"name", UpdateRepositorySetRequest{Name: stringPointer("  Renamed  ")}, "Renamed", "Original description"},
		{"description", UpdateRepositorySetRequest{Description: stringPointer("  Changed  ")}, "Original name", "Changed"},
		{"clear", UpdateRepositorySetRequest{Description: stringPointer("")}, "Original name", ""},
		{"no-op", UpdateRepositorySetRequest{}, "Original name", "Original description"},
		{"members", UpdateRepositorySetRequest{Repositories: &members}, "Original name", "Original description"},
		{"combined", UpdateRepositorySetRequest{Name: stringPointer("Both"), Description: stringPointer("Updated"),
			Repositories: &members}, "Both", "Updated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, eventBus, repo := newSetPatchServices(t)
			initial := seedSetPatch(t, svc, repo)
			eventBus.ClearEvents()
			updated, err := svc.UpdateRepositorySet(context.Background(), initial.ID, &tc.req)
			require.NoError(t, err)
			stored, err := repo.GetRepositorySet(context.Background(), initial.ID)
			require.NoError(t, err)
			require.Equal(t, stored, updated)
			require.Equal(t, tc.wantName, stored.Name)
			require.Equal(t, tc.wantDescription, stored.Description)
			require.True(t, stored.UpdatedAt.After(initial.UpdatedAt))
			if tc.req.Repositories == nil {
				require.Equal(t, initial.Items, stored.Items)
			} else {
				require.Equal(t, []string{"repo-orders"}, stored.RepositoryIDs())
				require.Equal(t, "orders-base", stored.Items[0].BaseBranch)
			}
			published := eventBus.GetPublishedEvents()
			require.Len(t, published, 1)
			data := published[0].Data.(map[string]interface{})
			require.Equal(t, stored.Name, data["name"])
			require.Equal(t, stored.Description, data["description"])
		})
	}
}

func testSetPatchValidation(t *testing.T) {
	svc, _, eventBus, repo := newSetPatchServices(t)
	initial := seedSetPatch(t, svc, repo)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "other", Name: "Other"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{
		ID: "foreign", WorkspaceID: "other", Name: "Foreign", SourceType: "local", LocalPath: t.TempDir(),
	}))
	_, err := svc.CreateRepositorySet(ctx, &CreateRepositorySetRequest{
		WorkspaceID: "ws-1", Name: "Taken", RepositoryIDs: []string{"repo-web"},
	})
	require.NoError(t, err)
	cases := []struct {
		name string
		req  UpdateRepositorySetRequest
		want error
	}{
		{"empty name", UpdateRepositorySetRequest{Name: stringPointer(" ")}, ErrInvalidRepositorySet},
		{"long name", UpdateRepositorySetRequest{Name: stringPointer(strings.Repeat("x", 101))}, ErrInvalidRepositorySet},
		{"empty members", UpdateRepositorySetRequest{RepositoryIDs: &[]string{}}, ErrInvalidRepositorySet},
		{"duplicate", UpdateRepositorySetRequest{RepositoryIDs: &[]string{"repo-web", "repo-web"}}, ErrInvalidRepositorySet},
		{"unsafe base", UpdateRepositorySetRequest{Repositories: &[]RepositorySetMemberInput{
			{RepositoryID: "repo-web", BaseBranch: "--unsafe"}}}, ErrInvalidRepositorySet},
		{"both inputs", UpdateRepositorySetRequest{RepositoryIDs: &[]string{"repo-web"},
			Repositories: &[]RepositorySetMemberInput{{RepositoryID: "repo-web"}}}, ErrInvalidRepositorySet},
		{"missing", UpdateRepositorySetRequest{RepositoryIDs: &[]string{"missing"}}, ErrUnknownRepositorySetMembers},
		{"foreign", UpdateRepositorySetRequest{RepositoryIDs: &[]string{"foreign"}}, ErrUnknownRepositorySetMembers},
		{"conflict", UpdateRepositorySetRequest{Name: stringPointer("tAKEN")}, ErrRepositorySetNameConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eventBus.ClearEvents()
			tc.req.Description = stringPointer("must not survive")
			_, err := svc.UpdateRepositorySet(ctx, initial.ID, &tc.req)
			require.ErrorIs(t, err, tc.want)
			stored, err := repo.GetRepositorySet(ctx, initial.ID)
			require.NoError(t, err)
			require.Equal(t, initial, stored)
			require.Empty(t, eventBus.GetPublishedEvents())
		})
	}
}

func TestRepositorySetPatchDeletedBeforeWrite(t *testing.T) {
	a, b, eventBus, repo := newSetPatchServices(t)
	initial := seedSetPatch(t, a, repo)
	eventBus.ClearEvents()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gate, results := startGatedSetPatch(t, ctx, a, initial.ID,
		&UpdateRepositorySetRequest{Name: stringPointer("Late name")})
	require.NoError(t, b.DeleteRepositorySet(ctx, initial.ID))
	gate.unblock()
	result := awaitSetPatch(t, ctx, results)
	require.ErrorIs(t, result.err, repoerrors.ErrRepositorySetNotFound)
	_, err := repo.GetRepositorySet(ctx, initial.ID)
	require.ErrorIs(t, err, repoerrors.ErrRepositorySetNotFound)
	published := eventBus.GetPublishedEvents()
	require.Len(t, published, 1)
	require.Equal(t, "repository_set.deleted", published[0].Type)
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.11
func TestRepositorySetConcurrentSameFieldPatches(t *testing.T) {
	a, b, _, repo := newSetPatchServices(t)
	initial := seedSetPatch(t, a, repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gate, results := startGatedSetPatch(t, ctx, a, initial.ID,
		&UpdateRepositorySetRequest{Name: stringPointer("Last committed")})
	_, err := b.UpdateRepositorySet(ctx, initial.ID, &UpdateRepositorySetRequest{Name: stringPointer("First committed")})
	require.NoError(t, err)
	gate.unblock()
	require.NoError(t, awaitSetPatch(t, ctx, results).err)
	stored, err := repo.GetRepositorySet(ctx, initial.ID)
	require.NoError(t, err)
	require.Equal(t, "Last committed", stored.Name)
	require.Equal(t, initial.Description, stored.Description)
	require.Equal(t, initial.Items, stored.Items)
}
