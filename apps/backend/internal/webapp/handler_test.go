package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesStaticAssetBeforeRouteClassification(t *testing.T) {
	t.Parallel()

	handler := NewHandler(fstest.MapFS{
		"index.html":             {Data: []byte("<html><head></head><body></body></html>")},
		"fonts/seti/seti.woff":   {Data: []byte("font")},
		"assets/index-abc123.js": {Data: []byte("console.log('ok')")},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/fonts/seti/seti.woff", nil)
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(resp.Body.String()); got != "font" {
		t.Fatalf("body = %q, want font", got)
	}
}

func TestHandlerInjectsBootPayloadForSPARoutes(t *testing.T) {
	t.Parallel()

	handler := NewHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html><head></head><body><div id=\"root\"></div></body></html>")},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/t/task-1", nil)
	handler.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, bootPayloadGlobal) {
		t.Fatalf("body missing boot payload: %s", body)
	}
	if !strings.Contains(body, `"taskId":"task-1"`) {
		t.Fatalf("body missing task route params: %s", body)
	}
	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestHandlerReturnsNotFoundForAPIMiss(t *testing.T) {
	t.Parallel()

	handler := NewHandler(fstest.MapFS{
		"index.html": {Data: []byte("<html><head></head><body></body></html>")},
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusNotFound)
	}
}
