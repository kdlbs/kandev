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

// ExplicitCleanupProvider may opt into behavior reserved for a specifically named manual run.
type ExplicitCleanupProvider interface {
	CleanupExplicit(ctx context.Context) (map[string]any, error)
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
	result, runErr := r.runProviders(lease.Context())
	return r.finishRun(ctx, run.ID, lease.Context(), result, runErr)
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

func (r *Runner) runProviders(ctx context.Context) (map[string]any, error) {
	results := make(map[string]any, len(r.providers))
	var errs []error
	for _, provider := range r.providers {
		if ctx.Err() != nil {
			break
		}
		providerResult, err := provider.Cleanup(ctx)
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
	if state == RunStateSucceeded && r.overview != nil {
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
