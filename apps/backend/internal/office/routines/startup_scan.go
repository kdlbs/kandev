package routines

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/logger"

	"go.uber.org/zap"
)

// ReconcileSignal proves Office startup reconciliation has returned. Only
// SignalReconcileComplete, called after infra.Reconciler.ReconcileAll
// returns, can construct a received signal — a zero-value ReconcileSignal
// (as an unauthorized caller, or a test proving the refusal path, would
// hold) carries no completion. RunStartupScan uses this instead of a timer
// or an inspection of trigger rows to order itself after reconciliation
// (AC-OFFICE-ROUTINE-ARMING-003.1).
type ReconcileSignal struct{ received bool }

// SignalReconcileComplete constructs a received ReconcileSignal. Call this
// only after ReconcileAll has returned.
func SignalReconcileComplete() ReconcileSignal {
	return ReconcileSignal{received: true}
}

// StartupScanReader is the read surface RunStartupScan needs: workspace and
// routine enumeration, plus Task 01's trigger reads for classification.
type StartupScanReader interface {
	RoutineTriggerReader
	ListWorkspaceIDsOrdered(ctx context.Context) ([]string, error)
	ListRoutinesOrdered(ctx context.Context, workspaceID string) ([]*Routine, error)
}

// RunStartupScan performs one read-only pass over every enumerable routine
// in every enumerable workspace, classifying each via Task 01 and emitting a
// structured log record plus expvar counters per REQ-OFFICE-ROUTINE-ARMING-003.
// It writes nothing, returns nothing, and never blocks its caller past its
// own execution: no outcome here can fail or change Office startup
// (AC-OFFICE-ROUTINE-ARMING-003.10), which is why this is meant to be
// launched with `go` after the signal is available.
func RunStartupScan(ctx context.Context, signal ReconcileSignal, reader StartupScanReader, log *logger.Logger, now time.Time) {
	if !signal.received {
		armingScanSkipped(armingSkipNoSignal)
		log.Warn("office routine arming scan: no reconciliation completion signal")
		return
	}

	workspaceIDs, err := reader.ListWorkspaceIDsOrdered(ctx)
	if err != nil {
		armingScanSkipped(armingSkipWorkspaceEnumFailed)
		log.Warn("office routine arming scan: failed to enumerate workspaces", zap.Error(err))
		return
	}

	for _, workspaceID := range workspaceIDs {
		scanWorkspace(ctx, reader, log, now, workspaceID)
	}
}

// scanWorkspace classifies and records every routine in one workspace. A
// failure to list the workspace's routines is recorded and skipped without
// retry or abandoning the scan (AC-OFFICE-ROUTINE-ARMING-003.13).
func scanWorkspace(ctx context.Context, reader StartupScanReader, log *logger.Logger, now time.Time, workspaceID string) {
	routineList, err := reader.ListRoutinesOrdered(ctx, workspaceID)
	if err != nil {
		armingScanSkipped(armingSkipRoutineEnumFailed)
		log.Warn("office routine arming scan: failed to enumerate workspace routines",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return
	}
	if len(routineList) == 0 {
		return
	}

	ids := make([]string, len(routineList))
	for i, r := range routineList {
		ids[i] = r.ID
	}
	// ClassifyRoutines never returns a top-level error: a batch read failure
	// falls back to a per-routine re-read (Task 01), and a routine whose own
	// re-read also fails carries ScheduleStateUnknown in the result map
	// instead (AC-OFFICE-ROUTINE-ARMING-003.9), so there is nothing to check
	// here beyond the map it returns.
	classifications, _ := ClassifyRoutines(ctx, reader, ids, now)

	for _, routine := range routineList {
		recordRoutine(log, workspaceID, routine, classifications[routine.ID])
	}
}

// cannotFireStates are the AC-OFFICE-ROUTINE-ARMING-003.3 schedule states: an
// active routine that owns a cron trigger which cannot currently fire it.
var cannotFireStates = map[ScheduleState]bool{
	ScheduleStateTriggerDisabled:    true,
	ScheduleStateTriggerUnscheduled: true,
	ScheduleStateTriggerInvalid:     true,
}

// noScheduleStates are the AC-OFFICE-ROUTINE-ARMING-003.14 schedule states:
// an active routine that owns no cron trigger at all.
var noScheduleStates = map[ScheduleState]bool{
	ScheduleStateUnscheduledManualOnly: true,
	ScheduleStateUnscheduledNoTrigger:  true,
}

const routineIntentActive = "active"

// recordRoutine emits exactly one log record for one routine reached by the
// scan, and the AC-OFFICE-ROUTINE-ARMING-003.6 observation counter.
func recordRoutine(log *logger.Logger, workspaceID string, routine *Routine, c RoutineClassification) {
	armingScanObserved(workspaceID, routine.Status, c.State)

	fields := []zap.Field{
		zap.String("workspace_id", workspaceID),
		zap.String("routine_id", routine.ID),
		zap.String("schedule_state", string(c.State)),
	}

	if c.State == ScheduleStateUnknown {
		armingScanSkipped(armingSkipClassificationUnknown)
		log.Info("office routine arming scan: schedule state unknown", fields...)
		return
	}

	if routine.Status != routineIntentActive {
		log.Info("office routine arming scan: routine intent is not active", fields...)
		return
	}

	recordActiveRoutine(log, fields, c)
}

// recordActiveRoutine emits the AC-OFFICE-ROUTINE-ARMING-003.3/.5/.14 record
// for a routine whose intent is active.
func recordActiveRoutine(log *logger.Logger, fields []zap.Field, c RoutineClassification) {
	switch {
	case cannotFireStates[c.State]:
		fields = append(fields, zap.Any("unarmed_cron_triggers", c.Unarmed))
		log.Warn("office routine arming scan: routine cannot fire", fields...)
	case noScheduleStates[c.State]:
		log.Warn("office routine arming scan: routine has no schedule", fields...)
	default:
		log.Info("office routine arming scan: routine observed", fields...)
	}
}
