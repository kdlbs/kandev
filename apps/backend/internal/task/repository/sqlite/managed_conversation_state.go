package sqlite

import (
	"context"
	"errors"
	"strconv"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	managed "github.com/kandev/kandev/internal/task/repository/managedconversation"
)

func (r *Repository) ChangeManagedConversationState(ctx context.Context, input managed.StateRequest) (managed.Result, error) {
	tx, task, err := r.beginManagedAdmission(ctx, input.Identity)
	if err != nil {
		return managed.Result{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if task == nil {
		return managed.Result{}, managed.ErrNotFound
	}
	if task.Metadata[models.MetaKeyManagedConversationDetached] == true && input.Kind != managed.Detach {
		return managed.Result{}, managed.ErrNotFound
	}
	values, changed, replayed, err := managedStateValues(task, input)
	if err != nil {
		return managed.Result{}, err
	}
	primary, err := r.managedStatePrimaryTx(ctx, tx, task, input, replayed)
	if err != nil {
		return managed.Result{}, err
	}
	if len(values) != 0 {
		if err := r.patchManagedTaskTx(ctx, tx, task, values); err != nil {
			return managed.Result{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return managed.Result{}, err
	}
	if err := tx.Commit(); err != nil {
		return managed.Result{}, managedContextError(ctx, err)
	}
	return managed.Result{Task: task, Primary: primary, Changed: changed, Replayed: replayed}, nil
}

func (r *Repository) managedStatePrimaryTx(ctx context.Context, tx *sqlx.Tx, task *models.Task, input managed.StateRequest, replayed bool) (*models.TaskSession, error) {
	primary, err := r.managedPrimaryTx(ctx, tx, task.ID)
	if err != nil || input.Kind != managed.PauseExact || primary != nil {
		return primary, err
	}
	if !replayed {
		return nil, ErrNoPrimarySession
	}
	return r.repairManagedPrimaryTx(ctx, tx, task, input.PrimaryID, managed.FromTask(task))
}

func validateManagedStateOperation(task *models.Task, input managed.StateRequest) (bool, error) {
	if input.Kind != managed.PauseExact {
		return false, nil
	}
	if input.OperationID == "" || input.PayloadDigest == "" {
		return false, errors.New("managed state operation identity is required")
	}
	if managed.Replay(task, input.OperationID, input.PayloadDigest) {
		return true, nil
	}
	if managed.Revision(task) != input.ExpectedRevision {
		return false, managed.ErrRevision
	}
	return false, nil
}

func managedStateValues(task *models.Task, input managed.StateRequest) (map[string]interface{}, bool, bool, error) {
	replayed, err := validateManagedStateOperation(task, input)
	if err != nil || replayed {
		return nil, false, replayed, err
	}
	values := make(map[string]interface{})
	changed := false
	switch input.Kind {
	case managed.PauseExact, managed.PauseInstallation:
		paused := input.Paused || input.Kind == managed.PauseInstallation
		if current, _ := task.Metadata[models.MetaKeyManagedConversationPaused].(bool); current != paused {
			values[models.MetaKeyManagedConversationPaused], changed = paused, true
		}
	case managed.Invalidate:
		if task.Metadata[models.MetaKeyManagedPolicyInvalidated] != true {
			values[models.MetaKeyManagedPolicyInvalidated], changed = true, true
			values[managed.OperationKey], values[managed.PayloadKey] = "", ""
		}
	case managed.Detach:
		if task.Metadata[models.MetaKeyManagedConversationDetached] != true {
			values[models.MetaKeyManagedConversationDetached], changed = true, true
			values[models.MetaKeyManagedConversationPaused] = true
		}
	default:
		return nil, false, false, errors.New("unknown managed state intent")
	}
	revision := managed.Revision(task)
	if changed {
		revision++
		values[models.MetaKeyManagedConversationRevision] = strconv.FormatUint(revision, 10)
	}
	if input.Kind == managed.PauseExact {
		stampManagedOperation(values, revision, input.OperationID, input.PayloadDigest)
	}
	return values, changed, false, nil
}
