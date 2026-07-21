package azuredevops

import (
	"context"
	"testing"
)

func TestMockClientSeedsWorkspaceReadState(t *testing.T) {
	mock := NewMockClient()
	mock.Seed(MockState{
		Authenticated: true,
		User:          TestConnectionResult{OK: true, ID: "me", DisplayName: "Ada"},
		Projects:      []Project{{ID: "p1", Name: "Platform"}},
		WorkItems:     []WorkItem{{ID: 101, Title: "Fix build"}},
		PullRequests:  []PullRequest{{ID: 42, ProjectID: "p1", RepositoryID: "r1", Title: "Ship it"}},
		Feedback:      map[int]PullRequestFeedback{42: {ReviewState: "approved"}},
	})

	auth, err := mock.TestAuth(context.Background())
	if err != nil || !auth.OK || auth.ID != "me" {
		t.Fatalf("auth = %+v, %v", auth, err)
	}
	projects, _ := mock.ListProjects(context.Background())
	work, _ := mock.QueryWIQL(context.Background(), "p1", "SELECT", 20)
	prs, _ := mock.ListPullRequests(context.Background(), PullRequestFilter{ProjectID: "p1", RepositoryID: "r1"})
	feedback, ok := mock.Feedback(42)
	if len(projects) != 1 || len(work.Items) != 1 || len(prs.Items) != 1 || !ok || feedback.ReviewState != "approved" {
		t.Fatalf("mock state projects=%+v work=%+v prs=%+v feedback=%+v", projects, work, prs, feedback)
	}
}
