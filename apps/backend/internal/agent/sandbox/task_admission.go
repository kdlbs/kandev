package sandbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	contract "github.com/kandev/kandev/internal/common/sandbox"
)

type taskGuard struct {
	WorkspaceID string    `json:"workspace_id"`
	Resolved    *Resolved `json:"resolved"`
}

// AdmitTask captures permissions before launch. Later context changes cannot
// remove confinement or expand these grants without operator authority.
func (s *Service) AdmitTask(ctx context.Context, selection SelectionContext, parentID string) (*Resolved, error) {
	if selection.TaskID == "" || selection.TaskID == parentID {
		return nil, ErrIneligible
	}
	prior, version, err := s.readTaskGuard(ctx, selection.OrgID, selection.TaskID, selection.WorkspaceID)
	if err != nil {
		return nil, err
	}
	selection.Prior = prior
	resolved, err := s.Resolve(ctx, selection)
	if err != nil {
		return nil, err
	}
	if err = s.checkParentGuard(ctx, selection, parentID, resolved); err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, nil
	}
	if err = s.saveTaskGuard(ctx, selection, resolved, version); err != nil {
		return nil, err
	}
	return resolved, nil
}

func (s *Service) checkParentGuard(ctx context.Context, selection SelectionContext, parentID string, resolved *Resolved) error {
	if parentID == "" {
		return nil
	}
	parent, _, err := s.readTaskGuard(ctx, selection.OrgID, parentID, selection.WorkspaceID)
	if err != nil {
		return err
	}
	if parent == nil {
		return nil
	}
	if resolved == nil {
		return ErrIneligible
	}
	selection.Prior = parent
	return s.checkPrior(ctx, selection, resolved.Revision)
}

func (s *Service) readTaskGuard(ctx context.Context, orgID, taskID, workspaceID string) (*Resolved, int64, error) {
	var raw string
	var version int64
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT content_json,version FROM sandbox_task_guards WHERE org_id=? AND task_id=?`), orgID, taskID).Scan(&raw, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	var guard taskGuard
	if err = json.Unmarshal([]byte(raw), &guard); err != nil {
		return nil, 0, err
	}
	if guard.WorkspaceID != workspaceID || guard.Resolved == nil || guard.Resolved.Revision == nil || guard.Resolved.Revision.OrgID != orgID {
		return nil, 0, ErrIneligible
	}
	return guard.Resolved, version, nil
}

func (s *Service) saveTaskGuard(ctx context.Context, selection SelectionContext, resolved *Resolved, expected int64) error {
	raw, err := json.Marshal(taskGuard{WorkspaceID: selection.WorkspaceID, Resolved: resolved})
	if err != nil {
		return err
	}
	var result sql.Result
	if expected == 0 {
		result, err = s.db.ExecContext(ctx, s.db.Rebind(`INSERT INTO sandbox_task_guards (org_id,task_id,version,content_json) VALUES (?,?,1,?) ON CONFLICT (org_id,task_id) DO NOTHING`), selection.OrgID, selection.TaskID, string(raw))
	} else {
		result, err = s.db.ExecContext(ctx, s.db.Rebind(`UPDATE sandbox_task_guards SET version=?,content_json=? WHERE org_id=? AND task_id=? AND version=?`), expected+1, string(raw), selection.OrgID, selection.TaskID, expected)
	}
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrConflict
	}
	return nil
}

// InheritTaskGuard carries a source task's restriction into a newly created task.
func (s *Service) InheritTaskGuard(ctx context.Context, selection SelectionContext, sourceIDs ...string) error {
	if selection.TaskID == "" || len(sourceIDs) > 16 {
		return ErrIneligible
	}
	var selected *Resolved
	for _, sourceID := range sourceIDs {
		if sourceID == "" {
			continue
		}
		if sourceID == selection.TaskID {
			return ErrIneligible
		}
		source, _, err := s.readTaskGuard(ctx, selection.OrgID, sourceID, selection.WorkspaceID)
		if err != nil {
			return err
		}
		if source == nil {
			continue
		}
		if selected == nil || noBroader(source, selected) {
			selected = source
			continue
		}
		if !noBroader(selected, source) {
			return ErrIneligible
		}
	}
	if selected == nil {
		return nil
	}
	return s.saveTaskGuard(ctx, selection, selected, 0)
}

func noBroader(candidate, limit *Resolved) bool {
	a, b := candidate.Revision.Policy, limit.Revision.Policy
	return a.InstallationID == b.InstallationID && a.ProviderKey == b.ProviderKey && contract.WithinCeiling(a.Grants, b.Grants) == nil
}
