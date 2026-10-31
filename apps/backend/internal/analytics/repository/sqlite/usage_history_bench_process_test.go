//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/testutil/sqlitememory"
)

const usageBenchChildMarker = "KANDEV_USAGE_HISTORY_BENCH_CHILD"
const usageBenchOutputMarker = "USAGE_BENCH_QUERY_RESULT="

type usageBenchProcessConfig struct {
	Engine, Path, Schema string
	Instrumented         bool
	Scenario             usageBenchCase
}

type usageBenchProcessResult struct {
	Publication      string                `json:"publication"`
	SQLiteVersion    string                `json:"sqlite_version,omitempty"`
	PostgresVersion  string                `json:"postgres_version,omitempty"`
	LatencyNS        int64                 `json:"query_latency_ns"`
	GoAllocatedBytes uint64                `json:"query_go_allocated_bytes"`
	GoAllocations    uint64                `json:"query_go_allocations"`
	ProcessPeakRSS   int64                 `json:"query_process_peak_rss_bytes"`
	NativeBaseline   *int64                `json:"sqlite_native_baseline_bytes"`
	NativePeak       *int64                `json:"sqlite_native_peak_bytes"`
	BackendPeakRSS   *int64                `json:"postgres_backend_process_peak_rss_bytes"`
	Statements       []usageBenchStatement `json:"statements,omitempty"`
	Scans            usageBenchScanTotals  `json:"scans"`
}

func runUsageBenchProcess(t testing.TB, fixture *usageBenchFixture, scenario *usageBenchCase, instrumented bool) usageBenchProcessResult {
	t.Helper()
	config := usageBenchProcessConfig{Engine: fixture.engine, Path: fixture.path, Schema: fixture.schema, Instrumented: instrumented, Scenario: *scenario}
	encoded, err := json.Marshal(config)
	usageBenchMust(t, err)
	path := filepath.Join(t.TempDir(), "query-process.json")
	usageBenchMust(t, os.WriteFile(path, encoded, 0o600))
	executable, err := os.Executable()
	usageBenchMust(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestUsageHistoryBenchmarkQueryProcess$", "-test.count=1")
	command.Env = append(os.Environ(), usageBenchChildMarker+"="+path)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("isolated query process failed: %v\n%s", err, output)
	}
	for _, line := range bytes.Split(output, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte(usageBenchOutputMarker)) {
			var result usageBenchProcessResult
			usageBenchMust(t, json.Unmarshal(bytes.TrimPrefix(line, []byte(usageBenchOutputMarker)), &result))
			return result
		}
	}
	t.Fatal("isolated query process returned no measurement receipt")
	return usageBenchProcessResult{}
}

// The private env marker and exact test selection prevent recursive suites.
func TestUsageHistoryBenchmarkQueryProcess(t *testing.T) {
	path := os.Getenv(usageBenchChildMarker)
	if path == "" {
		return
	}
	encoded, err := os.ReadFile(path)
	usageBenchMust(t, err)
	var config usageBenchProcessConfig
	usageBenchMust(t, json.Unmarshal(encoded, &config))
	observer := &usageBenchObserver{}
	database := openUsageBenchQueryPool(t, config, observer)
	repo, err := NewWithDB(database, database)
	usageBenchMust(t, err)
	result := prepareUsageBenchProcess(t, config, database, observer)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	report, err := fetchUsageBenchReport(context.Background(), repo, config.Scenario.Filter)
	result.LatencyNS = time.Since(started).Nanoseconds()
	runtime.ReadMemStats(&after)
	usageBenchMust(t, err)
	result.GoAllocatedBytes, result.GoAllocations = after.TotalAlloc-before.TotalAlloc, after.Mallocs-before.Mallocs
	result.ProcessPeakRSS, err = usageBenchProcBytes("/proc/self/status", "VmHWM:")
	usageBenchMust(t, err)
	if config.Engine == "sqlite" {
		peak := sqlitememory.Peak(false)
		result.NativePeak = &peak
	}
	validateUsageBenchResult(t, report, &config.Scenario)
	result.Publication = report.Pages[0].ProjectionGeneration
	finishUsageBenchProcess(t, config, database, observer, &result)
	encoded, err = json.Marshal(result)
	usageBenchMust(t, err)
	fmt.Printf("%s%s\n", usageBenchOutputMarker, encoded)
}

func openUsageBenchQueryPool(t testing.TB, config usageBenchProcessConfig, observer *usageBenchObserver) *sqlx.DB {
	t.Helper()
	var database *sqlx.DB
	if config.Engine == "postgres" {
		database = openUsageBenchPostgresQueryPool(t, config, observer)
	} else {
		raw, err := db.OpenSQLite(config.Path)
		usageBenchMust(t, err)
		database = sqlx.NewDb(raw, "sqlite3")
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func openUsageBenchPostgresQueryPool(t testing.TB, config usageBenchProcessConfig, observer *usageBenchObserver) *sqlx.DB {
	t.Helper()
	dsn := os.Getenv("KANDEV_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("isolated PostgreSQL query requires the private test DSN")
	}
	dsn = testutil.PostgresDSNWithSearchPath(t, dsn, config.Schema)
	var database *sqlx.DB
	var err error
	if config.Instrumented {
		database, err = usageBenchPostgresPool(dsn, observer)
	} else {
		var raw *sql.DB
		raw, err = db.OpenPostgres(dsn, 1, 1)
		if err == nil {
			database = sqlx.NewDb(raw, "pgx")
		}
	}
	if err != nil {
		t.Fatal("open isolated PostgreSQL query connection")
	}
	return database
}

func prepareUsageBenchProcess(t testing.TB, config usageBenchProcessConfig, database *sqlx.DB, observer *usageBenchObserver) usageBenchProcessResult {
	t.Helper()
	var result usageBenchProcessResult
	if config.Engine == "sqlite" {
		result.SQLiteVersion = sqlitememory.Version()
		if config.Instrumented {
			usageBenchMust(t, attachUsageBenchSQLite(context.Background(), database.DB, observer))
		} else {
			usageBenchMust(t, setUsageBenchSQLiteCollection(context.Background(), database.DB, false))
		}
		sqlitememory.Peak(true)
		baseline := sqlitememory.Used()
		result.NativeBaseline = &baseline
	} else {
		usageBenchMust(t, database.Get(&result.PostgresVersion, `SHOW server_version`))
	}
	if config.Instrumented {
		observer.start()
	}
	return result
}

func finishUsageBenchProcess(t testing.TB, config usageBenchProcessConfig, database *sqlx.DB, observer *usageBenchObserver, result *usageBenchProcessResult) {
	t.Helper()
	if !config.Instrumented {
		return
	}
	statements, err := observer.finish()
	usageBenchMust(t, err)
	if config.Engine == "postgres" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		usageBenchMust(t, explainUsageBenchPostgres(ctx, database, statements))
		// EXPLAIN uses exactly the captured selection, with no concurrent fixture
		// writes. Check the publication again before accepting its receipt.
		repo := &Repository{db: database, ro: database}
		after := readUsageBenchReport(t, repo, &config.Scenario)
		if after.ProjectionGeneration != result.Publication {
			t.Fatal("PostgreSQL publication changed during plan replay")
		}
	}
	usageBenchMust(t, validateUsageBenchStatements(config.Engine, statements))
	result.Statements = statements
	result.Scans, err = usageBenchStatementTotals(statements)
	usageBenchMust(t, err)
}

func usageBenchProcBytes(path, key string) (int64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == key {
			var value int64
			if _, err := fmt.Sscan(fields[1], &value); err != nil {
				return 0, err
			}
			return value * 1024, nil
		}
	}
	return 0, fmt.Errorf("process memory metric %s unavailable", key)
}
