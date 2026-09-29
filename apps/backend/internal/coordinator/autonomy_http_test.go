package coordinator

import (
	"net/http"
	"strings"
	"testing"

	svcpkg "github.com/kandev/kandev/internal/task/service"

	"github.com/gin-gonic/gin"
)

func phase3Handlers(t *testing.T, phase3 bool) (*Handlers, *phase3Env) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	env := newPhase3Env(t, phase3)
	return NewHandlers(env.svc, newTestLogger(t)), env
}

func TestHTTPPatchAutonomyRoundTripAndReads(t *testing.T) {
	h, env := phase3Handlers(t, true)
	params := workspaceParams(env.c.ID)
	rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/x", `{"autonomy_enabled":true,"cost_ceiling_usd":"5.50"}`, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status = %d body=%s", rec.Code, rec.Body.String())
	}
	var dto CoordinatorDTO
	decodeBody(t, rec, &dto)
	if dto.CoordinatorPhase3 == nil || !dto.AutonomyEnabled || dto.CostCeilingUSD == nil || *dto.CostCeilingUSD != "5.50" {
		t.Fatalf("patch dto = %+v", dto)
	}

	get := runHandler(h.httpGetCoordinator, http.MethodGet, "/x", "", params)
	list := runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams(""))
	for name, body := range map[string]string{"get": get.Body.String(), "list": list.Body.String()} {
		if !strings.Contains(body, `"autonomy_enabled":true`) || !strings.Contains(body, `"cost_ceiling_usd":"5.50"`) {
			t.Errorf("%s body lacks phase 3 keys: %s", name, body)
		}
	}
}

func TestHTTPCoordinatorReadsOmitPhase3KeysWhenNotEffective(t *testing.T) {
	h, env := phase3Handlers(t, false)
	params := workspaceParams(env.c.ID)
	for name, rec := range map[string]interface{ String() string }{
		"get":  runHandler(h.httpGetCoordinator, http.MethodGet, "/x", "", params).Body,
		"list": runHandler(h.httpListCoordinators, http.MethodGet, "/x", "", workspaceParams("")).Body,
	} {
		if strings.Contains(rec.String(), "autonomy_enabled") || strings.Contains(rec.String(), "cost_ceiling_usd") {
			t.Errorf("%s exposes phase 3 keys while not effective: %s", name, rec.String())
		}
	}
}

func TestHTTPPatchAutonomyMalformedBodyIs400BeforeScopeCheck(t *testing.T) {
	h, env := phase3Handlers(t, true)
	env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/x", `{"autonomy_enabled":`, workspaceParams(env.c.ID))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPPatchAutonomyByReaderIs403AndChangesNothing(t *testing.T) {
	h, env := phase3Handlers(t, true)
	env.svc.authz = &fakeWorkspaceAuthorizer{err: svcpkg.ErrForbidden}
	rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/x", `{"autonomy_enabled":true,"cost_ceiling_usd":"5"}`, workspaceParams(env.c.ID))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 body=%s", rec.Code, rec.Body.String())
	}
	if env.reload(t).AutonomyEnabled {
		t.Fatal("autonomy changed despite 403")
	}
}

func TestHTTPPatchAutonomyInterlockNamesTheCeilingField(t *testing.T) {
	h, env := phase3Handlers(t, true)
	rec := runHandler(h.httpPatchCoordinator, http.MethodPatch, "/x", `{"autonomy_enabled":true}`, workspaceParams(env.c.ID))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "cost_ceiling") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
