//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/analytics/models"
)

type usageBenchResult struct {
	Pages []*models.TokenUsageReport
	Rows  []models.TokenUsageRow
}

func fetchUsageBenchReport(ctx context.Context, repo *Repository, filter models.SessionUsageFilter) (usageBenchResult, error) {
	var result usageBenchResult
	for range 100 {
		report, err := repo.ListSessionUsage(ctx, filter)
		if err != nil {
			return result, err
		}
		if len(result.Pages) == 0 {
			filter.ProjectionGeneration = report.ProjectionGeneration
		}
		if report.ProjectionGeneration != filter.ProjectionGeneration {
			return result, fmt.Errorf("benchmark publication changed across pages")
		}
		result.Pages = append(result.Pages, report)
		result.Rows = append(result.Rows, report.Rows...)
		if !report.HasMore {
			return result, nil
		}
		if len(report.Rows) == 0 {
			return result, fmt.Errorf("benchmark pagination did not advance")
		}
		filter.Offset += len(report.Rows)
	}
	return result, fmt.Errorf("benchmark pagination exceeded 100 pages")
}

func validateUsageBenchResult(t testing.TB, result usageBenchResult, scenario *usageBenchCase) {
	t.Helper()
	if len(result.Pages) == 0 {
		t.Fatal("benchmark returned no report pages")
	}
	for _, page := range result.Pages {
		validateUsageBenchReport(t, page, result.Pages[0].ProjectionGeneration, scenario)
	}
	if len(result.Rows) != result.Pages[0].TotalRows {
		t.Fatalf("benchmark returned %d of %d grouped rows", len(result.Rows), result.Pages[0].TotalRows)
	}
	validateUsageBenchRows(t, result.Rows, scenario)
}

// @covers AC-COSTS-USAGE-HISTORY-005.5
func BenchmarkUsageHistoryFiveYears(b *testing.B) {
	benchmarkUsageHistory(b, "sqlite")
}

// @covers AC-COSTS-USAGE-HISTORY-005.5
func BenchmarkUsageHistoryPostgresFiveYears(b *testing.B) {
	benchmarkUsageHistory(b, "postgres")
}

func benchmarkUsageHistory(b *testing.B, engine string) {
	b.StopTimer()
	if os.Getenv("KANDEV_USAGE_BENCH_EVIDENCE_DIR") == "" {
		b.Fatal("benchmark requires KANDEV_USAGE_BENCH_EVIDENCE_DIR for durable evidence")
	}
	fixture := newUsageBenchFixture(b, engine, usageBenchSize{Native: 1_000_000, External: 100_000})
	if engine == "sqlite" && usageBenchSQLiteTraceEnabled() {
		fixture.observer.startWithLimit(64, true)
	}
	fixture.seedRecent(b)
	fixture.validateStorage(b, false)
	before := runUsageBenchPhase(b, fixture)
	fixture.seedOlder(b)
	fixture.validateStorage(b, true)
	after := runUsageBenchPhase(b, fixture)
	writeUsageBenchComparison(b, engine, before, after)
}

func runUsageBenchPhase(b *testing.B, fixture *usageBenchFixture) map[string]usageBenchComparisonReceipt {
	b.Helper()
	receipts := map[string]usageBenchComparisonReceipt{}
	for _, scenario := range fixture.cases {
		b.Run(fixture.phase+"/"+scenario.Name, func(b *testing.B) {
			warm, err := fetchUsageBenchReport(context.Background(), fixture.repo, scenario.Filter)
			usageBenchMust(b, err)
			validateUsageBenchResult(b, warm, scenario)
			plain := runUsageBenchProcess(b, fixture, scenario, false)
			instrumented := runUsageBenchProcess(b, fixture, scenario, true)
			if plain.Publication != instrumented.Publication {
				b.Fatal("instrumentation and latency reads used different publications")
			}
			samples := make([]int64, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				started := time.Now()
				result, err := fetchUsageBenchReport(context.Background(), fixture.repo, scenario.Filter)
				samples = append(samples, time.Since(started).Nanoseconds())
				b.StopTimer()
				usageBenchMust(b, err)
				validateUsageBenchResult(b, result, scenario)
				b.StartTimer()
			}
			b.StopTimer()
			writeUsageBenchEvidence(b, fixture, scenario, samples, plain, instrumented)
			receipts[scenario.Name] = usageBenchComparisonReceipt{SelectionDigest: usageBenchSelectionDigest(b, warm), Publication: plain.Publication, Scans: instrumented.Scans, P95NS: usageBenchPercentile(samples, 95), Samples: len(samples)}
		})
	}
	return receipts
}
