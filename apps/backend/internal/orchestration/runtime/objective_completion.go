package runtime

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestration/models"
)

// CompletionReader checks canonical worker activity, review gates and evidence.
// A model's status assertion never substitutes for these checks.
type CompletionReader interface {
	ValidateAssistantTaskCompletion(context.Context, string, string) error
	ValidateAssistantEvidence(context.Context, string, models.Evidence) error
}

func (s *Service) validateObjectiveCompletion(ctx context.Context, b *models.AssistantBinding, o *models.Objective) error {
	links, err := s.Repo.ObjectiveTasks(ctx, o.ID)
	if err != nil {
		return err
	}
	reader, ok := s.Manager.(CompletionReader)
	if len(links) > 0 && !ok {
		return fmt.Errorf("completion verification is unavailable")
	}
	linked := map[string]bool{}
	for _, link := range links {
		if !linked[link.TaskID] {
			if err := reader.ValidateAssistantTaskCompletion(ctx, o.WorkspaceID, link.TaskID); err != nil {
				return err
			}
		}
		linked[link.TaskID] = true
	}
	if (o.Mode == executionModeExecute || o.Mode == executionModeDesign) && len(links) == 0 {
		return fmt.Errorf("delivery objective has no linked task evidence")
	}
	covered := map[string]bool{}
	for _, e := range o.Evidence {
		if e.AcceptanceRevision != o.AcceptanceRevision {
			return fmt.Errorf("acceptance evidence is stale")
		}
		if err := s.validateObjectiveEvidenceSource(ctx, b, o, e, linked, reader); err != nil {
			return err
		}
		covered[e.CriterionID] = true
	}
	for _, criterion := range o.Acceptance {
		if !covered[criterion.ID] {
			return fmt.Errorf("acceptance criterion %s has no current evidence", criterion.ID)
		}
	}
	return nil
}

func (s *Service) validateObjectiveEvidenceSource(ctx context.Context, b *models.AssistantBinding, o *models.Objective, e models.Evidence, linked map[string]bool, reader CompletionReader) error {
	if e.SourceKind == "comment" {
		c, err := s.Repo.GetCommentByID(ctx, b.ConversationID, e.SourceID)
		if err != nil || c.AuthorType != authorTypeAgent || c.AuthorID != b.OrchestratorID {
			return fmt.Errorf("answer evidence is unavailable")
		}
	} else {
		if reader == nil || !linked[e.TaskID] {
			return fmt.Errorf("evidence task is not linked")
		}
		if err := reader.ValidateAssistantEvidence(ctx, o.WorkspaceID, e); err != nil {
			return err
		}
	}
	return nil
}
