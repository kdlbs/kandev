package github

import (
	"context"
	"errors"
	"testing"
)

// Compile-time interface check.
var _ Client = (*NoopClient)(nil)

func TestNoopClient_IsAuthenticated(t *testing.T) {
	c := &NoopClient{}
	ok, err := c.IsAuthenticated(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected authenticated=false for NoopClient")
	}
}

func TestNoopClient_ReturnsErrNoClient(t *testing.T) {
	c := &NoopClient{}
	ctx := context.Background()

	_, err := c.GetPR(ctx, "o", "r", 1)
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("GetPR error = %v, want ErrNoClient", err)
	}

	_, err = c.FindPRByBranch(ctx, "o", "r", "b")
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("FindPRByBranch error = %v, want ErrNoClient", err)
	}

	_, err = c.ListAuthoredPRs(ctx, "o", "r")
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("ListAuthoredPRs error = %v, want ErrNoClient", err)
	}

	err = c.SubmitReview(ctx, "o", "r", 1, "APPROVE", "lgtm")
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("SubmitReview error = %v, want ErrNoClient", err)
	}

	err = c.RequestReviewers(ctx, "o", "r", 1, []string{"octocat"})
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("RequestReviewers error = %v, want ErrNoClient", err)
	}

	// AC-11: the noop path never returns a PRStatus at all, so it can never
	// mark the outcome-field or closure-attribution groups populated.
	status, err := c.GetPRStatus(ctx, "o", "r", 1)
	if !errors.Is(err, ErrNoClient) {
		t.Errorf("GetPRStatus error = %v, want ErrNoClient", err)
	}
	if status != nil {
		t.Errorf("GetPRStatus status = %#v, want nil", status)
	}
}
