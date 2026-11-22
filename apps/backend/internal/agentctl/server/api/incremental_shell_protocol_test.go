package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncrementalShellUnsupportedReader(t *testing.T) {
	server, _, _ := newDurableDeliveryTestServer(t)
	for _, path := range []string{"/api/v1/agent/stream", "/api/v1/agent/delivery/stream", "/api/v1/agent/submissions", "/api/v1/agent/submissions/id", "/api/v1/agent/delivery/stream/ack"} {
		method := http.MethodGet
		if path == "/api/v1/agent/delivery/stream/ack" {
			method = http.MethodPost
		}
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(method, path, nil)
		server.Router().ServeHTTP(recorder, request)
		require.Equal(t, http.StatusConflict, recorder.Code, path)
		require.Contains(t, recorder.Body.String(), "INCOMPATIBLE_DELIVERY_VERSION")
	}
	recorder := httptest.NewRecorder()
	server.Router().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/agent/delivery", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
}
