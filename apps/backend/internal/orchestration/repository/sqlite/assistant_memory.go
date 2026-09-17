package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/common/redaction"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) AssistantMemory(ctx context.Context, agentID, id string) (*models.AgentMemory, error) {
	var row models.AgentMemory
	err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT * FROM orchestration_memory WHERE agent_profile_id=? AND id=? AND forgotten_at IS NULL`), agentID, id)
	return &row, err
}

func (r *Repository) AssistantMemoryPage(ctx context.Context, agentID, scope, after string) ([]*models.AgentMemory, error) {
	rows := []*models.AgentMemory{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT * FROM orchestration_memory WHERE agent_profile_id=?
	AND forgotten_at IS NULL AND (?='' OR scope=?) AND id>? ORDER BY id LIMIT 51`), agentID, scope, scope, after)
	return rows, err
}

// SaveAssistantMemory is human-owned CAS, separate from a model's unconfirmed
// observations. Tombstones prevent a later runtime upsert from resurrecting it.
func (r *Repository) SaveAssistantMemory(ctx context.Context, row *models.AgentMemory, expected int64) error {
	row.Content = redaction.NewRedactor().String(row.Content)
	now := time.Now().UTC()
	if expected == 0 {
		row.Revision, row.CreatedAt, row.UpdatedAt = 1, now, now
		result, err := r.db.NamedExecContext(ctx, `INSERT INTO orchestration_memory
		(id,agent_profile_id,layer,key,content,metadata,owner_user_id,scope,scope_id,source_comment_id,revision,confirmed,priority,expires_at,created_at,updated_at)
		VALUES(:id,:agent_profile_id,:layer,:key,:content,'{}',:owner_user_id,:scope,:scope_id,:source_comment_id,1,:confirmed,:priority,:expires_at,:created_at,:updated_at)
		ON CONFLICT DO NOTHING`, row)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			return models.ErrConflict
		}
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_memory SET content=?,owner_user_id=?,scope=?,scope_id=?,source_comment_id=?,
	confirmed=?,priority=?,expires_at=?,revision=revision+1,updated_at=? WHERE id=? AND agent_profile_id=? AND revision=? AND forgotten_at IS NULL`),
		row.Content, row.OwnerUserID, row.Scope, row.ScopeID, row.SourceCommentID, row.Confirmed, row.Priority, row.ExpiresAt, now, row.ID, row.AgentProfileID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrConflict
	}
	row.Revision, row.UpdatedAt = expected+1, now
	return nil
}

func (r *Repository) ForgetAssistantMemory(ctx context.Context, agentID, id string, expected int64) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_memory SET forgotten_at=?,content='',metadata='{}',revision=revision+1,updated_at=?
	WHERE id=? AND agent_profile_id=? AND revision=? AND forgotten_at IS NULL`), time.Now().UTC(), time.Now().UTC(), id, agentID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
