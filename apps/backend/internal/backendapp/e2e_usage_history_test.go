package backendapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

type e2eUsageAdoptionRecorder struct {
	db             *sqlx.DB
	contextID      string
	called         bool
	rewindObserved bool
	calls          int
}

func (r *e2eUsageAdoptionRecorder) BackfillUsageSourceAdoption(ctx context.Context, _ int) (bool, error) {
	r.called = true
	r.calls++
	var trusted int
	var evidence, lastKey string
	var complete int
	err := r.db.QueryRowxContext(ctx, `SELECT trusted, evidence FROM usage_collection_contexts WHERE context_id = ?`, r.contextID).Scan(&trusted, &evidence)
	if err != nil {
		return false, err
	}
	err = r.db.QueryRowxContext(ctx, `SELECT last_key, complete FROM usage_history_migration_state WHERE source_key = 'source-adoption'`).Scan(&lastKey, &complete)
	if err != nil {
		return false, err
	}
	r.rewindObserved = trusted == 0 && evidence == "" && lastKey == "" && complete == 0
	if r.rewindObserved {
		_, err = r.db.ExecContext(ctx, `UPDATE usage_collection_contexts SET trusted = 1, evidence = 'migration' WHERE context_id = ?`, r.contextID)
		if err != nil {
			return false, err
		}
		_, err = r.db.ExecContext(ctx, `UPDATE usage_collection_bindings SET external_session_id = 'transcript', source_scope_status = 'eligible' WHERE session_id = 'session'`)
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func TestLegacyUsageSessionFixtureRewindsSourceAdoption(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	t.Setenv("KANDEV_E2E_MOCK", "true")

	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "legacy.db"))
	require.NoError(t, err)
	writer := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, writer.Close()) })
	_, err = writer.Exec(`CREATE TABLE usage_collection_bindings (
		sequence INTEGER, workspace_id TEXT, task_id TEXT, task_incarnation_id TEXT,
		session_id TEXT, session_incarnation_id TEXT, external_session_id TEXT, source_scope_status TEXT
	)`)
	require.NoError(t, err)
	_, err = writer.Exec(`CREATE TABLE usage_collection_contexts (
		context_id TEXT PRIMARY KEY, workspace_id TEXT, trusted INTEGER, evidence TEXT
	)`)
	require.NoError(t, err)
	_, err = writer.Exec(`CREATE TABLE usage_history_migration_state (
		source_key TEXT PRIMARY KEY, high_water TEXT, last_key TEXT, complete INTEGER, updated_at TEXT
	)`)
	require.NoError(t, err)
	_, err = writer.Exec(`INSERT INTO usage_collection_bindings VALUES (1, 'ws', 'task', 'task-inc', 'session', 'session-inc', '', 'unknown')`)
	require.NoError(t, err)
	contextID := uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-source-context:ws:task:task-inc:session:session-inc")).String()
	_, err = writer.Exec(`INSERT INTO usage_collection_contexts VALUES (?, 'ws', 1, 'creation')`, contextID)
	require.NoError(t, err)
	_, err = writer.Exec(`INSERT INTO usage_history_migration_state VALUES ('source-adoption', '', 'before-session', 1, CURRENT_TIMESTAMP)`)
	require.NoError(t, err)
	pool := db.NewPool(writer, writer)
	recorder := &e2eUsageAdoptionRecorder{db: writer, contextID: contextID}
	router := gin.New()
	registerE2EUsageHistoryRoutes(router, e2eUsageAccess{}, recorder, pool, &usageHistoryProjectionControl{})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/e2e/usage-history/ws/adopt-legacy-session", strings.NewReader(`{"session_id":"session"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.True(t, recorder.called, "the route must run the real source-adoption worker")
	require.Equal(t, 1, recorder.calls, "the route must rewind and run source adoption")
	require.True(t, recorder.rewindObserved, fmt.Sprintf("legacy fixture was not prepared before adoption: %s", response.Body.String()))

	unsafeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/e2e/usage-history/ws/mark-legacy-session-unsafe", strings.NewReader(`{"session_id":"session"}`))
	unsafeRequest.Header.Set("Content-Type", "application/json")
	unsafeResponse := httptest.NewRecorder()
	router.ServeHTTP(unsafeResponse, unsafeRequest)
	require.Equal(t, http.StatusOK, unsafeResponse.Code, unsafeResponse.Body.String())
	var trusted int
	var evidence string
	require.NoError(t, writer.QueryRowx(`SELECT trusted, evidence FROM usage_collection_contexts WHERE context_id = ?`, contextID).Scan(&trusted, &evidence))
	require.Zero(t, trusted)
	require.Empty(t, evidence)
}

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
