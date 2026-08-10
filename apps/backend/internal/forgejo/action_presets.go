package forgejo

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ActionPreset struct {
	ID           string    `json:"id" db:"id"`
	WorkspaceID  string    `json:"workspace_id" db:"workspace_id"`
	Kind         string    `json:"kind" db:"kind"`
	Name         string    `json:"name" db:"name"`
	Instructions string    `json:"instructions" db:"instructions"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

func (s *Store) UpsertActionPreset(ctx context.Context, p *ActionPreset) error {
	if p == nil || strings.TrimSpace(p.WorkspaceID) == "" || strings.TrimSpace(p.Kind) == "" || strings.TrimSpace(p.Name) == "" {
		return errors.New("forgejo action preset workspace, kind, and name are required")
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO forgejo_action_presets (id,workspace_id,kind,name,instructions,created_at,updated_at) VALUES (?,?,?,?,?,?,?) ON CONFLICT(workspace_id,kind,name) DO UPDATE SET instructions=excluded.instructions,updated_at=excluded.updated_at`, p.ID, p.WorkspaceID, p.Kind, p.Name, p.Instructions, p.CreatedAt, p.UpdatedAt)
	return err
}
func (s *Store) ListActionPresets(ctx context.Context, workspaceID string) ([]*ActionPreset, error) {
	var rows []ActionPreset
	if err := s.ro.SelectContext(ctx, &rows, `SELECT * FROM forgejo_action_presets WHERE workspace_id=? ORDER BY kind,name`, workspaceID); err != nil {
		return nil, err
	}
	result := make([]*ActionPreset, len(rows))
	for i := range rows {
		result[i] = &rows[i]
	}
	return result, nil
}
func (s *Store) DeleteActionPreset(ctx context.Context, workspaceID, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM forgejo_action_presets WHERE workspace_id=? AND id=?`, workspaceID, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatchNotFound
	}
	return nil
}
func (s *Service) SaveActionPreset(ctx context.Context, workspaceID string, p *ActionPreset) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	if p == nil {
		return errors.New("forgejo action preset required")
	}
	p.WorkspaceID = workspaceID
	return s.store.UpsertActionPreset(ctx, p)
}
func (s *Service) ListActionPresets(ctx context.Context, workspaceID string) ([]*ActionPreset, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, ErrWorkspaceRequired
	}
	return s.store.ListActionPresets(ctx, workspaceID)
}
func (s *Service) DeleteActionPreset(ctx context.Context, workspaceID, id string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return ErrWorkspaceRequired
	}
	return s.store.DeleteActionPreset(ctx, workspaceID, id)
}
