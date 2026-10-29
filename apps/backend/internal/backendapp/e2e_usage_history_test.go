package backendapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/synctest"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/stretchr/testify/require"
)

type restartableUsageProjectionStore struct{ runs chan chan struct{} }

func (s *restartableUsageProjectionStore) RunUsageHistoryProjectionBatch(ctx context.Context, _ int) (bool, error) {
	joined := make(chan struct{})
	s.runs <- joined
	defer close(joined)
	<-ctx.Done()
	return false, ctx.Err()
}

func TestUsageHistoryReadHoldClosesWithItsOwner(t *testing.T) {
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "hold.db"))
	require.NoError(t, err)
	writer := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	writer.SetMaxOpenConns(1)
	hold := &e2eUsageReadHold{ctx: t.Context(), pool: db.NewPool(writer, writer)}
	t.Cleanup(func() { require.NoError(t, hold.close()) })
	require.NoError(t, hold.acquire())
	require.Equal(t, 1, writer.Stats().InUse, "the actual writer connection is occupied")
	require.NoError(t, hold.release())
	require.Zero(t, writer.Stats().InUse)
	require.NoError(t, hold.acquire())
	require.NoError(t, hold.close())
	require.Zero(t, writer.Stats().InUse)
	require.ErrorIs(t, hold.acquire(), context.Canceled, "restore cannot leave a restartable fault after database closure")
}

func TestUsageHistoryProjectionPauseResumeAndRestore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		store := &restartableUsageProjectionStore{runs: make(chan chan struct{}, 2)}
		control := newUsageHistoryProjection(t.Context(), store, nil)
		t.Cleanup(func() { require.NoError(t, control.stop()) })
		first := <-store.runs
		require.NoError(t, control.pause())
		select {
		case <-first:
		default:
			t.Fatal("pause returned before the projection writer joined")
		}
		require.NoError(t, control.resume())
		second := <-store.runs
		require.NoError(t, control.stop())
		select {
		case <-second:
		default:
			t.Fatal("restore returned before the resumed projection writer joined")
		}
		require.ErrorIs(t, control.resume(), context.Canceled)
		select {
		case <-store.runs:
			t.Fatal("a closed database must not acquire another projection writer")
		default:
		}
	})
}

type e2eUsageAccess struct{ err error }

func (a e2eUsageAccess) AuthorizeWorkspaceAccess(context.Context, string) error { return a.err }

func TestUsageHistoryControlsRequireBothE2EFlagsAndWorkspaceAccess(t *testing.T) {
	for _, tc := range []struct {
		name, mock, e2e string
		access          error
		status          int
	}{
		{"production", "", "", nil, http.StatusNotFound},
		{"mock-agent-alone", "only", "", nil, http.StatusNotFound},
		{"e2e-alone", "", "true", nil, http.StatusNotFound},
		{"foreign-workspace", "only", "true", errors.New("denied"), http.StatusNotFound},
		{"authorized-closed-projector", "only", "true", nil, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KANDEV_MOCK_AGENT", tc.mock)
			t.Setenv("KANDEV_E2E_MOCK", tc.e2e)
			router := gin.New()
			control := &usageHistoryProjectionControl{closed: true}
			registerE2EUsageHistoryRoutes(router, e2eUsageAccess{err: tc.access}, nil, nil, control)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/e2e/usage-history/ws/resume", nil))
			require.Equal(t, tc.status, response.Code, response.Body.String())
		})
	}
}
