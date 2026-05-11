package gitlab

import (
	"context"
	"errors"
	"testing"
)

func TestNoopClient_SatisfiesClient(t *testing.T) {
	var _ Client = (*NoopClient)(nil)
}

func TestNoopClient_IsAuthenticated_ReportsUnauthenticated(t *testing.T) {
	c := NewNoopClient("")
	ok, err := c.IsAuthenticated(context.Background())
	if err != nil {
		t.Fatalf("IsAuthenticated err = %v, want nil", err)
	}
	if ok {
		t.Fatalf("IsAuthenticated = true, want false for noop client")
	}
}

func TestNoopClient_Host_FallsBackToDefault(t *testing.T) {
	c := NewNoopClient("")
	if got := c.Host(); got != DefaultHost {
		t.Fatalf("Host = %q, want %q", got, DefaultHost)
	}
}

func TestNoopClient_Host_HonoursOverride(t *testing.T) {
	c := NewNoopClient("https://gitlab.acme.corp")
	if got := c.Host(); got != "https://gitlab.acme.corp" {
		t.Fatalf("Host = %q, want override", got)
	}
}

func TestNoopClient_DataMethods_ReturnErrNoClient(t *testing.T) {
	c := NewNoopClient("")
	ctx := context.Background()

	cases := []struct {
		name string
		err  error
	}{
		{"GetMR", mustErr2(c.GetMR(ctx, "g/p", 1))},
		{"FindMRByBranch", mustErr2(c.FindMRByBranch(ctx, "g/p", "feat"))},
		{"ListAuthoredMRs", mustErr2(c.ListAuthoredMRs(ctx, "g/p"))},
		{"ListReviewRequestedMRs", mustErr2(c.ListReviewRequestedMRs(ctx, "", ""))},
		{"ListMRApprovals", mustErr2(c.ListMRApprovals(ctx, "g/p", 1))},
		{"ListMRDiscussions", mustErr2(c.ListMRDiscussions(ctx, "g/p", 1, nil))},
		{"CreateMRDiscussionNote", mustErr2(c.CreateMRDiscussionNote(ctx, "g/p", 1, "d", "body"))},
		{"ResolveMRDiscussion", c.ResolveMRDiscussion(ctx, "g/p", 1, "d")},
		{"ListPipelines", mustErr2(c.ListPipelines(ctx, "g/p", "main"))},
		{"GetMRFeedback", mustErr2(c.GetMRFeedback(ctx, "g/p", 1))},
		{"GetMRStatus", mustErr2(c.GetMRStatus(ctx, "g/p", 1))},
		{"SubmitMRApproval", c.SubmitMRApproval(ctx, "g/p", 1)},
		{"SubmitMRUnapproval", c.SubmitMRUnapproval(ctx, "g/p", 1)},
		{"CreateMR", mustErr2(c.CreateMR(ctx, "g/p", "src", "dst", "t", "b", false))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, ErrNoClient) {
				t.Fatalf("err = %v, want ErrNoClient", tc.err)
			}
		})
	}
}

func TestDefaultActionPresets_AreNonEmpty(t *testing.T) {
	if len(DefaultMRActionPresets()) == 0 {
		t.Fatal("DefaultMRActionPresets returned empty slice")
	}
	if len(DefaultIssueActionPresets()) == 0 {
		t.Fatal("DefaultIssueActionPresets returned empty slice")
	}
}

// mustErr2 extracts the error from a (T, error) pair.
func mustErr2[T any](_ T, err error) error { return err }
