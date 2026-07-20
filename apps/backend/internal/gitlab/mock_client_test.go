package gitlab

import (
	"context"
	"testing"
)

func TestMockClientReviewerAndSubscriptionActions(t *testing.T) {
	var _ Client = (*MockClient)(nil)
	const project = "team/repo"
	mock := NewMockClient("")
	mock.SeedProjectMembers(project, []ProjectMember{
		{ID: 42, Username: "alice", Name: "Alice"},
		{ID: 91, Username: "bob", Name: "Bob"},
	})
	mock.SeedMR(project, &MR{IID: 7, State: mrStateOpen})
	mock.SeedIssue(project, &Issue{IID: 8, State: gitlabStateOpened})

	members, err := mock.ListProjectMembers(t.Context(), project, "ali")
	if err != nil || len(members) != 1 || members[0].ID != 42 {
		t.Fatalf("members = %#v, err=%v", members, err)
	}
	if err := mock.SetMRReviewers(t.Context(), project, 7, []int64{42}); err != nil {
		t.Fatalf("set reviewers: %v", err)
	}
	mr, err := mock.GetMR(t.Context(), project, 7)
	if err != nil || len(mr.Reviewers) != 1 || mr.Reviewers[0].ID != 42 {
		t.Fatalf("MR reviewers = %#v, err=%v", mr.Reviewers, err)
	}
	if err := mock.SetMRReviewers(t.Context(), project, 7, []int64{}); err != nil {
		t.Fatalf("clear reviewers: %v", err)
	}
	if len(mr.Reviewers) != 0 {
		t.Fatalf("reviewers after clear = %#v", mr.Reviewers)
	}

	mrState, err := mock.SetMRSubscription(t.Context(), project, 7, true)
	if err != nil || !mrState.Subscribed {
		t.Fatalf("MR subscription = %#v, err=%v", mrState, err)
	}
	issueState, err := mock.SetIssueSubscription(t.Context(), project, 8, true)
	if err != nil || !issueState.Subscribed {
		t.Fatalf("issue subscription = %#v, err=%v", issueState, err)
	}
}

func TestMockClientRejectsIneligibleReviewerID(t *testing.T) {
	mock := NewMockClient("")
	mr := &MR{IID: 7}
	mock.SeedMR("team/repo", mr)
	if err := mock.SetMRReviewers(t.Context(), "team/repo", 7, []int64{404}); err == nil {
		t.Fatal("set reviewers err = nil, want ineligible member error")
	}
	if len(mr.Reviewers) != 0 {
		t.Fatalf("reviewers changed after rejected update: %#v", mr.Reviewers)
	}
}

func TestMockClientSetMRLabelsReplacesAndFeedbackHydrates(t *testing.T) {
	mock := NewMockClient("")
	const project = "team/repo"
	mock.SeedMR(project, &MR{IID: 7, Labels: []string{"old"}})
	if err := mock.SetMRLabels(t.Context(), project, 7, []string{"bug", "backend"}); err != nil {
		t.Fatalf("set labels: %v", err)
	}
	feedback, err := mock.GetMRFeedback(t.Context(), project, 7)
	if err != nil {
		t.Fatalf("feedback: %v", err)
	}
	if got := feedback.MR.Labels; len(got) != 2 || got[0] != "bug" || got[1] != "backend" {
		t.Fatalf("labels = %#v, want replacement", got)
	}
}

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

func TestMockClientReviewActionsMutateVisibleState(t *testing.T) {
	mock := NewMockClient("https://gitlab.example.com")
	const project = "team/repo"
	mock.SetUser("reviewer")
	mock.SeedProjectMembers(project, []ProjectMember{{ID: 17, Username: "owner", Name: "Owner"}})
	mock.SeedMR(project, &MR{IID: 7, State: mrStateOpen})

	if err := mock.SubmitMRApproval(t.Context(), project, 7); err != nil {
		t.Fatalf("approve: %v", err)
	}
	approvals, err := mock.ListMRApprovals(t.Context(), project, 7)
	if err != nil || len(approvals) != 1 || approvals[0].Username != "reviewer" {
		t.Fatalf("approvals = %#v, err=%v", approvals, err)
	}
	if err := mock.SubmitMRUnapproval(t.Context(), project, 7); err != nil {
		t.Fatalf("unapprove: %v", err)
	}
	approvals, err = mock.ListMRApprovals(t.Context(), project, 7)
	if err != nil || len(approvals) != 0 {
		t.Fatalf("approvals after unapprove = %#v, err=%v", approvals, err)
	}

	if err := mock.SetMRAssignees(t.Context(), project, 7, []int{17}); err != nil {
		t.Fatalf("set assignees: %v", err)
	}
	mr, err := mock.GetMR(t.Context(), project, 7)
	if err != nil || len(mr.Assignees) != 1 || mr.Assignees[0].ID != 17 {
		t.Fatalf("assignees = %#v, err=%v", mr.Assignees, err)
	}
}

func TestMockClientSeedsFilesCommitsAndResetClearsAllState(t *testing.T) {
	mock := NewMockClient("https://gitlab.example.com")
	const project = "team/repo"
	mock.SetUser("changed-user")
	mock.SeedProjectMembers(project, []ProjectMember{{ID: 17, Username: "owner"}})
	mock.SeedMR(project, &MR{IID: 7, State: mrStateOpen})
	mock.SeedIssue(project, &Issue{IID: 8, State: gitlabStateOpened})
	mock.SeedFiles(project, 7, []MRFile{{Filename: "main.go", Status: "modified"}})
	mock.SeedCommits(project, 7, []MRCommitInfo{{SHA: "abc", Message: "change"}})
	mock.SeedDiscussions(project, 7, []MRDiscussion{{ID: "thread-1"}})
	mock.SeedPipelines(project, []Pipeline{{ID: 9, Status: "success"}})
	mock.SeedApprovals(project, 7, []MRApproval{{Username: "owner"}}, 1)
	mock.SeedBranches(project, []RepoBranch{{Name: "main"}})
	_, _ = mock.SetMRSubscription(t.Context(), project, 7, true)
	_, _ = mock.SetIssueSubscription(t.Context(), project, 8, true)

	files, err := mock.ListMRFiles(t.Context(), project, 7)
	if err != nil || len(files) != 1 || files[0].Filename != "main.go" {
		t.Fatalf("files = %#v, err=%v", files, err)
	}
	commits, err := mock.ListMRCommits(t.Context(), project, 7)
	if err != nil || len(commits) != 1 || commits[0].SHA != "abc" {
		t.Fatalf("commits = %#v, err=%v", commits, err)
	}

	mock.Reset()
	if got := mock.Stats(); got != "mrs=0 discussions=0 issues=0" {
		t.Fatalf("stats after reset = %q", got)
	}
	if user, _ := mock.GetAuthenticatedUser(t.Context()); user != "kandev-tester" {
		t.Fatalf("user after reset = %q", user)
	}
	created, err := mock.CreateMR(t.Context(), project, "feature", "main", "Fresh", "", false)
	if err != nil || created.IID != 100 {
		t.Fatalf("created after reset = %#v, err=%v", created, err)
	}
	if state, _ := mock.GetMRSubscription(t.Context(), project, 7); state.Subscribed {
		t.Fatal("MR subscription survived reset")
	}
	if state, _ := mock.GetIssueSubscription(t.Context(), project, 8); state.Subscribed {
		t.Fatal("issue subscription survived reset")
	}
}
