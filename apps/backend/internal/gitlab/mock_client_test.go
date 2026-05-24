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
	mock.SeedPipelines(project, []Pipeline{{Status: "failed"}})
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

// SeedPipelines is keyed by project so successive calls for the same project
// overwrite rather than living side-by-side in the map. Previously the seed
// API took (project, iid), which let two seeds for the same project coexist
// and made ListPipelines's iteration-order pick non-deterministic.
func TestMockClient_SeedPipelines_OverwritesByProject(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"
	mock.SeedPipelines(project, []Pipeline{{Status: "failed"}})
	mock.SeedPipelines(project, []Pipeline{{Status: "success"}})

	got, err := mock.ListPipelines(context.Background(), project, "feat/x")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 1 || got[0].Status != "success" {
		t.Errorf("pipelines = %#v, want [{Status: success}] (second seed must overwrite)", got)
	}
}

// MockClient.GetMRStatus must derive approval state from SeedApprovals so
// the same approval-gating logic the real client uses (summarizeApprovals
// against approved-vs-required) can be exercised end-to-end in tests.
func TestMockClient_GetMRStatus_UsesSeededApprovals(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"
	mock.SeedMR(project, &MR{IID: 7, State: "open", HeadBranch: "feat/x", HeadSHA: "abc"})
	mock.SeedApprovals(project, 7, []MRApproval{{Username: "alice"}}, 2)

	st, err := mock.GetMRStatus(context.Background(), project, 7)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if st.ApprovalCount != 1 {
		t.Errorf("ApprovalCount = %d, want 1", st.ApprovalCount)
	}
	if st.RequiredApprovals != 2 {
		t.Errorf("RequiredApprovals = %d, want 2", st.RequiredApprovals)
	}
	if st.ApprovalState != "pending" {
		t.Errorf("ApprovalState = %q, want pending (1/2 approved)", st.ApprovalState)
	}
}

func TestMockClient_GetMRFeedback_ReportsPipelinesWhenHeadPresent(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"
	mock.SeedPipelines(project, []Pipeline{{Status: "failed"}})
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
