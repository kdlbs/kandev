package labels

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// labelRepo is the persistence interface required by LabelService.
type labelRepo interface {
	GetOrCreateLabel(ctx context.Context, workspaceID, name string) (*sqlite.Label, error)
	GetLabelByName(ctx context.Context, workspaceID, name string) (*sqlite.Label, error)
	AddLabelToTask(ctx context.Context, taskID, labelID string) error
	RemoveLabelFromTask(ctx context.Context, taskID, labelID string) error
	ListLabelsForTask(ctx context.Context, taskID string) ([]*sqlite.Label, error)
	ListLabelsForTasks(ctx context.Context, taskIDs []string) (map[string][]*sqlite.Label, error)
	ListLabelsByWorkspace(ctx context.Context, workspaceID string) ([]*sqlite.Label, error)
	UpdateLabel(ctx context.Context, id, name, color string) error
	DeleteLabel(ctx context.Context, id string) error
}

// LabelService provides label CRUD and task-label operations.
type LabelService struct {
	repo labelRepo
}

// NewLabelService creates a new LabelService.
func NewLabelService(repo labelRepo) *LabelService {
	return &LabelService{repo: repo}
}

// AddLabelToTask resolves (or creates) a label by name in the workspace, then
// attaches it to the task. Returns the label that was attached.
func (s *LabelService) AddLabelToTask(ctx context.Context, taskID, workspaceID, labelName string) (*Label, error) {
	lbl, err := s.repo.GetOrCreateLabel(ctx, workspaceID, labelName)
	if err != nil {
		return nil, fmt.Errorf("resolve label %q: %w", labelName, err)
	}
	if err := s.repo.AddLabelToTask(ctx, taskID, lbl.ID); err != nil {
		return nil, fmt.Errorf("attach label to task: %w", err)
	}
	return repoToModel(lbl), nil
}

// RemoveLabelFromTask finds a label by name in the workspace and removes it
// from the task. It is not an error if the label does not exist.
func (s *LabelService) RemoveLabelFromTask(ctx context.Context, taskID, workspaceID, labelName string) error {
	lbl, err := s.repo.GetLabelByName(ctx, workspaceID, labelName)
	if err != nil {
		// Label doesn't exist; nothing to remove.
		return nil
	}
	return s.repo.RemoveLabelFromTask(ctx, taskID, lbl.ID)
}

// ListLabelsForTask returns all labels attached to a task.
func (s *LabelService) ListLabelsForTask(ctx context.Context, taskID string) ([]*Label, error) {
	rows, err := s.repo.ListLabelsForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return repoSliceToModel(rows), nil
}

// ListWorkspaceLabels returns the full label catalog for a workspace.
func (s *LabelService) ListWorkspaceLabels(ctx context.Context, workspaceID string) ([]*Label, error) {
	rows, err := s.repo.ListLabelsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return repoSliceToModel(rows), nil
}

// UpdateLabel renames a label and/or changes its color.
func (s *LabelService) UpdateLabel(ctx context.Context, labelID, name, color string) error {
	return s.repo.UpdateLabel(ctx, labelID, name, color)
}

// DeleteLabel removes a label from the catalog. Junction rows are cascade-deleted.
func (s *LabelService) DeleteLabel(ctx context.Context, labelID string) error {
	return s.repo.DeleteLabel(ctx, labelID)
}

// repoToModel converts a sqlite.Label to the package model.
func repoToModel(l *sqlite.Label) *Label {
	return &Label{
		ID:          l.ID,
		WorkspaceID: l.WorkspaceID,
		Name:        l.Name,
		Color:       l.Color,
		CreatedAt:   l.CreatedAt,
		UpdatedAt:   l.UpdatedAt,
	}
}

// repoSliceToModel converts a slice of sqlite.Label to the package model.
func repoSliceToModel(rows []*sqlite.Label) []*Label {
	out := make([]*Label, len(rows))
	for i, r := range rows {
		out[i] = repoToModel(r)
	}
	return out
}
