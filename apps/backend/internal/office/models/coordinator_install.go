package models

import (
	"context"
	"errors"
)

// ErrCoordinatorInstallContention reports that a coordinator install's
// serialized section (internal/office/repository/sqlite.Repository.
// InstallCoordinatorRoutine) could not be entered: its acquisition bound
// was reached before the cross-process lock was won
// (AC-OFFICE-COORDINATOR-INSTALL-001.9/.13). Declared here, not in the
// sqlite package that raises it, so internal/office/routines can classify
// it via errors.Is without importing sqlite — the same reason
// CoordinatorInstallTx lives here. Distinguishable from a plain
// identity-lookup/trigger-read failure (AC-OFFICE-COORDINATOR-INSTALL-001.7)
// and from the caller's own context being cancelled while waiting, which
// surfaces as ctx.Err() (context.Canceled/context.DeadlineExceeded)
// instead of this sentinel.
var ErrCoordinatorInstallContention = errors.New("coordinator install: lock contention")

// CoordinatorInstallTx is the transaction-scoped write surface the
// coordinator-install path (internal/office/routines) uses to read a
// matched routine's triggers and, when needed, create the routine and/or
// its canonical trigger — all inside the single transaction
// internal/office/repository/sqlite.Repository.InstallCoordinatorRoutine
// opens and locks for one (workspace, assignee, canonical name) identity.
//
// Declared here, rather than in internal/office/routines where the install
// logic lives, so the sqlite repository can implement it without importing
// routines, and routines can require it on its own Repository interface
// without importing sqlite.
type CoordinatorInstallTx interface {
	ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*RoutineTrigger, error)
	CreateRoutine(ctx context.Context, routine *Routine) error
	CreateRoutineTrigger(ctx context.Context, t *RoutineTrigger) error
}
