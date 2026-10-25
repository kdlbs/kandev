package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/analytics/models"
	"github.com/kandev/kandev/internal/analytics/repository"
)

type busyStatsRepo struct {
	recordingRepo
}

func newStatsRouterWithRepo(t *testing.T, repo repository.Repository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterStatsRoutes(router, repo, newOwnerAuthorizer(), authzTestLogger(t))
	return router
}

func (r *busyStatsRepo) GetGlobalStats(
	context.Context,
	string,
	*time.Time,
) (*models.GlobalStats, error) {
	return nil, repository.NewAnalyticsBusyError(context.DeadlineExceeded)
}

func TestStatsBusyResponseIsRetryable(t *testing.T) {
	router := newStatsRouterWithRepo(t, &busyStatsRepo{})
	rec := doGet(t, router, "/api/v1/workspaces/"+wsA+"/stats/global")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want 2", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if body["error_code"] != "analytics_busy" || body["error"] != "statistics are temporarily busy" {
		t.Fatalf("body = %v, want sanitized retryable error", body)
	}
}
