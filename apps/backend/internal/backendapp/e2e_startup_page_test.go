package backendapp

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

func newE2EStartupPageFixtureRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger.NewFromZap: %v", err)
	}
	router := gin.New()
	registerE2EStartupPageFixtureRoute(router, log)
	return router
}

func TestE2EStartupPageFixtureRendersRealTemplate(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	router := newE2EStartupPageFixtureRouter(t)

	body := strings.NewReader(`{
		"phase": "applying_migrations",
		"boot": 1,
		"seq": 1,
		"elapsed_ms": 12000,
		"phase_elapsed_ms": 12000,
		"step": {
			"id": "stores.repositories",
			"label_key": "startup.step.stores_repositories",
			"measure": "counted",
			"unit": "stores",
			"elapsed_ms": 12000,
			"done": 400,
			"total": 1000,
			"eta_ms": 30000,
			"stalled": false
		}
	}`)
	req := httptest.NewRequest("POST", "/api/v1/e2e/startup-page-fixture", body)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != 503 {
		t.Fatalf("status = %d, want 503", resp.Code)
	}
	html := resp.Body.String()
	if !strings.Contains(html, `id="startup-data"`) {
		t.Fatalf("response does not contain the embedded poll script's data island: %s", html)
	}
	if !strings.Contains(html, "400 of 1000") {
		t.Fatalf("response does not render the step's counted progress: %s", html)
	}
}

func TestE2EStartupPageFixtureRejectsUnparseableBody(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	router := newE2EStartupPageFixtureRouter(t)

	req := httptest.NewRequest("POST", "/api/v1/e2e/startup-page-fixture", strings.NewReader(`not json`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != 400 {
		t.Fatalf("status = %d, want 400", resp.Code)
	}
}

func TestE2EStartupPageFixtureRouteHiddenOutsideMockAgent(t *testing.T) {
	t.Setenv("KANDEV_MOCK_AGENT", "")
	router := newE2EStartupPageFixtureRouter(t)

	req := httptest.NewRequest("POST", "/api/v1/e2e/startup-page-fixture", strings.NewReader(`{}`))
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != 404 {
		t.Fatalf("status = %d, want 404 when KANDEV_MOCK_AGENT is unset", resp.Code)
	}
}
