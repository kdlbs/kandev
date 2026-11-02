package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
)

type circuitReadStub struct {
	rows []controller.RoutingCircuit
	err  error
}

func (s circuitReadStub) ListRoutingCircuits(context.Context) ([]controller.RoutingCircuit, error) {
	return s.rows, s.err
}

func TestRoutingCircuitReadEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader controller.RoutingCircuitReader
		status int
		count  int
	}{
		{"unwired", nil, http.StatusServiceUnavailable, 0},
		{"empty", circuitReadStub{}, http.StatusOK, 0},
		{"live", circuitReadStub{rows: []controller.RoutingCircuit{{ResourceKey: "credential:opaque", Scope: "credential", ScopeValue: "opaque", State: "open", Failures: 3}}}, http.StatusOK, 1},
		{"failed", circuitReadStub{err: errors.New("private backend details")}, http.StatusInternalServerError, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
			if err != nil {
				t.Fatal(err)
			}
			ctrl := controller.NewController(nil, nil, nil, nil, log)
			ctrl.SetRoutingCircuitReader(tc.reader)
			router := gin.New()
			useSyntheticSettingsIdentity(router)
			RegisterRoutes(router, ctrl, nil, log, "test-interlock")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/routing/circuits", nil))
			if response.Code != tc.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "private backend details") {
				t.Fatal("internal error exposed")
			}
			if tc.status == http.StatusOK {
				var got controller.RoutingCircuitsResponse
				if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Circuits == nil || len(got.Circuits) != tc.count {
					t.Fatalf("response=%s", response.Body.String())
				}
			}
		})
	}
}
