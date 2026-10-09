package routines

import (
	"context"
	"errors"
	"expvar"
	"reflect"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newArmingScanRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if _, err := repo.ExecRaw(context.Background(), `
		CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)
	`); err != nil {
		t.Fatalf("create workspaces table: %v", err)
	}
	return repo
}

func mustCreateArmingWorkspace(t *testing.T, repo *sqlite.Repository, id string, createdAt time.Time) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(),
		`INSERT INTO workspaces (id, created_at) VALUES (?, ?)`, id, createdAt); err != nil {
		t.Fatalf("create workspace %s: %v", id, err)
	}
}

func mustCreateArmingRoutine(t *testing.T, repo *sqlite.Repository, r *Routine, createdAt time.Time) {
	t.Helper()
	if err := repo.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine %s: %v", r.ID, err)
	}
	if _, err := repo.ExecRaw(context.Background(),
		`UPDATE office_routines SET created_at = ? WHERE id = ?`, createdAt, r.ID); err != nil {
		t.Fatalf("set created_at for %s: %v", r.ID, err)
	}
}

func mustCreateArmingTrigger(t *testing.T, repo *sqlite.Repository, tr *RoutineTrigger) {
	t.Helper()
	if err := repo.CreateRoutineTrigger(context.Background(), tr); err != nil {
		t.Fatalf("create trigger %s: %v", tr.ID, err)
	}
}

func newObservedLogger(t *testing.T, level zapcore.Level) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(level)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	return log, logs
}

func counterValue(v expvar.Var) int64 {
	if v == nil {
		return 0
	}
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// panicIfCalledReader proves RunStartupScan makes no reader call at all when
// invoked without a completion signal (AC-OFFICE-ROUTINE-ARMING-003.1).
type panicIfCalledReader struct{}

func (panicIfCalledReader) ListWorkspaceIDsOrdered(context.Context) ([]string, error) {
	panic("ListWorkspaceIDsOrdered called despite missing completion signal")
}

func (panicIfCalledReader) ListRoutinesOrdered(context.Context, string) ([]*Routine, error) {
	panic("ListRoutinesOrdered called despite missing completion signal")
}

func (panicIfCalledReader) ListTriggersByRoutineIDs(
	context.Context, []string,
) (map[string][]*RoutineTrigger, error) {
	panic("ListTriggersByRoutineIDs called despite missing completion signal")
}

func (panicIfCalledReader) ListTriggersByRoutineID(context.Context, string) ([]*RoutineTrigger, error) {
	panic("ListTriggersByRoutineID called despite missing completion signal")
}

// armingScanFailures wraps a real repository, letting a test force a
// workspace-enumeration failure, a per-workspace routine-enumeration
// failure, or a classification failure for named routines.
type armingScanFailures struct {
	*sqlite.Repository
	failWorkspaceEnum  bool
	failRoutineEnumFor map[string]bool
	forceBatchErr      bool
	forceSingleErrOn   map[string]bool
}

func (f *armingScanFailures) ListWorkspaceIDsOrdered(ctx context.Context) ([]string, error) {
	if f.failWorkspaceEnum {
		return nil, errors.New("forced workspace enumeration failure")
	}
	return f.Repository.ListWorkspaceIDsOrdered(ctx)
}

func (f *armingScanFailures) ListRoutinesOrdered(ctx context.Context, workspaceID string) ([]*Routine, error) {
	if f.failRoutineEnumFor[workspaceID] {
		return nil, errors.New("forced routine enumeration failure")
	}
	return f.Repository.ListRoutinesOrdered(ctx, workspaceID)
}

func (f *armingScanFailures) ListTriggersByRoutineIDs(
	ctx context.Context, routineIDs []string,
) (map[string][]*RoutineTrigger, error) {
	if f.forceBatchErr {
		return nil, errors.New("forced batch trigger read failure")
	}
	return f.Repository.ListTriggersByRoutineIDs(ctx, routineIDs)
}

func (f *armingScanFailures) ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*RoutineTrigger, error) {
	if f.forceSingleErrOn[routineID] {
		return nil, errors.New("forced single trigger read failure")
	}
	return f.Repository.ListTriggersByRoutineID(ctx, routineID)
}

func TestRunStartupScan_NoSignalClassifiesNothing(t *testing.T) {
	log, logs := newObservedLogger(t, zapcore.InfoLevel)
	before := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipNoSignal)))

	RunStartupScan(context.Background(), ReconcileSignal{}, panicIfCalledReader{}, log, time.Now().UTC())

	after := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipNoSignal)))
	if after != before+1 {
		t.Errorf("no_signal skip counter = %d, want %d", after, before+1)
	}
	if logs.Len() != 1 {
		t.Fatalf("log entries = %d, want 1: %+v", logs.Len(), logs.All())
	}
	if logs.All()[0].Level != zapcore.WarnLevel {
		t.Errorf("level = %v, want warn", logs.All()[0].Level)
	}
}

func TestRunStartupScan_WorkspaceEnumerationFailure(t *testing.T) {
	repo := newArmingScanRepo(t)
	reader := &armingScanFailures{Repository: repo, failWorkspaceEnum: true}
	log, logs := newObservedLogger(t, zapcore.InfoLevel)
	before := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipWorkspaceEnumFailed)))

	RunStartupScan(context.Background(), SignalReconcileComplete(), reader, log, time.Now().UTC())

	after := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipWorkspaceEnumFailed)))
	if after != before+1 {
		t.Errorf("workspace_enum_failed skip counter = %d, want %d", after, before+1)
	}
	if logs.Len() != 1 {
		t.Fatalf("log entries = %d, want 1: %+v", logs.Len(), logs.All())
	}
	if logs.All()[0].Level != zapcore.WarnLevel {
		t.Errorf("level = %v, want warn", logs.All()[0].Level)
	}
}

func TestRunStartupScan_PerWorkspaceRoutineEnumerationFailureContinues(t *testing.T) {
	repo := newArmingScanRepo(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mustCreateArmingWorkspace(t, repo, "ws-a", base)
	mustCreateArmingWorkspace(t, repo, "ws-b", base.Add(time.Minute))
	mustCreateArmingWorkspace(t, repo, "ws-c", base.Add(2*time.Minute))

	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-a", WorkspaceID: "ws-a", Name: "A", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base)
	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-c", WorkspaceID: "ws-c", Name: "C", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base)

	reader := &armingScanFailures{Repository: repo, failRoutineEnumFor: map[string]bool{"ws-b": true}}
	log, logs := newObservedLogger(t, zapcore.InfoLevel)
	before := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipRoutineEnumFailed)))

	RunStartupScan(context.Background(), SignalReconcileComplete(), reader, log, time.Now().UTC())

	after := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipRoutineEnumFailed)))
	if after != before+1 {
		t.Errorf("routine_enum_failed skip counter = %d, want %d", after, before+1)
	}

	var sawA, sawC, warnedB bool
	for _, entry := range logs.All() {
		ctx := entry.ContextMap()
		ws, _ := ctx["workspace_id"].(string)
		routineID, _ := ctx["routine_id"].(string)
		switch {
		case routineID == "r-a":
			sawA = true
		case routineID == "r-c":
			sawC = true
		case ws == "ws-b" && entry.Level == zapcore.WarnLevel:
			warnedB = true
		}
	}
	if !sawA {
		t.Error("ws-a's routine record was not preserved")
	}
	if !sawC {
		t.Error("scan did not continue to ws-c after ws-b's enumeration failed")
	}
	if !warnedB {
		t.Error("ws-b's enumeration failure was not recorded as a warning")
	}
}

func TestRunStartupScan_AllStatesFixtureRecordMatrix(t *testing.T) {
	repo := newArmingScanRepo(t)
	now := time.Now().UTC()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mustCreateArmingWorkspace(t, repo, "ws-1", base)

	future := now.Add(time.Hour)
	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-cannot-fire", WorkspaceID: "ws-1", Name: "CannotFire", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base)
	mustCreateArmingTrigger(t, repo, &RoutineTrigger{
		ID: "t-cannot-fire", RoutineID: "r-cannot-fire", Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: false,
	})

	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-no-schedule", WorkspaceID: "ws-1", Name: "NoSchedule", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base.Add(time.Minute))

	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-observed", WorkspaceID: "ws-1", Name: "Observed", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base.Add(2*time.Minute))
	mustCreateArmingTrigger(t, repo, &RoutineTrigger{
		ID: "t-observed", RoutineID: "r-observed", Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &future,
	})

	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-inactive", WorkspaceID: "ws-1", Name: "Inactive", TaskTemplate: "{}",
		Status: "paused", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base.Add(3*time.Minute))
	mustCreateArmingTrigger(t, repo, &RoutineTrigger{
		ID: "t-inactive", RoutineID: "r-inactive", Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: false,
	})

	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-unknown", WorkspaceID: "ws-1", Name: "Unknown", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base.Add(4*time.Minute))

	reader := &armingScanFailures{
		Repository:       repo,
		forceBatchErr:    true,
		forceSingleErrOn: map[string]bool{"r-unknown": true},
	}
	log, logs := newObservedLogger(t, zapcore.InfoLevel)
	obsBefore := counterValue(armingScanObservationsTotal.Get(
		armingLabel("workspace", "ws-1", "intent", "active", "schedule_state", "unknown")))
	skipBefore := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipClassificationUnknown)))

	RunStartupScan(context.Background(), SignalReconcileComplete(), reader, log, now)

	if logs.Len() != 5 {
		t.Fatalf("log entries = %d, want 5: %+v", logs.Len(), logs.All())
	}

	byRoutine := map[string]observer.LoggedEntry{}
	for _, entry := range logs.All() {
		if id, ok := entry.ContextMap()["routine_id"].(string); ok {
			byRoutine[id] = entry
		}
	}

	cannotFire := byRoutine["r-cannot-fire"]
	if cannotFire.Level != zapcore.WarnLevel || cannotFire.Message != "office routine arming scan: routine cannot fire" {
		t.Errorf("r-cannot-fire record = %+v", cannotFire)
	}
	if _, ok := cannotFire.ContextMap()["unarmed_cron_triggers"]; !ok {
		t.Error("r-cannot-fire record missing unarmed_cron_triggers")
	}

	noSchedule := byRoutine["r-no-schedule"]
	if noSchedule.Level != zapcore.WarnLevel || noSchedule.Message != "office routine arming scan: routine has no schedule" {
		t.Errorf("r-no-schedule record = %+v", noSchedule)
	}
	if _, ok := noSchedule.ContextMap()["unarmed_cron_triggers"]; ok {
		t.Error("r-no-schedule record should carry no unarmed_cron_triggers")
	}

	observed := byRoutine["r-observed"]
	if observed.Level != zapcore.InfoLevel || observed.Message != "office routine arming scan: routine observed" {
		t.Errorf("r-observed record = %+v", observed)
	}

	inactive := byRoutine["r-inactive"]
	if inactive.Level != zapcore.InfoLevel || inactive.Message != "office routine arming scan: routine intent is not active" {
		t.Errorf("r-inactive record = %+v", inactive)
	}

	unknown := byRoutine["r-unknown"]
	if unknown.Level != zapcore.InfoLevel || unknown.Message != "office routine arming scan: schedule state unknown" {
		t.Errorf("r-unknown record = %+v", unknown)
	}

	obsAfter := counterValue(armingScanObservationsTotal.Get(
		armingLabel("workspace", "ws-1", "intent", "active", "schedule_state", "unknown")))
	if obsAfter != obsBefore+1 {
		t.Errorf("unknown observation counter = %d, want %d", obsAfter, obsBefore+1)
	}
	skipAfter := counterValue(armingScanSkippedTotal.Get(armingLabel("reason", armingSkipClassificationUnknown)))
	if skipAfter != skipBefore+1 {
		t.Errorf("classification_unknown skip counter = %d, want %d", skipAfter, skipBefore+1)
	}
}

// summarizeRecords strips wall-clock/logger-added fields so two scans over
// unchanged data can be compared per AC-OFFICE-ROUTINE-ARMING-003.12.
func summarizeRecords(entries []observer.LoggedEntry) []map[string]interface{} {
	out := make([]map[string]interface{}, len(entries))
	for i, e := range entries {
		ctx := e.ContextMap()
		ctx["__level"] = e.Level.String()
		ctx["__message"] = e.Message
		out[i] = ctx
	}
	return out
}

func TestRunStartupScan_IdempotentOverUnchangedData(t *testing.T) {
	repo := newArmingScanRepo(t)
	now := time.Now().UTC()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mustCreateArmingWorkspace(t, repo, "ws-1", base)

	future := now.Add(time.Hour)
	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-armed", WorkspaceID: "ws-1", Name: "Armed", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base)
	mustCreateArmingTrigger(t, repo, &RoutineTrigger{
		ID: "t-armed", RoutineID: "r-armed", Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &future,
	})
	mustCreateArmingRoutine(t, repo, &Routine{
		ID: "r-disabled", WorkspaceID: "ws-1", Name: "Disabled", TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}, base.Add(time.Minute))
	mustCreateArmingTrigger(t, repo, &RoutineTrigger{
		ID: "t-disabled", RoutineID: "r-disabled", Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: false,
	})

	log1, logs1 := newObservedLogger(t, zapcore.InfoLevel)
	RunStartupScan(context.Background(), SignalReconcileComplete(), repo, log1, now)

	log2, logs2 := newObservedLogger(t, zapcore.InfoLevel)
	RunStartupScan(context.Background(), SignalReconcileComplete(), repo, log2, now)

	first := summarizeRecords(logs1.All())
	second := summarizeRecords(logs2.All())
	if !reflect.DeepEqual(first, second) {
		t.Errorf("scan not idempotent over unchanged data:\nfirst:  %+v\nsecond: %+v", first, second)
	}
}
