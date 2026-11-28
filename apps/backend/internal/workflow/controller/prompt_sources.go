package controller

import (
	"context"
	"github.com/kandev/kandev/internal/workflow/models"
)

func (c *Controller) ImportPromptSources(ctx context.Context, request models.PromptSourceImport, preview bool) (*models.PromptSourceResult, error) {
	if preview {
		return c.svc.PreviewPromptSources(ctx, request)
	}
	return c.svc.ImportPromptSources(ctx, request)
}
func (c *Controller) DiffPromptSources(ctx context.Context) (*models.PromptSourceResult, error) {
	return c.svc.DiffPromptSources(ctx)
}
