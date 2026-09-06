package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
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

	blocked := httptest.NewRecorder()
	router.ServeHTTP(blocked, httptest.NewRequest(http.MethodPost, "/api/v1/tasks", nil))
	if blocked.Code != http.StatusServiceUnavailable {
		t.Fatalf("blocked status = %d, want %d", blocked.Code, http.StatusServiceUnavailable)
	}
	if blocked.Header().Get("Content-Type") == "" {
		t.Fatal("blocked response has no content type")
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
