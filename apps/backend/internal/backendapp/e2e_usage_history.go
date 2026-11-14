package backendapp

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
)

type e2eUsageWorkspaceAuthorizer interface {
	AuthorizeWorkspaceAccess(context.Context, string) error
}

type e2eUsageRebuilder interface {
	StartUsageHistoryRebuild(context.Context, string) (string, error)
}

type e2eUsageSourceAdoptionRunner interface {
	BackfillUsageSourceAdoption(context.Context, int) (bool, error)
}

const (
	e2eUsageHistoryResumePath        = "/resume"
	e2eUsageHistoryReleaseReaderPath = "/release-reader"
)

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
	api.POST(e2eUsageHistoryResumePath, e2eUsageProjectionAction(control.resume))
	if rebuilder, ok := repo.(e2eUsageRebuilder); ok && pool != nil {
		api.POST("/rebuild", e2eUsageHistoryRebuild(pool, rebuilder))
	}
	if pool != nil && pool.Writer().DriverName() == "sqlite3" {
		hold := &e2eUsageReadHold{pool: pool, ctx: control.ctx}
		control.mu.Lock()
		control.beforeStop = hold.close
		control.mu.Unlock()
		api.POST("/hold-reader", e2eUsageProjectionAction(hold.acquire))
		api.POST(e2eUsageHistoryReleaseReaderPath, e2eUsageProjectionAction(hold.release))
		if adopter, ok := repo.(e2eUsageSourceAdoptionRunner); ok {
			api.POST("/adopt-legacy-session", e2eUsageLegacySourceAdoption(pool, adopter))
		}
		api.POST("/mark-legacy-session-unsafe", e2eUsageMarkLegacySourceUnsafe(pool))
	}
}

type e2eLegacyUsageSessionRequest struct {
	SessionID string `json:"session_id"`
}

type e2eLegacyUsageBinding struct {
	TaskID               string `db:"task_id"`
	TaskIncarnationID    string `db:"task_incarnation_id"`
	SessionIncarnationID string `db:"session_incarnation_id"`
}

func e2eLegacyUsageContextID(workspaceID string, binding e2eLegacyUsageBinding, sessionID string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("usage-source-context:"+workspaceID+":"+binding.TaskID+":"+binding.TaskIncarnationID+":"+sessionID+":"+binding.SessionIncarnationID)).String()
}

func e2eUsageLegacySourceAdoption(pool *db.Pool, adopter e2eUsageSourceAdoptionRunner) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request e2eLegacyUsageSessionRequest
		if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.SessionID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "session id is required"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		tx, err := pool.Writer().BeginTxx(ctx, nil)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		defer func() { _ = tx.Rollback() }()
		var binding e2eLegacyUsageBinding
		err = tx.GetContext(ctx, &binding, tx.Rebind(`SELECT task_id, task_incarnation_id, session_incarnation_id
			FROM usage_collection_bindings WHERE workspace_id = ? AND session_id = ?
			ORDER BY sequence DESC LIMIT 1`), c.Param("workspaceId"), request.SessionID)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{errKey: "eligible session not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		contextID := e2eLegacyUsageContextID(c.Param("workspaceId"), binding, request.SessionID)
		result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE usage_collection_contexts SET trusted = 0, evidence = ''
			WHERE context_id = ? AND workspace_id = ?`), contextID, c.Param("workspaceId"))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		rows, err := result.RowsAffected()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		if rows != 1 {
			c.JSON(http.StatusConflict, gin.H{errKey: "session has no creation evidence to reset"})
			return
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO usage_history_migration_state
			(source_key, high_water, last_key, complete, updated_at) VALUES ('source-adoption', '', '', 0, CURRENT_TIMESTAMP)
			ON CONFLICT(source_key) DO UPDATE SET last_key = '', complete = 0, updated_at = excluded.updated_at`)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}

		if err := e2eRunUsageSourceAdoption(ctx, adopter); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture did not complete"})
			return
		}
		var adoption struct {
			Trusted           int    `db:"trusted"`
			Evidence          string `db:"evidence"`
			SourceScopeStatus string `db:"source_scope_status"`
			ExternalSessionID string `db:"external_session_id"`
		}
		if err := pool.Reader().GetContext(ctx, &adoption, `SELECT c.trusted, c.evidence, b.source_scope_status, b.external_session_id
			FROM usage_collection_contexts c JOIN usage_collection_bindings b ON b.workspace_id = c.workspace_id
			WHERE c.context_id = ? AND b.session_id = ? AND b.task_id = ? AND b.task_incarnation_id = ? AND b.session_incarnation_id = ?
			ORDER BY b.sequence DESC LIMIT 1`, contextID, request.SessionID, binding.TaskID, binding.TaskIncarnationID, binding.SessionIncarnationID); err != nil ||
			adoption.Trusted != 1 || adoption.Evidence != "migration" {
			c.JSON(http.StatusConflict, gin.H{errKey: "usage adoption fixture did not adopt the session"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			statusKey: "adopted", "source_scope_status": adoption.SourceScopeStatus,
			"has_transcript_id": adoption.ExternalSessionID != "",
		})
	}
}

func e2eRunUsageSourceAdoption(ctx context.Context, adopter e2eUsageSourceAdoptionRunner) error {
	for range 100 {
		complete, err := adopter.BackfillUsageSourceAdoption(ctx, 200)
		if err != nil {
			return err
		}
		if complete {
			return nil
		}
	}
	return errors.New("usage source adoption batch limit exceeded")
}

func e2eUsageMarkLegacySourceUnsafe(pool *db.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request e2eLegacyUsageSessionRequest
		if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.SessionID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "session id is required"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
		defer cancel()
		var binding e2eLegacyUsageBinding
		err := pool.Reader().GetContext(ctx, &binding, pool.Reader().Rebind(`SELECT task_id, task_incarnation_id, session_incarnation_id
			FROM usage_collection_bindings WHERE workspace_id = ? AND session_id = ?
			ORDER BY sequence DESC LIMIT 1`), c.Param("workspaceId"), request.SessionID)
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{errKey: "eligible session not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		result, err := pool.Writer().ExecContext(ctx, pool.Writer().Rebind(`UPDATE usage_collection_contexts SET trusted = 0, evidence = ''
			WHERE context_id = ? AND workspace_id = ?`), e2eLegacyUsageContextID(c.Param("workspaceId"), binding, request.SessionID), c.Param("workspaceId"))
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		rows, err := result.RowsAffected()
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "usage adoption fixture unavailable"})
			return
		}
		if rows != 1 {
			c.JSON(http.StatusConflict, gin.H{errKey: "session source context not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{statusKey: "unsafe"})
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
