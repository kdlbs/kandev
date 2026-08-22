package automation

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// newExportTestRouter mounts only the export routes over svc, mirroring
// office/config's newTestRouter helper.
func newExportTestRouter(svc *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerHTTPRoutes(router, svc, logger.Default())
	return router
}

func doExportRequest(router *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// API surface: GET .../automations/export returns 200,
// Content-Type: application/yaml, body is the YAML document.

func TestExportDocumentHandler_Success_ReturnsYAMLBody(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync"})
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-1/automations/export")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/yaml" {
		t.Errorf("Content-Type = %q, want application/yaml", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected a non-empty body")
	}
}

// API surface: GET .../automations/export/zip returns 200,
// Content-Type: application/zip, Content-Disposition attachment.

func TestExportZipHandler_Success_ReturnsZipBodyWithDisposition(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	createExportTestAutomation(t, svc, &CreateAutomationRequest{WorkspaceID: "ws-1", Name: "Daily Sync"})
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-1/automations/export/zip")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	want := "attachment; filename=kandev-automations.zip"
	if cd := rec.Header().Get("Content-Disposition"); cd != want {
		t.Errorf("Content-Disposition = %q, want %q", cd, want)
	}
	if rec.Body.Len() == 0 {
		t.Error("expected a non-empty body")
	}
}

// AC-32: an existing empty workspace is 200 with an empty automations list,
// not an error status.

func TestExportDocumentHandler_EmptyWorkspace_ReturnsOK(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-empty"] = true
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-empty/automations/export")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

// AC-31/AC-35/AC-44: both a denied caller and a nonexistent workspace
// return 404 with no distinguishing text.

func TestExportDocumentHandler_WorkspaceNotFound_Returns404(t *testing.T) {
	svc, _ := exportServiceTestFixture(t)
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-missing/automations/export")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestExportDocumentHandler_AuthorizeDenied_Returns404(t *testing.T) {
	svc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		return errors.Join(errors.New("denied"), repoerrors.ErrWorkspaceNotFound)
	})
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-1/automations/export")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}

// AC-45: any other failure (here, an unwired workspace lookup) is 500.

func TestExportDocumentHandler_WorkspaceLookupUnavailable_Returns500(t *testing.T) {
	svc, _, _, _, _, _ := resolveTestFixture(t)
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-1/automations/export")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body: %s)", rec.Code, rec.Body.String())
	}
}

// AC-35: the two 404 paths above must be indistinguishable in body, not just
// status. Round 7 testing proved this was previously uncovered: mutating
// respondExportError to leak "workspace access denied: <err>" into the
// denied-caller body passed the full suite before this test existed.

func TestExportDocumentHandler_DeniedAndNotFound_ProduceIdenticalBody(t *testing.T) {
	notFoundSvc, _ := exportServiceTestFixture(t)
	notFoundRouter := newExportTestRouter(notFoundSvc)
	notFoundRec := doExportRequest(notFoundRouter, "/api/v1/workspaces/ws-missing/automations/export")

	deniedSvc, wsLookup := exportServiceTestFixture(t)
	wsLookup.exists["ws-1"] = true
	deniedSvc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		return errors.Join(errors.New("denied"), repoerrors.ErrWorkspaceNotFound)
	})
	deniedRouter := newExportTestRouter(deniedSvc)
	deniedRec := doExportRequest(deniedRouter, "/api/v1/workspaces/ws-1/automations/export")

	if notFoundRec.Code != http.StatusNotFound || deniedRec.Code != http.StatusNotFound {
		t.Fatalf("status codes = %d, %d, want both 404", notFoundRec.Code, deniedRec.Code)
	}
	if notFoundRec.Body.String() != deniedRec.Body.String() {
		t.Errorf("bodies differ: not-found=%q, denied=%q; AC-35 requires them indistinguishable", notFoundRec.Body.String(), deniedRec.Body.String())
	}
}

func TestExportZipHandler_WorkspaceNotFound_Returns404(t *testing.T) {
	svc, _ := exportServiceTestFixture(t)
	router := newExportTestRouter(svc)

	rec := doExportRequest(router, "/api/v1/workspaces/ws-missing/automations/export/zip")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body: %s)", rec.Code, rec.Body.String())
	}
}
