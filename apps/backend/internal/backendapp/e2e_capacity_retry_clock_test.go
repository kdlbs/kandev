package backendapp

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCapacityRetryClockProductionConstruction(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "false")
	t.Setenv("KANDEV_MOCK_AGENT", "true")
	require.Nil(t, newE2ECapacityRetryClock())
}

func TestCapacityRetryClockAdvancesAndCancels(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_MOCK_AGENT", "true")
	clock := newE2ECapacityRetryClock()
	require.NotNil(t, clock)
	clock.freeze()
	deadline := clock.Now().Add(time.Minute)
	clock.advance(time.Minute)
	require.True(t, clock.WaitUntil(context.Background(), deadline), "advancing before wait registration must not lose the wake")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, clock.WaitUntil(ctx, deadline.Add(time.Minute)))
}

func TestCapacityRetryClockRegisteredWait(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_MOCK_AGENT", "true")
	clock := newE2ECapacityRetryClock()
	require.True(t, clock.freeze())
	due := clock.Now().Add(5 * time.Minute)
	result := make(chan bool, 1)
	go func() { result <- clock.WaitUntil(context.Background(), due) }()
	require.True(t, clock.advance(5*time.Minute))
	select {
	case woke := <-result:
		require.True(t, woke)
	case <-time.After(time.Second):
		t.Fatal("advancing the clock did not release its waiter")
	}
}

func TestCapacityRetryClockRoutesRequireBothMockSelectors(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "production", true: "isolated mock"}[enabled], func(t *testing.T) {
			t.Setenv("KANDEV_E2E_MOCK", "true")
			t.Setenv("KANDEV_MOCK_AGENT", strconv.FormatBool(enabled))
			clock := newE2ECapacityRetryClock()
			router := gin.New()
			registerE2ECapacityRetryClockRoutes(router, clock)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/e2e/capacity-retry-clock/freeze", nil))
			if enabled {
				require.Equal(t, http.StatusNoContent, response.Code)
			} else {
				require.Equal(t, http.StatusNotFound, response.Code)
			}
		})
	}
}

func TestCapacityRetryClockMockOnlyConstruction(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("KANDEV_MOCK_AGENT", "only")
	require.NotNil(t, newE2ECapacityRetryClock())
}
