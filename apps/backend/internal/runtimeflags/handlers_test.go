package runtimeflags

import (
	"encoding/json"
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
	assertJSONError(t, rec.Body.String(), "runtime flag is controlled by environment")
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
	assertJSONError(t, rec.Body.String(), "runtime flag not found")
}

func TestHandlersPatchMalformedJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&memoryStore{}, Options{})
	router := gin.New()
	RegisterRoutes(router, svc)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/runtime-flags/features.office", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	assertJSONError(t, rec.Body.String(), "invalid runtime flag payload")
}

func TestHandlersPatchEmptyBodyClearsOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &memoryStore{values: map[string]bool{"features.office": true}}
	svc := NewService(store, Options{})
	router := gin.New()
	RegisterRoutes(router, svc)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/runtime-flags/features.office", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if _, ok := store.values["features.office"]; ok {
		t.Fatal("features.office override still present after empty-body clear")
	}
}

func assertJSONError(t *testing.T, body, want string) {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal error body: %v; body=%s", err, body)
	}
	if payload.Error != want {
		t.Fatalf("error = %q, want %q", payload.Error, want)
	}
}
