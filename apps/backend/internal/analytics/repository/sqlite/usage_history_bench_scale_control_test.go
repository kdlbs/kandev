//go:build cgo && sqlite_trace && usage_history_scanstatus

package sqlite

import "testing"

const (
	usageBenchNativeSpecialRows   = 5
	usageBenchExternalSpecialRows = 13
)

func TestUsageHistoryBenchmarkConstructionScaleSQLite(t *testing.T) {
	checkUsageHistoryConstructionScale(t, "sqlite")
}

func TestUsageHistoryBenchmarkConstructionScalePostgres(t *testing.T) {
	checkUsageHistoryConstructionScale(t, "postgres")
}

func checkUsageHistoryConstructionScale(t *testing.T, engine string) {
	t.Helper()
	fixture := newUsageBenchFixture(t, engine, usageBenchSize{Native: 100_000, External: 10_000})
	if engine == "sqlite" && usageBenchSQLiteTraceEnabled() {
		fixture.observer.startWithLimit(64, true)
	}
	if engine == "postgres" && usageBenchPostgresTraceEnabled() {
		fixture.observer.startWithLimit(64, true)
	}
	fixture.seedRecent(t)
	fixture.validateStorage(t, false)
	wantNative := fixture.size.Native/5 + usageBenchNativeSpecialRows
	wantExternal := fixture.size.External/5 + usageBenchExternalSpecialRows
	if fixture.storage.RetainedNative != wantNative || fixture.storage.RetainedExternal != wantExternal {
		t.Fatalf("scaled retained rows native=%d external=%d", fixture.storage.RetainedNative, fixture.storage.RetainedExternal)
	}
}
