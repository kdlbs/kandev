package github

import (
	"context"
	"testing"
	"time"
)

func TestEnsureMockPRForRequestCopiesMergeableState(t *testing.T) {
	mock := NewMockClient()
	controller := &MockController{mock: mock}
	req := &associateTaskPRRequest{
		Owner:          "testorg",
		Repo:           "testrepo",
		PRNumber:       102,
		PRURL:          "https://github.com/testorg/testrepo/pull/102",
		PRTitle:        "Ready to ship",
		HeadBranch:     "feat/ready",
		BaseBranch:     "main",
		AuthorLogin:    "test-user",
		State:          "open",
		MergeableState: "clean",
	}

	controller.ensureMockPRForRequest(context.Background(), req, time.Now().UTC())

	pr, err := mock.GetPR(context.Background(), req.Owner, req.Repo, req.PRNumber)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr == nil {
		t.Fatal("expected synthetic PR")
	}
	if pr.MergeableState != "clean" {
		t.Fatalf("MergeableState = %q, want clean", pr.MergeableState)
	}
}
