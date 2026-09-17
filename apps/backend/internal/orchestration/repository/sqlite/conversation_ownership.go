package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/db"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (r *Repository) PersonaUserOwner(ctx context.Context, id string) (string, error) {
	var owner string
	err := r.db.GetContext(ctx, &owner, r.db.Rebind(`SELECT COALESCE(MAX(owner_user_id),'')
		FROM orchestration_conversations WHERE agent_profile_id=?`), id)
	return owner, err
}

// AuthorizePersona requires a human identity for a private configuration.
// A runtime identity or an identity-free scheduled delivery is not its owner.
func (r *Repository) AuthorizePersona(ctx context.Context, id string) error {
	owner, err := r.PersonaUserOwner(ctx, id)
	if err != nil {
		return err
	}
	identity, ok := authn.IdentityFromContext(ctx)
	if owner != "" && (!ok || identity.UserID != owner) {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

// AuthorizeTask is an additional core task/session guard. Identity-free calls
// are trusted internal services; synthetic identities must still match owners.
func (r *Repository) AuthorizeTask(ctx context.Context, taskID string) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return nil
	}
	owner, err := r.ConversationUserOwner(ctx, taskID)
	if err != nil {
		return err
	}
	if owner != "" && owner != identity.UserID {
		return repoerrors.ErrTaskNotFound
	}
	return nil
}

func authorizeIntake(ctx context.Context, tx *sqlx.Tx, agentID string, c *models.TaskComment) error {
	var allowed int
	err := tx.GetContext(ctx, &allowed, tx.Rebind(`SELECT COUNT(*) FROM orchestration_conversations c
		LEFT JOIN orchestration_assistant_bindings b ON b.conversation_id=c.task_id AND b.owner_user_id=c.owner_user_id
		WHERE c.task_id=? AND c.agent_profile_id=? AND c.platform='web'
		AND (c.owner_user_id='' OR (c.owner_user_id=? AND b.id IS NOT NULL))`), c.TaskID, agentID, c.AuthorID)
	if err == nil && allowed != 1 {
		return models.ErrConflict
	}
	return err
}

func supersedePrivateIntake(ctx context.Context, tx *sqlx.Tx, binding *models.AssistantBinding) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_intake SET status='superseded'
		WHERE status='accepted' AND EXISTS (SELECT 1 FROM orchestration_conversations c
		WHERE c.task_id=orchestration_intake.task_id AND c.owner_user_id=?
		AND (c.task_id<>? OR orchestration_intake.owner_user_id<>c.owner_user_id))`), binding.OwnerUserID, binding.ConversationID)
	return err
}

// The registry has no binding/profile cascade: deleting a default or retiring
// a persona must not publish retained task history or persona memory.
func (r *Repository) migrateConversationOwnership() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := db.ColumnExists(tx, "orchestration_conversations", "owner_user_id")
	if err != nil {
		return err
	}
	if !exists {
		if _, err := tx.Exec(`ALTER TABLE orchestration_conversations ADD COLUMN owner_user_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`UPDATE orchestration_conversations SET owner_user_id=(
		SELECT b.owner_user_id FROM orchestration_assistant_bindings b WHERE b.conversation_id=task_id)
		WHERE owner_user_id='' AND EXISTS (
		SELECT 1 FROM orchestration_assistant_bindings b WHERE b.conversation_id=task_id)`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
