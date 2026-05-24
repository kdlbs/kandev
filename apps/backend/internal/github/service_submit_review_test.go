package github

import (
	"context"
	"errors"
	"testing"
)

// TestService_SubmitReview_RejectsSelfApprove verifies the server-side guard
// against approving your own PR. GitHub returns a 422 in that case; we catch
// it server-side so the UI gets a typed error rather than an opaque upstream
// failure when the frontend's visibility guard is bypassed (e.g. a stale
// page that didn't see the author update).
func TestService_SubmitReview_RejectsSelfApprove(t *testing.T) {
	client := NewMockClient()
	client.SetUser("octocat")
	client.AddPR(&PR{
		Number:      7,
		RepoOwner:   "octocat",
		RepoName:    "hello",
		AuthorLogin: "OCTOCAT", // case-insensitive match
		State:       "open",
	})
	svc := newTestService(client)

	err := svc.SubmitReview(context.Background(), "octocat", "hello", 7, "APPROVE", "")
	if !errors.Is(err, ErrSelfApprove) {
		t.Fatalf("expected ErrSelfApprove, got %v", err)
	}
	if len(client.SubmittedReviews()) != 0 {
		t.Fatalf("expected no review submitted, got %d", len(client.SubmittedReviews()))
	}
}

// TestService_SubmitReview_AllowsOthersPR verifies the guard only blocks the
// self-approval case — approving someone else's PR must still flow through.
func TestService_SubmitReview_AllowsOthersPR(t *testing.T) {
	client := NewMockClient()
	client.SetUser("reviewer")
	client.AddPR(&PR{
		Number:      8,
		RepoOwner:   "octocat",
		RepoName:    "hello",
		AuthorLogin: "octocat",
		State:       "open",
	})
	svc := newTestService(client)

	if err := svc.SubmitReview(context.Background(), "octocat", "hello", 8, "APPROVE", ""); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	reviews := client.SubmittedReviews()
	if len(reviews) != 1 || reviews[0].Event != "APPROVE" {
		t.Fatalf("expected one APPROVE review, got %#v", reviews)
	}
}

// TestService_SubmitReview_CommentOnOwnPRAllowed verifies the guard is
// scoped to APPROVE — leaving a COMMENT or REQUEST_CHANGES review on your
// own PR is allowed by GitHub and must not be blocked.
func TestService_SubmitReview_CommentOnOwnPRAllowed(t *testing.T) {
	client := NewMockClient()
	client.SetUser("octocat")
	client.AddPR(&PR{
		Number:      9,
		RepoOwner:   "octocat",
		RepoName:    "hello",
		AuthorLogin: "octocat",
		State:       "open",
	})
	svc := newTestService(client)

	if err := svc.SubmitReview(context.Background(), "octocat", "hello", 9, "COMMENT", "looks ok"); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	reviews := client.SubmittedReviews()
	if len(reviews) != 1 || reviews[0].Event != "COMMENT" {
		t.Fatalf("expected one COMMENT review, got %#v", reviews)
	}
}
