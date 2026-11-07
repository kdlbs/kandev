package review

import (
	"context"
)

// ReviewSourceMetadataLookup supplies the task source, or the writer session's
// source when no task source is configured.
type ReviewSourceMetadataLookup interface {
	ReviewSourceMetadata(ctx context.Context, taskID, sessionID string) (map[string]any, error)
}

func (r *Runner) collectRequestChanges(ctx context.Context, req RunRequest) ([]ChangedFile, error) {
	if req.WorktreePath != "" || req.BaseRef != "" || req.HeadRef != "" {
		return CollectLocalChanges(ctx, Source{WorktreePath: req.WorktreePath, BaseRef: req.BaseRef, HeadRef: req.HeadRef})
	}
	files, workspaceErr := CollectChanges(ctx, r.changes, req.SessionID, req.RepositoryID)
	if workspaceErr == nil && len(files) > 0 {
		return files, nil
	}
	lookup, ok := r.taskContext.(ReviewSourceMetadataLookup)
	if !ok {
		return files, workspaceErr
	}
	metadata, err := lookup.ReviewSourceMetadata(ctx, req.TaskID, req.SessionID)
	if err != nil {
		return nil, err
	}
	source, err := SourceFromMetadata(metadata)
	if err != nil {
		return nil, err
	}
	if source == nil {
		return files, workspaceErr
	}
	return CollectLocalChanges(ctx, *source)
}
