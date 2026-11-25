package sqlite

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
)

// ExecutorBindingGuard participates in the caller's transaction so a managed
// connection reference and its saved executor cannot diverge.
type ExecutorBindingGuard interface {
	BindExecutorIn(context.Context, *sqlx.Tx, string, string, map[string]string) error
}

// SetExecutorBindingGuard is configured before serving requests.
func (r *Repository) SetExecutorBindingGuard(guard ExecutorBindingGuard) {
	r.executorBindingGuard = guard
}

func (r *Repository) beginExecutorWrite(ctx context.Context, id, kind string, config map[string]string) (*sqlx.Tx, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	if r.executorBindingGuard != nil {
		err = r.executorBindingGuard.BindExecutorIn(ctx, tx, id, kind, config)
	} else if config["ssh_connection_id"] != "" {
		err = fmt.Errorf("managed SSH connections unavailable")
	}
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	return tx, nil
}

type runtimeReferenceGuard interface {
	ReleaseRuntimeIn(context.Context, *sqlx.Tx, string) error
}

func (r *Repository) releaseRuntimeReferenceIn(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if guard, ok := r.executorBindingGuard.(runtimeReferenceGuard); ok {
		return guard.ReleaseRuntimeIn(ctx, tx, sessionID)
	}
	return nil
}
