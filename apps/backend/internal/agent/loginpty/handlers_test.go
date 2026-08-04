package loginpty

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newHostShellRouter(t *testing.T, mgr *Manager) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandlers(mgr, nil, mgr.log.Zap(), nil).RegisterRoutes(router)
	return router
}

func startHostShellRequest(t *testing.T, router http.Handler, body map[string]any) (Status, int) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/host-shell/start", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		return Status{}, response.Code
	}
	var status Status
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	return status, response.Code
}

func stopSession(t *testing.T, mgr *Manager, sessionID string) {
	t.Helper()
	if sess := mgr.GetByID(sessionID); sess != nil {
		sess.stop()
	}
}

func TestHostShellClientIDIsIdempotentAndIndependent(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)

	firstClient := uuid.NewString()
	secondClient := uuid.NewString()
	first, code := startHostShellRequest(t, router, map[string]any{
		"client_id": firstClient,
		"cols":      80,
		"rows":      24,
	})
	if code != http.StatusOK {
		t.Fatalf("first start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, first.ID) })
	if first.AgentID != hostShellAgentID {
		t.Fatalf("agent id = %q, want %q", first.AgentID, hostShellAgentID)
	}

	repeated, code := startHostShellRequest(t, router, map[string]any{"client_id": firstClient})
	if code != http.StatusOK {
		t.Fatalf("repeated start status = %d", code)
	}
	if repeated.ID != first.ID {
		t.Fatalf("repeated start id = %q, want %q", repeated.ID, first.ID)
	}

	second, code := startHostShellRequest(t, router, map[string]any{"client_id": secondClient})
	if code != http.StatusOK {
		t.Fatalf("second start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, second.ID) })
	if second.ID == first.ID {
		t.Fatalf("different client id reused session %q", second.ID)
	}
	if second.AgentID != hostShellAgentID {
		t.Fatalf("second agent id = %q, want %q", second.AgentID, hostShellAgentID)
	}

	stopSession(t, mgr, first.ID)
	if got := mgr.GetByID(second.ID); got == nil || !got.Status().Running {
		t.Fatal("stopping the first client session affected its sibling")
	}
}

func TestHostShellClientIDValidationAndLegacySingleton(t *testing.T) {
	mgr := newTestManager(t, nil)
	router := newHostShellRouter(t, mgr)

	invalid, code := startHostShellRequest(t, router, map[string]any{"client_id": "not-a-uuid"})
	if code != http.StatusBadRequest {
		if invalid.ID != "" {
			stopSession(t, mgr, invalid.ID)
		}
		t.Fatalf("invalid client id status = %d, want %d", code, http.StatusBadRequest)
	}

	first, code := startHostShellRequest(t, router, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("legacy first start status = %d", code)
	}
	t.Cleanup(func() { stopSession(t, mgr, first.ID) })
	second, code := startHostShellRequest(t, router, map[string]any{})
	if code != http.StatusOK {
		t.Fatalf("legacy repeated start status = %d", code)
	}
	if second.ID != first.ID {
		t.Fatalf("legacy repeated start id = %q, want %q", second.ID, first.ID)
	}
}
