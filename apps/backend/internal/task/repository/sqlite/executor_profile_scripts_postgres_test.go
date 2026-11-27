package sqlite_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
)

func postgresProfileScriptService(t *testing.T, repo *tasksqlite.Repository) *service.Service {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return service.NewService(service.Repos{Executors: repo}, nil, log, service.RepositoryDiscoveryConfig{})
}

func seedPostgresProfileScripts(t *testing.T, repo *tasksqlite.Repository) *models.ExecutorProfile {
	t.Helper()
	require.NoError(t, repo.CreateExecutor(t.Context(), &models.Executor{ID: "pg-script-executor", Name: "Local", Type: models.ExecutorTypeLocal}))
	profile := &models.ExecutorProfile{ID: "pg-script-profile", ExecutorID: "pg-script-executor", Name: "Before", PrepareScript: "before-prepare", CleanupScript: "before-cleanup"}
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), profile))
	return profile
}

type postgresProfileScriptResult struct {
	profile *models.ExecutorProfile
	err     error
}

func lockedPostgresProfileScriptSave(t *testing.T, request *service.UpdateExecutorProfileRequest, holder func(context.Context, *sql.Tx) error) postgresProfileScriptResult {
	t.Helper()
	a, b, observer := workflowPostgresPair(t)
	seedPostgresProfileScripts(t, a)
	version := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := a.DB().ExecContext(t.Context(), `UPDATE executor_profiles SET updated_at = $1 WHERE id = 'pg-script-profile'`, version)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	holderPID, writerPID := hierarchyBackendPID(t, a.DB()), hierarchyBackendPID(t, b.DB())
	require.NotEqual(t, holderPID, writerPID)
	tx, err := a.DB().BeginTx(ctx, nil)
	if err != nil {
		cancel()
		require.NoError(t, err)
	}
	var workers sync.WaitGroup
	defer func() { _ = tx.Rollback(); cancel(); workers.Wait() }()
	_, err = tx.ExecContext(ctx, `SELECT id FROM executor_profiles WHERE id='pg-script-profile' FOR UPDATE`)
	require.NoError(t, err)
	svc := postgresProfileScriptService(t, b)
	done := make(chan postgresProfileScriptResult, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		profile, err := svc.UpdateExecutorProfile(ctx, "pg-script-profile", request)
		done <- postgresProfileScriptResult{profile, err}
	}()
	waitSettingsPostgresWriter(t, ctx, observer, holderPID, writerPID)
	t.Logf("executor scripts PostgreSQL holder PID=%d writer PID=%d physically blocked", holderPID, writerPID)
	require.NoError(t, holder(ctx, tx))
	require.NoError(t, tx.Commit())
	var result postgresProfileScriptResult
	select {
	case result = <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	stored, err := a.GetExecutorProfile(t.Context(), "pg-script-profile")
	require.NoError(t, err)
	if result.err == nil {
		require.Equal(t, result.profile.PrepareScript, stored.PrepareScript)
		require.Equal(t, result.profile.CleanupScript, stored.CleanupScript)
		require.True(t, result.profile.UpdatedAt.Equal(stored.UpdatedAt))
	} else {
		require.True(t, stored.UpdatedAt.Equal(version.Add(time.Hour)))
	}
	return result
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.8 AC-EXECUTORS-PROFILE-EDITOR-001.9 AC-EXECUTORS-PROFILE-EDITOR-001.10
func TestPostgresExecutorProfileScriptsPhysicalConcurrency(t *testing.T) {
	name, prepare, cleanup, empty := "Rename", "saved-prepare", "saved-cleanup", ""
	for _, tc := range []struct {
		name                                        string
		request                                     *service.UpdateExecutorProfileRequest
		holderColumn, holderValue, prepare, cleanup string
	}{
		{"name_after_prepare", &service.UpdateExecutorProfileRequest{Name: &name}, "prepare_script", prepare, prepare, "before-cleanup"},
		{"name_after_cleanup", &service.UpdateExecutorProfileRequest{Name: &name}, "cleanup_script", cleanup, "before-prepare", cleanup},
		{"prepare_after_cleanup", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare}, "cleanup_script", cleanup, prepare, cleanup},
		{"cleanup_after_prepare", &service.UpdateExecutorProfileRequest{CleanupScript: &cleanup}, "prepare_script", prepare, prepare, cleanup},
		{"name_after_prepare_clear", &service.UpdateExecutorProfileRequest{Name: &name}, "prepare_script", empty, empty, "before-cleanup"},
		{"prepare_after_cleanup_clear", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare}, "cleanup_script", empty, prepare, empty},
		{"explicit_prepare_last_commit", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare}, "prepare_script", cleanup, prepare, "before-cleanup"},
		{"explicit_clear_last_commit", &service.UpdateExecutorProfileRequest{PrepareScript: &empty}, "prepare_script", prepare, empty, "before-cleanup"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := lockedPostgresProfileScriptSave(t, tc.request, func(ctx context.Context, tx *sql.Tx) error {
				_, err := tx.ExecContext(ctx, fmt.Sprintf("UPDATE executor_profiles SET %s=$1,updated_at=$2 WHERE id='pg-script-profile'", tc.holderColumn), tc.holderValue, time.Date(2020, 1, 1, 1, 0, 0, 0, time.UTC))
				return err
			})
			require.NoError(t, result.err)
			require.Equal(t, tc.prepare, result.profile.PrepareScript)
			require.Equal(t, tc.cleanup, result.profile.CleanupScript)
		})
	}
}

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.11 AC-EXECUTORS-PROFILE-EDITOR-001.12
func TestPostgresExecutorProfileScriptsCompatibility(t *testing.T) {
	t.Run("exact_after_row_wait", func(t *testing.T) {
		version := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		prepare := "stale-exact"
		result := lockedPostgresProfileScriptSave(t, &service.UpdateExecutorProfileRequest{PrepareScript: &prepare, ExpectedUpdatedAt: &version}, func(ctx context.Context, tx *sql.Tx) error {
			_, err := tx.ExecContext(ctx, `UPDATE executor_profiles SET updated_at=$1 WHERE id='pg-script-profile'`, version.Add(time.Hour))
			return err
		})
		require.Error(t, result.err)
	})
	t.Run("presence_exact_legacy_and_failures", postgresProfileScriptControls)
}

func postgresProfileScriptControls(t *testing.T) {
	a, b, _ := workflowPostgresPair(t)
	profile := seedPostgresProfileScripts(t, a)
	svc := postgresProfileScriptService(t, b)
	version := time.Date(2020, 2, 1, 0, 0, 0, 0, time.UTC)
	_, err := a.DB().ExecContext(t.Context(), `UPDATE executor_profiles SET updated_at=$1 WHERE id=$2`, version, profile.ID)
	require.NoError(t, err)
	empty := ""
	saved, err := svc.UpdateExecutorProfile(t.Context(), profile.ID, &service.UpdateExecutorProfileRequest{CleanupScript: &empty, ExpectedUpdatedAt: &version})
	require.NoError(t, err)
	require.Empty(t, saved.CleanupScript)
	require.Equal(t, "before-prepare", saved.PrepareScript)
	saved.PrepareScript, saved.CleanupScript = "legacy-prepare", "legacy-cleanup"
	require.NoError(t, a.UpdateExecutorProfile(t.Context(), saved))
	before, err := b.GetExecutorProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	_, err = a.DB().ExecContext(t.Context(), `ALTER TABLE executor_profiles ADD CONSTRAINT reject_script_name CHECK (name <> 'rejected')`)
	require.NoError(t, err)
	name, prepare := "rejected", "not-stored"
	_, err = svc.UpdateExecutorProfile(t.Context(), profile.ID, &service.UpdateExecutorProfileRequest{Name: &name, PrepareScript: &prepare})
	require.Error(t, err)
	stored, err := b.GetExecutorProfile(t.Context(), profile.ID)
	require.NoError(t, err)
	require.Equal(t, before, stored, "failed UPDATE rolls back scripts and timestamp")
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = svc.UpdateExecutorProfile(cancelled, profile.ID, &service.UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.ErrorIs(t, err, context.Canceled)
	_, err = svc.UpdateExecutorProfile(t.Context(), "missing-profile", &service.UpdateExecutorProfileRequest{PrepareScript: &prepare})
	require.Error(t, err)
	name = "reused"
	saved, err = svc.UpdateExecutorProfile(t.Context(), profile.ID, &service.UpdateExecutorProfileRequest{Name: &name, PrepareScript: &empty, CleanupScript: &empty})
	require.NoError(t, err)
	require.Empty(t, saved.PrepareScript)
	require.Empty(t, saved.CleanupScript)
}
