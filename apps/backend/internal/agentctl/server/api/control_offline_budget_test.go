package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/pkg/agent"
)

func postCreateInstance(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	mgr := instance.NewManager(&config.Config{Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}, logger.Default())
	t.Cleanup(func() { _ = mgr.Shutdown(t.Context()) })
	server := NewControlServer(&config.Config{}, mgr, logger.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/v1/instances", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.Router().ServeHTTP(resp, req)
	return resp
}

func TestCreateInstanceRejectsInvalidOfflineBudgetNamingFieldAndRange(t *testing.T) {
	for _, value := range []string{"0", "-1", "1441", "1.5", "1e2", `"15"`, "true", "99999999999999999999"} {
		resp := postCreateInstance(t, `{"workspace_path":"/tmp","offline_budget_minutes":`+value+`}`)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("offline_budget_minutes=%s status = %d, want 400", value, resp.Code)
			continue
		}
		body := resp.Body.String()
		if !strings.Contains(body, "offline_budget_minutes") || !strings.Contains(body, "1440") {
			t.Errorf("offline_budget_minutes=%s body %q does not name the field and range", value, body)
		}
	}
}

func TestCreateInstanceAcceptsAbsentAndNullOfflineBudget(t *testing.T) {
	for _, extra := range []string{``, `,"offline_budget_minutes":null`, `,"offline_budget_minutes":20`} {
		resp := postCreateInstance(t, `{"workspace_path":"/nonexistent-offline-budget-path"`+extra+`}`)
		if resp.Code == http.StatusBadRequest && strings.Contains(resp.Body.String(), "offline_budget_minutes") {
			t.Errorf("valid budget %q rejected: %s", extra, resp.Body.String())
		}
	}
}
