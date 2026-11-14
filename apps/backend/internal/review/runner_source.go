package review

import (
	"context"
)

// ReviewSourceLookup supplies an authorized source for immediate collection.
type ReviewSourceLookup interface {
	SavedReviewSource(ctx context.Context, taskID, sessionID string) (*Source, error)
	ExplicitReviewSource(ctx context.Context, taskID string, source Source) (*Source, error)
}

func (r *Runner) collectRequestChanges(ctx context.Context, req RunRequest) ([]ChangedFile, error) {
	if req.WorktreePath != "" || req.BaseRef != "" || req.HeadRef != "" {
		lookup, ok := r.taskContext.(ReviewSourceLookup)
		if !ok {
			return nil, ErrWorkspaceUnavailable
		}
		source, err := lookup.ExplicitReviewSource(ctx, req.TaskID, Source{
			WorktreePath: req.WorktreePath, BaseRef: req.BaseRef, HeadRef: req.HeadRef,
		})
		if err != nil {
			return nil, err
		}
		if source == nil {
			return nil, ErrWorkspaceUnavailable
		}
		return CollectLocalChanges(ctx, *source)
	}
	files, workspaceErr := CollectChanges(ctx, r.changes, req.SessionID, req.RepositoryID)
	if workspaceErr == nil && len(files) > 0 {
		return files, nil
	}
	lookup, ok := r.taskContext.(ReviewSourceLookup)
	if !ok {
		return files, workspaceErr
	}
	source, err := lookup.SavedReviewSource(ctx, req.TaskID, req.SessionID)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return files, workspaceErr
	}
	return CollectLocalChanges(ctx, *source)
}
