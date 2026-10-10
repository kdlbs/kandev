package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	restoremetrics "github.com/kandev/kandev/internal/agent/runtime/lifecycle/metrics"
	"github.com/kandev/kandev/internal/task/models"
)

// ErrSessionRecoveryRequired is returned before prompt admission when the
// session has an unresolved recovery block.
var ErrSessionRecoveryRequired = errors.New("session recovery required")

// SessionRecoveryBlockedError keeps the operator-visible identity without
// exposing snapshot text or provider paths.
type SessionRecoveryBlockedError struct {
	Block *models.SessionRecoveryBlock
}

func (e *SessionRecoveryBlockedError) Error() string {
	if e == nil || e.Block == nil {
		return ErrSessionRecoveryRequired.Error()
	}
	return fmt.Sprintf("%s: %s", ErrSessionRecoveryRequired, e.Block.Reason)
}

func (e *SessionRecoveryBlockedError) Unwrap() error { return ErrSessionRecoveryRequired }

// RecoveryAdmission owns the shared admission check used by interactive and
// autonomous callers. It never resolves a block as a side effect of reading.
type SessionRecoveryStore interface {
	UpsertSessionRecoveryBlock(context.Context, *models.SessionRecoveryBlock) error
	GetOpenSessionRecoveryBlock(context.Context, string, string, int64) (*models.SessionRecoveryBlock, error)
	ResolveSessionRecoveryBlock(context.Context, string, string, time.Time) (bool, error)
}

type RecoveryAdmission struct {
	Store SessionRecoveryStore
	Now   func() time.Time
}

func (a *RecoveryAdmission) Check(ctx context.Context, sessionID, incarnationID string, generation int64) error {
	if a == nil || a.Store == nil {
		return nil
	}
	block, err := a.Store.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, generation)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("check session recovery block: %w", err)
	}
	if err != nil {
		if isNoRows(err) {
			return nil
		}
		return fmt.Errorf("check session recovery block: %w", err)
	}
	return &SessionRecoveryBlockedError{Block: block}
}

func (a *RecoveryAdmission) Block(ctx context.Context, identity RestoreIdentity, reason, consumer string) (*models.SessionRecoveryBlock, error) {
	if a == nil || a.Store == nil {
		return nil, fmt.Errorf("session recovery store is required")
	}
	now := time.Now().UTC()
	if a.Now != nil {
		now = a.Now().UTC()
	}
	block := &models.SessionRecoveryBlock{
		SessionID:          identity.SessionID,
		IncarnationID:      identity.IncarnationID,
		ExpectedGeneration: int64(identity.HarnessGeneration),
		Reason:             reason,
		State:              models.RecoveryBlockOpen,
		ConsumerReference:  consumer,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := a.Store.UpsertSessionRecoveryBlock(ctx, block); err != nil {
		return nil, err
	}
	isNew := true
	if existing, err := a.Store.GetOpenSessionRecoveryBlock(
		ctx, block.SessionID, block.IncarnationID, block.ExpectedGeneration,
	); err == nil && existing != nil && existing.ID != block.ID {
		isNew = false
	}
	if isNew {
		restoremetrics.RecordRecoveryRequired(consumer, reason)
	}
	return block, nil
}

func (a *RecoveryAdmission) Resolve(ctx context.Context, blockID, action string) (bool, error) {
	now := time.Now().UTC()
	if a != nil && a.Now != nil {
		now = a.Now().UTC()
	}
	if a == nil || a.Store == nil {
		return false, fmt.Errorf("session recovery store is required")
	}
	return a.Store.ResolveSessionRecoveryBlock(ctx, blockID, action, now)
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
