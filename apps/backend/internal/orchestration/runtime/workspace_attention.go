package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Service) attentionWorkspace(ctx context.Context, b *models.AssistantBinding, task string) (*models.AssistantBinding, *models.WorkspaceGrant, error) {
	row, err := s.Tasks.GetTask(ctx, task)
	if err != nil {
		return nil, nil, err
	}
	if row.WorkspaceID == b.WorkspaceID {
		return b, nil, nil
	}
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, row.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	g, err = s.currentWorkspaceGrant(ctx, b, row.WorkspaceID, g.Revision, workspaceObserve, workspaceTaskSummaryExport)
	if err != nil {
		return nil, nil, err
	}
	scoped := *b
	scoped.WorkspaceID = row.WorkspaceID
	return &scoped, g, nil
}

func linkedAttentionSources(sources []models.AttentionSource) []models.AttentionSource {
	rows := append([]models.AttentionSource(nil), sources...)
	for i := range rows {
		rows[i].Summary = "Linked task attention changed. Read the current request or result through its scoped tool."
		rows[i].Friction = nil
	}
	return rows
}

func (s *Service) attentionWakeScope(ctx context.Context, b *models.AssistantBinding, task string, refs []attentionWakeRef) ([]attentionWakeRef, error) {
	_, grant, err := s.attentionWorkspace(ctx, b, task)
	if err != nil {
		return nil, err
	}
	if grant == nil {
		return refs, nil
	}
	if err = s.Repo.RecordWorkspaceExport(ctx, b, grant, workspaceTaskSummaryExport); err != nil {
		return nil, err
	}
	for i := range refs {
		refs[i].WorkspaceID = grant.WorkspaceID
		refs[i].WorkspaceGrantRevision = grant.Revision
	}
	return refs, nil
}

func (s *Service) validateWorkspaceWake(ctx context.Context, b *models.AssistantBinding, row *models.Attention, ref attentionWakeRef) error {
	if row.WorkspaceID == b.WorkspaceID {
		if ref.WorkspaceID != "" || ref.WorkspaceGrantRevision != 0 {
			return fmt.Errorf("invalid home wake scope")
		}
		return nil
	}
	if ref.WorkspaceID != row.WorkspaceID {
		return fmt.Errorf("attention workspace changed")
	}
	_, err := s.currentWorkspaceGrant(ctx, b, row.WorkspaceID, ref.WorkspaceGrantRevision, workspaceObserve, workspaceTaskSummaryExport)
	return err
}

func (s *Service) managedStopWorkspace(ctx context.Context, b *models.AssistantBinding, task string) (*models.AssistantBinding, context.Context, error) {
	scoped, g, err := s.attentionWorkspace(ctx, b, task)
	if err != nil || g == nil {
		return scoped, ctx, err
	}
	if _, err = s.currentWorkspaceGrant(ctx, b, g.WorkspaceID, g.Revision, workspaceCoordinate, ""); err != nil {
		return nil, ctx, err
	}
	return scoped, s.workspaceEffectContext(ctx, b, g, workspaceCoordinate, ""), nil
}

func (s *Service) validateWorkspaceWakeAuthority(ctx context.Context, binding string, refs []attentionWakeRef) error {
	if len(refs) == 0 {
		return nil
	}
	if len(refs) > attentionBatch {
		return models.ErrConflict
	}
	b, err := s.Repo.AssistantBindingByID(ctx, binding)
	if err != nil {
		return err
	}
	// Wake freshness is checked before launch. An active session can acknowledge
	// resolved requests, but every further call still needs the original grant.
	for _, ref := range refs {
		row, err := s.Repo.AttentionByID(ctx, binding, ref.ID)
		if err != nil {
			return err
		}
		if err = s.validateWorkspaceWake(ctx, b, row, ref); err != nil {
			return err
		}
	}
	return nil
}
