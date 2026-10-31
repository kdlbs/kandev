package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

type e2eUsageWorkspaceAuthorizer interface {
	AuthorizeWorkspaceAccess(context.Context, string) error
}

type e2eUsageRebuilder interface {
	StartUsageHistoryRebuild(context.Context, string) (string, error)
}

// These controls drive the real durable projector. They never supply report
// values, mint collector bindings, or bypass the request's workspace policy.
func registerE2EUsageHistoryRoutes(router *gin.Engine, authorizer e2eUsageWorkspaceAuthorizer, repo any, pool *db.Pool, control *usageHistoryProjectionControl) {
	mock := os.Getenv("KANDEV_MOCK_AGENT")
	if os.Getenv("KANDEV_E2E_MOCK") != "true" || (mock != "true" && mock != "only") || control == nil {
		return
	}
	api := router.Group("/api/v1/e2e/usage-history/:workspaceId")
	api.Use(func(c *gin.Context) {
		if authorizer == nil || authorizer.AuthorizeWorkspaceAccess(c.Request.Context(), c.Param("workspaceId")) != nil {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errKey: "workspace not found"})
		}
	})
	api.POST("/pause", e2eUsageProjectionAction(control.pause))
	api.POST("/resume", e2eUsageProjectionAction(control.resume))
	if rebuilder, ok := repo.(e2eUsageRebuilder); ok && pool != nil {
		api.POST("/rebuild", e2eUsageHistoryRebuild(pool, rebuilder))
	}
	if pool != nil && pool.Writer().DriverName() == "sqlite3" {
		hold := &e2eUsageReadHold{pool: pool, ctx: control.ctx}
		control.mu.Lock()
		control.beforeStop = hold.close
		control.mu.Unlock()
		api.POST("/hold-reader", e2eUsageProjectionAction(hold.acquire))
		api.POST("/release-reader", e2eUsageProjectionAction(hold.release))
	}
}

// Occupy the real SQLite writer connection so the report's existing queue and
// query deadline returns analytics_busy. No handler response is substituted.
type e2eUsageReadHold struct {
	mu         sync.Mutex
	pool       *db.Pool
	ctx        context.Context
	connection *sqlx.Conn
	closed     bool
}

func (h *e2eUsageReadHold) acquire() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return context.Canceled
	}
	if h.connection != nil {
		return nil
	}
	if h.pool.Writer().Stats().MaxOpenConnections != 1 {
		return errors.New("usage reader hold requires the single SQLite writer")
	}
	ctx, cancel := context.WithTimeout(h.ctx, 10*time.Second)
	defer cancel()
	connection, err := h.pool.Writer().Connx(ctx)
	if err == nil {
		h.connection = connection
	}
	return err
}

func (h *e2eUsageReadHold) release() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.releaseLocked()
}

func (h *e2eUsageReadHold) close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	return h.releaseLocked()
}

func (h *e2eUsageReadHold) releaseLocked() error {
	if h.connection == nil {
		return nil
	}
	err := h.connection.Close()
	h.connection = nil
	return err
}

func e2eUsageProjectionAction(action func() error) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := action(); err != nil {
			c.JSON(http.StatusConflict, gin.H{errKey: "usage projector is closed"})
			return
		}
		c.JSON(http.StatusOK, gin.H{statusKey: "ok"})
	}
}

func e2eUsageHistoryRebuild(pool *db.Pool, repo e2eUsageRebuilder) gin.HandlerFunc {
	return func(c *gin.Context) {
		var scope string
		reader := pool.Reader()
		err := reader.GetContext(c.Request.Context(), &scope, reader.Rebind(`SELECT s.scope_id FROM usage_scopes s
			JOIN workspaces w ON w.id=s.workspace_id AND COALESCE(w.org_id,'')=s.organization_id WHERE w.id=?`), c.Param("workspaceId"))
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{errKey: "retained scope not found"})
			return
		}
		if err == nil {
			_, err = repo.StartUsageHistoryRebuild(c.Request.Context(), scope)
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage rebuild failed"})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{statusKey: "pending"})
	}
}
