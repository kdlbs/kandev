package azuredevops

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const maxSavedViews = 50

func (s *Service) GetSavedViewsForWorkspace(ctx context.Context, workspaceID string) ([]SavedView, error) {
	raw, err := s.store.GetSavedViewsJSON(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	var candidates []SavedView
	if json.Unmarshal([]byte(raw), &candidates) != nil {
		return []SavedView{}, nil
	}
	views := make([]SavedView, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		view, validationErr := validateSavedView(candidate)
		if validationErr != nil {
			continue
		}
		if _, duplicate := seen[view.ID]; duplicate {
			continue
		}
		seen[view.ID] = struct{}{}
		views = append(views, view)
		if len(views) == maxSavedViews {
			break
		}
	}
	return views, nil
}

func (s *Service) SetSavedViewsForWorkspace(
	ctx context.Context,
	workspaceID string,
	views []SavedView,
) ([]SavedView, error) {
	if len(views) > maxSavedViews {
		return nil, fmt.Errorf("%w: at most %d saved views are allowed", ErrInvalidConfig, maxSavedViews)
	}
	normalized := make([]SavedView, 0, len(views))
	seen := make(map[string]struct{}, len(views))
	for _, candidate := range views {
		view, err := validateSavedView(candidate)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[view.ID]; duplicate {
			return nil, fmt.Errorf("%w: duplicate saved view id", ErrInvalidConfig)
		}
		seen[view.ID] = struct{}{}
		normalized = append(normalized, view)
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode Azure DevOps saved views: %w", err)
	}
	if err := s.store.PutSavedViewsJSON(ctx, workspaceID, string(raw)); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateSavedView(view SavedView) (SavedView, error) {
	view = normalizeSavedView(view)
	if err := validateSavedViewIdentity(view); err != nil {
		return SavedView{}, err
	}
	if err := validateSavedViewQuery(&view); err != nil {
		return SavedView{}, err
	}
	if view.CreatedAt.IsZero() {
		view.CreatedAt = time.Now().UTC()
	}
	return view, nil
}

func normalizeSavedView(view SavedView) SavedView {
	view.ID = strings.TrimSpace(view.ID)
	view.Kind = strings.TrimSpace(view.Kind)
	view.Label = strings.TrimSpace(view.Label)
	view.ProjectID = strings.TrimSpace(view.ProjectID)
	view.RepositoryID = strings.TrimSpace(view.RepositoryID)
	view.WIQL = strings.TrimSpace(view.WIQL)
	view.Status = strings.TrimSpace(view.Status)
	view.Creator = strings.TrimSpace(view.Creator)
	view.Reviewer = strings.TrimSpace(view.Reviewer)
	return view
}

func validateSavedViewIdentity(view SavedView) error {
	if view.ID == "" || len(view.ID) > 100 || view.Label == "" || len(view.Label) > 80 {
		return fmt.Errorf("%w: saved view id and label are required", ErrInvalidConfig)
	}
	if view.ProjectID == "" {
		return fmt.Errorf("%w: saved view project is required", ErrInvalidConfig)
	}
	return nil
}

func validateSavedViewQuery(view *SavedView) error {
	switch view.Kind {
	case "work_item":
		if view.WIQL == "" || len(view.WIQL) > 10_000 {
			return fmt.Errorf("%w: work-item view requires WIQL", ErrInvalidConfig)
		}
		if view.Top == 0 {
			view.Top = 50
		}
		if view.Top < 1 || view.Top > 200 {
			return fmt.Errorf("%w: saved view result limit is invalid", ErrInvalidConfig)
		}
	case "pull_request":
		if view.RepositoryID == "" {
			return fmt.Errorf("%w: pull-request view requires a repository", ErrInvalidConfig)
		}
	default:
		return fmt.Errorf("%w: saved view kind is invalid", ErrInvalidConfig)
	}
	return nil
}
