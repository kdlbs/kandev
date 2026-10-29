package sandbox

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSandboxAvailabilityRefusesUnavailableRuntime(t *testing.T) {
	service, _, _ := policySelectionFixture(t)
	response := httptest.NewRecorder()
	sandboxOperatorRouter(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/availability", strings.NewReader(`{"executor_id":"executor","agent_profile_id":"agent"}`)))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"available":false,"blocker":"sandbox_runtime_unavailable"}`, response.Body.String())
}

func TestSandboxAvailabilityUsesBoundedCurrentSelection(t *testing.T) {
	service, _, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	request := PreviewContext{ExecutorID: "executor", AgentProfileID: "agent"}
	calls := 0
	service.SetAvailabilityCheck(func(probe context.Context, resolved *Resolved) error {
		calls++
		deadline, ok := probe.Deadline()
		require.True(t, ok)
		require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		require.Equal(t, "executor", resolved.ExecutorID)
		return nil
	})
	result, err := service.CheckAvailability(ctx, request)
	require.NoError(t, err)
	require.True(t, result.Available)
	require.Equal(t, 1, calls)
	unchanged, err := service.GetBinding(ctx, ScopeAgent, "agent")
	require.NoError(t, err)
	require.Zero(t, unchanged.Version)
	service.SetAvailabilityCheck(func(context.Context, *Resolved) error {
		_, changeErr := service.PutBinding(ctx, Binding{Scope: ScopeAgent, TargetID: "agent", RevisionID: read.ID}, 0)
		return changeErr
	})
	result, err = service.CheckAvailability(ctx, request)
	require.ErrorIs(t, err, ErrConflict)
	require.Nil(t, result)
}

func TestSandboxAvailabilitySanitizesProviderFailure(t *testing.T) {
	service, _, _ := policySelectionFixture(t)
	service.SetAvailabilityCheck(func(context.Context, *Resolved) error { return errors.New("private runtime path and credential") })
	response := httptest.NewRecorder()
	sandboxOperatorRouter(t, service).ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/availability", strings.NewReader(`{"executor_id":"executor"}`)))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"available":false,"blocker":"sandbox_runtime_unavailable"}`, response.Body.String())
}
