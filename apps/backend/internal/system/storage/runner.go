package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

type RunStore interface {
	CreateRun(ctx context.Context, run *MaintenanceRun) error
	TransitionRun(ctx context.Context, id string, next RunState, result json.RawMessage, message string) (MaintenanceRun, error)
}

type CleanupProvider interface {
	Name() string
	Cleanup(ctx context.Context) (map[string]any, error)
}

// SettingsSnapshotCleanupProvider receives the immutable settings captured when
// a maintenance run was queued. Providers use it to keep eligibility and
// retention decisions consistent for the lifetime of the run.
type SettingsSnapshotCleanupProvider interface {
	CleanupWithSettings(context.Context, StorageMaintenanceSettings) (map[string]any, error)
}

// ExplicitCleanupProvider may opt into behavior reserved for a specifically named manual run.
type ExplicitCleanupProvider interface {
	CleanupExplicit(ctx context.Context) (map[string]any, error)
}

type ExplicitlySelectedCleanupProvider interface {
	ExplicitlySelected() bool
}

type RunnerConfig struct {
	Activity  *activity.Coordinator
	Store     RunStore
	Providers []CleanupProvider
	Overview  OverviewInvalidator
	Force     bool
	NewID     func() string
	Now       func() time.Time
}

type Runner struct {
	activity  *activity.Coordinator
	store     RunStore
	providers []CleanupProvider
	overview  OverviewInvalidator
	force     bool
	newID     func() string
	now       func() time.Time
}

const terminalTransitionTimeout = 5 * time.Second

type BusyError struct {
	Resources      []activity.BusyResource `json:"busy_resources"`
	ForceAvailable bool                    `json:"force_available"`
}

func (e *BusyError) Error() string { return "storage cleanup is blocked by active Kandev work" }

func NewRunner(config RunnerConfig) *Runner {
	newID := config.NewID
	if newID == nil {
		newID = uuid.NewString
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &Runner{
		activity: config.Activity, store: config.Store, providers: config.Providers,
		overview: config.Overview, force: config.Force, newID: newID, now: now,
	}
}

func (r *Runner) Run(
	ctx context.Context,
	trigger RunTrigger,
	settings StorageMaintenanceSettings,
) (MaintenanceRun, error) {
	run, err := r.createRun(ctx, trigger, settings)
	if err != nil {
		return MaintenanceRun{}, err
	}
	if r.shouldRunGoCacheWhileBusy(trigger, settings) {
		return r.runGoCacheWhileBusy(ctx, run, trigger, settings)
	}
	quietPeriod := quietPeriodForTrigger(trigger, settings)
	lease, busy, err := r.acquireMaintenance(ctx, quietPeriod, trigger)
	if errors.Is(err, activity.ErrBusy) {
		result := marshalRunResult(map[string]any{"busy_resources": busy})
		run, transitionErr := r.transitionRun(ctx, run.ID, RunStateSkippedBusy, result, "host resources are busy")
		if transitionErr != nil {
			return MaintenanceRun{}, transitionErr
		}
		if trigger == RunTriggerManual {
			return run, &BusyError{
				Resources:      activity.BusyResourcesForKinds(busy),
				ForceAvailable: forceAvailable(busy),
			}
		}
		return run, nil
	}
	if err != nil {
		state := RunStateFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			state = RunStateCancelled
		}
		run, transitionErr := r.transitionRun(ctx, run.ID, state, nil, err.Error())
		if transitionErr != nil {
			return MaintenanceRun{}, transitionErr
		}
		return run, err
	}
	defer lease.Release()
	if _, err := r.transitionRun(ctx, run.ID, RunStateRunning, nil, ""); err != nil {
		return MaintenanceRun{}, err
	}
	result, runErr := r.runProviders(lease.Context(), settings, r.providers)
	return r.finishRun(ctx, run.ID, lease.Context(), result, runErr)
}

func (r *Runner) shouldRunGoCacheWhileBusy(
	trigger RunTrigger,
	settings StorageMaintenanceSettings,
) bool {
	if !settings.GoCache.AllowCleanupWhileBusy {
		return false
	}
	if trigger == RunTriggerScheduled && (!settings.Enabled || !settings.GoCache.Enabled) {
		return false
	}
	for _, provider := range r.providers {
		if provider.Name() != string(ResourceTypeGoCache) {
			continue
		}
		if settings.GoCache.Enabled {
			return true
		}
		if explicitlySelected, ok := provider.(ExplicitlySelectedCleanupProvider); ok && explicitlySelected.ExplicitlySelected() {
			return true
		}
	}
	return false
}

func (r *Runner) runGoCacheWhileBusy(
	ctx context.Context,
	run MaintenanceRun,
	trigger RunTrigger,
	settings StorageMaintenanceSettings,
) (MaintenanceRun, error) {
	releaseBusyMaintenance, busy, err := r.activity.TryAcquireMaintenanceWhileBusy(ctx)
	if errors.Is(err, activity.ErrBusy) {
		result := marshalRunResult(map[string]any{"busy_resources": activity.BusyResourcesForKinds(busy)})
		run, transitionErr := r.transitionRun(ctx, run.ID, RunStateSkippedBusy, result, "another maintenance run is active")
		if transitionErr != nil {
			return MaintenanceRun{}, transitionErr
		}
		if trigger == RunTriggerManual {
			return run, &BusyError{Resources: activity.BusyResourcesForKinds(busy)}
		}
		return run, nil
	}
	if err != nil {
		return r.finishRun(ctx, run.ID, ctx, nil, err)
	}
	if _, err := r.transitionRun(ctx, run.ID, RunStateRunning, nil, ""); err != nil {
		releaseBusyMaintenance()
		return MaintenanceRun{}, err
	}
	goProviders, otherProviders := partitionGoCacheProviders(r.providers)
	result, goErr := r.runProviders(ctx, settings, goProviders)
	releaseBusyMaintenance()
	if len(otherProviders) == 0 {
		return r.finishRun(ctx, run.ID, ctx, result, goErr)
	}
	lease, busy, err := r.acquireMaintenance(ctx, quietPeriodForTrigger(trigger, settings), trigger)
	if errors.Is(err, activity.ErrBusy) {
		result["skipped_providers"] = skippedProviders(otherProviders, busy)
		return r.finishRun(ctx, run.ID, ctx, result, goErr)
	}
	if err != nil {
		return r.finishRun(ctx, run.ID, ctx, result, errors.Join(goErr, err))
	}
	defer lease.Release()
	otherResult, otherErr := r.runProviders(lease.Context(), settings, otherProviders)
	for name, providerResult := range otherResult {
		result[name] = providerResult
	}
	return r.finishRun(ctx, run.ID, lease.Context(), result, errors.Join(goErr, otherErr))
}

func partitionGoCacheProviders(providers []CleanupProvider) ([]CleanupProvider, []CleanupProvider) {
	goCache := make([]CleanupProvider, 0, 1)
	others := make([]CleanupProvider, 0, len(providers))
	for _, provider := range providers {
		if provider.Name() == string(ResourceTypeGoCache) {
			goCache = append(goCache, provider)
			continue
		}
		others = append(others, provider)
	}
	return goCache, others
}

func skippedProviders(providers []CleanupProvider, busy []activity.Kind) map[string]any {
	skipped := make(map[string]any, len(providers))
	reason := "activity_busy"
	if len(busy) > 0 {
		quietPeriodOnly := true
		for _, kind := range busy {
			if kind != activity.KindQuietPeriod {
				quietPeriodOnly = false
				break
			}
		}
		if quietPeriodOnly {
			reason = "quiet_period"
		}
	}
	for _, provider := range providers {
		skipped[provider.Name()] = map[string]any{
			"reason": reason, "busy_resources": activity.BusyResourcesForKinds(busy),
		}
	}
	return skipped
}

func (r *Runner) acquireMaintenance(
	ctx context.Context,
	quietPeriod time.Duration,
	trigger RunTrigger,
) (*activity.MaintenanceLease, []activity.Kind, error) {
	if r.force && trigger == RunTriggerManual {
		return r.activity.TryAcquireMaintenanceForce(ctx)
	}
	return r.activity.TryAcquireMaintenance(ctx, quietPeriod)
}

func forceAvailable(kinds []activity.Kind) bool {
	for _, kind := range kinds {
		if kind == activity.KindMaintenanceRunning {
			return false
		}
	}
	return len(kinds) > 0
}

func quietPeriodForTrigger(trigger RunTrigger, settings StorageMaintenanceSettings) time.Duration {
	if trigger == RunTriggerManual {
		return 0
	}
	return time.Duration(settings.IdleForMinutes) * time.Minute
}

func (r *Runner) createRun(
	ctx context.Context,
	trigger RunTrigger,
	settings StorageMaintenanceSettings,
) (MaintenanceRun, error) {
	snapshot, err := json.Marshal(settings)
	if err != nil {
		return MaintenanceRun{}, fmt.Errorf("encode storage settings snapshot: %w", err)
	}
	run := MaintenanceRun{
		ID: r.newID(), Trigger: trigger, State: RunStateQueued,
		SettingsSnapshot: snapshot, Result: json.RawMessage(`{}`), StartedAt: r.now().UTC(),
	}
	if err := r.store.CreateRun(ctx, &run); err != nil {
		return MaintenanceRun{}, err
	}
	return run, nil
}

func (r *Runner) runProviders(
	ctx context.Context,
	settings StorageMaintenanceSettings,
	providers []CleanupProvider,
) (map[string]any, error) {
	results := make(map[string]any, len(providers))
	var errs []error
	for _, provider := range providers {
		if ctx.Err() != nil {
			break
		}
		var (
			providerResult map[string]any
			err            error
		)
		if snapshotProvider, ok := provider.(SettingsSnapshotCleanupProvider); ok {
			providerResult, err = snapshotProvider.CleanupWithSettings(ctx, settings)
		} else {
			providerResult, err = provider.Cleanup(ctx)
		}
		entry := map[string]any{"result": providerResult}
		if err != nil {
			entry["error"] = err.Error()
			errs = append(errs, fmt.Errorf("%s: %w", provider.Name(), err))
		}
		results[provider.Name()] = entry
	}
	return results, errors.Join(errs...)
}

func (r *Runner) finishRun(
	ctx context.Context,
	id string,
	maintenanceCtx context.Context,
	result map[string]any,
	runErr error,
) (MaintenanceRun, error) {
	state := RunStateSucceeded
	message := ""
	if maintenanceCtx.Err() != nil {
		state = RunStateCancelled
		message = "maintenance preempted by task activity"
		if runErr == nil {
			runErr = maintenanceCtx.Err()
		}
	} else if runErr != nil {
		state = RunStateFailed
		message = runErr.Error()
	}
	run, err := r.transitionRun(ctx, id, state, marshalRunResult(result), message)
	if err != nil {
		return MaintenanceRun{}, err
	}
	_, goCacheResultPresent := result[string(ResourceTypeGoCache)]
	if r.overview != nil && (state == RunStateSucceeded || goCacheResultPresent) {
		r.overview.Invalidate()
	}
	return run, runErr
}

func (r *Runner) transitionRun(
	ctx context.Context,
	id string,
	state RunState,
	result json.RawMessage,
	message string,
) (MaintenanceRun, error) {
	transitionCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), terminalTransitionTimeout)
	defer cancel()
	return r.store.TransitionRun(transitionCtx, id, state, result, message)
}

func marshalRunResult(result any) json.RawMessage {
	encoded, err := json.Marshal(result)
	if err != nil {
		return json.RawMessage(`{"error":"encode maintenance result"}`)
	}
	return encoded
}
