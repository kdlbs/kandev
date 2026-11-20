package controller

import (
	"context"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/service"
)

type FormatterSettings struct {
	Binding    *models.FormatterBinding     `json:"binding"`
	Formatters []models.FormatterDescriptor `json:"formatters"`
	Events     []string                     `json:"events"`
}

func (c *Controller) FormatterSettings(ctx context.Context, id string) (FormatterSettings, error) {
	binding, choices, err := c.service.FormatterSettings(ctx, callerUserID(ctx), id)
	return FormatterSettings{Binding: binding, Formatters: choices, Events: c.service.AvailableEvents()}, err
}

func (c *Controller) ActivateFormatter(ctx context.Context, id, expected string, binding *models.FormatterBinding) (*models.FormatterBinding, error) {
	return c.service.ActivateFormatter(ctx, callerUserID(ctx), id, expected, binding)
}

func (c *Controller) PreviewFormatter(ctx context.Context, id string, binding models.FormatterBinding, event, sample string, draft []byte) (*service.FormatterPreview, error) {
	return c.service.PreviewFormatter(ctx, callerUserID(ctx), id, binding, event, sample, draft)
}

func (c *Controller) StartFormatterCapture(ctx context.Context) (*service.FormatterCaptureLease, error) {
	return c.service.StartFormatterCapture(ctx, callerUserID(ctx))
}

func (c *Controller) FormatterCapture(ctx context.Context, id string) (*service.FormatterCaptureLease, error) {
	return c.service.FormatterCapture(callerUserID(ctx), id)
}

func (c *Controller) ReleaseFormatterCapture(ctx context.Context, id string) error {
	return c.service.ReleaseFormatterCapture(callerUserID(ctx), id)
}
