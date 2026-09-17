package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// EnsureAgentConversation atomically creates the stable native conversation and
// its runner. Deterministic IDs make concurrent opens and retries idempotent.
func (r *Repository) EnsureAgentConversation(ctx context.Context, agent *models.AgentInstance) (*Conversation, error) {
	var workspaceID string
	if err := r.ro.GetContext(ctx, &workspaceID, r.ro.Rebind(`SELECT id FROM workspaces WHERE id = ?`), agent.WorkspaceID); err != nil {
		return nil, fmt.Errorf("load workspace: %w", err)
	}
	id := uuid.NewSHA1(uuid.NameSpaceOID, []byte("kandev:conversation:"+agent.ID)).String()
	taskID := uuid.NewSHA1(uuid.NameSpaceOID, []byte("kandev:conversation-task:"+agent.ID)).String()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO tasks
		(id, workspace_id, workflow_id, workflow_step_id, metadata, title, state, is_ephemeral, origin, created_at, updated_at)
		VALUES (?, ?, ?, '', '{"native_conversation":true}', ?, 'IN_PROGRESS', 1, 'native_conversation', ?, ?) ON CONFLICT (id) DO NOTHING`),
		taskID, workspaceID, "", fmt.Sprintf("Conversation with %s", agent.Name), now, now)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO workflow_step_participants
		(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES (?, '', ?, 'runner', ?, 0, 0) ON CONFLICT (id) DO NOTHING`), id, taskID, agent.ID)
	if err != nil {
		return nil, err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_conversations
		(id, workspace_id, agent_profile_id, platform, config, webhook_secret,
		status, task_id, created_at, updated_at)
		VALUES (?, ?, ?, 'web', '{}', '', 'active', ?, ?, ?) ON CONFLICT (id) DO NOTHING`),
		id, agent.WorkspaceID, agent.ID, taskID, now, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Conversation{TaskID: taskID}, nil
}

// IsNativeConversation identifies a persisted native conversation without
// trusting a caller-supplied flag in a wake payload.
func (r *Repository) IsNativeConversation(ctx context.Context, taskID string) (bool, error) {
	var count int
	err := r.ro.GetContext(ctx, &count, r.ro.Rebind(`SELECT COUNT(*) FROM orchestration_conversations WHERE task_id = ? AND platform = 'web'`), taskID)
	return count > 0, err
}

type Conversation struct {
	TaskID string `json:"task_id"`
}

func (r *Repository) ValidateWorkspace(ctx context.Context, id string) error {
	var count int
	if err := r.ro.GetContext(ctx, &count, r.ro.Rebind("SELECT count(*) FROM workspaces WHERE id=?"), id); err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("workspace not found")
	}
	return nil
}
