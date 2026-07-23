package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/system/jobs"
	systemsettings "github.com/kandev/kandev/internal/system/settings"
	"go.uber.org/zap"
)

func TestRunNowIgnoresQuietPeriodWithoutActiveTaskWork(t *testing.T) {
	connection := newSQLite(t)
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	provider := &signallingCleanupProvider{called: make(chan struct{})}
	tracker := jobs.NewTracker(nil, newOperationsTestLogger(t))
	operations := NewOperations(OperationsConfig{
		Settings: NewSettingsStore(rawSettings), Store: newStorageStore(t, pool),
		Jobs: tracker, Activity: coordinator, Providers: []CleanupProvider{provider},
	})

	jobID, err := operations.RunNow(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if jobID == "" {
		t.Fatal("RunNow returned an empty job ID")
	}
	select {
	case <-provider.called:
	case <-time.After(time.Second):
		t.Fatal("manual provider did not run")
	}
	deadline := time.Now().Add(time.Second)
	for {
		job := tracker.Get(jobID)
		if job != nil && (job.State == jobs.StateSucceeded || job.State == jobs.StateFailed) {
			if job.State != jobs.StateSucceeded {
				t.Fatalf("manual job state=%q message=%q, want succeeded", job.State, job.Message)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("manual job did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRunNowRejectsCurrentTaskActivity(t *testing.T) {
	connection := newSQLite(t)
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	taskLease, err := coordinator.AcquireTask(context.Background(), activity.KindTestCommand)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer taskLease.Release()
	operations := NewOperations(OperationsConfig{
		Settings: NewSettingsStore(rawSettings), Store: newStorageStore(t, pool),
		Jobs: jobs.NewTracker(nil, newOperationsTestLogger(t)), Activity: coordinator,
	})

	jobID, err := operations.RunNow(context.Background(), nil)
	var busyErr *BusyError
	if !errors.As(err, &busyErr) {
		t.Fatalf("RunNow error = %v, want BusyError", err)
	}
	if jobID != "" {
		t.Fatalf("busy RunNow job ID = %q, want empty", jobID)
	}
}

func TestRunNowMarksPreemptedTrackedJobFailed(t *testing.T) {
	connection := newSQLite(t)
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	coordinator := activity.NewCoordinator(activity.Options{})
	provider := &preemptingSuccessfulProvider{
		coordinator: coordinator,
		lease:       make(chan *activity.TaskLease, 1),
	}
	tracker := jobs.NewTracker(nil, newOperationsTestLogger(t))
	operations := NewOperations(OperationsConfig{
		Settings: NewSettingsStore(rawSettings), Store: newStorageStore(t, pool),
		Jobs: tracker, Activity: coordinator, Providers: []CleanupProvider{provider},
	})

	jobID, err := operations.RunNow(context.Background(), nil)
	if err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	waitForJobState(t, tracker, jobID, jobs.StateFailed)
	select {
	case lease := <-provider.lease:
		lease.Release()
	case <-time.After(time.Second):
		t.Fatal("preempting task lease was not acquired")
	}
}

func TestSelectCleanupProvidersUsesExplicitCleanupOnlyForNamedResources(t *testing.T) {
	provider := &explicitRecordingCleanupProvider{}

	selected, err := selectCleanupProviders([]CleanupProvider{provider}, nil)
	if err != nil {
		t.Fatalf("select all providers: %v", err)
	}
	if _, err := selected[0].Cleanup(context.Background()); err != nil {
		t.Fatalf("normal cleanup: %v", err)
	}
	if provider.normalCalls != 1 || provider.explicitCalls != 0 {
		t.Fatalf("global cleanup calls = normal:%d explicit:%d", provider.normalCalls, provider.explicitCalls)
	}

	selected, err = selectCleanupProviders([]CleanupProvider{provider}, []string{"explicit", "explicit"})
	if err != nil {
		t.Fatalf("select named provider: %v", err)
	}
	if len(selected) != 1 {
		t.Fatalf("selected providers = %d, want duplicate resource de-duplicated", len(selected))
	}
	if _, err := selected[0].Cleanup(context.Background()); err != nil {
		t.Fatalf("explicit cleanup: %v", err)
	}
	if provider.normalCalls != 1 || provider.explicitCalls != 1 {
		t.Fatalf("named cleanup calls = normal:%d explicit:%d", provider.normalCalls, provider.explicitCalls)
	}
}

func TestDeleteQuarantineRejectsBeforeRetentionWithoutStartingDelete(t *testing.T) {
	connection := newSQLite(t)
	store := newStorageStore(t, db.NewPool(connection, connection))
	entry := testQuarantineEntry("retained-entry")
	entry.DeleteAfter = time.Now().UTC().Add(time.Hour)
	if err := store.CreateQuarantineEntry(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	quarantine := &recordingQuarantineController{}
	operations := NewOperations(OperationsConfig{
		Store: store, Jobs: jobs.NewTracker(nil, newOperationsTestLogger(t)), Quarantine: quarantine,
	})

	jobID, err := operations.DeleteQuarantine(context.Background(), entry.ID, "DELETE")
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("DeleteQuarantine error = %v, want ErrConflict", err)
	}
	if jobID != "" || quarantine.deleteCalls != 0 {
		t.Fatalf("early deletion started: job_id=%q delete_calls=%d", jobID, quarantine.deleteCalls)
	}
}

func TestAnalyzeForcesOverviewRefresh(t *testing.T) {
	connection := newSQLite(t)
	pool := db.NewPool(connection, connection)
	rawSettings, err := systemsettings.NewStore(pool)
	if err != nil {
		t.Fatalf("new settings store: %v", err)
	}
	overview := &recordingRefreshOverview{called: make(chan struct{})}
	tracker := jobs.NewTracker(nil, newOperationsTestLogger(t))
	operations := NewOperations(OperationsConfig{
		Settings: NewSettingsStore(rawSettings), Store: newStorageStore(t, pool),
		Jobs: tracker, Overview: overview,
	})

	jobID, err := operations.Analyze(context.Background())
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	select {
	case <-overview.called:
	case <-time.After(time.Second):
		t.Fatal("Analyze did not request an overview refresh")
	}
	if overview.refreshCalls != 1 {
		t.Fatalf("Refresh calls = %d, want 1", overview.refreshCalls)
	}
	if overview.summaryCalls != 0 {
		t.Fatalf("Summary calls = %d, want 0", overview.summaryCalls)
	}
	waitForJobState(t, tracker, jobID, jobs.StateSucceeded)
}

func waitForJobState(t *testing.T, tracker *jobs.Tracker, id string, state jobs.State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		job := tracker.Get(id)
		if job != nil && (job.State == jobs.StateSucceeded || job.State == jobs.StateFailed) {
			if job.State != state {
				t.Fatalf("job state=%q message=%q, want %q", job.State, job.Message, state)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
}

type recordingQuarantineController struct {
	deleteCalls int
}

type recordingRefreshOverview struct {
	called       chan struct{}
	refreshCalls int
	summaryCalls int
}

func (o *recordingRefreshOverview) Summary(context.Context) (Summary, error) {
	o.summaryCalls++
	close(o.called)
	return Summary{}, nil
}

func (o *recordingRefreshOverview) Refresh(context.Context) (OverviewSnapshot, error) {
	o.refreshCalls++
	close(o.called)
	return OverviewSnapshot{Summary: Summary{}}, nil
}

func (o *recordingRefreshOverview) Get(context.Context) (OverviewSnapshot, error) {
	return OverviewSnapshot{}, nil
}

func (o *recordingRefreshOverview) Capabilities(context.Context, StorageMaintenanceSettings) Capabilities {
	return Capabilities{}
}

type signallingCleanupProvider struct {
	called chan struct{}
}

type explicitRecordingCleanupProvider struct {
	normalCalls   int
	explicitCalls int
}

func (p *explicitRecordingCleanupProvider) Name() string { return "explicit" }

func (p *explicitRecordingCleanupProvider) Cleanup(context.Context) (map[string]any, error) {
	p.normalCalls++
	return nil, nil
}

func (p *explicitRecordingCleanupProvider) CleanupExplicit(context.Context) (map[string]any, error) {
	p.explicitCalls++
	return nil, nil
}

func (p *signallingCleanupProvider) Name() string { return "signalling" }

func (p *signallingCleanupProvider) Cleanup(context.Context) (map[string]any, error) {
	close(p.called)
	return map[string]any{"cleaned": true}, nil
}

func (c *recordingQuarantineController) Restore(context.Context, string) (QuarantineEntry, error) {
	return QuarantineEntry{}, nil
}

func (c *recordingQuarantineController) PermanentDelete(
	context.Context,
	string,
	string,
) (QuarantineEntry, error) {
	c.deleteCalls++
	return QuarantineEntry{}, nil
}

func newOperationsTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return log
}
