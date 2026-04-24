package github

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

type countingStatusClient struct {
	*MockClient
	calls atomic.Int32
}

func (c *countingStatusClient) GetPRStatus(_ context.Context, owner, repo string, number int) (*PRStatus, error) {
	c.calls.Add(1)
	return &PRStatus{PR: &PR{RepoOwner: owner, RepoName: repo, Number: number}}, nil
}

func TestService_GetPRStatus_UsesCache(t *testing.T) {
	client := &countingStatusClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if _, err := svc.GetPRStatus(ctx, "o", "r", 1); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if got := client.calls.Load(); got != 1 {
		t.Fatalf("expected 1 upstream call, got %d", got)
	}

	if _, err := svc.GetPRStatus(ctx, "o", "r", 2); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 2 {
		t.Fatalf("expected new fetch for different number, got %d", got)
	}
}

func TestService_GetPRStatusesBatch_FetchesAndCaches(t *testing.T) {
	client := &countingStatusClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	refs := []PRRef{
		{Owner: "o", Repo: "r", Number: 1},
		{Owner: "o", Repo: "r", Number: 2},
		{Owner: "o", Repo: "r", Number: 3},
	}
	statuses, err := svc.GetPRStatusesBatch(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 3 {
		t.Fatalf("expected 3 statuses, got %d", len(statuses))
	}
	if got := client.calls.Load(); got != 3 {
		t.Fatalf("expected 3 upstream calls, got %d", got)
	}

	// Second batch hits the per-PR cache — no new upstream calls.
	if _, err := svc.GetPRStatusesBatch(ctx, refs); err != nil {
		t.Fatal(err)
	}
	if got := client.calls.Load(); got != 3 {
		t.Fatalf("expected no new upstream calls on 2nd batch, got %d", got)
	}
}

func TestService_GetPRStatusesBatch_SkipsInvalidRefs(t *testing.T) {
	client := &countingStatusClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	refs := []PRRef{
		{Owner: "", Repo: "r", Number: 1},
		{Owner: "o", Repo: "", Number: 2},
		{Owner: "o", Repo: "r", Number: 0},
		{Owner: "o", Repo: "r", Number: 7},
	}
	statuses, err := svc.GetPRStatusesBatch(context.Background(), refs)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 valid status, got %d", len(statuses))
	}
	if _, ok := statuses["o/r#7"]; !ok {
		t.Fatalf("expected status for valid ref, got keys %v", keys(statuses))
	}
}

type failingStatusClient struct {
	*MockClient
}

func (failingStatusClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	return nil, fmt.Errorf("upstream boom")
}

func TestService_GetPRStatusesBatch_OmitsFailedRefs(t *testing.T) {
	svc := newTestService(failingStatusClient{MockClient: NewMockClient()})
	refs := []PRRef{
		{Owner: "o", Repo: "r", Number: 1},
		{Owner: "o", Repo: "r", Number: 2},
	}
	statuses, err := svc.GetPRStatusesBatch(context.Background(), refs)
	if err != nil {
		t.Fatalf("unexpected batch error: %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("expected empty map when all refs fail, got %d", len(statuses))
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	out := make([]K, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
