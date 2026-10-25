package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type statsReferenceAuthorizer struct{}

func (statsReferenceAuthorizer) AuthorizeWorkspaceAccess(context.Context, string) error {
	return nil
}

var statsReferenceSections = []string{
	"global",
	"tasks",
	"daily-activity",
	"completed-activity",
	"model-usage",
	"repositories",
	"git",
}

// BenchmarkStatsHTTPReference measures seven authorized HTTP requests issued
// concurrently through one production-shaped repository. The fixture is
// disposable, the repository uses the shared two-slot admission gate, and
// custom cycle metrics record queue plus query latency that the handler
// serialization benchmark cannot observe.
func BenchmarkStatsHTTPReference(b *testing.B) {
	fixture := newStatsReferenceFixture(b, statsReferenceWorkload, true)
	gin.SetMode(gin.TestMode)
	log, err := commonlogger.NewFromZap(zap.NewNop())
	if err != nil {
		b.Fatalf("logger: %v", err)
	}
	router := gin.New()
	RegisterStatsRoutes(router, fixture.repo, statsReferenceAuthorizer{}, log)

	for _, rangeKey := range []string{"month", "all"} {
		rangeKey := rangeKey
		b.Run(rangeKey, func(b *testing.B) {
			paths := statsReferencePaths(fixture.workspaceID, rangeKey)
			cycleDurations := make([]time.Duration, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for cycle := 0; cycle < b.N; cycle++ {
				cycleDuration, requestDurations, cycleErr := runStatsReferenceCycle(router, paths)
				if cycleErr != nil {
					b.Fatal(cycleErr)
				}
				cycleDurations = append(cycleDurations, cycleDuration)
				if cycle < 10 {
					b.Logf("cycle=%d total=%s requests=%s", cycle, cycleDuration, formatDurations(requestDurations))
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(percentileDuration(cycleDurations, 95).Microseconds()), "cycle-p95-us")
			b.ReportMetric(float64(maxDuration(cycleDurations).Microseconds()), "cycle-max-us")
		})
	}
}

func statsReferencePaths(workspaceID, rangeKey string) []string {
	paths := make([]string, len(statsReferenceSections))
	for i, section := range statsReferenceSections {
		paths[i] = fmt.Sprintf(
			"/api/v1/workspaces/%s/stats/%s?range=%s",
			workspaceID,
			section,
			rangeKey,
		)
	}
	return paths
}

func runStatsReferenceCycle(router http.Handler, paths []string) (time.Duration, []time.Duration, error) {
	startedAt := time.Now()
	durations := make([]time.Duration, len(paths))
	errs := make(chan error, len(paths))
	var waitGroup sync.WaitGroup
	waitGroup.Add(len(paths))
	for i, path := range paths {
		go func(index int, requestPath string) {
			defer waitGroup.Done()
			requestStartedAt := time.Now()
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			durations[index] = time.Since(requestStartedAt)
			if response.Code != http.StatusOK {
				errs <- fmt.Errorf("%s returned HTTP %d: %s", requestPath, response.Code, response.Body.String())
			}
		}(i, path)
	}
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		return 0, nil, err
	}
	return time.Since(startedAt), durations, nil
}

func percentileDuration(values []time.Duration, percentile int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*percentile+99)/100 - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func maxDuration(values []time.Duration) time.Duration {
	var maximum time.Duration
	for _, value := range values {
		if value > maximum {
			maximum = value
		}
	}
	return maximum
}

func formatDurations(values []time.Duration) string {
	formatted := make([]string, len(values))
	for i, value := range values {
		formatted[i] = value.String()
	}
	return strings.Join(formatted, ",")
}
