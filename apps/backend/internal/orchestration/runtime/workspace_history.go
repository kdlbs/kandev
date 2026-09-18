package runtime

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// History may already contain foreign exports. Changing the receiver therefore
// requires every historical field to be confirmed for that receiver, even when
// the original grant has since been revoked or the local packet cache is empty.
func (s *Service) validateWorkspaceHistory(ctx context.Context, b *models.AssistantBinding, authority models.AssistantAuthority, record bool) error {
	rows, err := s.workspaceHistory(ctx, b)
	if err != nil || len(rows) == 0 {
		return err
	}
	revision, err := s.contextProfileRevision(ctx, b.WorkspaceID, authority.ProfileID)
	if err != nil {
		return err
	}
	already := workspaceDeliveredFields(rows, authority, revision)
	checked := map[string]*models.WorkspaceGrant{}
	for _, row := range rows {
		key := row.WorkspaceID + ":" + row.Kind
		if already[key] || checked[key] != nil {
			continue
		}
		g, err := s.reconfirmedHistoryGrant(ctx, b, row)
		if err != nil {
			return fmt.Errorf("history requires workspace reconfirmation")
		}
		checked[key] = g
	}
	if record {
		for _, row := range rows {
			key := row.WorkspaceID + ":" + row.Kind
			if g := checked[key]; g != nil {
				if err = s.Repo.RecordWorkspaceExport(ctx, b, g, row.Kind); err != nil {
					return err
				}
				delete(checked, key)
			}
		}
	}
	return nil
}

func workspaceDeliveredFields(rows []models.WorkspaceExport, authority models.AssistantAuthority, revision string) map[string]bool {
	already := map[string]bool{}
	for _, row := range rows {
		if row.ReceiverProfileID == authority.ProfileID && row.ReceiverProfileRevision == revision && row.AuthorityRevision == authority.Revision {
			already[row.WorkspaceID+":"+row.Kind] = true
		}
	}
	return already
}

func (s *Service) reconfirmedHistoryGrant(ctx context.Context, b *models.AssistantBinding, row models.WorkspaceExport) (*models.WorkspaceGrant, error) {
	g, err := s.Repo.WorkspaceGrant(ctx, b.ID, row.WorkspaceID)
	if err != nil {
		return nil, err
	}
	kind := row.Kind
	if kind == "workspace_link" {
		kind = ""
	}
	return s.currentWorkspaceGrant(ctx, b, row.WorkspaceID, g.Revision, workspaceObserve, kind)
}

func (s *Service) workspaceHistory(ctx context.Context, b *models.AssistantBinding) ([]models.WorkspaceExport, error) {
	rows := []models.WorkspaceExport{}
	after := ""
	for len(rows) < 10000 {
		page, err := s.Repo.WorkspaceExports(ctx, b.ID, b.ConversationID, after, 100)
		if err != nil {
			return nil, err
		}
		rows = append(rows, page...)
		if len(page) < 100 {
			return rows, nil
		}
		after = page[len(page)-1].ID
	}
	return nil, fmt.Errorf("workspace history exceeds the validation budget")
}

func (s *Service) recordWorkspaceHistory(ctx context.Context, taskID string, authority *models.AssistantAuthority) error {
	if authority == nil {
		return nil
	}
	b, err := s.Repo.AssistantForConversation(ctx, taskID)
	if err != nil {
		return err
	}
	return s.validateWorkspaceHistory(ctx, b, *authority, true)
}

func (s *Service) authorizeHistoryLaunch(ctx context.Context, taskID, payload string) (*models.AssistantAuthority, error) {
	authority, err := s.validateAssistantAuthority(ctx, taskID, payload)
	if err != nil {
		return nil, err
	}
	if err = s.recordWorkspaceHistory(ctx, taskID, authority); err != nil {
		return nil, err
	}
	return authority, nil
}
