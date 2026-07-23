package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
	"github.com/kandev/kandev/internal/system/jobs"
)

const (
	JobKindAnalysis         = "storage-analysis"
	JobKindCleanup          = "storage-cleanup"
	JobKindQuarantineDelete = "storage-quarantine-delete"
)

type GoCacheAdopter interface {
	ValidateAdoption(context.Context, string, string) error
}

type QuarantineController interface {
	Restore(context.Context, string) (QuarantineEntry, error)
	PermanentDelete(context.Context, string, string) (QuarantineEntry, error)
}

type OperationsConfig struct {
	Settings   *SettingsStore
	Store      *Store
	Jobs       *jobs.Tracker
	Activity   *activity.Coordinator
	Providers  []CleanupProvider
	Overview   OverviewRefresher
	GoCache    GoCacheAdopter
	Quarantine QuarantineController
}

type Operations struct {
	config OperationsConfig
}

func NewOperations(config OperationsConfig) *Operations {
	return &Operations{config: config}
}

func (o *Operations) AdoptGoCache(
	ctx context.Context,
	path string,
	confirmation string,
) (StorageMaintenanceSettings, Capabilities, error) {
	if o.config.GoCache == nil {
		return StorageMaintenanceSettings{}, Capabilities{}, errors.New("go-cache provider is unavailable")
	}
	if err := o.config.GoCache.ValidateAdoption(ctx, path, confirmation); err != nil {
		return StorageMaintenanceSettings{}, Capabilities{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	settings, err := o.config.Settings.AdoptGoCachePath(ctx, path)
	if err != nil {
		return StorageMaintenanceSettings{}, Capabilities{}, err
	}
	return settings, o.config.Overview.Capabilities(ctx, settings), nil
}

func (o *Operations) Analyze(ctx context.Context) (string, error) {
	settings, err := o.config.Settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	return o.startTracked(ctx, JobKindAnalysis, func(jobCtx context.Context, id string) (map[string]any, error) {
		if err := o.createRun(jobCtx, id, RunTriggerAnalysis, settings); err != nil {
			return nil, err
		}
		if _, err := o.config.Store.TransitionRun(jobCtx, id, RunStateRunning, nil, ""); err != nil {
			return nil, err
		}
		snapshot, analyzeErr := o.config.Overview.Refresh(jobCtx)
		return o.finishAnalysis(jobCtx, id, snapshot.Summary, analyzeErr)
	}), nil
}

type OverviewRefresher interface {
	OverviewReader
	Refresh(context.Context) (OverviewSnapshot, error)
}

func (o *Operations) RunNow(ctx context.Context, resources []string) (string, error) {
	settings, err := o.config.Settings.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	providers, err := selectCleanupProviders(o.config.Providers, resources)
	if err != nil {
		return "", err
	}
	if err := o.preflight(ctx); err != nil {
		return "", err
	}
	return o.startTracked(ctx, JobKindCleanup, func(jobCtx context.Context, id string) (map[string]any, error) {
		runner := NewRunner(RunnerConfig{
			Activity: o.config.Activity, Store: o.config.Store, Providers: providers,
			NewID: func() string { return id },
		})
		run, runErr := runner.Run(jobCtx, RunTriggerManual, settings)
		return runResultMap(run), runErr
	}), nil
}

func (o *Operations) RestoreQuarantine(ctx context.Context, id string) (QuarantineEntry, error) {
	if o.config.Quarantine == nil {
		return QuarantineEntry{}, errors.New("quarantine provider is unavailable")
	}
	return o.config.Quarantine.Restore(ctx, id)
}

func (o *Operations) DeleteQuarantine(ctx context.Context, id, confirmation string) (string, error) {
	if confirmation != "DELETE" {
		return "", validationError("quarantine deletion requires DELETE confirmation")
	}
	if o.config.Quarantine == nil {
		return "", errors.New("quarantine provider is unavailable")
	}
	entry, err := o.config.Store.GetQuarantineEntry(ctx, id)
	if err != nil {
		return "", err
	}
	if time.Now().UTC().Before(entry.DeleteAfter) {
		return "", fmt.Errorf("%w: quarantine retention deadline has not elapsed", ErrConflict)
	}
	return o.startTracked(ctx, JobKindQuarantineDelete, func(jobCtx context.Context, _ string) (map[string]any, error) {
		entry, err := o.config.Quarantine.PermanentDelete(jobCtx, id, confirmation)
		return map[string]any{"entry": entry}, err
	}), nil
}

func (o *Operations) preflight(ctx context.Context) error {
	lease, busy, err := o.config.Activity.TryAcquireMaintenance(ctx, 0)
	if errors.Is(err, activity.ErrBusy) {
		return &BusyError{Resources: busy}
	}
	if err != nil {
		return err
	}
	lease.Release()
	return nil
}

func (o *Operations) startTracked(
	ctx context.Context,
	kind string,
	fn func(context.Context, string) (map[string]any, error),
) string {
	ready := make(chan struct{})
	var id string
	jobCtx := context.WithoutCancel(ctx)
	id = o.config.Jobs.Start(jobCtx, kind, func(runCtx context.Context) (map[string]any, error) {
		<-ready
		return fn(runCtx, id)
	})
	close(ready)
	return id
}

func (o *Operations) createRun(
	ctx context.Context,
	id string,
	trigger RunTrigger,
	settings StorageMaintenanceSettings,
) error {
	snapshot, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return o.config.Store.CreateRun(ctx, &MaintenanceRun{
		ID: id, Trigger: trigger, State: RunStateQueued,
		SettingsSnapshot: snapshot, Result: json.RawMessage(`{}`), StartedAt: time.Now().UTC(),
	})
}

func (o *Operations) finishAnalysis(
	ctx context.Context,
	id string,
	summary Summary,
	analyzeErr error,
) (map[string]any, error) {
	result := valueMap(summary)
	state := RunStateSucceeded
	message := ""
	if analyzeErr != nil {
		state = RunStateFailed
		message = analyzeErr.Error()
	}
	_, transitionErr := o.config.Store.TransitionRun(ctx, id, state, marshalRunResult(result), message)
	if transitionErr != nil {
		return nil, transitionErr
	}
	return result, analyzeErr
}

func selectCleanupProviders(all []CleanupProvider, requested []string) ([]CleanupProvider, error) {
	if len(requested) == 0 {
		return all, nil
	}
	byName := make(map[string]CleanupProvider, len(all))
	for _, provider := range all {
		byName[provider.Name()] = provider
	}
	selected := make([]CleanupProvider, 0, len(requested))
	seen := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		provider, ok := byName[name]
		if !ok {
			return nil, validationError("unknown storage resource %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		if explicit, ok := provider.(ExplicitCleanupProvider); ok {
			provider = explicitCleanupSelection{CleanupProvider: provider, explicit: explicit}
		}
		selected = append(selected, provider)
	}
	return selected, nil
}

type explicitCleanupSelection struct {
	CleanupProvider
	explicit ExplicitCleanupProvider
}

func (p explicitCleanupSelection) Cleanup(ctx context.Context) (map[string]any, error) {
	return p.explicit.CleanupExplicit(ctx)
}

func runResultMap(run MaintenanceRun) map[string]any {
	result := valueMap(run.Result)
	result["run_id"] = run.ID
	result["state"] = run.State
	return result
}

func valueMap(value any) map[string]any {
	encoded, _ := json.Marshal(value)
	result := make(map[string]any)
	_ = json.Unmarshal(encoded, &result)
	return result
}
