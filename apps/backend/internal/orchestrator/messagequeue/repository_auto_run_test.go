package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type autoRunRepository interface {
	GetAutoRun(context.Context, string) (bool, error)
	SetAutoRun(context.Context, string, bool) error
	ReserveHeadIfAutoRun(context.Context, string) (*QueuedMessage, bool, error)
}

type autoRunService interface {
	SetAutoRun(context.Context, string, bool) error
	PauseAutoRunIfPending(context.Context, string) (bool, error)
}

var autoRunRepositoryFactories = []struct {
	name string
	new  func(*testing.T) Repository
}{
	{name: "memory", new: func(*testing.T) Repository { return NewMemoryRepository() }},
	{name: "sqlite", new: newTestSQLiteRepo},
}

func requireAutoRunRepository(t *testing.T, repo Repository) autoRunRepository {
	t.Helper()
	autoRunRepo, ok := repo.(autoRunRepository)
	require.True(t, ok, "%T must implement the queue Auto-run repository contract", repo)
	return autoRunRepo
}

func TestGetStatus_DefaultAutoRunOn(t *testing.T) {
	status := setupService(t).GetStatus(context.Background(), "new-session")

	payload, err := json.Marshal(status)
	require.NoError(t, err)
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(payload, &decoded))

	autoRun, present := decoded["auto_run"]
	require.True(t, present, "queue status must always project auto_run")
	assert.Equal(t, true, autoRun)
}

func TestGetStatus_ProjectsPersistedAutoRunOff(t *testing.T) {
	svc := setupService(t)
	repo := requireAutoRunRepository(t, svc.repo)
	require.NoError(t, repo.SetAutoRun(context.Background(), "session-1", false))

	assert.False(t, svc.GetStatus(context.Background(), "session-1").AutoRun)
}

func TestRepositories_AutoRunDefaultsOnAndPersistsExplicitState(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repo := requireAutoRunRepository(t, factory.new(t))
			ctx := context.Background()

			enabled, err := repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, err)
			assert.True(t, enabled)

			require.NoError(t, repo.SetAutoRun(ctx, "session-1", false))
			enabled, err = repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, err)
			assert.False(t, enabled)

			require.NoError(t, repo.SetAutoRun(ctx, "session-1", true))
			enabled, err = repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, err)
			assert.True(t, enabled)
		})
	}
}

func TestRepositories_ReserveHeadIfAutoRunDistinguishesPausedAndEmpty(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			base := factory.new(t)
			repo := requireAutoRunRepository(t, base)
			ctx := context.Background()
			entry := &QueuedMessage{SessionID: "session-1", TaskID: "task-1", Content: "queued", QueuedBy: QueuedByUser}
			require.NoError(t, base.Insert(ctx, entry, 10))
			require.NoError(t, repo.SetAutoRun(ctx, "session-1", false))

			reserved, enabled, err := repo.ReserveHeadIfAutoRun(ctx, "session-1")
			require.NoError(t, err)
			assert.Nil(t, reserved)
			assert.False(t, enabled)
			remaining, err := base.ListBySession(ctx, "session-1")
			require.NoError(t, err)
			require.Len(t, remaining, 1)
			assert.Equal(t, entry.ID, remaining[0].ID)

			require.NoError(t, repo.SetAutoRun(ctx, "session-1", true))
			reserved, enabled, err = repo.ReserveHeadIfAutoRun(ctx, "session-1")
			require.NoError(t, err)
			require.NotNil(t, reserved)
			assert.True(t, enabled)
			assert.Equal(t, entry.ID, reserved.ID)

			reserved, enabled, err = repo.ReserveHeadIfAutoRun(ctx, "session-1")
			require.NoError(t, err)
			assert.Nil(t, reserved)
			assert.True(t, enabled)
		})
	}
}

func TestRepositories_ClaimSendNowResumesOnlyAcceptedClaim(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			base := factory.new(t)
			repo := requireAutoRunRepository(t, base)
			ctx := context.Background()
			entry := &QueuedMessage{SessionID: "session-1", TaskID: "task-1", Content: "queued", QueuedBy: QueuedByUser}
			require.NoError(t, base.Insert(ctx, entry, 10))
			require.NoError(t, repo.SetAutoRun(ctx, "session-1", false))

			_, err := base.ClaimSendNow(ctx, "session-1", []QueuedMessage{{ID: "missing"}})
			require.Error(t, err)
			enabled, getErr := repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, getErr)
			assert.False(t, enabled)

			claim, err := base.ClaimSendNow(ctx, "session-1", []QueuedMessage{*entry})
			require.NoError(t, err)
			enabled, getErr = repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, getErr)
			assert.True(t, enabled)

			require.NoError(t, base.RestoreSendNowClaim(ctx, claim))
			enabled, getErr = repo.GetAutoRun(ctx, "session-1")
			require.NoError(t, getErr)
			assert.True(t, enabled)
		})
	}
}

func TestRepositories_TransferSessionUsesPauseWinsAndReplacePreservesPolicy(t *testing.T) {
	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			base := factory.new(t)
			repo := requireAutoRunRepository(t, base)
			ctx := context.Background()
			require.NoError(t, repo.SetAutoRun(ctx, "source", false))
			require.NoError(t, repo.SetAutoRun(ctx, "destination", true))
			require.NoError(t, base.Insert(ctx, &QueuedMessage{SessionID: "source", TaskID: "task-1", QueuedBy: QueuedByUser}, 10))

			require.NoError(t, base.TransferSession(ctx, "source", "destination"))
			destinationEnabled, err := repo.GetAutoRun(ctx, "destination")
			require.NoError(t, err)
			assert.False(t, destinationEnabled)
			sourceEnabled, err := repo.GetAutoRun(ctx, "source")
			require.NoError(t, err)
			assert.True(t, sourceEnabled, "removed source state must fall back to ON")

			require.NoError(t, base.ReplaceSession(ctx, "destination", nil, nil))
			destinationEnabled, err = repo.GetAutoRun(ctx, "destination")
			require.NoError(t, err)
			assert.False(t, destinationEnabled)
		})
	}
}

func TestSQLiteRepository_AutoRunSurvivesReconstruction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queue.db")
	openDB := func() *sqlx.DB {
		raw, err := sql.Open("sqlite3", path)
		require.NoError(t, err)
		return sqlx.NewDb(raw, "sqlite3")
	}

	db := openDB()
	repo, err := NewSQLiteRepository(db, db)
	require.NoError(t, err)
	require.NoError(t, requireAutoRunRepository(t, repo).SetAutoRun(context.Background(), "session-1", false))
	require.NoError(t, db.Close())

	db = openDB()
	t.Cleanup(func() { _ = db.Close() })
	repo, err = NewSQLiteRepository(db, db)
	require.NoError(t, err)
	enabled, err := requireAutoRunRepository(t, repo).GetAutoRun(context.Background(), "session-1")
	require.NoError(t, err)
	assert.False(t, enabled)
}

func TestService_AutomaticReserveHonorsAutoRun(t *testing.T) {
	svc := setupService(t)
	controller, ok := interface{}(svc).(autoRunService)
	require.True(t, ok, "messagequeue Service must expose Auto-run control")
	ctx := context.Background()
	require.NoError(t, controller.SetAutoRun(ctx, "session-1", false))
	queued, err := svc.QueueMessage(ctx, "session-1", "task-1", "held", "", QueuedByUser, false, nil)
	require.NoError(t, err)

	reserved, exists := svc.ReserveQueued(ctx, "session-1")
	assert.Nil(t, reserved)
	assert.False(t, exists)
	status := svc.GetStatus(ctx, "session-1")
	require.Len(t, status.Entries, 1)
	assert.Equal(t, queued.ID, status.Entries[0].ID)
	assert.False(t, status.AutoRun)
}

func TestService_PauseAutoRunOnlyWhenPending(t *testing.T) {
	t.Run("empty queue preserves ON", func(t *testing.T) {
		svc := setupService(t)
		controller := interface{}(svc).(autoRunService)
		paused, err := controller.PauseAutoRunIfPending(context.Background(), "session-1")
		require.NoError(t, err)
		assert.False(t, paused)
		assert.True(t, svc.GetStatus(context.Background(), "session-1").AutoRun)
	})

	t.Run("pending queue becomes OFF", func(t *testing.T) {
		svc := setupService(t)
		controller := interface{}(svc).(autoRunService)
		ctx := context.Background()
		_, err := svc.QueueMessage(ctx, "session-1", "task-1", "held", "", QueuedByUser, false, nil)
		require.NoError(t, err)
		paused, err := controller.PauseAutoRunIfPending(ctx, "session-1")
		require.NoError(t, err)
		assert.True(t, paused)
		assert.False(t, svc.GetStatus(ctx, "session-1").AutoRun)
	})
}
