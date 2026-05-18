package runtime

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// SkillLister is the narrow runtime-facing skill package catalog.
type SkillLister interface {
	ListSkillsFromConfig(ctx context.Context, workspaceID string) ([]*models.Skill, error)
}
