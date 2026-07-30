package logbundle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
)

func TestBundleRoutesEnforceIdentityOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newTestService(t, Config{HomeDir: t.TempDir(), CaptureWindow: time.Hour})
	router := gin.New()
	group := router.Group("/api/v1/system")
	group.Use(func(c *gin.Context) {
		userID := c.GetHeader("X-Test-User")
		if userID != "" {
			authn.SetOnGin(c, authn.Identity{UserID: userID, Role: authn.RoleMember})
		}
		c.Next()
	})
	RegisterRoutes(group, service)

	create := requestBundle(t, router, http.MethodPost, "/api/v1/system/logs/bundles",
		"user-1", `{"sources":["frontend"]}`)
	if create.Code != http.StatusAccepted {
		t.Fatalf("create status = %d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	foreign := requestBundle(t, router, http.MethodGet,
		"/api/v1/system/logs/bundles/"+created.ID, "user-2", "")
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("foreign status = %d body=%s", foreign.Code, foreign.Body.String())
	}
}

func TestLosingFrontendStreamIsAcknowledgedBeforeEntriesDecode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newTestService(t, Config{HomeDir: t.TempDir(), CaptureWindow: time.Hour})
	job, _, err := service.Create("user-1", []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.ClaimStream("user-1", job.ID, "browser-a", "winner", "memory"); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api/v1/system")
	group.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "user-1", Role: authn.RoleMember})
		c.Next()
	})
	RegisterRoutes(group, service)

	body := `{"browser_id":"browser-a","capture_stream_id":"loser","chunk_index":99,` +
		`"done":false,"storage_mode":"memory","capture_metadata":null,"entries":{"not":"an array"}}`
	response := requestBundle(t, router, http.MethodPost,
		"/api/v1/system/logs/bundles/"+job.ID+"/frontend", "", body)
	if response.Code != http.StatusNoContent {
		t.Fatalf("losing stream status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestFrontendUploadBodyIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := newTestService(t, Config{HomeDir: t.TempDir(), CaptureWindow: time.Hour})
	job, _, err := service.Create("user-1", []string{"frontend"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	group := router.Group("/api/v1/system")
	group.Use(func(c *gin.Context) {
		authn.SetOnGin(c, authn.Identity{UserID: "user-1"})
		c.Next()
	})
	RegisterRoutes(group, service)

	body := `{"browser_id":"browser-a","capture_stream_id":"stream-a","chunk_index":0,` +
		`"done":false,"entries":["` + strings.Repeat("x", 1024*1024) + `"]}`
	response := requestBundle(t, router, http.MethodPost,
		"/api/v1/system/logs/bundles/"+job.ID+"/frontend", "", body)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d body=%s", response.Code, response.Body.String())
	}
}

func requestBundle(
	t *testing.T, router http.Handler, method, path, userID, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		request.Header.Set("X-Test-User", userID)
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
