package toolretention

import (
	"context"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/settings"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func testService(t *testing.T) *Service {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	w, err := db.OpenSQLite(path)
	require.NoError(t, err)
	r, err := db.OpenSQLiteReader(path)
	if err != nil {
		_ = w.Close()
		t.Fatal(err)
	}
	pool := db.NewPool(sqlx.NewDb(w, "sqlite3"), sqlx.NewDb(r, "sqlite3"))
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	_, err = settings.NewStore(pool)
	require.NoError(t, err)
	return New(pool)
}

// @covers AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-001.1, 001.7, 002.1
func TestPolicyPersistenceAndExplicitPreparation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	got, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, DefaultPolicy(), got.Policy)
	_, err = s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}})
	require.ErrorContains(t, err, "backup_choice_required")
	got, err = s.Save(ctx, Update{Enabled: true, Age: Age{3, "months"}, BackupChoice: "backup"})
	require.NoError(t, err)
	require.False(t, got.Policy.Enabled)
	require.Equal(t, "pending", got.Preparation.State)
	require.EqualValues(t, 1, got.Policy.Revision)
	_, err = s.Save(ctx, Update{Age: Age{4, "months"}})
	require.ErrorContains(t, err, "conflict")
	reopened := New(s.pool)
	restored, err := reopened.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, got, restored)
	got, err = s.Save(ctx, Update{Age: Age{3, "months"}, Revision: 1})
	require.NoError(t, err)
	require.False(t, got.Policy.Enabled)
	require.Equal(t, "none", got.Preparation.State)
}

func TestUnreadablePolicyCannotAuthorizeCleanup(t *testing.T) {
	s := testService(t)
	_, err := s.pool.Writer().Exec(`INSERT INTO settings(key,value,updated_at) VALUES('tool_payload_retention','garbage',CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	_, err = s.Get(context.Background())
	require.Error(t, err)
	_, err = s.Save(context.Background(), Update{Enabled: true, Age: Age{1, "weeks"}, BackupChoice: "skip"})
	require.Error(t, err)
}

func TestUnreadableRuntimeStateFailsClosed(t *testing.T) {
	for _, value := range []string{
		`{"version":1,"policy":{"enabled":false,"age":{"value":3,"unit":"months"}},"preparation":{"state":"pending","choice":"unknown"}}`,
		`{"version":1,"policy":{"enabled":true,"revision":2,"age":{"value":3,"unit":"months"}},"preparation":{"state":"ready","choice":"skip"},"approved_revision":1}`,
		`{"version":1,"policy":{"age":{"value":3,"unit":"months"}},"preparation":{"state":"none"},"operation":{"id":"x","kind":"unknown","state":"running"}}`,
	} {
		s := testService(t)
		_, err := s.pool.Writer().Exec(`INSERT INTO settings(key,value,updated_at) VALUES('tool_payload_retention',?,CURRENT_TIMESTAMP)`, value)
		require.NoError(t, err)
		_, err = s.Get(context.Background())
		require.Error(t, err)
	}
}
