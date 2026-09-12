package clarification

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func TestInboxBootSummary_UsesBundleCountAndHiddenSummary(t *testing.T) {
	bundles := &fakeInboxBundleStore{
		page: &taskmodels.ClarificationBundlePage{
			Bundles: []taskmodels.ClarificationBundleSummary{{PendingID: "p1"}, {PendingID: "p2"}},
			HasMore: true,
		},
		summary: taskmodels.ClarificationInboxHiddenSummary{
			HiddenCount:      3,
			NextSnoozeExpiry: &testInboxNow,
		},
	}

	count, hasMore, nextSnoozeExpiry, err := InboxBootSummary(context.Background(), bundles, "workspace-1", "user-1", testInboxNow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2 (len(page.Bundles), not the hidden-filtered view count)", count)
	}
	if !hasMore {
		t.Error("hasMore = false, want true from page.HasMore")
	}
	if nextSnoozeExpiry == nil || !nextSnoozeExpiry.Equal(testInboxNow) {
		t.Errorf("nextSnoozeExpiry = %v, want %v", nextSnoozeExpiry, testInboxNow)
	}

	if bundles.lastOpts.WorkspaceID != "workspace-1" || bundles.lastOpts.Limit != defaultInboxLimit {
		t.Errorf("list opts = %+v, want workspace-1 at the default limit (AC .40: same bound as the list endpoint)", bundles.lastOpts)
	}
	if bundles.lastOpts.Sidecar == nil || bundles.lastOpts.Sidecar.UserID != "user-1" || bundles.lastOpts.Sidecar.Only {
		t.Errorf("sidecar filter = %+v, want the main-read exclusion filter for user-1", bundles.lastOpts.Sidecar)
	}
}

func TestInboxBootSummary_ListErrorPropagates(t *testing.T) {
	bundles := &fakeInboxBundleStore{listErr: errors.New("boom")}

	_, _, _, err := InboxBootSummary(context.Background(), bundles, "workspace-1", "user-1", testInboxNow)
	if err == nil {
		t.Fatal("expected the list error to propagate")
	}
}

func TestInboxBootSummary_HiddenSummaryErrorPropagates(t *testing.T) {
	bundles := &fakeInboxBundleStore{
		page:       &taskmodels.ClarificationBundlePage{},
		summaryErr: errors.New("boom"),
	}

	_, _, _, err := InboxBootSummary(context.Background(), bundles, "workspace-1", "user-1", testInboxNow)
	if err == nil {
		t.Fatal("expected the hidden-summary error to propagate rather than defaulting (F45's rule applies here too)")
	}
}
