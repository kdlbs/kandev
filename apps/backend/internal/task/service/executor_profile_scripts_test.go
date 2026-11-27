package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
)

var profileScriptTemplate = testutil.NewSQLiteTemplate(func(database *sqlx.DB) error {
	_, err := tasksqlite.NewWithDB(database, database, nil)
	return err
})

type profileScriptSnapshotGate struct {
	repository.ExecutorRepository
	read    chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *profileScriptSnapshotGate) GetExecutorProfile(ctx context.Context, id string) (*models.ExecutorProfile, error) {
	profile, err := g.ExecutorRepository.GetExecutorProfile(ctx, id)
	if err == nil {
		g.once.Do(func() {
			close(g.read)
			select {
			case <-g.release:
			case <-ctx.Done():
				err = ctx.Err()
			}
		})
	}
	return profile, err
}

func profileScriptService(t *testing.T, repo repository.ExecutorRepository) (*Service, *MockEventBus) {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	bus := NewMockEventBus()
	return NewService(Repos{Executors: repo}, bus, log, RepositoryDiscoveryConfig{}), bus
}

func profileScriptStores(t *testing.T) (*tasksqlite.Repository, *tasksqlite.Repository) {
	t.Helper()
	a, path := profileScriptTemplate.Open(t)
	raw, err := db.OpenSQLite(path)
	require.NoError(t, err)
	b := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	identities := make([]string, 0, 2)
	for _, database := range []*sqlx.DB{a, b} {
		conn, connErr := database.Conn(t.Context())
		require.NoError(t, connErr)
		err = conn.Raw(func(driverConn any) error {
			identities = append(identities, fmt.Sprintf("%p", driverConn))
			return nil
		})
		require.NoError(t, conn.Close())
		require.NoError(t, err)
	}
	require.NotEqual(t, identities[0], identities[1], "independent physical SQLite connections")
	t.Logf("independent SQLite connections %s / %s", identities[0], identities[1])
	return tasksqlite.NewWithInitializedDB(a, a, nil), tasksqlite.NewWithInitializedDB(b, b, nil)
}

func seedProfileScripts(t *testing.T, repo *tasksqlite.Repository) *models.ExecutorProfile {
	t.Helper()
	require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{
		ID: "script-executor", Name: "Script executor", Type: models.ExecutorTypeLocal, Status: models.ExecutorStatusActive,
	}))
	profile := &models.ExecutorProfile{
		ID: "script-profile", ExecutorID: "script-executor", Name: "Before",
		PrepareScript: "prepare-before", CleanupScript: "cleanup-before",
	}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	return profile
}

func assertProfileScripts(t *testing.T, profile *models.ExecutorProfile, prepare, cleanup string) {
	t.Helper()
	require.NotNil(t, profile)
	assert.Equal(t, prepare, profile.PrepareScript, "acknowledged prepare script")
	assert.Equal(t, cleanup, profile.CleanupScript, "acknowledged cleanup script")
}

type profileScriptSaveResult struct {
	profile *models.ExecutorProfile
	err     error
}

type heldProfileScriptResult struct {
	result  <-chan profileScriptSaveResult
	release func()
	bus     *MockEventBus
}

func heldProfileScriptSave(t *testing.T, repo repository.ExecutorRepository, req *UpdateExecutorProfileRequest) heldProfileScriptResult {
	t.Helper()
	gate := &profileScriptSnapshotGate{ExecutorRepository: repo, read: make(chan struct{}), release: make(chan struct{})}
	svc, bus := profileScriptService(t, gate)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan profileScriptSaveResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		profile, err := svc.UpdateExecutorProfile(ctx, "script-profile", req)
		result <- profileScriptSaveResult{profile, err}
	}()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-gate.read:
	case saved := <-result:
		t.Fatalf("save ended before snapshot gate: %v", saved.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return heldProfileScriptResult{result: result, release: func() { close(gate.release) }, bus: bus}
}

func finishProfileScriptSave(t *testing.T, held heldProfileScriptResult) *models.ExecutorProfile {
	t.Helper()
	held.release()
	select {
	case saved := <-held.result:
		require.NoError(t, saved.err)
		return saved.profile
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
		return nil
	}
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9
func TestExecutorProfileScriptsIndependentSQLite(t *testing.T) {
	name, prepare, cleanup, empty := "Rename", "prepare-saved", "cleanup-saved", ""
	for _, tc := range []struct {
		name       string
		held, next *UpdateExecutorProfileRequest
		prepare    string
		cleanup    string
	}{
		{"name_after_prepare", &UpdateExecutorProfileRequest{Name: &name}, &UpdateExecutorProfileRequest{PrepareScript: &prepare}, prepare, "cleanup-before"},
		{"name_after_cleanup", &UpdateExecutorProfileRequest{Name: &name}, &UpdateExecutorProfileRequest{CleanupScript: &cleanup}, "prepare-before", cleanup},
		{"prepare_after_cleanup", &UpdateExecutorProfileRequest{PrepareScript: &prepare}, &UpdateExecutorProfileRequest{CleanupScript: &cleanup}, prepare, cleanup},
		{"cleanup_after_prepare", &UpdateExecutorProfileRequest{CleanupScript: &cleanup}, &UpdateExecutorProfileRequest{PrepareScript: &prepare}, prepare, cleanup},
		{"explicit_same_script_last_commit", &UpdateExecutorProfileRequest{PrepareScript: &prepare}, &UpdateExecutorProfileRequest{PrepareScript: &cleanup}, prepare, "cleanup-before"},
		{"name_after_prepare_clear", &UpdateExecutorProfileRequest{Name: &name}, &UpdateExecutorProfileRequest{PrepareScript: &empty}, "", "cleanup-before"},
		{"name_after_cleanup_clear", &UpdateExecutorProfileRequest{Name: &name}, &UpdateExecutorProfileRequest{CleanupScript: &empty}, "prepare-before", ""},
		{"prepare_after_cleanup_clear", &UpdateExecutorProfileRequest{PrepareScript: &prepare}, &UpdateExecutorProfileRequest{CleanupScript: &empty}, prepare, ""},
		{"cleanup_after_prepare_clear", &UpdateExecutorProfileRequest{CleanupScript: &cleanup}, &UpdateExecutorProfileRequest{PrepareScript: &empty}, "", cleanup},
		{"explicit_clear_last_commit", &UpdateExecutorProfileRequest{PrepareScript: &empty}, &UpdateExecutorProfileRequest{PrepareScript: &prepare}, "", "cleanup-before"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := profileScriptStores(t)
			seedProfileScripts(t, a)
			held := heldProfileScriptSave(t, a, tc.held)
			other, _ := profileScriptService(t, b)
			_, err := other.UpdateExecutorProfile(t.Context(), "script-profile", tc.next)
			require.NoError(t, err, "competing script save was acknowledged")
			returned := finishProfileScriptSave(t, held)
			stored, err := b.GetExecutorProfile(t.Context(), returned.ID)
			require.NoError(t, err)
			assertProfileScripts(t, returned, tc.prepare, tc.cleanup)
			assertProfileScripts(t, stored, tc.prepare, tc.cleanup)
			assertProfileScriptEvent(t, held.bus, returned)
		})
	}
}

func assertProfileScriptEvent(t *testing.T, bus *MockEventBus, profile *models.ExecutorProfile) {
	t.Helper()
	published := bus.GetPublishedEvents()
	require.Len(t, published, 1)
	require.Equal(t, events.ExecutorProfileUpdated, published[0].Type)
	data := published[0].Data.(map[string]interface{})
	require.Equal(t, profile.PrepareScript, data["prepare_script"])
	require.Equal(t, profile.CleanupScript, data["cleanup_script"])
	require.Equal(t, profile.UpdatedAt.Format(time.RFC3339), data["updated_at"])
}

type profileScriptCommitGate struct {
	repository.ExecutorRepository
	committed   *models.ExecutorProfile
	afterCommit func(context.Context) error
}

func (g *profileScriptCommitGate) UpdateExecutorProfileWithScriptIntent(ctx context.Context, profile *models.ExecutorProfile, intent models.ExecutorProfileScriptIntent) error {
	if err := g.ExecutorRepository.UpdateExecutorProfileWithScriptIntent(ctx, profile, intent); err != nil {
		return err
	}
	copy := *profile
	g.committed = &copy
	return g.afterCommit(ctx)
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.10
func TestExecutorProfileScriptsOwnCommit(t *testing.T) {
	a, b := profileScriptStores(t)
	seedProfileScripts(t, a)
	later, _ := profileScriptService(t, b)
	gate := &profileScriptCommitGate{ExecutorRepository: a}
	prepare, cleanup := "own-prepare", "later-cleanup"
	gate.afterCommit = func(ctx context.Context) error {
		_, err := later.UpdateExecutorProfile(ctx, "script-profile", &UpdateExecutorProfileRequest{CleanupScript: &cleanup})
		return err
	}
	svc, bus := profileScriptService(t, gate)
	returned, err := svc.UpdateExecutorProfile(t.Context(), "script-profile", &UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.NoError(t, err)
	assertProfileScripts(t, returned, prepare, "cleanup-before")
	require.True(t, returned.UpdatedAt.Equal(gate.committed.UpdatedAt))
	assertProfileScriptEvent(t, bus, returned)
	stored, err := b.GetExecutorProfile(t.Context(), returned.ID)
	require.NoError(t, err)
	assertProfileScripts(t, stored, prepare, cleanup)
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.11
func TestExecutorProfileScriptsFailures(t *testing.T) {
	a, b := profileScriptStores(t)
	before := seedProfileScripts(t, a)
	svc, bus := profileScriptService(t, b)
	prepare := "rejected-prepare"
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := svc.UpdateExecutorProfile(cancelled, before.ID, &UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.ErrorIs(t, err, context.Canceled)
	_, err = svc.UpdateExecutorProfile(t.Context(), "missing-profile", &UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.Error(t, err)
	_, err = a.DB().ExecContext(t.Context(), `CREATE TRIGGER reject_profile_script BEFORE UPDATE ON executor_profiles BEGIN SELECT RAISE(ABORT, 'script rejected'); END`)
	require.NoError(t, err)
	_, err = svc.UpdateExecutorProfile(t.Context(), before.ID, &UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.ErrorContains(t, err, "script rejected")
	stored, err := a.GetExecutorProfile(t.Context(), before.ID)
	require.NoError(t, err)
	assertProfileScripts(t, stored, before.PrepareScript, before.CleanupScript)
	require.True(t, before.UpdatedAt.Equal(stored.UpdatedAt))
	require.Empty(t, bus.GetPublishedEvents())
	_, err = a.DB().ExecContext(t.Context(), `DROP TRIGGER reject_profile_script`)
	require.NoError(t, err)
	saved, err := svc.UpdateExecutorProfile(t.Context(), before.ID, &UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.NoError(t, err, "writer connection reused after rollback")
	assertProfileScriptEvent(t, bus, saved)
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.12
func TestExecutorProfileScriptsCompatibility(t *testing.T) {
	t.Run("fixed_exact_and_legacy", profileScriptExactCompatibility)
	t.Run("admin_and_config_admission", profileScriptAdmissionCompatibility)
	t.Run("plugin_keeps_separate_writer", profileScriptPluginCompatibility)
}

type rejectingOrdinaryScriptStore struct{ repository.ExecutorRepository }

func (*rejectingOrdinaryScriptStore) UpdateExecutorProfileWithScriptIntent(context.Context, *models.ExecutorProfile, models.ExecutorProfileScriptIntent) error {
	return fmt.Errorf("unexpected ordinary script writer")
}

func profileScriptPluginCompatibility(t *testing.T) {
	a, _ := profileScriptStores(t)
	svc, bus := profileScriptService(t, &rejectingOrdinaryScriptStore{ExecutorRepository: a})
	svc.SetExecutorProviderCatalog(pluginExecutorTestCatalog(true))
	executors, err := svc.ListExecutors(t.Context())
	require.NoError(t, err)
	var remote *models.Executor
	for _, candidate := range executors {
		if candidate.Type == models.ExecutorTypePluginRemote {
			remote = candidate
			break
		}
	}
	require.NotNil(t, remote)
	profile, err := svc.CreateExecutorProfile(t.Context(), &CreateExecutorProfileRequest{ExecutorID: remote.ID, Name: "Plugin", Config: map[string]string{"region": "eu-west-1"}})
	require.NoError(t, err)
	bus.ClearEvents()
	name := "Plugin renamed"
	saved, err := svc.UpdateExecutorProfile(t.Context(), profile.ID, &UpdateExecutorProfileRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, name, saved.Name)
	assertProfileScriptEvent(t, bus, saved)
	bus.ClearEvents()
	script := "not allowed"
	_, err = svc.UpdateExecutorProfile(t.Context(), profile.ID, &UpdateExecutorProfileRequest{PrepareScript: &script})
	require.ErrorIs(t, err, ErrInvalidExecutorConfig)
	require.Empty(t, bus.GetPublishedEvents())
}

func profileScriptExactCompatibility(t *testing.T) {
	a, b := profileScriptStores(t)
	before := seedProfileScripts(t, a)
	version := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := a.DB().ExecContext(t.Context(), `UPDATE executor_profiles SET updated_at = ? WHERE id = ?`, version, before.ID)
	require.NoError(t, err)
	svc, bus := profileScriptService(t, b)
	prepare := "exact-prepare"
	saved, err := svc.UpdateExecutorProfile(t.Context(), before.ID, &UpdateExecutorProfileRequest{PrepareScript: &prepare, ExpectedUpdatedAt: &version})
	require.NoError(t, err)
	assertProfileScripts(t, saved, prepare, "cleanup-before")
	assertProfileScriptEvent(t, bus, saved)
	bus.ClearEvents()
	newVersion := version.Add(time.Hour)
	_, err = a.DB().ExecContext(t.Context(), `UPDATE executor_profiles SET updated_at = ? WHERE id = ?`, newVersion, before.ID)
	require.NoError(t, err)
	empty := ""
	_, err = svc.UpdateExecutorProfile(t.Context(), before.ID, &UpdateExecutorProfileRequest{PrepareScript: &empty, ExpectedUpdatedAt: &version})
	require.Error(t, err, "timestamp-only intervening write rejects exact request")
	require.Empty(t, bus.GetPublishedEvents())
	stored, err := a.GetExecutorProfile(t.Context(), before.ID)
	require.NoError(t, err)
	assertProfileScripts(t, stored, prepare, "cleanup-before")
	require.True(t, stored.UpdatedAt.Equal(newVersion))
	stored.PrepareScript, stored.CleanupScript = "legacy-prepare", ""
	require.NoError(t, b.UpdateExecutorProfile(t.Context(), stored))
	stored, err = a.GetExecutorProfile(t.Context(), before.ID)
	require.NoError(t, err)
	assertProfileScripts(t, stored, "legacy-prepare", "")
}

func profileScriptAdmissionCompatibility(t *testing.T) {
	for _, executorType := range []models.ExecutorType{models.ExecutorTypeKubernetes, models.ExecutorTypeRemoteDocker} {
		t.Run(string(executorType), func(t *testing.T) {
			a, b := profileScriptStores(t)
			before := seedProfileScripts(t, a)
			executor, err := a.GetExecutor(t.Context(), before.ExecutorID)
			require.NoError(t, err)
			executor.Type = executorType
			require.NoError(t, a.UpdateExecutor(t.Context(), executor))
			svc, bus := profileScriptService(t, b)
			member := authn.WithIdentity(t.Context(), authn.Identity{UserID: "member", Role: authn.RoleMember})
			prepare := "denied"
			_, err = svc.UpdateExecutorProfile(member, before.ID, &UpdateExecutorProfileRequest{PrepareScript: &prepare})
			require.Error(t, err)
			require.Empty(t, bus.GetPublishedEvents())
			if executorType == models.ExecutorTypeKubernetes {
				admin := authn.WithIdentity(t.Context(), authn.Identity{Synthetic: true})
				_, err = svc.UpdateExecutorProfile(admin, before.ID, &UpdateExecutorProfileRequest{Config: map[string]string{"pod_template_yaml": "invalid: ["}})
				require.Error(t, err)
				require.Empty(t, bus.GetPublishedEvents())
			}
			stored, err := a.GetExecutorProfile(t.Context(), before.ID)
			require.NoError(t, err)
			assertProfileScripts(t, stored, before.PrepareScript, before.CleanupScript)
		})
	}
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.9
func TestExecutorProfileScriptsPresence(t *testing.T) {
	a, b := profileScriptStores(t)
	seedProfileScripts(t, a)
	svc, _ := profileScriptService(t, b)
	for _, tc := range []struct{ payload, prepare, cleanup string }{
		{`{"name":"Renamed"}`, "prepare-before", "cleanup-before"},
		{`{"prepare_script":null,"cleanup_script":null}`, "prepare-before", "cleanup-before"},
		{`{"prepare_script":""}`, "", "cleanup-before"},
		{`{"prepare_script":"both-prepare","cleanup_script":"both-cleanup"}`, "both-prepare", "both-cleanup"},
		{`{"cleanup_script":""}`, "both-prepare", ""},
	} {
		t.Run(tc.payload, func(t *testing.T) {
			var req UpdateExecutorProfileRequest
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &req))
			profile, err := svc.UpdateExecutorProfile(t.Context(), "script-profile", &req)
			require.NoError(t, err)
			stored, err := a.GetExecutorProfile(t.Context(), profile.ID)
			require.NoError(t, err)
			assertProfileScripts(t, profile, tc.prepare, tc.cleanup)
			assertProfileScripts(t, stored, tc.prepare, tc.cleanup)
		})
	}
}
