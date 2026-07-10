package restart

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandleCapabilityReportsUnsupportedManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/system/restart-capability", HandleCapability(NewUnsupportedManager("")))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/restart-capability", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body Capability
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Supported {
		t.Fatal("Supported = true, want false")
	}
	if body.Mode != ModeManual {
		t.Fatalf("Mode = %q, want %q", body.Mode, ModeManual)
	}
	if body.Adapter != AdapterUnsupported {
		t.Fatalf("Adapter = %q, want %q", body.Adapter, AdapterUnsupported)
	}
	if body.Reason == "" {
		t.Fatal("Reason empty, want manual restart guidance")
	}
}

func TestHandleRequestRejectsUnsupportedRestart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/system/restart", HandleRequest(NewUnsupportedManager("")))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	var body RestartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Accepted {
		t.Fatal("Accepted = true, want false")
	}
}

func TestHandleRequestDelegatesToSupportedManager(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := &fakeManager{
		resp: RestartResponse{Accepted: true, Message: "Restart requested."},
	}
	r := gin.New()
	r.POST("/api/v1/system/restart", HandleRequest(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if mgr.requestCalls != 1 {
		t.Fatalf("requestCalls = %d, want 1", mgr.requestCalls)
	}
}

func TestHandleRequestReportsManagerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/system/restart", HandleRequest(&fakeManager{err: errors.New("boom")}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/system/restart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
}

type fakeManager struct {
	resp         RestartResponse
	err          error
	requestCalls int
}

func (m *fakeManager) Capability(context.Context) Capability {
	return Capability{
		Supported: true,
		Mode:      ModeSupervisor,
		Adapter:   AdapterSupervisor,
	}
}

func (m *fakeManager) RequestRestart(context.Context) (RestartResponse, error) {
	m.requestCalls++
	return m.resp, m.err
}
