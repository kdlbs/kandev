package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/user/models"
)

// @covers AC-UI-THREADS-SAVED-VIEWS-005.1, AC-UI-THREADS-SAVED-VIEWS-005.7
func TestBootThreadPresentation(t *testing.T) {
	views := mapThreadViews([]models.ThreadView{{ID: "presentation", Layout: "grid", AutoHideComposer: true}})
	draft := mapThreadViewDraft(&models.ThreadViewDraft{BaseViewID: "presentation", Layout: "columns", AutoHideComposer: true})
	if views[0]["layout"] != "grid" || views[0]["autoHideComposer"] != true {
		t.Fatalf("boot view lost presentation: %#v", views[0])
	}
	if draft["layout"] != "columns" || draft["autoHideComposer"] != true {
		t.Fatalf("boot draft lost presentation: %#v", draft)
	}
}
