package sqlite

import (
	"context"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/plancommenttx"
)

// AcquirePlanCommentAdmission holds one task-scoped guard from preflight
// through final message acceptance. PostgreSQL uses a session advisory lock so
// turn-start hooks may run their own database transactions without holding a
// task row lock open.
func (r *Repository) AcquirePlanCommentAdmission(
	ctx context.Context,
	taskID string,
) (context.Context, func(), error) {
	return r.acquireCrossProcessAdmission(ctx, taskID, taskID)
}

// AcquireMessageAdmission serializes one caller-owned message identity across
// backend processes. The caller keeps this lease through mutable turn-start
// hooks and final persistence, so only the request owning the ID may run them.
func (r *Repository) AcquireMessageAdmission(
	ctx context.Context,
	messageID string,
) (context.Context, func(), error) {
	if messageID == "" {
		return nil, nil, fmt.Errorf("message id is required for admission")
	}
	key := "message:" + messageID
	return r.acquireCrossProcessAdmission(ctx, key, key)
}

// AcquirePlanCommentAndMessageAdmission locks task-comment mutations and one
// caller-owned message ID in a stable order. PostgreSQL holds both advisory
// locks on one connection, leaving a second pooled connection for hook and
// admission transactions.
func (r *Repository) AcquirePlanCommentAndMessageAdmission(
	ctx context.Context,
	taskID, messageID string,
) (context.Context, func(), error) {
	if taskID == "" || messageID == "" {
		return nil, nil, fmt.Errorf("task id and message id are required for admission")
	}
	messageKey := "message:" + messageID
	taskHeld := plancommenttx.AdmissionLeaseHeld(ctx, taskID)
	messageHeld := plancommenttx.AdmissionLeaseHeld(ctx, messageKey)
	if taskHeld || messageHeld {
		if taskHeld && messageHeld {
			return ctx, func() {}, nil
		}
		return nil, nil, fmt.Errorf("combined plan-comment message admission is only partially held")
	}

	taskRelease, err := plancommenttx.AcquireLocalAdmission(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	messageRelease, err := plancommenttx.AcquireLocalAdmission(ctx, messageKey)
	if err != nil {
		taskRelease()
		return nil, nil, err
	}
	releaseLocal := func() {
		messageRelease()
		taskRelease()
	}
	leaseCtx := plancommenttx.WithAdmissionLease(ctx, taskID)
	leaseCtx = plancommenttx.WithAdmissionLease(leaseCtx, messageKey)
	if !dialect.IsPostgres(r.db.DriverName()) {
		return leaseCtx, releaseLocal, nil
	}
	return r.acquirePostgresCombinedAdmission(ctx, leaseCtx, []string{taskID, messageKey}, releaseLocal)
}

func (r *Repository) acquirePostgresCombinedAdmission(
	ctx, leaseCtx context.Context,
	keys []string,
	releaseLocal func(),
) (context.Context, func(), error) {
	if r.db.Stats().MaxOpenConnections == 1 {
		releaseLocal()
		return nil, nil, fmt.Errorf("postgresql plan-comment admission requires at least two database connections")
	}
	conn, err := r.db.Connx(ctx)
	if err != nil {
		releaseLocal()
		return nil, nil, fmt.Errorf("reserve PostgreSQL plan-comment connection: %w", err)
	}
	lockedKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, key); err != nil {
			if !unlockPostgresAdmissionKeys(conn, lockedKeys) {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
			releaseLocal()
			return nil, nil, fmt.Errorf("lock PostgreSQL plan-comment admission: %w", err)
		}
		lockedKeys = append(lockedKeys, key)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if !unlockPostgresAdmissionKeys(conn, lockedKeys) {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
			releaseLocal()
		})
	}
	return leaseCtx, release, nil
}

func unlockPostgresAdmissionKeys(conn *sqlx.Conn, keys []string) bool {
	unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	unlockedAll := true
	for index := len(keys) - 1; index >= 0; index-- {
		var unlocked bool
		err := conn.QueryRowxContext(
			unlockCtx, `SELECT pg_advisory_unlock(hashtextextended($1, 0))`, keys[index],
		).Scan(&unlocked)
		if err != nil || !unlocked {
			unlockedAll = false
		}
	}
	return unlockedAll
}

func (r *Repository) acquireCrossProcessAdmission(
	ctx context.Context,
	localKey, databaseKey string,
) (context.Context, func(), error) {
	if plancommenttx.AdmissionLeaseHeld(ctx, localKey) {
		return ctx, func() {}, nil
	}
	localRelease, err := plancommenttx.AcquireLocalAdmission(ctx, localKey)
	if err != nil {
		return nil, nil, err
	}
	if !dialect.IsPostgres(r.db.DriverName()) {
		return plancommenttx.WithAdmissionLease(ctx, localKey), localRelease, nil
	}
	if r.db.Stats().MaxOpenConnections == 1 {
		localRelease()
		return nil, nil, fmt.Errorf("postgresql plan-comment admission requires at least two database connections")
	}
	conn, err := r.db.Connx(ctx)
	if err != nil {
		localRelease()
		return nil, nil, fmt.Errorf("reserve PostgreSQL plan-comment connection: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(hashtextextended($1, 0))`, databaseKey); err != nil {
		_ = conn.Close()
		localRelease()
		return nil, nil, fmt.Errorf("lock PostgreSQL plan-comment admission: %w", err)
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			if !unlockPostgresAdmissionKeys(conn, []string{databaseKey}) {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
			_ = conn.Close()
			localRelease()
		})
	}
	return plancommenttx.WithAdmissionLease(ctx, localKey), release, nil
}
