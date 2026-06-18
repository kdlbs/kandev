package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDevHandlerInjectsBootPayloadIntoViteIndexForSPARoutes(t *testing.T) {
	t.Parallel()

	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			t.Fatalf("vite received path %q, want /", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head></head><body><script type="module" src="/src/main.tsx"></script></body></html>`))
	}))
	t.Cleanup(vite.Close)

	handler, err := NewDevHandler(vite.URL, WithPayloadBuilder(func(_ *http.Request, route RouteClassification) BootPayload {
		payload := NewBootPayload(route, RuntimeConfig{APIPrefix: "/api/v1", WebSocketPath: "/ws"}, nil)
		payload.RouteData = map[string]any{"from": "go"}
		return payload
	}))
	if err != nil {
		t.Fatalf("NewDevHandler: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tasks", nil)
	req.Header.Set("Accept", "text/html")
	handler.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, body)
	}
	if !strings.Contains(body, bootPayloadGlobal) {
		t.Fatalf("body missing boot payload: %s", body)
	}
	if !strings.Contains(body, `"route":"tasks"`) {
		t.Fatalf("body missing route metadata: %s", body)
	}
	if !strings.Contains(body, `"from":"go"`) {
		t.Fatalf("body missing route data: %s", body)
	}
	if got := resp.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestDevHandlerProxiesViteModuleRequests(t *testing.T) {
	t.Parallel()

	vite := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/src/main.tsx" {
			t.Fatalf("vite received path %q, want /src/main.tsx", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`console.log("vite module")`))
	}))
	t.Cleanup(vite.Close)

	handler, err := NewDevHandler(vite.URL)
	if err != nil {
		t.Fatalf("NewDevHandler: %v", err)
	}

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/src/main.tsx", nil)
	req.Header.Set("Accept", "*/*")
	handler.ServeHTTP(resp, req)

	body := resp.Body.String()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d body=%s", resp.Code, http.StatusOK, body)
	}
	if got := strings.TrimSpace(body); got != `console.log("vite module")` {
		t.Fatalf("body = %q, want proxied module", got)
	}
}
