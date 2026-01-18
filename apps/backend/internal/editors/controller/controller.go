package controller

import (
	"context"
	"sort"

	"github.com/kandev/kandev/internal/editors/discovery"
	"github.com/kandev/kandev/internal/editors/dto"
	"github.com/kandev/kandev/internal/editors/service"
)

type Controller struct {
	service *service.Service
}

func NewController(svc *service.Service) *Controller {
	return &Controller{service: svc}
}

func (c *Controller) ListEditors(ctx context.Context) (dto.EditorsResponse, error) {
	editors, err := c.service.ListEditors(ctx)
	if err != nil {
		return dto.EditorsResponse{}, err
	}
	result := make([]dto.EditorDTO, 0, len(editors))
	for _, editor := range editors {
		if editor == nil || !editor.Enabled {
			continue
		}
		result = append(result, dto.FromEditor(editor))
	}
	if order, err := discovery.LoadDefaults(); err == nil {
		orderIndex := make(map[string]int, len(order))
		for index, editor := range order {
			if editor.Type == "" {
				continue
			}
			orderIndex[editor.Type] = index
		}
		sort.SliceStable(result, func(i, j int) bool {
			left, leftOk := orderIndex[result[i].Type]
			right, rightOk := orderIndex[result[j].Type]
			if leftOk && rightOk {
				return left < right
			}
			if leftOk {
				return true
			}
			if rightOk {
				return false
			}
			return result[i].Name < result[j].Name
		})
	}
	return dto.EditorsResponse{Editors: result}, nil
}

func (c *Controller) OpenSessionEditor(ctx context.Context, sessionID string, req dto.OpenEditorRequest) (dto.OpenEditorResponse, error) {
	url, err := c.service.OpenEditor(ctx, service.OpenEditorInput{
		SessionID:  sessionID,
		EditorID:   req.EditorID,
		EditorType: req.EditorType,
		FilePath:   req.FilePath,
		Line:       req.Line,
		Column:     req.Column,
	})
	if err != nil {
		return dto.OpenEditorResponse{}, err
	}
	return dto.OpenEditorResponse{URL: url}, nil
}

func (c *Controller) CreateEditor(ctx context.Context, req dto.CreateEditorRequest) (dto.EditorDTO, error) {
	editor, err := c.service.CreateEditor(ctx, service.CreateEditorInput{
		Name:    req.Name,
		Kind:    req.Kind,
		Config:  req.Config,
		Enabled: req.Enabled,
	})
	if err != nil {
		return dto.EditorDTO{}, err
	}
	return dto.FromEditor(editor), nil
}

func (c *Controller) UpdateEditor(ctx context.Context, editorID string, req dto.UpdateEditorRequest) (dto.EditorDTO, error) {
	editor, err := c.service.UpdateEditor(ctx, service.UpdateEditorInput{
		EditorID: editorID,
		Name:     req.Name,
		Kind:     req.Kind,
		Config:   req.Config,
		Enabled:  req.Enabled,
	})
	if err != nil {
		return dto.EditorDTO{}, err
	}
	return dto.FromEditor(editor), nil
}

func (c *Controller) DeleteEditor(ctx context.Context, editorID string) error {
	return c.service.DeleteEditor(ctx, editorID)
}
