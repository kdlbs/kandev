package gitlab

import (
	"context"
	"testing"
)

func TestUpsertTaskMRRoundTripsCommitSHAs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	mr := newTestMR("task-1", "repo-1", "group/project", 224)
	mr.HeadSHA = "head-sha"
	mr.BaseSHA = "base-sha"
	if err := store.UpsertTaskMR(ctx, mr); err != nil {
		t.Fatalf("upsert task MR: %v", err)
	}

	got, err := store.GetTaskMR(ctx, "task-1", "repo-1", "group/project", 224)
	if err != nil {
		t.Fatalf("get task MR: %v", err)
	}
	if got == nil {
		t.Fatal("task MR is missing")
	}
	if got.HeadSHA != "head-sha" {
		t.Errorf("HeadSHA = %q, want head-sha", got.HeadSHA)
	}
	if got.BaseSHA != "base-sha" {
		t.Errorf("BaseSHA = %q, want base-sha", got.BaseSHA)
	}
}
