package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/analytics"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
	"github.com/stretchr/testify/require"
)

type erasedUsageRepo struct {
	recordingRepo
	failure error
}

func (r *erasedUsageRepo) ListSessionUsage(context.Context, models.SessionUsageFilter) (*models.TokenUsageReport, error) {
	if r.failure != nil {
		return nil, r.failure
	}
	return nil, repository.ErrUsageNotVisible
}

func TestUsageHistoryAmountOverflowHasExplicitHTTPOutcome(t *testing.T) {
	router := newStatsRouterWithRepo(t, &erasedUsageRepo{failure: analytics.ErrUsageAmountOverflow})
	response := doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/token-usage")
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.JSONEq(t, `{"error_code":"usage_amount_overflow","error":"usage totals exceed the supported integer range"}`, response.Body.String())
}
func (r *erasedUsageRepo) UpsertSessionUsage(context.Context, []models.SessionUsageMeasurement) ([]models.SessionUsageUpsertResult, error) {
	return nil, nil
}
func (r *erasedUsageRepo) ListSessionUsageMeasurements(context.Context, models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	return nil, nil
}
func (r *erasedUsageRepo) ListCanonicalSessionUsageMeasurements(context.Context, models.SessionUsageFilter) ([]models.SessionUsageMeasurement, error) {
	return nil, nil
}

func TestUsageHistoryErasedScopeDiscardsSavedClientValues(t *testing.T) {
	router := newStatsRouterWithRepo(t, &erasedUsageRepo{})
	response := doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/token-usage")
	require.Equal(t, http.StatusNotFound, response.Code, "erased and inaccessible retained scopes must use the client cache-clearing response")
	require.JSONEq(t, `{"error":"workspace not found"}`, response.Body.String())
}

// @covers AC-COSTS-USAGE-HISTORY-006.5
func TestUsageHistoryMissingRepositoryCapabilityReturnsError(t *testing.T) {
	router := newStatsRouterWithRepo(t, &recordingRepo{})
	response := doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/token-usage")
	require.Equal(t, http.StatusInternalServerError, response.Code,
		"a repository without the retained usage capability must not report an authoritative empty report")
}

// @covers AC-COSTS-USAGE-HISTORY-007.8
func TestUsageHistoryRouteAbsentWhenToggleOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterStatsRoutes(router, &recordingRepo{}, newOwnerAuthorizer(), authzTestLogger(t), false)
	response := doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/token-usage")
	require.Equal(t, http.StatusNotFound, response.Code)
	response = doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/global")
	require.Equal(t, http.StatusOK, response.Code)
}
