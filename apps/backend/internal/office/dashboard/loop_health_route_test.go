package dashboard_test

import (
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

// degradedExpvarInt reads one office_loop_liveness_degraded_total cell
// by its full label key, mirroring the terminalShapeExpvarInt helper
// pattern used for the same expvar.Map shape elsewhere in this suite.
func degradedExpvarInt(t *testing.T, key string) int64 {
	t.Helper()
	v := expvar.Get("office_loop_liveness_degraded_total")
	if v == nil {
		t.Fatal("expvar map office_loop_liveness_degraded_total not registered")
	}
	m, ok := v.(*expvar.Map)
	if !ok {
		t.Fatal("office_loop_liveness_degraded_total is not a Map")
	}
	sub := m.Get(key)
	if sub == nil {
		return 0
	}
	iv, ok := sub.(*expvar.Int)
	if !ok {
		t.Fatalf("office_loop_liveness_degraded_total[%q] is not an Int", key)
	}
	return iv.Value()
}

// AC-003.4: the loop-health and loop-counters routes are reachable
// with no dev-mode/pprof env var set — newTestDeps's router mounts
// them with no such gate, matching production registration.
func TestGetLoopHealth_ReachableWithoutDevMode(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-route-check/loop-health", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestGetLoopCounters_ReachableWithoutDevMode(t *testing.T) {
	deps := newTestDeps(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/ws-route-check/loop-counters", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
}

// AC-004.10: a read failure through the real, database-backed
// LoopHealthRepo (not the pure-Go fake used by loop_health_test.go)
// must reach the HTTP layer as a 503 carrying a reason field, and
// must increment office_loop_liveness_degraded_total by that reason
// — exercising the full handler wiring, not just EvaluateLoopHealth
// in isolation.
func TestGetLoopHealth_ReadFailureRespondsServiceUnavailableAndIncrementsCounter(t *testing.T) {
	deps := newTestDeps(t)
	wsID := "ws-loop-health-db-closed"
	key := service.LoopMetricLabel("workspace", wsID, "reason", "activation_read_failed")
	before := degradedExpvarInt(t, key)

	if err := deps.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/office/workspaces/"+wsID+"/loop-health", nil)
	w := httptest.NewRecorder()
	deps.router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", w.Code, w.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["reason"] != "activation_read_failed" {
		t.Errorf("reason = %v, want activation_read_failed", body["reason"])
	}
	if _, ok := body["error"]; !ok {
		t.Errorf("body = %+v, want an error field", body)
	}

	after := degradedExpvarInt(t, key)
	if after != before+1 {
		t.Errorf("degraded counter delta = %d, want 1", after-before)
	}
}
