package backendapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth"
	authhttpmw "github.com/kandev/kandev/internal/auth/httpmw"
	authstore "github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func TestRequiredPersistenceMiddlewareBlocksStatefulTrafficButAllowsDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "task", OwnerPackage: "internal/task", RequiredTables: []string{"tasks"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("task"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := tracker.RecordProbe("task", errTestPersistence); err != nil {
		t.Fatalf("RecordProbe: %v", err)
	}

	router := gin.New()
	router.Use(requiredPersistenceMiddleware(requiredstores.NewHealth(tracker, nil, nil)))
	router.GET("/api/v1/tasks", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/api/v1/system/diagnostics/persistence", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/ready", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/ws", func(c *gin.Context) { c.Status(http.StatusOK) })
	router.GET("/mcp", func(c *gin.Context) { c.Status(http.StatusOK) })

	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil))
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("blocked status = %d, want %d", blocked.Code, http.StatusServiceUnavailable)
	}
	if blocked.Header().Get("Content-Type") == "" {
		t.Fatal("blocked response has no content type")
	}

	for _, path := range []string{"/ws", "/mcp"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusServiceUnavailable)
		}
	}

	for _, path := range []string{
		"/api/v1/system/diagnostics/persistence", "/health", "/ready",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}
}

func TestRequiredPersistenceMiddlewareRunsBeforeAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "task", OwnerPackage: "internal/task", RequiredTables: []string{"tasks"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("task"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := tracker.RecordProbe("task", errTestPersistence); err != nil {
		t.Fatalf("RecordProbe: %v", err)
	}

	authResolved := false
	router := gin.New()
	// This middleware stands in for auth-enabled token resolution. If the
	// persistence gate is registered after it, a failed DB lookup becomes 401.
	router.Use(requiredPersistenceMiddleware(requiredstores.NewHealth(tracker, nil, nil)))
	router.Use(func(c *gin.Context) {
		authResolved = true
		c.AbortWithStatus(http.StatusUnauthorized)
	})
	router.GET("/api/v1/tasks", func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if authResolved {
		t.Fatal("authentication middleware ran before the persistence gate")
	}
}

func TestRequiredPersistenceMiddlewarePrecedesDatabaseBackedAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() {
		_ = cleanup()
		_ = conn.Close()
	})
	store, err := authstore.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = true
	svc, err := auth.NewService(context.Background(), auth.Deps{Cfg: cfg, Store: store, Users: users})
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	_, token, err := svc.Setup(context.Background(), "admin@example.test", "adminpass123", "Admin", "", "")
	if err != nil {
		t.Fatalf("setup auth session: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close database: %v", err)
	}

	tracker, err := requiredstores.NewTracker([]requiredstores.Descriptor{{
		ID: "task", OwnerPackage: "internal/task", RequiredTables: []string{"tasks"},
	}})
	if err != nil {
		t.Fatalf("NewTracker: %v", err)
	}
	if err := tracker.RecordSuccess("task"); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}
	if err := tracker.RecordProbe("task", errTestPersistence); err != nil {
		t.Fatalf("RecordProbe: %v", err)
	}

	router := gin.New()
	router.Use(requiredPersistenceMiddleware(requiredstores.NewHealth(tracker, nil, nil)))
	router.Use(authhttpmw.Middleware(svc))
	router.GET("/api/v1/tasks", func(c *gin.Context) { c.Status(http.StatusOK) })
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	req.AddCookie(&http.Cookie{Name: svc.CookieName(), Value: token})
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (not authentication failure)", rec.Code, http.StatusServiceUnavailable)
	}
}
