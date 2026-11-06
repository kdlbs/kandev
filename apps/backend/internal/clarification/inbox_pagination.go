package clarification

import (
	"sort"
	"strings"
	"time"
)

type inboxCombinedItem struct {
	kind      string
	createdAt time.Time
	id        string
	bundle    inboxBundleView
	async     inboxAsyncQuestionView
}

type inboxCombinedPage struct {
	bundles  []inboxBundleView
	async    []inboxAsyncQuestionView
	hasMore  bool
	lastItem *inboxCombinedItem
}

// mergeInboxPage applies one page bound to the blocking and async sources.
// Cursors carry a source prefix so the next read resumes both sources at the
// same wall-clock boundary.
func mergeInboxPage(
	bundles []inboxBundleView,
	async []inboxAsyncQuestionView,
	blockingHasMore, asyncHasMore bool,
	limit int,
) inboxCombinedPage {
	items := make([]inboxCombinedItem, 0, len(bundles)+len(async))
	for _, bundle := range bundles {
		createdAt, _ := time.Parse(time.RFC3339, bundle.CreatedAt)
		items = append(items, inboxCombinedItem{kind: "b", createdAt: createdAt, id: bundle.PendingID, bundle: bundle})
	}
	for _, question := range async {
		createdAt, _ := time.Parse(time.RFC3339, question.CreatedAt)
		items = append(items, inboxCombinedItem{kind: "a", createdAt: createdAt, id: question.QuestionID, async: question})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].createdAt.Equal(items[j].createdAt) {
			return items[i].createdAt.Before(items[j].createdAt)
		}
		if items[i].kind != items[j].kind {
			return items[i].kind < items[j].kind
		}
		return items[i].id < items[j].id
	})
	hasMore := blockingHasMore || asyncHasMore || len(items) > limit
	if len(items) > limit {
		items = items[:limit]
	}
	page := inboxCombinedPage{
		bundles: make([]inboxBundleView, 0, len(items)),
		async:   make([]inboxAsyncQuestionView, 0, len(items)),
		hasMore: hasMore,
	}
	for index := range items {
		if items[index].kind == "b" {
			page.bundles = append(page.bundles, items[index].bundle)
		} else {
			page.async = append(page.async, items[index].async)
		}
	}
	if len(items) > 0 {
		page.lastItem = &items[len(items)-1]
	}
	return page
}

// inboxCursorSource splits a combined cursor into its source kind and identity.
func inboxCursorSource(cursorID string) (string, string) {
	if strings.HasPrefix(cursorID, "a:") {
		return "a", strings.TrimPrefix(cursorID, "a:")
	}
	if strings.HasPrefix(cursorID, "b:") {
		return "b", strings.TrimPrefix(cursorID, "b:")
	}
	return "", cursorID
}
