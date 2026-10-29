//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

const usageBenchReceiptPrefix = "usage-history-worker4-bench-"

type usageBenchStorage struct {
	RetainedNative         int    `json:"retained_native_observations"`
	RetainedExternal       int    `json:"retained_external_observations"`
	PublishedContributions int64  `json:"published_metric_contributions"`
	PublishedFacts         int64  `json:"published_canonical_facts"`
	DailySummaries         int64  `json:"daily_summary_rows"`
	LifetimeSummaries      int64  `json:"lifetime_summary_rows"`
	DiskBytes              int64  `json:"fixture_disk_bytes"`
	DiskScope              string `json:"fixture_disk_scope"`
}

type usageBenchEvidence struct {
	Engine             string                  `json:"engine"`
	Phase              string                  `json:"phase"`
	Scenario           usageBenchCase          `json:"scenario"`
	Storage            usageBenchStorage       `json:"fixture"`
	ProjectionPages    int                     `json:"fixture_projection_pages"`
	ConstructionNS     int64                   `json:"cumulative_fixture_construction_ns"`
	ConstructionMethod string                  `json:"fixture_construction_method"`
	Hardware           map[string]any          `json:"hardware"`
	Samples            []int64                 `json:"latency_samples_ns"`
	P50                int64                   `json:"latency_p50_ns"`
	P95                int64                   `json:"latency_p95_ns"`
	P99                int64                   `json:"latency_p99_ns"`
	Plain              usageBenchProcessResult `json:"isolated_plain_query"`
	Instrumented       usageBenchProcessResult `json:"isolated_instrumented_query"`
	LatencyScope       string                  `json:"latency_scope"`
	MemoryScope        string                  `json:"memory_scope"`
}

func usageBenchPercentile(samples []int64, percentile int) int64 {
	ordered := slices.Clone(samples)
	slices.Sort(ordered)
	if len(ordered) == 0 {
		return 0
	}
	return ordered[(len(ordered)*percentile+99)/100-1]
}

func writeUsageBenchEvidence(b *testing.B, fixture *usageBenchFixture, scenario *usageBenchCase, samples []int64, plain, instrumented usageBenchProcessResult) {
	b.Helper()
	evidence := usageBenchEvidence{
		Engine: fixture.engine, Phase: fixture.phase, Scenario: *scenario, Storage: fixture.storage,
		ProjectionPages: fixture.projectionSteps, ConstructionNS: fixture.seedTime.Nanoseconds(),
		ConstructionMethod: "query-only retained snapshot in batches of at most 128; production native/contract normalizers and immutable captured targets; specials use real admission; all migration, source bounds, canonical selection, contributions, summaries, applied pointers and publication use the ordinary bounded projection worker; not measured production ingestion",
		Hardware:           usageBenchHardware(), Samples: samples,
		P50: usageBenchPercentile(samples, 50), P95: usageBenchPercentile(samples, 95), P99: usageBenchPercentile(samples, 99),
		Plain: plain, Instrumented: instrumented,
		LatencyScope: "complete actual ListSessionUsage report including every page; validation and fixture construction excluded; no trace callbacks; SQLite collection disabled in a scanstatus-capable build",
		MemoryScope:  "isolated query process peak RSS includes initialization but excludes bulk seeding; SQLite allocator high-water resets before query; Go allocation counters are client-only; PostgreSQL plan-node memory is not whole-server peak",
	}
	encoded, err := json.MarshalIndent(evidence, "", "  ")
	usageBenchMust(b, err)
	root := os.Getenv("KANDEV_USAGE_BENCH_EVIDENCE_DIR")
	usageBenchMust(b, os.MkdirAll(root, 0o755))
	name := usageBenchReceiptPrefix + fixture.engine + "-" + fixture.phase + "-" + strings.ReplaceAll(scenario.Name, "/", "-") + ".json"
	path := filepath.Join(root, name)
	usageBenchMust(b, os.WriteFile(path, append(encoded, '\n'), 0o644))
	b.ReportMetric(float64(len(samples)), "samples")
	b.ReportMetric(float64(evidence.P95)/1000, "p95-us/report")
	b.ReportMetric(float64(plain.ProcessPeakRSS), "query-process-peak-rss-B")
	if plain.NativePeak != nil {
		b.ReportMetric(float64(*plain.NativePeak), "sqlite-native-peak-B")
	}
	if fixture.engine == "sqlite" {
		b.ReportMetric(float64(instrumented.Scans.SQLiteVisits), "nvisit/report")
	} else {
		b.ReportMetric(instrumented.Scans.PostgresRows, "scan-output-rows/report")
		b.ReportMetric(instrumented.Scans.PostgresRejected, "scan-rejected-rows/report")
	}
	b.Logf("evidence=%s construction=%s samples=%d trace-query=%dns plain-query=%dns", path, fixture.seedTime, len(samples), instrumented.LatencyNS, plain.LatencyNS)
}

func usageBenchHardware() map[string]any {
	info := map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "logical_cpus": runtime.NumCPU(), "gomaxprocs": runtime.GOMAXPROCS(0), "gomemlimit": os.Getenv("GOMEMLIMIT")}
	for _, item := range []struct{ path, key string }{{"/proc/cpuinfo", "cpu"}, {"/proc/sys/kernel/osrelease", "kernel"}, {"/sys/fs/cgroup/cpu.max", "cgroup_cpu_max"}, {"/sys/fs/cgroup/memory.max", "cgroup_memory_max"}} {
		data, err := os.ReadFile(item.path)
		if err != nil {
			info[item.key] = fmt.Sprintf("unavailable: %T", err)
			continue
		}
		value := strings.TrimSpace(string(data))
		if item.key == "cpu" {
			for _, line := range strings.Split(value, "\n") {
				if strings.HasPrefix(line, "model name") {
					value = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
					break
				}
			}
		}
		info[item.key] = value
	}
	return info
}
