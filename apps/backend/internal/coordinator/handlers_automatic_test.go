package coordinator

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func automaticRouter(t *testing.T, svc *Service) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterRoutes(router, svc, newTestLogger(t))
	return router
}

func sendJSON(router *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func classPath(c *Coordinator, class, tail string) string {
	return "/api/v1/workspaces/" + c.WorkspaceID + "/coordinators/" + c.ID + "/classes/" + class + "/" + tail
}

func TestAutomaticRoutes_EligibilityWireShape(t *testing.T) {
	_, c, _, svc, _ := automaticFixture(t)
	router := automaticRouter(t, svc)

	rec := sendJSON(router, http.MethodGet, classPath(c, "create_task", "eligibility"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("eligibility = %d %s", rec.Code, rec.Body.String())
	}
	var dto struct {
		Eligible   bool `json:"eligible"`
		Conditions []struct {
			Name string `json:"name"`
			Met  bool   `json:"met"`
		} `json:"conditions"`
		Setting string `json:"setting"`
	}
	decodeBody(t, rec, &dto)
	if !dto.Eligible || len(dto.Conditions) != 5 || dto.Conditions[0].Name != conditionHistory30d || dto.Setting != string(SettingRequiresApproval) {
		t.Fatalf("dto = %+v", dto)
	}
	rec = sendJSON(router, http.MethodGet, classPath(c, "merge", "eligibility"), "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"class"`) {
		t.Fatalf("other class = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAutomaticRoutes_ReviewIgnoresBodyAndAnswers503OnLogError(t *testing.T) {
	store, c, _, svc, log := automaticFixture(t)
	router := automaticRouter(t, svc)
	body := `{"reviewed_by":"someone-else","row_count":999,"window_start":"2000-01-01T00:00:00Z","window_end":"2000-02-01T00:00:00Z"}`

	rec := sendJSON(router, http.MethodPost, classPath(c, "create_task", "reviews"), body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("review = %d %s", rec.Code, rec.Body.String())
	}
	stored, err := store.NewestClassReview(t.Context(), c.ID, ActionCreateTask)
	if err != nil || stored.RowCount != 20 || !stored.WindowEnd.Equal(automaticNow) || stored.ReviewedBy == "someone-else" {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}

	before := countReviews(t, store, c)
	log.err = errors.New("boom")
	rec = sendJSON(router, http.MethodPost, classPath(c, "create_task", "reviews"), "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), decisionLogField) || countReviews(t, store, c) != before {
		t.Fatalf("log error = %d %s", rec.Code, rec.Body.String())
	}
	rec = sendJSON(router, http.MethodPost, classPath(c, "move", "reviews"), "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("other class = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAutomaticRoutes_RaiseRefusalIs409NamingCondition(t *testing.T) {
	_, c, _, svc, log := automaticFixture(t)
	log.rows = decisions(19, 19)
	router := automaticRouter(t, svc)
	rec := sendJSON(router, http.MethodPut, "/api/v1/workspaces/"+c.WorkspaceID+"/coordinators/"+c.ID+"/settings", raiseBody())
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), conditionVolume) {
		t.Fatalf("raise = %d %s", rec.Code, rec.Body.String())
	}
	rec = sendJSON(router, http.MethodPut, "/api/v1/workspaces/"+c.WorkspaceID+"/coordinators/"+c.ID+"/settings", policyBody(map[string]string{"move": "automatic"}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "policy.actions.move") {
		t.Fatalf("other class = %d %s", rec.Code, rec.Body.String())
	}
}

func TestAutomaticRoutes_AbsentWithoutPhase3(t *testing.T) {
	_, c, _, svc, _ := automaticFixture(t)
	svc.phase3 = false
	router := automaticRouter(t, svc)
	if rec := sendJSON(router, http.MethodGet, classPath(c, "create_task", "eligibility"), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("eligibility = %d", rec.Code)
	}
	if rec := sendJSON(router, http.MethodPost, classPath(c, "create_task", "reviews"), ""); rec.Code != http.StatusNotFound {
		t.Fatalf("reviews = %d", rec.Code)
	}
}
