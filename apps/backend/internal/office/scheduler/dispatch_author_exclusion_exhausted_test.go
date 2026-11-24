package scheduler_test

import (
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
)

func TestDispatchAuthorExclusion_ExhaustedFallbackRecordsAuthorSkip(t *testing.T) {
	repo := newExclusionRepo(t, exclReviewEvents)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	seatExclusionReviewer(t, repo)
	seedWorkAuthor(t, repo, "claude-acp")
	starter := newFakeTaskStarter()
	starter.failFor["codex-acp"] = fmt.Errorf("insufficient_quota")

	run, launched, parked, err := dispatchExclusionRun(t, repo, starter, exclReviewPayload)
	if err != nil || launched || !parked {
		t.Fatalf("launched=%v parked=%v err=%v, want park after the only other provider fails", launched, parked, err)
	}
	for _, call := range starter.calls {
		if call.route.ProviderID == "claude-acp" {
			t.Fatalf("author provider was attempted: %+v", starter.calls)
		}
	}
	assertAuthorSkipAttempt(t, repo, run.ID, "claude-acp")
}
