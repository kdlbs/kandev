package dataretention

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/kandev/kandev/internal/db"
)

func TestMain(m *testing.M) { goleak.VerifyTestMain(m) }

func testService(t *testing.T) *Service {
	t.Helper()
	d, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	d.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, d.Close()) })
	_, err = d.Exec(`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL DEFAULT '')`)
	require.NoError(t, err)
	return New(db.NewPool(d, d))
}

func TestWorkerStopsAndRestarts(t *testing.T) {
	s := testService(t)
	for i := 0; i < 2; i++ {
		s.Start(context.Background())
		s.Start(context.Background())
		s.mu.Lock()
		running := s.cancel != nil
		s.mu.Unlock()
		require.True(t, running)
		s.Stop()
		s.Stop()
	}
	status, err := s.Get(context.Background())
	require.NoError(t, err)
	require.Nil(t, status.Operation)
}

// Retry exhaustion is recorded only against the operation that failed, and a
// backup verification failure keeps its own code.
func TestFailActiveFinishesOnlyTheFailedOperation(t *testing.T) {
	s := testService(t)
	ctx := context.Background()
	_, err := s.change(ctx, func(r *record, _ *sqlx.Tx) error {
		r.Operation = &Operation{ID: "new", Kind: kindCleanup, State: stateRunning, StartedAt: time.Now()}
		return nil
	})
	require.NoError(t, err)

	s.failActive(ctx, "old", errors.New("boom"))
	status, err := s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, stateRunning, status.Operation.State)

	s.failActive(ctx, "new", errBackupVerificationFailed)
	status, err = s.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, stateFailed, status.Operation.State)
	require.Equal(t, "backup_verification_failed", status.Operation.Error)
}

func TestLimitPreparationDetailStaysValidUTF8(t *testing.T) {
	long := strings.Repeat("a", preparationDetailLimit-1) + "éé"
	got := limitPreparationDetail(long)
	require.LessOrEqual(t, len(got), preparationDetailLimit)
	require.True(t, utf8.ValidString(got))
	require.Equal(t, strings.Repeat("a", preparationDetailLimit-1), got)

	invalid := limitPreparationDetail("bad \xff byte")
	require.True(t, utf8.ValidString(invalid))
	require.Equal(t, "bad � byte", invalid)
}
