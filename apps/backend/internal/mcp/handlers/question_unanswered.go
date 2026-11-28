package handlers

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// UnansweredClarificationBundleLister is the optional read surface behind
// list_pending_questions_kandev's include_unanswered option. A bundle lister
// without it contributes no unanswered bundles.
type UnansweredClarificationBundleLister interface {
	ListUnansweredClarificationBundles(ctx context.Context, opts models.ListUnansweredClarificationsOptions) (*models.ClarificationBundlePage, error)
}

const (
	pendingQuestionStateActive     = "active"
	pendingQuestionStateUnanswered = "unanswered"
)

// mergeUnansweredQuestionBundles merges the caller's visible unanswered
// bundles into the active page, preserving the shared (created_at,
// pending_id) order and cursor so pagination stays one sequence. Unlike the
// active page, it keeps an automation caller's own task: unanswered bundles
// cannot be answered through answer_question_kandev, and seeing its own
// earlier questions is what lets an agent avoid asking them again.
func (h *Handlers) mergeUnansweredQuestionBundles(
	ctx context.Context,
	opts models.ListClarificationBundlesOptions,
	active *models.ClarificationBundlePage,
) (*models.ClarificationBundlePage, map[string]struct{}, error) {
	lister, ok := h.clarificationBundles.(UnansweredClarificationBundleLister)
	if !ok {
		return active, nil, nil
	}
	unansweredOpts, visible := unansweredOptionsFor(opts)
	if !visible {
		return active, nil, nil
	}
	unanswered, err := lister.ListUnansweredClarificationBundles(ctx, unansweredOpts)
	if err != nil {
		return nil, nil, err
	}
	ids := make(map[string]struct{}, len(unanswered.Bundles))
	for _, bundle := range unanswered.Bundles {
		ids[bundle.PendingID] = struct{}{}
	}
	combined := make([]models.ClarificationBundleSummary, 0, len(active.Bundles)+len(unanswered.Bundles))
	combined = append(append(combined, active.Bundles...), unanswered.Bundles...)
	sort.SliceStable(combined, func(i, j int) bool {
		if !combined[i].CreatedAt.Equal(combined[j].CreatedAt) {
			return combined[i].CreatedAt.Before(combined[j].CreatedAt)
		}
		return combined[i].PendingID < combined[j].PendingID
	})
	merged := &models.ClarificationBundlePage{
		Bundles: combined,
		HasMore: active.HasMore || unanswered.HasMore,
	}
	if len(combined) > opts.Limit {
		merged.Bundles = combined[:opts.Limit]
		merged.HasMore = true
	}
	return merged, ids, nil
}

// unansweredOptionsFor maps the active read's resolved visibility onto the
// unanswered read. It reports false when the requested workspace is outside
// the caller's visible set.
func unansweredOptionsFor(opts models.ListClarificationBundlesOptions) (models.ListUnansweredClarificationsOptions, bool) {
	out := models.ListUnansweredClarificationsOptions{
		CreatedSince:    opts.CreatedSince,
		CursorCreatedAt: opts.CursorCreatedAt,
		CursorPendingID: opts.CursorPendingID,
		Limit:           opts.Limit,
		Now:             time.Now().UTC(),
	}
	switch {
	case opts.WorkspaceID != "" && !opts.Unscoped && !slices.Contains(opts.VisibleWorkspaceIDs, opts.WorkspaceID):
		return out, false
	case opts.WorkspaceID != "":
		out.WorkspaceIDs = []string{opts.WorkspaceID}
	case opts.Unscoped:
		out.AnyWorkspace = true
	default:
		out.WorkspaceIDs = opts.VisibleWorkspaceIDs
	}
	return out, true
}

func labelPendingQuestionStates(bundles []pendingQuestionBundleView, unanswered map[string]struct{}) {
	for i := range bundles {
		if _, ok := unanswered[bundles[i].PendingID]; ok {
			bundles[i].State = pendingQuestionStateUnanswered
		} else {
			bundles[i].State = pendingQuestionStateActive
		}
	}
}
