package store

import (
	"context"

	"github.com/kandev/kandev/internal/editors/models"
)

type Repository interface {
	ListEditors(ctx context.Context) ([]*models.Editor, error)
	GetEditorByType(ctx context.Context, editorType string) (*models.Editor, error)
	GetEditorByID(ctx context.Context, editorID string) (*models.Editor, error)
	UpsertEditors(ctx context.Context, editors []*models.Editor) error
	CreateEditor(ctx context.Context, editor *models.Editor) error
	UpdateEditor(ctx context.Context, editor *models.Editor) error
	DeleteEditor(ctx context.Context, editorID string) error
	Close() error
}
