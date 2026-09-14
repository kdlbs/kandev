package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	commonlogger "github.com/kandev/kandev/internal/common/logger"
)

// BenchmarkStatsHTTPSerializationReference measures handler and JSON
// serialization overhead with a stub repository. The production-shaped
// concurrent database benchmark lives in repository/sqlite and is the source
// for the admission and latency evidence.
func BenchmarkStatsHTTPSerializationReference(b *testing.B) {
	gin.SetMode(gin.TestMode)
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		b.Fatalf("logger: %v", err)
	}
	router := gin.New()
	repo := &recordingRepo{}
	RegisterStatsRoutes(router, repo, newOwnerAuthorizer(), log)
	paths := []string{
		"/api/v1/workspaces/" + wsA + "/stats/global?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/tasks?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/daily-activity?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/completed-activity?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/model-usage?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/repositories?range=month",
		"/api/v1/workspaces/" + wsA + "/stats/git?range=month",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		repo.calls = repo.calls[:0]
		for _, path := range paths {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, path, nil)
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				b.Fatalf("%s returned HTTP %d: %s", path, recorder.Code, recorder.Body.String())
			}
		}
	}
}
