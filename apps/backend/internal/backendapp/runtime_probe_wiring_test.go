package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/runtimeprobe"
)

func TestRegisterRuntimeProbeRouteAnswersOnlyGetWithALiveToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	registry := runtimeprobe.New()
	router := gin.New()
	registerRuntimeProbeRoute(router, registry)
	probe := registry.Begin("environment-1")

	serve := func(method, token string) int {
		request := httptest.NewRequest(method, runtimeprobe.PingPath, http.NoBody)
		if token != "" {
			request.Header.Set(runtimeprobe.HeaderName, token)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := serve(http.MethodGet, probe.Token()); code != http.StatusNoContent {
		t.Fatalf("GET with a live token = %d, want 204", code)
	}
	if code := serve(http.MethodGet, ""); code != http.StatusUnauthorized {
		t.Fatalf("GET without a token = %d, want 401", code)
	}
	if code := serve(http.MethodPost, probe.Token()); code != http.StatusNotFound {
		t.Fatalf("POST = %d, want 404 (the route answers no other method)", code)
	}
}

func TestRegisterRuntimeProbeRouteToleratesAMissingRegistry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerRuntimeProbeRoute(router, nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, runtimeprobe.PingPath, http.NoBody))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 with no registry", recorder.Code)
	}
}

func TestPluginHostAPIURLIsTheBackendsOwnLoopbackListener(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Port = 38917
	if got := pluginHostAPIURL(cfg); got != "http://127.0.0.1:38917" {
		t.Fatalf("pluginHostAPIURL() = %q", got)
	}
	cfg.Server.Port = 0
	if got := pluginHostAPIURL(cfg); got != "" {
		t.Fatalf("with no listener port the URL must be unknown, got %q", got)
	}
	if got := pluginHostAPIURL(nil); got != "" {
		t.Fatalf("nil config = %q, want empty", got)
	}
}
