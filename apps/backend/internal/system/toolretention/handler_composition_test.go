package toolretention_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	systemsvc "github.com/kandev/kandev/internal/system"
	"github.com/kandev/kandev/internal/system/jobs"
	"github.com/kandev/kandev/internal/system/maintenance"
	"github.com/kandev/kandev/internal/system/toolretention"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSystemCompositionRegistersRetentionAuthorization(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "test.db")
	writer, err := db.OpenSQLite(path)
	require.NoError(t, err)
	pool := db.NewPool(sqlx.NewDb(writer, "sqlite3"), sqlx.NewDb(writer, "sqlite3"))
	t.Cleanup(func() { _ = writer.Close() })
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	service := systemsvc.Provide(&config.Config{HomeDir: root, Database: config.DatabaseConfig{Driver: "sqlite", Path: path}}, log, pool, nil, systemsvc.BuildInfo{}, systemsvc.Wiring{})
	router := gin.New()
	router.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{Role: authn.RoleMember, UserID: "member"})
		c.Next()
	})
	service.RegisterRoutes(router, log)
	for _, tc := range []struct {
		method, suffix string
		status         int
	}{{"GET", "", 200}, {"PUT", "", 403}, {"POST", "/analyze", 403}, {"POST", "/run", 403}, {"POST", "/cancel", 403}} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, "/api/v1/system/database/tool-payload-retention"+tc.suffix, strings.NewReader(`{}`))
		router.ServeHTTP(w, req)
		require.Equal(t, tc.status, w.Code, w.Body.String())
	}
	var n int
	require.NoError(t, pool.Reader().Get(&n, `SELECT count(*) FROM settings WHERE key='tool_payload_retention'`))
	require.Zero(t, n)
}

func TestSystemQuiescenceCancelsBackupAdmission(t *testing.T) {
	for _, kind := range []string{"reset", "restore"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "test.db")
			writer, err := db.OpenSQLite(path)
			require.NoError(t, err)
			t.Cleanup(func() { _ = writer.Close() })
			pool := db.NewPool(sqlx.NewDb(writer, "sqlite3"), sqlx.NewDb(writer, "sqlite3"))
			log, err := logger.NewFromZap(zap.NewNop())
			require.NoError(t, err)
			eventBus := bus.NewMemoryEventBus(log)
			t.Cleanup(eventBus.Close)
			reports := make(chan *jobs.Job, 4)
			_, err = eventBus.Subscribe(events.SystemJobUpdate, func(_ context.Context, e *bus.Event) error {
				if job, ok := e.Data.(*jobs.Job); ok && job.Kind == "tool-payload-retention-backup" {
					select {
					case reports <- job:
					default:
					}
				}
				return nil
			})
			require.NoError(t, err)
			quiesced := false
			next := func() error { quiesced = true; return nil }
			service := systemsvc.Provide(&config.Config{HomeDir: root, Database: config.DatabaseConfig{Driver: "sqlite", Path: path}}, log, pool, eventBus, systemsvc.BuildInfo{}, systemsvc.Wiring{DatabaseQuiesce: next, RestoreQuiesce: next})
			release, ok := maintenance.ForPool(pool).TryAcquire()
			require.True(t, ok)
			defer release()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			service.ToolRetention.Start(ctx)
			defer service.ToolRetention.Stop()
			_, err = service.ToolRetention.Save(ctx, toolretention.Update{Enabled: true, Age: toolretention.Age{Value: 3, Unit: "months"}, BackupChoice: "backup"})
			require.NoError(t, err)
			select {
			case job := <-reports:
				require.Equal(t, jobs.StateRunning, job.State)
				require.NotContains(t, job.Result, "receipt")
				require.Empty(t, service.Jobs.List())
			case <-ctx.Done():
				t.Fatal("preparation did not publish progress")
			}
			done := make(chan error, 1)
			go func() {
				if kind == "reset" {
					done <- service.Database.DatabaseQuiesce()
				} else {
					done <- service.Backups.RestoreQuiesce()
				}
			}()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-ctx.Done():
				t.Fatal("quiescence deadlocked behind maintenance admission")
			}
			require.True(t, quiesced)
			snapshots, err := service.Backups.List()
			require.NoError(t, err)
			require.Empty(t, snapshots)
		})
	}
}
