package gitlab

import (
	"context"
	"testing"
)

// MockClient.ListPipelines is keyed by project only — it returns every
// pipeline seeded under the project regardless of the branch argument. Without
// the head-ref guard in GetMRFeedback that means a brand-new MR with no head
// SHA / branch would still inherit a sibling MR's failing pipeline and flip
// HasIssues to true. The real client guards on HeadSHA before probing
// pipelines; the mock must match.
func TestMockClient_GetMRFeedback_SkipsPipelinesWhenHeadEmpty(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"

	// Seed a failing pipeline for the project — any MR without a head ref
	// must NOT inherit it.
	mock.pipelines[mockMRKey{Project: project, IID: 1}] = []Pipeline{{Status: "failed"}}
	mock.SeedMR(project, &MR{IID: 7, State: "open"}) // no HeadSHA, no HeadBranch

	fb, err := mock.GetMRFeedback(context.Background(), project, 7)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(fb.Pipelines) != 0 {
		t.Errorf("pipelines = %d, want 0 (head ref empty — should not inherit project pipelines)", len(fb.Pipelines))
	}
	if fb.HasIssues {
		t.Error("HasIssues = true on an MR with empty head ref and no failing discussions, want false")
	}
}

func TestMockClient_GetMRFeedback_ReportsPipelinesWhenHeadPresent(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"
	mock.pipelines[mockMRKey{Project: project, IID: 1}] = []Pipeline{{Status: "failed"}}
	mock.SeedMR(project, &MR{IID: 7, State: "open", HeadBranch: "feat/x", HeadSHA: "abc"})

	fb, err := mock.GetMRFeedback(context.Background(), project, 7)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(fb.Pipelines) != 1 {
		t.Fatalf("pipelines = %d, want 1", len(fb.Pipelines))
	}
	if !fb.HasIssues {
		t.Error("HasIssues = false despite a failing pipeline; want true")
	}
}
