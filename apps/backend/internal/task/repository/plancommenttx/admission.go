package plancommenttx

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

type admissionLeaseContextKey struct{}

type admissionLeaseContext struct {
	parent       *admissionLeaseContext
	taskID       string
	databaseLock bool
}

type localAdmissionGuard struct {
	token chan struct{}
	refs  int
}

var localAdmissionGuards = struct {
	sync.Mutex
	byTask map[string]*localAdmissionGuard
}{byTask: make(map[string]*localAdmissionGuard)}

// AcquireLocalAdmission serializes the preflight-to-commit window with plan
// comment mutations in this process. Database advisory locks extend the same
// boundary across PostgreSQL processes.
func AcquireLocalAdmission(ctx context.Context, taskID string) (func(), error) {
	if taskID == "" {
		return nil, errors.New("task id is required for plan-comment admission")
	}
	if LocalAdmissionHeld(ctx, taskID) {
		return func() {}, nil
	}
	localAdmissionGuards.Lock()
	guard := localAdmissionGuards.byTask[taskID]
	if guard == nil {
		guard = &localAdmissionGuard{token: make(chan struct{}, 1)}
		guard.token <- struct{}{}
		localAdmissionGuards.byTask[taskID] = guard
	}
	guard.refs++
	localAdmissionGuards.Unlock()

	select {
	case <-ctx.Done():
		releaseLocalAdmissionReference(taskID, guard)
		return nil, ctx.Err()
	case <-guard.token:
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			guard.token <- struct{}{}
			releaseLocalAdmissionReference(taskID, guard)
		})
	}, nil
}

func releaseLocalAdmissionReference(taskID string, guard *localAdmissionGuard) {
	localAdmissionGuards.Lock()
	defer localAdmissionGuards.Unlock()
	guard.refs--
	if guard.refs == 0 && localAdmissionGuards.byTask[taskID] == guard {
		delete(localAdmissionGuards.byTask, taskID)
	}
}

// WithAdmissionLease marks ctx as owning taskID's long admission guard so the
// final repository transaction does not deadlock by reacquiring it.
func WithAdmissionLease(ctx context.Context, taskID string) context.Context {
	return withAdmissionContext(ctx, taskID, true)
}

// WithLocalAdmission marks ctx as owning only the process-local task guard.
// The final PostgreSQL transaction must still acquire its advisory lock.
func WithLocalAdmission(ctx context.Context, taskID string) context.Context {
	return withAdmissionContext(ctx, taskID, false)
}

func withAdmissionContext(ctx context.Context, taskID string, databaseLock bool) context.Context {
	parent, _ := ctx.Value(admissionLeaseContextKey{}).(*admissionLeaseContext)
	return context.WithValue(ctx, admissionLeaseContextKey{}, &admissionLeaseContext{
		parent: parent, taskID: taskID, databaseLock: databaseLock,
	})
}

// LocalAdmissionHeld reports whether ctx owns taskID's process-local guard.
func LocalAdmissionHeld(ctx context.Context, taskID string) bool {
	lease, _ := ctx.Value(admissionLeaseContextKey{}).(*admissionLeaseContext)
	for lease != nil {
		if lease.taskID == taskID {
			return true
		}
		lease = lease.parent
	}
	return false
}

// AdmissionLeaseHeld reports whether ctx owns taskID's cross-process database
// guard in addition to its process-local guard.
func AdmissionLeaseHeld(ctx context.Context, taskID string) bool {
	lease, _ := ctx.Value(admissionLeaseContextKey{}).(*admissionLeaseContext)
	for lease != nil {
		if lease.taskID == taskID && lease.databaseLock {
			return true
		}
		lease = lease.parent
	}
	return false
}

// LockAdmissionTransaction joins a normal mutation to the cross-process
// PostgreSQL admission lock. A long lease already held by ctx owns that lock.
func LockAdmissionTransaction(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	taskID string,
) error {
	if AdmissionLeaseHeld(ctx, taskID) || !dialect.IsPostgres(db.DriverName()) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, taskID); err != nil {
		return fmt.Errorf("lock PostgreSQL plan-comment admission: %w", err)
	}
	return nil
}
