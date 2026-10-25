package github

import (
	"context"
	"strings"
	"testing"
)

func TestMockClientMergePRRecordsExpectedHeadAndRejectsMismatch(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{RepoOwner: "acme", RepoName: "demo", Number: 42, HeadSHA: "head-current"})

	_, err := client.MergePR(context.Background(), "acme", "demo", 42, MergePRRequest{
		MergeMethod: "squash", ExpectedHeadSHA: "head-reviewed",
	})
	if err == nil || !strings.Contains(err.Error(), "head") {
		t.Fatalf("head mismatch error = %v, want head diagnostic", err)
	}
	attempts := client.MergedPRs()
	if len(attempts) != 1 || attempts[0].ExpectedHeadSHA != "head-reviewed" {
		t.Fatalf("attempts = %+v, want reviewed head", attempts)
	}
}

func TestMockClientMergePRReturnsConfiguredFailure(t *testing.T) {
	client := NewMockClient()
	client.AddPR(&PR{RepoOwner: "acme", RepoName: "demo", Number: 42, HeadSHA: "head-current"})
	client.SetMergeFailure("acme", "demo", 42, "provider unavailable")

	_, err := client.MergePR(context.Background(), "acme", "demo", 42, MergePRRequest{
		MergeMethod: "squash", ExpectedHeadSHA: "head-current",
	})
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("configured failure = %v, want provider diagnostic", err)
	}
}
