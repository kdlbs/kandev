package runtimeflags

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHandlersPatchRejectsEnvLockedFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&memoryStore{}, Options{
		DefaultValues: map[string]bool{"features.office": false},
		RuntimeValues: map[string]bool{"features.office": true},
		EnvValues:     map[string]bool{"KANDEV_FEATURES_OFFICE": true},
		IsExplicitEnv: func(name string) bool { return name == "KANDEV_FEATURES_OFFICE" },
	})
	router := gin.New()
	RegisterRoutes(router, svc)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/runtime-flags/features.office", strings.NewReader(`{"override":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestHandlersPatchUnknownFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&memoryStore{}, Options{})
	router := gin.New()
	RegisterRoutes(router, svc)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/runtime-flags/missing.flag", strings.NewReader(`{"override":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
